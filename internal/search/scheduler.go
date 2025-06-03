package search

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"talent-rodeo/internal"
	"talent-rodeo/internal/cloudflare"
	"time"

	"github.com/getsentry/sentry-go"
)

const (
	defaultSearchTickInterval = 5 * time.Minute
	SchedulerSettingsKey      = "search-scheduler"
)

// Mailer interface for sending notifications
type Mailer interface {
	// func (m *Mailer) Send(recipient string, data any, patterns ...string) error {
	Send(recipient string, data any, patterns ...string) error
}

type SchedulerSettings struct {
	Paused    bool
	Interval  time.Duration
	StartHour int // Hour to start processing (24-hour format, UTC)
	EndHour   int // Hour to end processing (24-hour format, UTC)
}

var DefaultSchedulerSettings = SchedulerSettings{
	Paused:    false, // Default to not paused (active)
	Interval:  defaultSearchTickInterval,
	StartHour: 17, // 17:00 UTC
	EndHour:   0,  // 00:00 UTC (crosses midnight)
}

// The search scheduler determines which SearchCriteria need to be processed.
type Scheduler struct {
	fs        *internal.FirestoreClient
	logger    *slog.Logger
	mailer    Mailer
	recipient string
	ticker    *time.Ticker
	wg        sync.WaitGroup
	done      chan struct{}

	queue internal.SearchQueue
	kv    *cloudflare.KV

	settings SchedulerSettings

	disableSearchSchedulingLogOnce sync.Once
}

// NewScheduler creates a new search service
func NewScheduler(fs *internal.FirestoreClient, logger *slog.Logger, queue internal.SearchQueue, mailer Mailer, recipient string) *Scheduler {

	logger = logger.With("pkg", "search.Scheduler")

	// Initialize Cloudflare KV client for feature flag
	kv, err := cloudflare.NewKV()
	if err != nil {
		internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
			"action": "NewScheduler: failed to initialize Cloudflare KV client",
		})
		logger.Error("failed to initialize Cloudflare KV client", "error", err)
	}

	if queue == nil {
		internal.SentryReportError(context.Background(), errors.New("queue is nil"), sentry.LevelError, map[string]string{
			"action": "NewScheduler: queue is nil",
		})
		panic("NewScheduler: queue is nil")
	}

	return &Scheduler{
		fs:        fs,
		logger:    logger,
		mailer:    mailer,
		recipient: recipient,
		queue:     queue,
		ticker:    time.NewTicker(defaultSearchTickInterval),
		wg:        sync.WaitGroup{},
		done:      make(chan struct{}),
		kv:        kv,
		// pusher:    pusher,
		// progressCache: progressCache,
	}
}

// Start begins the background search processing
func (s *Scheduler) Start(ctx context.Context) error {
	s.logger.Info("starting search scheduler")

	if s.queue == nil {
		internal.SentryReportError(context.Background(), errors.New("queue is nil"), sentry.LevelError, map[string]string{
			"action": "NewScheduler::Start: queue is nil",
		})
		panic("NewScheduler::Start: queue is nil")
	}

	s.RefreshSettings()

	// Then requeue any users that were previously queued but not completed
	if err := s.RecoverIncompleteSearches(); err != nil {
		internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
			"action": "NewScheduler::Start: failed to requeue incomplete searches",
		})
		return fmt.Errorf("failed to requeue incomplete searches: %w", err)
	}

	// Start the background processing
	s.wg.Add(1) // used for graceful shutdown if Close is called
	go func() {
		// Process searches immediately on start
		s.StartCriteriaSearch(ctx)
		s.wg.Done()

		// Then continue with regular interval processing
		s.run(ctx)
	}()

	return nil
}

func (s *Scheduler) RefreshSettings() error {
	var settings SchedulerSettings
	if s.kv != nil {
		if err := s.kv.Get(SchedulerSettingsKey, &settings); err != nil {
			settings = DefaultSchedulerSettings
			s.logger.Info("No search scheduler settings found, using defaults")
			if err := s.kv.Set(SchedulerSettingsKey, settings, 0); err != nil {
				internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
					"action": "NewScheduler::RefreshSettings: failed to set default search scheduler settings",
				})
				return err
			}
			s.logger.Info("Default search scheduler settings saved to Cloudflare KV", "settings", settings)
		}
	} else {
		s.logger.Info("Cloudflare KV not initialized, using default settings")
	}

	s.settings = settings
	if s.settings.Interval <= 0 {
		s.settings.Interval = defaultSearchTickInterval
	}
	// Set default time window if not configured
	if s.settings.StartHour == 0 && s.settings.EndHour == 0 {
		s.settings.StartHour = DefaultSchedulerSettings.StartHour
		s.settings.EndHour = DefaultSchedulerSettings.EndHour
	}
	s.ticker.Reset(s.settings.Interval)
	return nil
}

// Close gracefully shuts down the search service
func (s *Scheduler) Close(_ context.Context) error {
	s.logger.Info("stopping search service")
	if s.ticker != nil {
		s.ticker.Stop()
	}

	// Signal processing to stop
	close(s.done)

	// Wait for any in-progress processing to complete
	s.wg.Wait()
	s.logger.Info("search service stopped")

	return nil
}

// run handles the background search processing
func (s *Scheduler) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("background processing stopped due to context cancellation")
			return
		case <-s.ticker.C:
			s.wg.Add(1) // used for graceful shutdown if Close is called
			s.StartCriteriaSearch(ctx)
			s.wg.Done()
		case <-s.done:
			s.logger.Info("background processing stopped")
			return
		}
	}
}

