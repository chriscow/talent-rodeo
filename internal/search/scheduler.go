package search

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"talent-rodeo/internal"
	"time"
)

const (
	// defaultSearchTickInterval = 15 * time.Minute
	defaultSearchTickInterval = 5 * time.Second
)

// Mailer interface for sending notifications
type Mailer interface {
	// func (m *Mailer) Send(recipient string, data any, patterns ...string) error {
	Send(recipient string, data any, patterns ...string) error
}

// Scheduler manages background job searches for users
type Scheduler struct {
	fs        *internal.FirestoreClient
	logger    *slog.Logger
	mailer    Mailer
	recipient string
	ticker    *time.Ticker
	wg        sync.WaitGroup
	done      chan struct{}

	queue JobQueue

	disableSearchSchedulingLogOnce sync.Once
}

// NewScheduler creates a new search service
func NewScheduler(fs *internal.FirestoreClient, logger *slog.Logger, queue JobQueue, mailer Mailer, recipient string) *Scheduler {

	// Initialize the progress cache
	// progressCache, err := NewSearchProgressCache()
	// if err != nil {
	// 	logger.Error("failed to initialize progress cache", "error", err)
	// 	// Continue with nil cache, will fallback to map-based implementation
	// }
	logger = logger.With("pkg", "search.Scheduler")

	if queue == nil {
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
		// pusher:    pusher,
		// progressCache: progressCache,
	}
}

// Start begins the background search processing
func (s *Scheduler) Start(ctx context.Context) error {
	s.logger.Info("starting search scheduler")
	if s.ticker == nil {
		s.ticker = time.NewTicker(defaultSearchTickInterval)
	}

	if s.queue == nil {
		panic("NewScheduler::Start: queue is nil")
	}

	// Then requeue any users that were previously queued but not completed
	if err := s.requeueIncompleteSearches(); err != nil {
		return fmt.Errorf("failed to requeue incomplete searches: %w", err)
	}

	// Start the background processing
	go func() {
		s.logger.Info("checking for past-due searches")
		// Process searches immediately on start
		s.wg.Add(1)
		s.processSearches(ctx)
		s.wg.Done()
		s.logger.Info("completed past-due searches")

		// Then continue with regular interval processing
		s.run(ctx)
	}()

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
			s.wg.Add(1)
			s.processSearches(ctx)
			s.wg.Done()
		case <-s.done:
			s.logger.Info("background processing stopped")
			return
		}
	}
}

// processSearches finds users who need searches performed
func (s *Scheduler) processSearches(ctx context.Context) {
	if os.Getenv("DISABLE_SEARCH_SCHEDULING") == "true" {
		s.disableSearchSchedulingLogOnce.Do(func() {
			s.logger.Info(internal.BGYellow("search scheduling disabled"))
		})
		return
	}

	items, err := internal.GetSearchCriteriaNeedsSearch(ctx, s.fs)
	if err != nil {
		s.logger.Error("error getting search criteria", "error", err)
		return
	}

	processedCount := 0
	for _, criteria := range items {
		if err := s.enqueueSearch(ctx, criteria); err != nil {
			criteria.Error = err.Error()
		}
		processedCount++
	}

	if processedCount > 0 {
		depth, err := s.queue.GetQueueDepth(ctx)
		if err != nil {
			s.logger.Error("error getting queue depth", "error", err)
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
		return fmt.Errorf("failed to save job search: %w", err)
	}

	// Queue the search with the provider
	s.logger.Info("enqueueSearch", "criteria", criteria)
	if err := s.queue.QueueSearch(ctx, *jobSearch); err != nil {
		return fmt.Errorf("failed to queue search: %w", err)
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

// requeueIncompleteSearches finds and requeues any users that were previously queued but not completed
func (s *Scheduler) requeueIncompleteSearches() error {

	// get all SearchJobs with a status that is not "complete"
	// and requeue thems
	searchTasks, err := internal.GetIncompleteSearchTasks(context.Background(), s.fs)
	if err != nil {
		return fmt.Errorf("failed to get incomplete search tasks: %w", err)
	}

	for _, searchTask := range searchTasks {
		if err := internal.SetSearchTaskStatus(context.Background(), s.fs, searchTask.ID, internal.SearchTaskStatusQueued); err != nil {
			return fmt.Errorf("failed to set search task status: %w", err)
		}
		if err := s.queue.QueueSearch(context.Background(), *searchTask); err != nil {
			return fmt.Errorf("failed to requeue search task: %w", err)
		}
	}

	return nil
}
