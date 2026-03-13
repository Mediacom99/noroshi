package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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

	// Open database and run migrations
	db, err := storage.OpenDB(cfg.DatabasePath)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := storage.RunMigrations(db); err != nil {
		slog.Error("run migrations", "error", err)
		os.Exit(1)
	}

	store := storage.NewSQLiteStore(db)

	// Create checker
	checker := monitor.NewChecker(cfg.HTTPTimeout)

	// Create bot (without scheduler — circular dependency resolution)
	teleBot, err := bot.NewBot(cfg.TelegramToken, cfg.TelegramChatID, store, checker, cfg.MaxFailureNotifications, ctx)
	if err != nil {
		slog.Error("create bot", "error", err)
		os.Exit(1)
	}

	// Create notifier from bot
	notifier := bot.NewTelegramNotifier(teleBot, cfg.MaxFailureNotifications)

	// Create scheduler with notifier
	scheduler, err := monitor.NewScheduler(ctx, store, checker, notifier, cfg.MaxFailureNotifications)
	if err != nil {
		slog.Error("create scheduler", "error", err)
		os.Exit(1)
	}

	// Close circular dependency
	teleBot.SetScheduler(scheduler)

	// Load existing endpoints and add to scheduler
	endpoints, err := store.ListEndpoints(ctx)
	if err != nil {
		slog.Error("list endpoints", "error", err)
		os.Exit(1)
	}
	for _, ep := range endpoints {
		if err := scheduler.Add(ctx, ep); err != nil {
			slog.Error("add endpoint to scheduler", "id", ep.ID, "error", err)
		}
	}
	slog.Info("loaded endpoints", "count", len(endpoints))

	// Start scheduler
	scheduler.Start()

	// Start bot
	teleBot.Start()

	// Start health server
	healthSrv := startHealthServer(cfg.HealthPort)

	// Wait for shutdown signal
	<-ctx.Done()
	slog.Info("shutting down...")

	// Graceful shutdown
	teleBot.Stop()

	if err := scheduler.Shutdown(); err != nil {
		slog.Error("shutdown scheduler", "error", err)
	}

	if err := healthSrv.Shutdown(context.Background()); err != nil {
		slog.Error("shutdown health server", "error", err)
	}

	slog.Info("shutdown complete")
}

func startHealthServer(port int) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		slog.Info("health server started", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("health server", "error", err)
		}
	}()

	return srv
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