func (s *Scheduler) Paused() bool {
	return s.settings.Paused
}

// IsWithinAllowedHours checks if the current time (UTC) is within the configured off-peak hours.
// The time window can span midnight (e.g., 17:00-00:00).
func (s *Scheduler) IsWithinAllowedHours() bool {
	// Special case: if both are 0, run 24/7
	if s.settings.StartHour == 0 && s.settings.EndHour == 0 {
		return true
	}

	now := time.Now().UTC()
	currentHour := now.Hour()

	// Handle the case where the time window crosses midnight
	if s.settings.StartHour > s.settings.EndHour {
		// Window crosses midnight (e.g., 17:00-00:00)
		return currentHour >= s.settings.StartHour || currentHour < s.settings.EndHour
	} else {
		// Window doesn't cross midnight (e.g., 09:00-17:00)
		return currentHour >= s.settings.StartHour && currentHour < s.settings.EndHour
	}
}

// StartCriteriaSearch finds criteria that need searches performed
func (s *Scheduler) StartCriteriaSearch(ctx context.Context) {
	s.RefreshSettings()

	if s.settings.Paused {
		s.disableSearchSchedulingLogOnce.Do(func() {
			s.logger.Info(internal.BGYellow("search scheduler paused"))
		})
		return
	}

	if !s.IsWithinAllowedHours() {
		s.logger.Debug("search scheduler outside allowed hours",
			"start_hour", s.settings.StartHour,
			"end_hour", s.settings.EndHour,
			"current_time_utc", time.Now().UTC().Format("15:04"))
		return
	}

	items, err := internal.GetSearchCriteriaNeedsSearch(ctx, s.fs)
	if err != nil {
		s.logger.Error("error getting search criteria", "error", err)
		return
	}

	processedCount := 0
	s.logger.Debug("processing search criteria", "count", len(items))
	for _, criteria := range items {
		if err := s.enqueueSearch(ctx, criteria); err != nil {
			criteria.Error = err.Error()
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action":   "NewScheduler::StartCriteriaSearch: failed to enqueue search",
				"criteria": criteria.Query,
			})
		}
		processedCount++
	}

	if processedCount > 0 {
		depth, err := s.queue.GetQueueDepth(ctx)
		if err != nil {
			s.logger.Error("error getting queue depth", "error", err)
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action": "NewScheduler::StartCriteriaSearch: failed to get queue depth",
			})
		}
		s.logger.Info("newly enqueued", "new", processedCount, "queue_depth", depth)
	}
}

// enqueueSearch queues a job search for a user
func (s *Scheduler) enqueueSearch(ctx context.Context, criteria internal.SearchCriteria) error {
	if criteria.Query == "" {
		return fmt.Errorf("nil search")
	}

	jobSearch, err := internal.CreateSearchTask(ctx, s.fs, criteria, "scrapingdog")
	if err != nil {
		if errors.Is(err, internal.ErrSearchTaskExists) {
			s.logger.Debug("search task already exists", "criteria", criteria)
			return nil
		}
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action":   "NewScheduler::enqueueSearch: failed to save job search",
			"criteria": criteria.Query,
		})
		return err
	}

	// Queue the search with the provider
	s.logger.Info("enqueueSearch", "criteria", criteria)
	if err := s.queue.QueueSearch(ctx, *jobSearch); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action":   "NewScheduler::enqueueSearch: failed to queue search",
			"criteria": criteria.Query,
		})
		return err
	}

	return nil
}

// notifyAdmin sends an error notification to the admin
func (s *Scheduler) notifyAdmin(_ context.Context, user *internal.User, err error) {
	data := map[string]interface{}{
		"UserID":    user.ID,
		"UserEmail": user.Email,
		"Error":     err.Error(),
		"Timestamp": time.Now().Format(time.RFC3339),
	}

	if err := s.mailer.Send(s.recipient, data, "search-error.html"); err != nil {
		s.logger.Error("failed to send admin notification",
			"error", err,
			"user_id", user.ID)
	}
}

// RecoverIncompleteSearches finds and requeues any users that were previously queued but not completed
func (s *Scheduler) RecoverIncompleteSearches() error {
	if s.settings.Paused {
		return nil
	}

	if !s.IsWithinAllowedHours() {
		s.logger.Debug("skipping recovery of incomplete searches - outside allowed hours")
		return nil
	}

	// TODO: If several servers start at the same time, they will all try to requeue the same jobs.

	// get all SearchJobs with a status that is not "complete"
	// and requeue thems

	s.logger.Info("checking for past-due searches")

	searchTasks, err := internal.GetIncompleteSearchTasks(context.Background(), s.fs)
	if err != nil {
		internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
			"action": "NewScheduler::RecoverIncompleteSearches: failed to get incomplete search tasks",
		})
		return err
	}

	for _, searchTask := range searchTasks {
		if err := internal.SetSearchTaskStatus(context.Background(), s.fs, searchTask.ID, internal.SearchTaskStatusQueued); err != nil {
			internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
				"action": "NewScheduler::RecoverIncompleteSearches: failed to set search task status",
			})
			return err
		}
		if err := s.queue.QueueSearch(context.Background(), *searchTask); err != nil {
			internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
				"action": "NewScheduler::RecoverIncompleteSearches: failed to requeue search task",
			})
			return err
		}
	}

	return nil
}
