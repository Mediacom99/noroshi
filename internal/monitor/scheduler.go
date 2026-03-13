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
	UpdateEndpointStatus(ctx context.Context, id int64, status string, statusCode int) error
	RecordFailure(ctx context.Context, id int64, statusCode int) (storage.Endpoint, error)
	RecordRecovery(ctx context.Context, id int64, statusCode int) (storage.Endpoint, error)
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
	checker                 *Checker
	notifier                Notifier
	maxFailureNotifications int
	ctx                     context.Context
}

// NewScheduler creates a Scheduler. Call Start() to begin running jobs.
func NewScheduler(ctx context.Context, store Store, checker *Checker, notifier Notifier, maxFailureNotifications int) (*Scheduler, error) {
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

	previousStatus := ep.Status

	statusCode, checkErr := s.checker.Check(ctx, ep.URL)

	if checkErr != nil || statusCode != 200 {
		// NOT_OK
		updated, err := s.store.RecordFailure(ctx, endpointID, statusCode)
		if err != nil {
			slog.Error("scheduler: record failure", "id", endpointID, "error", err)
			return
		}

		if updated.FailureNotificationsSent <= s.maxFailureNotifications {
			if err := s.notifier.NotifyFailure(ctx, updated); err != nil {
				slog.Error("scheduler: notify failure", "id", endpointID, "error", err)
			}
		}
	} else {
		// OK
		if previousStatus != "ok" && previousStatus != "unknown" {
			// Recovery
			recovered, err := s.store.RecordRecovery(ctx, endpointID, statusCode)
			if err != nil {
				slog.Error("scheduler: record recovery", "id", endpointID, "error", err)
				return
			}

			var downtime time.Duration
			if recovered.LastFailureAt.Valid {
				downtime = time.Since(recovered.LastFailureAt.Time)
			}

			if err := s.notifier.NotifyRecovery(ctx, recovered, downtime); err != nil {
				slog.Error("scheduler: notify recovery", "id", endpointID, "error", err)
			}
		} else {
			// Already OK, just update status
			if err := s.store.UpdateEndpointStatus(ctx, endpointID, "ok", statusCode); err != nil {
				slog.Error("scheduler: update status", "id", endpointID, "error", err)
			}
		}
	}
}
