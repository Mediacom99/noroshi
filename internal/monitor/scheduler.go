package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"noroshi/internal/storage"

	"github.com/go-co-op/gocron/v2"
)

// Store defines the storage methods the scheduler needs.
type Store interface {
	GetEndpoint(ctx context.Context, id int64) (storage.Endpoint, error)
	UpdateEndpointStatus(ctx context.Context, id int64, o storage.CheckOutcome) error
	RecordFailure(ctx context.Context, id int64, o storage.CheckOutcome, maxNotifications int, failureThreshold int) (storage.Endpoint, error)
	RecordRecovery(ctx context.Context, id int64, o storage.CheckOutcome) (storage.Endpoint, error)
	ListExpiredPauses(ctx context.Context, now time.Time) ([]storage.Endpoint, error)
	SetEndpointPaused(ctx context.Context, id int64, paused bool, until sql.NullTime) error
	SetAlertMessageID(ctx context.Context, id int64, messageID int64) error
	TouchLastNotified(ctx context.Context, id int64) error
	TouchCertWarning(ctx context.Context, id int64) error
	RecordCheck(ctx context.Context, endpointID int64, up bool, statusCode int, latencyMs int64) error
	PruneChecks(ctx context.Context, olderThan time.Time) (int64, error)
}

// Checker performs HTTP health checks.
type Checker interface {
	Check(ctx context.Context, url string, opts CheckOptions) CheckResult
}

// Notifier sends failure and recovery notifications.
// NotifyFailure returns the ID of the sent alert message (0 if unavailable),
// used to thread the recovery message as a reply.
type Notifier interface {
	NotifyFailure(ctx context.Context, endpoint storage.Endpoint) (int64, error)
	NotifyRecovery(ctx context.Context, endpoint storage.Endpoint, downtime time.Duration) error
	NotifyCertExpiry(ctx context.Context, endpoint storage.Endpoint, daysLeft int) error
}

// certWarningDays is how close to expiry a certificate must be to warn.
const certWarningDays = 14

// checkRetentionDays is how long check history is kept for stats.
const checkRetentionDays = 30

// certWarningCooldown is the minimum time between two cert warnings per endpoint.
const certWarningCooldown = 24 * time.Hour

// Scheduler manages periodic health checks using gocron.
type Scheduler struct {
	cron                    gocron.Scheduler
	store                   Store
	checker                 Checker
	notifier                Notifier
	maxFailureNotifications int
	failureThreshold        int
	reminderInterval        time.Duration
	ctx                     context.Context
	logger                  *slog.Logger
}

