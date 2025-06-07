package scrapingdog

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"talent-rodeo/internal"
	"time"

	"github.com/getsentry/sentry-go"
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

// WorkerSettings holds the settings for the worker manager
type WorkerSettings struct {
	Paused bool // Whether the workers should process the search queue
}

type JobDetailTask struct {
	SearchTask internal.SearchTask
	JobListing internal.JobListing
}

// WorkerTask is a union type for all possible queue items
// Only one of SearchTask or JobListing should be set, depending on Type
// (Extendable for future types)
type WorkerTask struct {
	UserID        *string              // optional user ID for live updates
	SearchTask    *internal.SearchTask // for search tasks
	JobDetailTask *JobDetailTask       // for job detail fetches
}

// WorkerManager handles background processing of search jobs
// (Refactored: now uses a single shared queue for all task types)
type WorkerManager struct {
	provider     SearchProvider
	taskQueue    chan WorkerTask // Shared queue for all task types
	semaphore    chan struct{}
	workerWg     sync.WaitGroup
	logger       *slog.Logger
	shutdownCtx  context.Context
	shutdownFunc context.CancelFunc
	shutdownFlag int32
	eventHandler internal.JobEventHandler
	pausedFlag   int32 // 0 = not paused, 1 = paused
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

// NewWorkerManager creates a new worker manager (single shared queue)
func NewWorkerManager(provider SearchProvider, maxConcurrent int, queueSize int, opts ...WorkerOption) *WorkerManager {
	ctx, cancel := context.WithCancel(context.Background())

	wm := &WorkerManager{
		provider:     provider,
		taskQueue:    make(chan WorkerTask, queueSize*10), // Shared queue for all tasks
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

// Start launches worker goroutines to process the shared task queue
func (wm *WorkerManager) Start(ctx context.Context, numWorkers int) error {
	wm.logger.Info("starting worker manager",
		"numWorkers", numWorkers,
		"queueSize", cap(wm.taskQueue),
		"maxConcurrent", cap(wm.semaphore))

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
	if !atomic.CompareAndSwapInt32(&wm.shutdownFlag, 0, 1) {
		return nil
	}
	wm.logger.InfoContext(ctx, "Shutting down worker manager")
	wm.shutdownFunc()
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

// QueueTask adds a WorkerTask to the shared queue
func (wm *WorkerManager) QueueTask(ctx context.Context, task WorkerTask) error {
	wm.logger.Debug(internal.Cyan("QueueTask"), "task", task)

	// Check if we're shutting down
	if atomic.LoadInt32(&wm.shutdownFlag) == 1 {
		internal.SentryReportError(ctx, fmt.Errorf("worker manager is shutting down"), sentry.LevelError, map[string]string{
			"action": "WorkerManager::QueueTask: worker manager is shutting down",
		})
		return fmt.Errorf("worker manager is shutting down")
	}

	// Try to add to the queue
	select {
	case wm.taskQueue <- task:
		wm.logger.Info("task queued successfully", "task", task)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("task queue is full")
	}
}

// GetQueueDepth returns the current number of items in the queue
func (wm *WorkerManager) GetQueueDepth() int {
	return len(wm.taskQueue)
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

// startWorker runs a worker goroutine to process tasks from the shared queue
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

			// Attempt to get a task from the queue
			var task WorkerTask
			var ok bool

			select {
			case task, ok = <-wm.taskQueue:
				if !ok {
					// Channel closed
					logger.Info("task queue closed, worker exiting")
					return
				}

				logger.Debug("received task", "task", task)
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

			// Process the task
			startTime := time.Now()
			wm.processWorkerTask(ctx, task)

			// Release the semaphore
			<-wm.semaphore

			// Log the result
			duration := time.Since(startTime)
			logger.Debug("completed task", "task", task, "duration", duration)
		}
	}
}

// processWorkerTask dispatches a WorkerTask by type
func (wm *WorkerManager) processWorkerTask(ctx context.Context, task WorkerTask) {
	wm.logger.Debug(internal.Cyan("processWorkerTask"), "task", task)

	if task.SearchTask != nil {
		wm.handleSearchTask(ctx, task)
	}
	if task.JobDetailTask != nil {
		wm.handleJobDetailTask(ctx, task)
	}
}

// handleSearchTask Worsses a SearchTask (pagination, enqueues job detail tasks)
func (wm *WorkerManager) handleSearchTask(ctx context.Context, wt WorkerTask) {
	if wt.SearchTask == nil {
		wm.logger.Error("search task is nil, skipping", "task", wt)
		return
	}
	searchJob := *wt.SearchTask
	logger := wm.logger.With("search", "jobID", searchJob.ID, "task", wt)
	if wt.UserID != nil {
		logger = logger.With("userID", *wt.UserID)
	}

	if wm.eventHandler != nil {
		wm.eventHandler.OnSearchStarted(searchJob, wt.UserID)
	}

	logger.Info(internal.Cyan("processing search task"))
	var searchErrors []error
	var allJobs []internal.JobListing
	if searchJob.Criteria.MaxPages == 0 {
		searchJob.Criteria.MaxPages = internal.DefaultMaxPages
	}
	for page := searchJob.PagesCompleted + 1; page <= searchJob.Criteria.MaxPages; page++ {
		select {
		case <-ctx.Done():
			logger.Warn("context cancelled during search task")
			return
		default:
		}
		listings, err := wm.provider.SearchJobListings(ctx, searchJob.Criteria, page)
		if err != nil {
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action": "WorkerManager::handleSearchTask: failed to search for jobs",
			})
			logger.Error("failed to search for jobs", "page", page, "error", err)
			searchErrors = append(searchErrors, fmt.Errorf("page %d: %w", page, err))
			break
		}
		if len(listings) == 0 {
			logger.Debug("no more results found", "page", page)
			break
		}
		var validJobs []internal.JobListing
		for _, job := range listings {
			select {
			case <-ctx.Done():
				logger.Warn("context cancelled during job listing processing")
				return
			default:
			}
			// Enqueue job detail fetch as a WorkerTask
			jobDetailTask := JobDetailTask{
				SearchTask: searchJob,
				JobListing: job,
			}
			workerTask := WorkerTask{UserID: wt.UserID, JobDetailTask: &jobDetailTask}
			logger.Debug(internal.Cyan("created WorkerTask for job detail fetch"))
			select {
			case wm.taskQueue <- workerTask:
				logger.Debug("enqueued job detail fetch")
				validJobs = append(validJobs, job)
			default:
				logger.Warn("task queue full, dropping job detail fetch")
			}

			if wm.eventHandler != nil {
				wm.eventHandler.OnJobListing(searchJob, job, wt.UserID)
			}
		}
		logger.Debug("found job listings", "page", page, "count", len(listings), "validJobs", len(validJobs))
		searchJob.PagesCompleted = page
		now := time.Now().UTC()
		searchJob.UpdatedAt = &now
		if wm.eventHandler != nil {
			wm.eventHandler.OnPageComplete(searchJob, page, len(validJobs), searchErrors, wt.UserID)
		}
		allJobs = append(allJobs, validJobs...)
	}
	now := time.Now().UTC()
	searchJob.Status = internal.SearchTaskStatusCompleted
	searchJob.PagesCompleted = searchJob.Criteria.MaxPages
	searchJob.UpdatedAt = &now
	if wm.eventHandler != nil {
		wm.eventHandler.OnSearchCompleted(searchJob, len(allJobs), searchErrors, wt.UserID)
	}
}

// handleJobDetailTask processes a JobDetail fetch (deduplication, fetch, store, event)
func (wm *WorkerManager) handleJobDetailTask(ctx context.Context, wt WorkerTask) {
	if wt.JobDetailTask == nil {
		wm.logger.Error("job detail task is nil, skipping", "task", wt)
		return
	}
	task := *wt.JobDetailTask
	logger := wm.logger.With("jobID", task.JobListing.ID)

	if wt.UserID != nil {
		logger = logger.With("userID", *wt.UserID)
	}
	logger.Debug("handleJobDetailTask")

	jobDetailID := task.JobListing.ID
	if jobDetailID == "" {
		logger.Error("job listing missing ID, skipping", "job", task.JobListing)
		return
	}

	jobDetail, err := wm.provider.GetJobDetails(ctx, task.JobListing.ProviderJobID)
	if err != nil {
		logger.Error("failed to fetch job details", "jobID", task.JobListing.ProviderJobID, "error", err)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "WorkerManager::handleJobDetailTask: failed to get job details",
			"jobID":  task.JobListing.ProviderJobID,
		})
		return
	}
	jobDetail.URL = task.JobListing.URL
	jobDetail.CompanyLogo = task.JobListing.CompanyLogo
	jobDetail.CompanyProfile = task.JobListing.CompanyProfile
	if wm.eventHandler != nil {
		wm.eventHandler.OnJobDetail(task.SearchTask, task.JobListing, *jobDetail, wt.UserID)
	}
}
