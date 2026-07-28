package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"noroshi/internal/storage"

	"github.com/go-co-op/gocron/v2"
)

// Store defines the storage methods the scheduler needs.
type Store interface {
	GetEndpoint(ctx context.Context, id int64) (storage.Endpoint, error)
	UpdateEndpointStatus(ctx context.Context, id int64, status string, statusCode int, latencyMs int64) error
	RecordFailure(ctx context.Context, id int64, statusCode int, latencyMs int64, maxNotifications int, failureThreshold int) (storage.Endpoint, error)
	RecordRecovery(ctx context.Context, id int64, statusCode int, latencyMs int64) (storage.Endpoint, error)
}

// Checker performs HTTP health checks.
type Checker interface {
	Check(ctx context.Context, url string) (statusCode int, latency time.Duration, err error)
}

// Notifier sends failure and recovery notifications.
type Notifier interface {
	NotifyFailure(ctx context.Context, endpoint storage.Endpoint) error
	NotifyRecovery(ctx context.Context, endpoint storage.Endpoint, downtime time.Duration) error
}

// Scheduler manages periodic health checks using gocron.
type Scheduler struct {
	cron                    gocron.Scheduler
	store                   Store
	checker                 Checker
	notifier                Notifier
	maxFailureNotifications int
	failureThreshold        int
	ctx                     context.Context
}

// NewScheduler creates a Scheduler. Call Start() to begin running jobs.
// failureThreshold is how many consecutive failures must occur before the
// first alert is sent (1 = alert on the first failure).
func NewScheduler(ctx context.Context, store Store, checker Checker, notifier Notifier, maxFailureNotifications int, failureThreshold int) (*Scheduler, error) {
	cron, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("create gocron scheduler: %w", err)
	}
	return &Scheduler{
		cron:                    cron,
		store:                   store,
		checker:                 checker,
		notifier:                notifier,
		maxFailureNotifications: maxFailureNotifications,
		failureThreshold:        failureThreshold,
		ctx:                     ctx,
	}, nil
}

// Start begins running scheduled jobs.
func (s *Scheduler) Start() {
	s.cron.Start()
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
		slog.Error("scheduler: get endpoint", "id", endpointID, "error", err)
		return
	}

	// Defensive: paused endpoints have no job, but a job may already be in
	// flight when the pause happens.
	if ep.Paused {
		return
	}

	statusCode, latency, checkErr := s.checker.Check(ctx, ep.URL)
	latencyMs := latency.Milliseconds()

	if checkErr != nil || statusCode < 200 || statusCode >= 300 {
		// NOT_OK
		updated, err := s.store.RecordFailure(ctx, endpointID, statusCode, latencyMs, s.maxFailureNotifications, s.failureThreshold)
		if err != nil {
			slog.Error("scheduler: record failure", "id", endpointID, "error", err)
			return
		}

		// The store caps failure_notifications_sent at maxFailureNotifications,
		// so notify only when this failure actually incremented the counter.
		if updated.FailureNotificationsSent > ep.FailureNotificationsSent {
			slog.Info("scheduler: endpoint down",
				"id", endpointID, "name", ep.Name, "url", ep.URL,
				"status_code", statusCode, "duration_ms", latencyMs,
				"consecutive_failures", updated.ConsecutiveFailures)
			if err := s.notifier.NotifyFailure(ctx, updated); err != nil {
				slog.Error("scheduler: notify failure", "id", endpointID, "error", err)
			}
		} else {
			// Below the alert threshold or past the notification cap — log at
			// debug to avoid spamming on every interval during a long outage.
			slog.Debug("scheduler: check failed",
				"id", endpointID, "name", ep.Name, "url", ep.URL,
				"status_code", statusCode, "duration_ms", latencyMs,
				"consecutive_failures", updated.ConsecutiveFailures)
		}
	} else {
		// OK
		if ep.Status != "ok" && ep.Status != "unknown" {
			// Recovery
			recovered, err := s.store.RecordRecovery(ctx, endpointID, statusCode, latencyMs)
			if err != nil {
				slog.Error("scheduler: record recovery", "id", endpointID, "error", err)
				return
			}

			// Only notify for a real tracked outage — a "not_ok" status without
			// last_failure_at comes from an ad-hoc /status probe, not an outage.
			if recovered.LastFailureAt.Valid {
				downtime := time.Since(recovered.LastFailureAt.Time)
				slog.Info("scheduler: endpoint recovered",
					"id", endpointID, "name", ep.Name, "url", ep.URL,
					"status_code", statusCode, "duration_ms", latencyMs,
					"downtime", downtime.String())
				if err := s.notifier.NotifyRecovery(ctx, recovered, downtime); err != nil {
					slog.Error("scheduler: notify recovery", "id", endpointID, "error", err)
				}
			}
		} else {
			// Already OK, just update status
			if err := s.store.UpdateEndpointStatus(ctx, endpointID, "ok", statusCode, latencyMs); err != nil {
				slog.Error("scheduler: update status", "id", endpointID, "error", err)
			}
		}
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

	statusCode, latency, checkErr := s.checker.Check(ctx, ep.URL)
	latencyMs := latency.Milliseconds()

	if checkErr != nil || statusCode < 200 || statusCode >= 300 {
		if err := s.store.UpdateEndpointStatus(ctx, endpointID, "not_ok", statusCode, latencyMs); err != nil {
			return storage.Endpoint{}, err
		}
	} else if ep.Status != "ok" {
		// Transitioning to OK from a tracked outage: reset failure state.
		if _, err := s.store.RecordRecovery(ctx, endpointID, statusCode, latencyMs); err != nil {
			return storage.Endpoint{}, err
		}
	} else {
		if err := s.store.UpdateEndpointStatus(ctx, endpointID, "ok", statusCode, latencyMs); err != nil {
			return storage.Endpoint{}, err
		}
	}

	return s.store.GetEndpoint(ctx, endpointID)
}