// NewScheduler creates a Scheduler. Call Start() to begin running jobs.
// failureThreshold is how many consecutive failures must occur before the
// first alert is sent (1 = alert on the first failure). reminderInterval
// controls "still down" re-alerts (0 = disabled). logger is scoped with a
// "component" attribute by the caller; nil falls back to slog.Default().
func NewScheduler(ctx context.Context, store Store, checker Checker, notifier Notifier, maxFailureNotifications int, failureThreshold int, reminderInterval time.Duration, logger *slog.Logger) (*Scheduler, error) {
	cron, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("create gocron scheduler: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}
	s := &Scheduler{
		cron:                    cron,
		store:                   store,
		checker:                 checker,
		notifier:                notifier,
		maxFailureNotifications: maxFailureNotifications,
		failureThreshold:        failureThreshold,
		reminderInterval:        reminderInterval,
		ctx:                     ctx,
		logger:                  logger,
	}

	// Periodic housekeeping: resume endpoints whose timed pause has expired.
	_, err = cron.NewJob(
		gocron.DurationJob(time.Minute),
		gocron.NewTask(s.resumeExpiredPauses),
		gocron.WithTags("internal-resume-expired"),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return nil, fmt.Errorf("add resume-expired job: %w", err)
	}

	// Housekeeping: prune check history older than the retention window.
	_, err = cron.NewJob(
		gocron.DurationJob(time.Hour),
		gocron.NewTask(s.pruneOldChecks),
		gocron.WithTags("internal-prune-checks"),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return nil, fmt.Errorf("add prune-checks job: %w", err)
	}

	return s, nil
}

// Start begins running scheduled jobs.
func (s *Scheduler) Start() {
	s.cron.Start()
	s.logger.Info("scheduler started")
}

// Add creates a gocron job for the given endpoint.
func (s *Scheduler) Add(ctx context.Context, ep storage.Endpoint) error {
	tag := fmt.Sprintf("endpoint-%d", ep.ID)
	_, err := s.cron.NewJob(
		gocron.DurationJob(time.Duration(ep.IntervalSeconds)*time.Second),
		gocron.NewTask(s.checkAndNotify, ep.ID),
		gocron.WithTags(tag),
		gocron.WithStartAt(gocron.WithStartImmediately()),
		// Never run two checks for the same endpoint concurrently — a check
		// slower than the interval would otherwise race on failure counters.
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return fmt.Errorf("add job for endpoint %d: %w", ep.ID, err)
	}
	return nil
}

// Remove stops the job for the given endpoint ID.
func (s *Scheduler) Remove(endpointID int64) error {
	tag := fmt.Sprintf("endpoint-%d", endpointID)
	s.cron.RemoveByTags(tag)
	return nil
}

// Shutdown stops the scheduler and waits for running jobs to finish.
func (s *Scheduler) Shutdown() error {
	return s.cron.Shutdown()
}

func (s *Scheduler) checkAndNotify(endpointID int64) {
	ctx := s.ctx

	ep, err := s.store.GetEndpoint(ctx, endpointID)
	if err != nil {
		s.logger.Error("get endpoint", "id", endpointID, "error", err)
		return
	}

	// Defensive: paused endpoints have no job, but a job may already be in
	// flight when the pause happens.
	if ep.Paused {
		return
	}

	log := s.logger.With("id", ep.ID, "name", ep.Name, "url", ep.URL)

	res := s.checker.Check(ctx, ep.URL, CheckOptions{
		ExpectedStatus: ep.ExpectedStatus,
		Keyword:        ep.ExpectedKeyword,
	})
	outcome := outcomeFromResult(res)
	latencyMs := res.Latency.Milliseconds()
	statusCode := res.StatusCode
	s.recordCheck(ctx, endpointID, res)

	if !res.Up {
		// NOT_OK
		updated, err := s.store.RecordFailure(ctx, endpointID, outcome, s.maxFailureNotifications, s.failureThreshold)
		if err != nil {
			log.Error("record failure", "error", err)
			return
		}

		// The store caps failure_notifications_sent at maxFailureNotifications,
		// so notify only when this failure actually incremented the counter.
		if updated.FailureNotificationsSent > ep.FailureNotificationsSent {
			log.Info("endpoint down",
				"status_code", statusCode, "duration_ms", latencyMs,
				"consecutive_failures", updated.ConsecutiveFailures)
			msgID, err := s.notifier.NotifyFailure(ctx, updated)
			if err != nil {
				log.Error("notify failure", "error", err)
			} else if msgID > 0 {
				if err := s.store.SetAlertMessageID(ctx, endpointID, msgID); err != nil {
					log.Error("set alert message id", "error", err)
				}
			}
		} else if s.reminderInterval > 0 && updated.LastNotifiedAt.Valid &&
			time.Since(updated.LastNotifiedAt.Time) >= s.reminderInterval {
			// Cap reached but the outage continues — send a reminder.
			log.Info("endpoint still down",
				"status_code", statusCode, "consecutive_failures", updated.ConsecutiveFailures)
			if _, err := s.notifier.NotifyFailure(ctx, updated); err != nil {
				log.Error("notify reminder", "error", err)
			} else if err := s.store.TouchLastNotified(ctx, endpointID); err != nil {
				log.Error("touch last notified", "error", err)
			}
		} else {
			// Below the alert threshold or past the notification cap — log at
			// debug to avoid spamming on every interval during a long outage.
			log.Debug("check failed",
				"status_code", statusCode, "duration_ms", latencyMs,
				"consecutive_failures", updated.ConsecutiveFailures)
		}
	} else {
		// OK — check certificate expiry while we're here.
		s.maybeWarnCertExpiry(ctx, ep, res.CertExpiry)

		if ep.Status != "ok" && ep.Status != "unknown" {
			// Recovery
			recovered, err := s.store.RecordRecovery(ctx, endpointID, outcome)
			if err != nil {
				log.Error("record recovery", "error", err)
				return
			}

			// Only notify for a real tracked outage — a "not_ok" status without
			// last_failure_at comes from an ad-hoc /status probe, not an outage.
			if recovered.LastFailureAt.Valid {
				downtime := time.Since(recovered.LastFailureAt.Time)
				log.Info("endpoint recovered",
					"status_code", statusCode, "duration_ms", latencyMs,
					"downtime", downtime.String())
				if err := s.notifier.NotifyRecovery(ctx, recovered, downtime); err != nil {
					log.Error("notify recovery", "error", err)
				}
			}
		} else {
			// Already OK, just update status
			if err := s.store.UpdateEndpointStatus(ctx, endpointID, outcome); err != nil {
				log.Error("update status", "error", err)
			}
		}
	}
}

// outcomeFromResult converts a checker result into the persisted outcome shape.
func outcomeFromResult(res CheckResult) storage.CheckOutcome {
	o := storage.CheckOutcome{
		StatusCode: res.StatusCode,
		LatencyMs:  res.Latency.Milliseconds(),
		Reason:     res.Reason,
	}
	if !res.CertExpiry.IsZero() {
		o.CertExpiry = sql.NullTime{Time: res.CertExpiry, Valid: true}
	}
	if res.Up {
		o.Status = "ok"
	} else {
		o.Status = "not_ok"
	}
	return o
}

// maybeWarnCertExpiry sends at most one warning per certWarningCooldown when
// the endpoint's certificate expires within certWarningDays.
func (s *Scheduler) maybeWarnCertExpiry(ctx context.Context, ep storage.Endpoint, certExpiry time.Time) {
	if certExpiry.IsZero() {
		return
	}
	daysLeft := int(time.Until(certExpiry).Hours() / 24)
	if daysLeft >= certWarningDays {
		return
	}
	if ep.LastCertWarningAt.Valid && time.Since(ep.LastCertWarningAt.Time) < certWarningCooldown {
		return
	}

	log := s.logger.With("id", ep.ID, "name", ep.Name, "url", ep.URL)
	log.Info("certificate expiring",
		"days_left", daysLeft, "expires_at", certExpiry.Format("2006-01-02"))
	if err := s.notifier.NotifyCertExpiry(ctx, ep, daysLeft); err != nil {
		log.Error("notify cert expiry", "error", err)
		return
	}
	if err := s.store.TouchCertWarning(ctx, ep.ID); err != nil {
		log.Error("touch cert warning", "error", err)
	}
}

// CheckNow performs an immediate ad-hoc check for an endpoint and updates its
// stored status, code, and latency. It deliberately does NOT touch failure
// counters or send notifications — the scheduled jobs own the
// failure/recovery state machine.
func (s *Scheduler) CheckNow(ctx context.Context, endpointID int64) (storage.Endpoint, error) {
	ep, err := s.store.GetEndpoint(ctx, endpointID)
	if err != nil {
		return storage.Endpoint{}, err
	}

	// Paused endpoints are not checked, not even on demand.
	if ep.Paused {
		return ep, nil
	}

	res := s.checker.Check(ctx, ep.URL, CheckOptions{
		ExpectedStatus: ep.ExpectedStatus,
		Keyword:        ep.ExpectedKeyword,
	})
	outcome := outcomeFromResult(res)
	s.recordCheck(ctx, endpointID, res)

	if !res.Up {
		if err := s.store.UpdateEndpointStatus(ctx, endpointID, outcome); err != nil {
			return storage.Endpoint{}, err
		}
	} else if ep.Status != "ok" {
		// Transitioning to OK from a tracked outage: reset failure state.
		if _, err := s.store.RecordRecovery(ctx, endpointID, outcome); err != nil {
			return storage.Endpoint{}, err
		}
	} else {
		if err := s.store.UpdateEndpointStatus(ctx, endpointID, outcome); err != nil {
			return storage.Endpoint{}, err
		}
	}

	return s.store.GetEndpoint(ctx, endpointID)
}

// resumeExpiredPauses unpauses endpoints whose timed pause has elapsed and
// restarts their monitoring jobs.
func (s *Scheduler) resumeExpiredPauses() {
	ctx := s.ctx

	expired, err := s.store.ListExpiredPauses(ctx, time.Now().UTC())
	if err != nil {
		s.logger.Error("list expired pauses", "error", err)
		return
	}

	for _, ep := range expired {
		log := s.logger.With("id", ep.ID, "name", ep.Name)
		if err := s.store.SetEndpointPaused(ctx, ep.ID, false, sql.NullTime{}); err != nil {
			log.Error("resume endpoint", "error", err)
			continue
		}
		ep.Paused = false
		ep.PausedUntil = sql.NullTime{}
		if err := s.Add(ctx, ep); err != nil {
			log.Error("restart job after pause", "error", err)
			continue
		}
		log.Info("timed pause ended")
	}
}

// recordCheck appends a result to the check history (stats). Failures are
// logged but never block the monitoring flow.
func (s *Scheduler) recordCheck(ctx context.Context, endpointID int64, res CheckResult) {
	if err := s.store.RecordCheck(ctx, endpointID, res.Up, res.StatusCode, res.Latency.Milliseconds()); err != nil {
		s.logger.Error("record check", "id", endpointID, "error", err)
	}
}

// pruneOldChecks deletes check history beyond the retention window.
func (s *Scheduler) pruneOldChecks() {
	deleted, err := s.store.PruneChecks(s.ctx, time.Now().UTC().AddDate(0, 0, -checkRetentionDays))
	if err != nil {
		s.logger.Error("prune checks", "error", err)
		return
	}
	if deleted > 0 {
		s.logger.Info("pruned check history", "deleted", deleted)
	}
}
