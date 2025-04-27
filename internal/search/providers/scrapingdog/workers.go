package scrapingdog

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search"
	"time"
)

const maxPages = 5

// Forward declaration to avoid circular dependencies
type SearchProvider interface {
	SearchJobListings(ctx context.Context, criteria internal.SearchCriteria, page int) ([]internal.JobListing, error)
	GetJobDetails(ctx context.Context, jobID string) (*internal.JobDetail, error)
}

// WorkerManager handles background processing of search jobs
type WorkerManager struct {
	provider     SearchProvider
	searchQueue  chan internal.SearchTask
	semaphore    chan struct{}
	workerWg     sync.WaitGroup
	logger       *slog.Logger
	shutdownCtx  context.Context
	shutdownFunc context.CancelFunc
	shutdownFlag int32
	eventHandler search.JobEventHandler
}

// WorkerOption configures the WorkerManager
type WorkerOption func(*WorkerManager)

// WithWorkerLogger sets a custom logger for the worker manager
func withWorkerLogger(logger *slog.Logger) WorkerOption {
	return func(wm *WorkerManager) {
		wm.logger = logger
	}
}

// WithEventHandler sets the event handler for the worker manager
func withEventHandler(handler search.JobEventHandler) WorkerOption {
	return func(wm *WorkerManager) {
		wm.eventHandler = handler
	}
}

// NewWorkerManager creates a new worker manager
func NewWorkerManager(provider SearchProvider, maxConcurrent int, queueSize int, opts ...WorkerOption) *WorkerManager {
	ctx, cancel := context.WithCancel(context.Background())

	wm := &WorkerManager{
		provider:     provider,
		searchQueue:  make(chan internal.SearchTask, queueSize),
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
func (wm *WorkerManager) Start(ctx context.Context, numWorkers int) error {
	wm.logger.Info("starting worker manager",
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

	return nil
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
func (wm *WorkerManager) QueueSearch(ctx context.Context, searchJob internal.SearchTask) error {
	// Check if we're shutting down
	if atomic.LoadInt32(&wm.shutdownFlag) == 1 {
		return fmt.Errorf("worker manager is shutting down")
	}

	// Try to add to the queue
	select {
	case wm.searchQueue <- searchJob:
		wm.logger.InfoContext(ctx, "Search queued successfully",
			"query", searchJob.Criteria.Query,
			"geoID", searchJob.Criteria.GeoID,
			"maxPages", searchJob.Criteria.MaxPages)
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
	logger := wm.logger.With("pkg", "scrapingdog", "workerID", workerID)

	logger.Debug("starting worker")

	for {
		// Check if we should shut down
		select {
		case <-wm.shutdownCtx.Done():
			logger.Info("shutting down worker")
			return
		default:
			// Continue processing
		}

		// Attempt to get a search request from the queue
		var searchJob internal.SearchTask
		var ok bool

		select {
		case searchJob, ok = <-wm.searchQueue:
			if !ok {
				// Channel closed
				logger.Info("search queue closed, worker exiting")
				return
			}
		case <-wm.shutdownCtx.Done():
			logger.Info("shutting down worker")
			return
		}

		// Acquire semaphore (limit concurrent processing)
		select {
		case wm.semaphore <- struct{}{}:
			// Continue processing
		case <-wm.shutdownCtx.Done():
			logger.Info("shutting down worker")
			return
		}

		// Process the search request
		startTime := time.Now()
		logger.Info("processing search",
			"query", searchJob.Criteria.Query)

		err := wm.processQueuedSearch(ctx, workerID, searchJob)

		// Release the semaphore
		<-wm.semaphore

		// Log the result
		duration := time.Since(startTime)
		if err != nil {
			logger.Error("failed to process search",
				"query", searchJob.Criteria.Query,
				"duration", duration,
				"error", err)
		} else {
			logger.Info("completed search",
				"query", searchJob.Criteria.Query,
				"duration", duration)
		}
	}
}

// processQueuedSearch handles the search processing workflow
func (wm *WorkerManager) processQueuedSearch(ctx context.Context, workerID int, searchJob internal.SearchTask) error {
	// Create a child context that can be canceled if the parent is done
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Set a timeout for the entire search process
	ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	searchJob.Status = internal.SearchTaskStatusProcessing
	if wm.eventHandler != nil {
		wm.eventHandler.OnSearchStarted(searchJob)
	}

	// Create new logger with request context
	logger := wm.logger.With(
		"pkg", "scrapingdog",
		"workerID", workerID,
		"query", searchJob.Criteria.Query,
		"location", searchJob.Criteria.GeoID)

	logger.Info("processing queued search")

	// Track errors for reporting
	var searchErrors []error

	// Handle pagination manually
	var allJobs []internal.JobListing

	if searchJob.Criteria.MaxPages == 0 {
		searchJob.Criteria.MaxPages = maxPages
	}

	for page := 1; page <= searchJob.Criteria.MaxPages; page++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Continue processing
		}

		// Execute the search
		jobs, err := wm.provider.SearchJobListings(ctx, searchJob.Criteria, page)

		// Handle errors
		if err != nil {
			logger.Error("failed to search for jobs",
				"page", page,
				"error", err)
			searchErrors = append(searchErrors, fmt.Errorf("page %d: %w", page, err))
			continue
		}

		// If we got no results, stop pagination
		if len(jobs) == 0 {
			logger.Info("no more results found", "page", page)
			break
		}

		// Add jobs to the list
		allJobs = append(allJobs, jobs...)

		wm.logger.Info("found jobs on page",
			"page", page,
			"count", len(jobs),
			"totalSoFar", len(allJobs))
	}

	// Log completion
	logger.Info("search completed",
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
			logger.Error("failed to get job details",
				"jobID", job.ID,
				"error", err)
			searchErrors = append(searchErrors, fmt.Errorf("get details for %s: %w", job.ID, err))
			continue
		}

		// Notify about the new job
		if wm.eventHandler != nil {
			wm.eventHandler.OnJobDetail(searchJob, *details)
		}
		processedCount++
	}

	logger.Info("job processing completed",
		"totalJobs", len(allJobs),
		"processedJobs", processedCount,
		"errors", len(searchErrors))

	// Update user's search status
	if wm.eventHandler != nil {
		wm.eventHandler.OnSearchCompleted(searchJob, len(allJobs), searchErrors)
	}

	return nil
}
