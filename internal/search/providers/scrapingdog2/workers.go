package scrapingdog2

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"talent-rodeo/internal/search"
)

const maxPages = 5

// Forward declaration to avoid circular dependencies
type Provider interface {
	SearchJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error)
	GetJobDetails(ctx context.Context, jobID string) (search.JobDetail, error)
}

// WorkerManager handles background processing of search jobs
type WorkerManager struct {
	provider     Provider
	searchQueue  chan QueuedSearch
	semaphore    chan struct{}
	workerWg     sync.WaitGroup
	logger       *slog.Logger
	shutdownCtx  context.Context
	shutdownFunc context.CancelFunc
	shutdownFlag int32
	store        JobStore
	eventHandler JobEventHandler
}

// WorkerOption configures the WorkerManager
type WorkerOption func(*WorkerManager)

// WithWorkerLogger sets a custom logger for the worker manager
func WithWorkerLogger(logger *slog.Logger) WorkerOption {
	return func(wm *WorkerManager) {
		wm.logger = logger
	}
}

// WithJobStore sets the job store for the worker manager
func WithJobStore(store JobStore) WorkerOption {
	return func(wm *WorkerManager) {
		wm.store = store
	}
}

// WithEventHandler sets the event handler for the worker manager
func WithEventHandler(handler JobEventHandler) WorkerOption {
	return func(wm *WorkerManager) {
		wm.eventHandler = handler
	}
}

// NewWorkerManager creates a new worker manager
func NewWorkerManager(provider Provider, maxConcurrent int, queueSize int, opts ...WorkerOption) *WorkerManager {
	ctx, cancel := context.WithCancel(context.Background())

	wm := &WorkerManager{
		provider:     provider,
		searchQueue:  make(chan QueuedSearch, queueSize),
		semaphore:    make(chan struct{}, maxConcurrent),
		logger:       slog.Default(),
		shutdownCtx:  ctx,
		shutdownFunc: cancel,
		shutdownFlag: 0,
	}

	for _, opt := range opts {
		opt(wm)
	}

	return wm
}

// Start launches worker goroutines to process the search queue
func (wm *WorkerManager) Start(ctx context.Context, numWorkers int) {
	wm.logger.InfoContext(ctx, "Starting worker manager",
		"numWorkers", numWorkers,
		"queueSize", cap(wm.searchQueue),
		"maxConcurrent", cap(wm.semaphore))

	// Launch the workers
	for i := 0; i < numWorkers; i++ {
		wm.workerWg.Add(1)
		id := i + 1
		go func(workerID int) {
			defer wm.workerWg.Done()
			wm.startWorker(ctx, workerID)
		}(id)
	}
}

// Shutdown gracefully stops the worker manager
func (wm *WorkerManager) Shutdown(ctx context.Context) error {
	// Set the atomic shutdown flag
	if !atomic.CompareAndSwapInt32(&wm.shutdownFlag, 0, 1) {
		// Already shutting down
		return nil
	}

	wm.logger.InfoContext(ctx, "Shutting down worker manager")

	// Signal cancellation to workers
	wm.shutdownFunc()

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		wm.workerWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		wm.logger.InfoContext(ctx, "Worker manager shutdown complete")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown timed out: %w", ctx.Err())
	}
}

// QueueSearch adds a search request to the queue
func (wm *WorkerManager) QueueSearch(ctx context.Context, userID string, criteria search.Criteria) error {
	// Check if we're shutting down
	if atomic.LoadInt32(&wm.shutdownFlag) == 1 {
		return fmt.Errorf("worker manager is shutting down")
	}

	// Try to add to the queue
	select {
	case wm.searchQueue <- QueuedSearch{UserID: userID, Criteria: criteria}:
		wm.logger.InfoContext(ctx, "Search queued successfully",
			"userID", userID,
			"keywords", criteria.Keywords)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("search queue is full")
	}
}

// GetQueueDepth returns the current number of items in the queue
func (wm *WorkerManager) GetQueueDepth() int {
	return len(wm.searchQueue)
}

