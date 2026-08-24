package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"noroshi/internal/apperror"
	"noroshi/internal/bot"
	"noroshi/internal/config"
	"noroshi/internal/monitor"
	"noroshi/internal/storage"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	setupLogging(cfg.LogLevel)
	base := slog.Default()
	log := base.With("component", "main")

	// Open database and run migrations
	db, err := storage.OpenDB(cfg.DatabasePath)
	if err != nil {
		log.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := storage.RunMigrations(db); err != nil {
		log.Error("run migrations", "error", err)
		os.Exit(1)
	}

	store := storage.NewSQLiteStore(db)

	// Create checker
	checker := monitor.NewChecker(cfg.HTTPTimeout)

	// Create bot (without scheduler — circular dependency resolution)
	var webhookCfg *bot.WebhookConfig
	if cfg.TelegramWebhookURL != "" {
		webhookCfg = &bot.WebhookConfig{
			PublicURL: cfg.TelegramWebhookURL,
			Port:      cfg.TelegramWebhookPort,
			Secret:    cfg.TelegramWebhookSecret,
		}
	}
	teleBot, err := bot.NewBot(cfg.TelegramToken, cfg.TelegramChatID, store, checker, cfg.SlowThresholdMs, webhookCfg, ctx, base.With("component", "bot"))
	if err != nil {
		log.Error("create bot", "error", err)
		os.Exit(1)
	}

	// Create notifier from bot (Telegram is always first — its message ID is
	// used to thread recovery messages), plus the generic webhook if configured.
	telegramNotifier := bot.NewTelegramNotifier(teleBot, cfg.MaxFailureNotifications)
	notifiers := []monitor.Notifier{telegramNotifier}
	if cfg.AlertWebhookURL != "" {
		notifiers = append(notifiers, monitor.NewWebhookNotifier(cfg.AlertWebhookURL, cfg.AlertWebhookToken))
	}
	notifier := monitor.NewMultiNotifier(base.With("component", "notifier"), notifiers...)

	// Create scheduler with notifier
	scheduler, err := monitor.NewScheduler(ctx, store, checker, notifier, cfg.MaxFailureNotifications, cfg.FailureThreshold, cfg.ReminderInterval,
		monitor.DigestConfig{Mode: cfg.Digest, TimeMinutes: cfg.DigestTimeMinutes}, base.With("component", "scheduler"))
	if err != nil {
		log.Error("create scheduler", "error", err)
		os.Exit(1)
	}

	// Close circular dependency
	teleBot.SetScheduler(scheduler)

	// Prometheus instrumentation (scraped via GET /metrics on the health server)
	metrics := monitor.NewMetrics()
	scheduler.SetMetrics(metrics)

	// Load existing endpoints and add to scheduler
	endpoints, err := store.ListEndpoints(ctx)
	if err != nil {
		log.Error("list endpoints", "error", err)
		os.Exit(1)
	}
	for _, ep := range endpoints {
		if ep.Paused {
			continue
		}
		if err := scheduler.Add(ctx, ep); err != nil {
			log.Error("add endpoint to scheduler", "id", ep.ID, "error", err)
		}
	}
	log.Info("loaded endpoints", "count", len(endpoints))

	// Start scheduler
	scheduler.Start()
	log.Info("noroshi started",
		"health_port", cfg.HealthPort, "database", cfg.DatabasePath,
		"log_level", cfg.LogLevel, "max_failure_notifications", cfg.MaxFailureNotifications,
		"failure_threshold", cfg.FailureThreshold)

	// Start bot
	teleBot.Start()

	// Start health server
	healthSrv := startHealthServer(cfg.HealthPort, store, metrics, log)

	// Wait for shutdown signal
	<-ctx.Done()
	log.Info("shutting down...")

	// Graceful shutdown
	teleBot.Stop()

	if err := scheduler.Shutdown(); err != nil {
		log.Error("shutdown scheduler", "error", err)
	}

	if err := healthSrv.Shutdown(context.Background()); err != nil {
		log.Error("shutdown health server", "error", err)
	}

	log.Info("shutdown complete")
}

func startHealthServer(port int, store *storage.SQLiteStore, metrics *monitor.Metrics, log *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Prometheus scrape endpoint (unauthenticated, like the badges).
	if metrics != nil {
		mux.Handle("GET /metrics", metrics.Handler())
	}

	// Shields-style status badge: /badge/<name>.svg
	mux.HandleFunc("GET /badge/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(r.PathValue("name"), ".svg")
		ep, err := store.GetEndpointByName(r.Context(), name)
		if err != nil {
			if !errors.Is(err, apperror.ErrNotFound) {
				log.Error("badge lookup", "name", name, "error", err)
			}
			w.Header().Set("Content-Type", "image/svg+xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(badgeSVG(name, "not found", "#9f9f9f")))
			return
		}
		label, color := "unknown", "#dfb317"
		switch {
		case ep.Paused:
			label, color = "paused", "#9f9f9f"
		case ep.Status == "ok":
			label, color = "up", "#4c1"
		case ep.Status == "not_ok":
			label, color = "down", "#e05d44"
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		_, _ = w.Write([]byte(badgeSVG(ep.Name, label, color)))
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("health server started", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("health server", "error", err)
		}
	}()

	return srv
}

// badgeSVG renders a minimal shields-style two-segment status badge.
func badgeSVG(label, value, color string) string {
	lw := 6*len(label) + 16
	vw := 6*len(value) + 16
	total := lw + vw
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">`+
		`<rect width="%d" height="20" rx="3" fill="#555"/>`+
		`<rect x="%d" width="%d" height="20" rx="3" fill="%s"/>`+
		`<rect x="%d" width="4" height="20" fill="%s"/>`+
		`<g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,sans-serif" font-size="11">`+
		`<text x="%d" y="14">%s</text>`+
		`<text x="%d" y="14">%s</text>`+
		`</g></svg>`,
		total, htmlEscapeAttr(label), htmlEscapeAttr(value),
		total,
		lw, vw, color,
		lw, color,
		lw/2, htmlEscapeAttr(label),
		lw+vw/2, htmlEscapeAttr(value),
	)
}

func htmlEscapeAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func setupLogging(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))
}
