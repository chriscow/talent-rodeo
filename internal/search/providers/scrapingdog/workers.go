package scrapingdog

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"talent-rodeo/internal"
	"time"
)

/*
# ScrapingDog Points to Remember

  - Each request will be retried until it can be successfully completed (up to 60
    seconds). Remember to set your timeout to 60 seconds to ensure this process
    goes smoothly. In cases where every request fails in 60 seconds, our API will
    return a 410 error, you may retry the request and you will not be charged for
    the unsuccessful request (you are only charged for successful requests, 200
    and 404 status codes). Make sure to catch these errors! They will occur on
    roughly 1-2% of requests for hard-to-scrape websites.

  - There is no overage allowed on the free plan, if you exceed 1000 requests per
    month on the free plan, you will receive a 403 error.

  - Each request will return a string containing the raw HTML from the page
    requested, along with any headers and cookies.

  - If you exceed your plan concurrent connection limit, the API will respond with
    a 429 status code, this can be solved by slowing down your request rate or
    upgrading to a bigger plan.

# Error Codes

| Code | Description |
|------|-------------|
| 200 | Successfull Request |
| 410 | Request timeout |
| 404 | URL is wrong |
| 202 | Your request is accepted and the scraping is still going on. | **This status code only appears in the LinkedIn Scraping API**
| 403 | Request Limit Reached. |
| 429 | Concurrent connection limit reached. |
| 401 | API Key is wrong. |
| 400 | Request failed. |
*/

// Forward declaration to avoid circular dependencies
type SearchProvider interface {
	SearchJobListings(ctx context.Context, criteria internal.SearchCriteria, page int) ([]internal.JobListing, error)
	GetJobDetails(ctx context.Context, jobID string) (*internal.JobDetail, error)
}

type WorkerSettings struct {
	Paused bool // Whether the workers should process the search queue
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
	eventHandler internal.JobEventHandler
	// Pause control using atomic operations
	pausedFlag int32 // 0 = not paused, 1 = paused
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
func withEventHandler(handler internal.JobEventHandler) WorkerOption {
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

	wm.logger = wm.logger.With("pkg", "scrapingdog.WorkerManager")

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
		wm.logger.InfoContext(ctx, "search queued successfully",
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

// Pause sets the pause state of the workers
func (wm *WorkerManager) Pause(paused bool) {
	var newValue int32
	if paused {
		newValue = 1
	}

	oldValue := atomic.SwapInt32(&wm.pausedFlag, newValue)
	oldPaused := (oldValue == 1)

	if oldPaused != paused {
		if paused {
			wm.logger.Info(internal.BGYellow("scrapingdog workers paused"), "paused", paused)
		} else {
			wm.logger.Info(internal.BGGreen("scrapingdog workers resumed"), "paused", paused)
		}
	}
}

// IsPaused returns the current pause state
func (wm *WorkerManager) IsPaused() bool {
	return atomic.LoadInt32(&wm.pausedFlag) == 1
}

// startWorker runs a worker goroutine to process search requests
func (wm *WorkerManager) startWorker(ctx context.Context, workerID int) {
	logger := wm.logger.With("pkg", "scrapingdog", "workerID", workerID)

	logger.Debug("starting worker")

	for {
		select {
		case <-wm.shutdownCtx.Done():
			logger.Info("shutting down worker")
			return
		default:
			// Check if paused
			if atomic.LoadInt32(&wm.pausedFlag) == 1 {
				time.Sleep(5 * time.Second)
				continue
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
			case <-time.After(1 * time.Second):
				// Timeout to allow pause check
				continue
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
}

// processSearchPage processes a single page of job search results
func (wm *WorkerManager) processSearchPage(ctx context.Context, searchJob *internal.SearchTask, page int, logger *slog.Logger) ([]internal.JobListing, []error) {
	var pageErrors []error

	// Execute the search for this page
	listings, err := wm.provider.SearchJobListings(ctx, searchJob.Criteria, page)

	// Handle errors
	if err != nil {
		logger.Error("failed to search for jobs",
			"page", page,
			"error", err)
		pageErrors = append(pageErrors, fmt.Errorf("page %d: %w", page, err))
		return nil, pageErrors
	}

	// If we got no results, this is not an error but indicates end of pagination
	if len(listings) == 0 {
		logger.Debug("no more results found", "page", page)
		return nil, pageErrors
	}

	// Process each job listing to get details
	var validJobs []internal.JobListing
	for _, job := range listings {
		select {
		case <-ctx.Done():
			pageErrors = append(pageErrors, ctx.Err())
			return validJobs, pageErrors
		default:
			// Continue processing
		}

		// Get job details
		details, err := wm.provider.GetJobDetails(ctx, job.ProviderJobID)
		if err != nil {
			logger.Error("failed to get job details",
				"jobID", job.ID,
				"error", err)
			pageErrors = append(pageErrors, fmt.Errorf("get details for %s: %w", job.ID, err))
			continue
		}

		details.URL = job.URL                       // this is the original URL from the job listing
		details.CompanyLogo = job.CompanyLogo       // use the logo from the listing
		details.CompanyProfile = job.CompanyProfile // use the profile from the listing

		// Notify about the new job
		if wm.eventHandler != nil {
			wm.eventHandler.OnJobDetail(*searchJob, *details)
		}

		validJobs = append(validJobs, job)
	}

	logger.Debug("found jobs",
		"page", page,
		"count", len(listings),
		"validJobs", len(validJobs))

	searchJob.PagesCompleted = page
	now := time.Now().UTC()
	searchJob.UpdatedAt = &now

	// Notify about page completion
	if wm.eventHandler != nil {
		wm.eventHandler.OnPageComplete(*searchJob, page, len(validJobs), pageErrors)
	}

	return validJobs, pageErrors
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
		searchJob.Criteria.MaxPages = internal.DefaultMaxPages
	}

	for page := searchJob.PagesCompleted + 1; page <= searchJob.Criteria.MaxPages; page++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Continue processing
		}

		// Process this page of search results
		pageJobs, pageErrors := wm.processSearchPage(ctx, &searchJob, page, logger)

		// Add any errors from this page to our overall error collection
		searchErrors = append(searchErrors, pageErrors...)

		// If we got no results, stop pagination
		if len(pageJobs) == 0 {
			break
		}

		// Add jobs to the list
		allJobs = append(allJobs, pageJobs...)
	}

	// Log completion
	logger.Info("search completed",
		"totalJobs", len(allJobs),
		"errors", len(searchErrors))

	// Update user's search status
	if wm.eventHandler != nil {
		wm.eventHandler.OnSearchCompleted(searchJob, len(allJobs), searchErrors)
	}

	return nil
}