// startWorker runs a worker goroutine to process search requests
func (wm *WorkerManager) startWorker(ctx context.Context, workerID int) {
	wm.logger.InfoContext(ctx, "Worker started", "workerID", workerID)

	for {
		// Check if we should shut down
		select {
		case <-wm.shutdownCtx.Done():
			wm.logger.InfoContext(ctx, "Worker shutting down", "workerID", workerID)
			return
		default:
			// Continue processing
		}

		// Attempt to get a search request from the queue
		var qs QueuedSearch
		var ok bool

		select {
		case qs, ok = <-wm.searchQueue:
			if !ok {
				// Channel closed
				wm.logger.InfoContext(ctx, "Search queue closed, worker exiting", "workerID", workerID)
				return
			}
		case <-wm.shutdownCtx.Done():
			wm.logger.InfoContext(ctx, "Worker shutting down", "workerID", workerID)
			return
		}

		// Acquire semaphore (limit concurrent processing)
		select {
		case wm.semaphore <- struct{}{}:
			// Continue processing
		case <-wm.shutdownCtx.Done():
			wm.logger.InfoContext(ctx, "Worker shutting down", "workerID", workerID)
			return
		}

		// Process the search request
		startTime := time.Now()
		wm.logger.InfoContext(ctx, "Worker processing search",
			"workerID", workerID,
			"userID", qs.UserID,
			"keywords", qs.Criteria.Keywords)

		err := wm.processQueuedSearch(ctx, qs)

		// Release the semaphore
		<-wm.semaphore

		// Log the result
		duration := time.Since(startTime)
		if err != nil {
			wm.logger.ErrorContext(ctx, "Worker failed to process search",
				"workerID", workerID,
				"userID", qs.UserID,
				"keywords", qs.Criteria.Keywords,
				"duration", duration,
				"error", err)
		} else {
			wm.logger.InfoContext(ctx, "Worker completed search",
				"workerID", workerID,
				"userID", qs.UserID,
				"keywords", qs.Criteria.Keywords,
				"duration", duration)
		}
	}
}

// processQueuedSearch handles the search processing workflow
func (wm *WorkerManager) processQueuedSearch(ctx context.Context, qs QueuedSearch) error {
	// Create a child context that can be canceled if the parent is done
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Set a timeout for the entire search process
	ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Create new logger with request context
	logger := wm.logger.With(
		"userID", qs.UserID,
		"keywords", qs.Criteria.Keywords,
		"location", qs.Criteria.Location)

	logger.InfoContext(ctx, "Processing queued search")

	// Track errors for reporting
	var searchErrors []error

	// Handle pagination manually
	var allJobs []search.JobListing

	for page := 1; page <= maxPages; page++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Continue processing
		}

		// Update criteria with page number
		criteria := qs.Criteria
		criteria.PageNumber = page

		// Execute the search
		jobs, err := wm.provider.SearchJobs(ctx, criteria)

		// Handle errors
		if err != nil {
			logger.ErrorContext(ctx, "Failed to search for jobs",
				"page", page,
				"error", err)
			searchErrors = append(searchErrors, fmt.Errorf("page %d: %w", page, err))
			continue
		}

		// If we got no results, stop pagination
		if len(jobs) == 0 {
			logger.InfoContext(ctx, "No more results found", "page", page)
			break
		}

		// Add jobs to the list
		allJobs = append(allJobs, jobs...)

		logger.InfoContext(ctx, "Found jobs on page",
			"page", page,
			"count", len(jobs),
			"totalSoFar", len(allJobs))

		// Avoid rate limiting
		time.Sleep(time.Second)
	}

	// Log completion
	logger.InfoContext(ctx, "Search completed",
		"totalJobs", len(allJobs),
		"errors", len(searchErrors))

	// Process found jobs (get details and create user jobs)
	processedCount := 0
	for _, job := range allJobs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Continue processing
		}

		// Get job details
		details, err := wm.provider.GetJobDetails(ctx, job.ProviderJobID)
		if err != nil {
			logger.ErrorContext(ctx, "Failed to get job details",
				"jobID", job.ID,
				"error", err)
			searchErrors = append(searchErrors, fmt.Errorf("get details for %s: %w", job.ID, err))
			continue
		}

		// Notify about the new job
		if wm.eventHandler != nil {
			if err := wm.eventHandler.OnNewJob(qs.UserID, qs.Criteria, job.ID, details); err != nil {
				logger.WarnContext(ctx, "Failed to notify about new job",
					"jobID", job.ID,
					"error", err)
				// Continue despite error
			}
		}

		processedCount++
	}

	logger.InfoContext(ctx, "Job processing completed",
		"totalJobs", len(allJobs),
		"processedJobs", processedCount,
		"errors", len(searchErrors))

	// Update user's search status
	if wm.eventHandler != nil {
		if err := wm.eventHandler.OnSearchCompleted(qs.UserID, qs.Criteria, len(allJobs), searchErrors); err != nil {
			logger.WarnContext(ctx, "Failed to notify about search completion", "error", err)
			// Continue despite error
		}
	}

	return nil
}
