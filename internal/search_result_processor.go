package internal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/getsentry/sentry-go"
	"github.com/pinecone-io/go-pinecone/v3/pinecone"
	"github.com/pusher/pusher-http-go/v5"
)

const (
	JobDetailNamespace               = "job-details" // Pinecone namespace for job details
	JobDetailExtractionQueueSize     = 2000
	MaxConcurrentJobDetailProcessing = 20
	queueLogInterval                 = 5 * time.Second
	queueRequeueInterval             = 4 * time.Hour
)

type ExtractionTask struct {
	SearchTaskID string
	UserID       *string
	JobDetail    JobDetail
}

type SearchProgress struct {
	UserID               string
	SearchTaskID         string
	TotalJobs            atomic.Int32
	ProcessedJobs        atomic.Int32
	AllJobsQueued        atomic.Bool
	mu                   sync.Mutex
	lastProgressNotified int
}

// JobProcessor is an interface that defines the methods for processing new job details
// returned from a search. It is used to enqueue jobs for post processing, such as
// normalizing and embedding the job details.
type JobProcessor interface {
	ProcessJob(ctx context.Context, jobID string) error
}

// SearchResultProcessor is a struct that implements the search.JobEventHandler interface.
// It is responsible for handling job listings and job details returned from a search.
// It saves the job listings and job details to Firestore and links them to the search criteria.
type SearchResultProcessor struct {
	fs            *FirestoreClient
	logger        *slog.Logger
	pusherClient  *pusher.Client
	eventDispatch *JobEventDispatch

	queue     chan ExtractionTask
	stopCh    chan struct{}
	stoppedCh chan struct{}
	workersWg sync.WaitGroup

	logTicker     *time.Ticker
	requeueTicker *time.Ticker

	queueHadItems    atomic.Bool // for logging when queue drains
	progressTrackers sync.Map
}

func NewSearchResultProcessor(fs *FirestoreClient, logger *slog.Logger, psh *pusher.Client, dispatch *JobEventDispatch) *SearchResultProcessor {
	logger = logger.With("pkg", "SearchResultProcessor")
	return &SearchResultProcessor{
		fs:            fs,
		logger:        logger,
		pusherClient:  psh,
		eventDispatch: dispatch,
		queue:         make(chan ExtractionTask, JobDetailExtractionQueueSize),
		stopCh:        make(chan struct{}),
		stoppedCh:     make(chan struct{}),
	}
}

/*
type JobEventHandler interface {
	OnSearchStarted(task SearchTask, userID *string)
	OnJobListing(task SearchTask, listing JobListing, userID *string)
	OnJobDetail(task SearchTask, listing JobListing, details JobDetail, userID *string)
	OnPageComplete(task SearchTask, page int, jobsFound int, errors []error, userID *string)
	OnSearchCompleted(task SearchTask, totalJobs int, errors []error, userID *string)
	OnJobDetailExtractionComplete(task SearchTask, jobDetail JobDetail, userID *string)
	OnAllExtractionsComplete(task SearchTask, totalJobs int, userID *string)
}
*/

// Start launches worker goroutines and periodic queue monitoring/requeueing.
func (s *SearchResultProcessor) Start() {
	// Start workers
	for i := 0; i < MaxConcurrentJobDetailProcessing; i++ {
		s.workersWg.Add(1)
		go s.worker()
	}

	s.logTicker = time.NewTicker(queueLogInterval)
	go s.queueLogger()

	s.requeueTicker = time.NewTicker(queueRequeueInterval)
	go s.periodicRequeue()

	// Initial requeue on start
	go s.requeueUnprocessedJobDetails()
}

// Stop signals workers to stop after finishing current job, and stops log/requeue tickers.
func (s *SearchResultProcessor) Stop() {
	close(s.stopCh)
	if s.logTicker != nil {
		s.logTicker.Stop()
	}
	if s.requeueTicker != nil {
		s.requeueTicker.Stop()
	}
	s.workersWg.Wait()
	close(s.stoppedCh)
}

// worker processes JobDetails from the queue, running ExtractJobDetailData.
func (s *SearchResultProcessor) worker() {
	// s.logger.Debug(BGYellow("starting job detail extraction worker"))
	defer s.workersWg.Done()
	for {
		select {
		case task, ok := <-s.queue:
			if !ok {
				s.logger.Debug(BGYellow("job detail extraction worker shutting down"))
				return
			}
			if task.JobDetail.ExtractedData == nil {
				s.logger.Debug(Cyan("extracting job detail data (async)"), "jobID", task.JobDetail.ID)
				ctx := context.Background()
				start := time.Now()
				err := ExtractJobDetailData(ctx, &task.JobDetail)
				if err != nil {
					s.logger.Error("failed to extract job detail data (async)", "error", err, "jobID", task.JobDetail.ID)
					SentryReportError(ctx, err, sentry.LevelError, map[string]string{
						"jobID": task.JobDetail.ID,
					})
					continue
				}
				s.logger.Debug(Cyan("job detail extraction complete (async)"), "jobID", task.JobDetail.ID, "duration", time.Since(start).String())
				// Save updated job detail
				err = SaveJobDetail(ctx, s.fs, &task.JobDetail)
				if err != nil {
					s.logger.Error("failed to save job detail after extraction", "error", err, "jobID", task.JobDetail.ID)
					SentryReportError(ctx, err, sentry.LevelError, map[string]string{
						"jobID": task.JobDetail.ID,
					})
				}
			}
			s.handleExtractionCompletion(task)

		case <-s.stopCh:
			s.logger.Debug(BGYellow("job detail extraction worker shutting down"))
			return
		}
	}
}

func (s *SearchResultProcessor) handleExtractionCompletion(task ExtractionTask) {
	fmt.Println(Yellow("PROGRESS_TRACKER_LOOKUP"), "searchTaskID:", task.SearchTaskID)
	if val, ok := s.progressTrackers.Load(task.SearchTaskID); ok {
		progress := val.(*SearchProgress)
		processed := progress.ProcessedJobs.Add(1)
		total := progress.TotalJobs.Load()

		fmt.Println(Green("PROGRESS_TRACKER_UPDATE"), "searchTaskID:", task.SearchTaskID, "processed:", processed, "total:", total)

		if progress.UserID != "" {
			// Notify every 5%
			currentProgress := int(float32(processed) / float32(total) * 100)
			if total > 0 && (currentProgress-progress.lastProgressNotified >= 5 || processed == total) {
				s.logger.Debug("sending progress update", "user", progress.UserID, "progress", currentProgress, "searchTaskID", task.SearchTaskID)
				SendPusherProgress(s.pusherClient, progress.UserID, "Extracting job details...", currentProgress, 100)
				progress.mu.Lock()
				progress.lastProgressNotified = currentProgress
				progress.mu.Unlock()
			}
		} else {
			s.logger.Debug(BGRed("no user ID"), "jobID", task.JobDetail.ID)
		}

		if progress.AllJobsQueued.Load() && processed == total {
			fmt.Println(Blue("PROGRESS_TRACKER_DELETE"), "searchTaskID:", task.SearchTaskID)
			s.logger.Info(Green("all extractions complete for search"), "searchTaskID", task.SearchTaskID)
			SendPusherRedirect(s.pusherClient, progress.UserID, "/dashboard")
			s.eventDispatch.OnAllExtractionsComplete(SearchTask{ID: task.SearchTaskID}, int(total), task.UserID)
			s.progressTrackers.Delete(task.SearchTaskID)
		} else {
			s.eventDispatch.OnJobDetailExtractionComplete(SearchTask{ID: task.SearchTaskID}, task.JobDetail, task.UserID)
		}

	} else {
		fmt.Println(Red("PROGRESS_TRACKER_NOT_FOUND"), "searchTaskID:", task.SearchTaskID, "jobID:", task.JobDetail.ID, "userID:", task.UserID)
		s.logger.Debug(BGRed("handleExtractionCompletion: progressTracker not found"), "jobID", task.JobDetail.ID)
	}
}

// queueLogger logs the queue size every 5s if it has items, and logs when it drains.
func (s *SearchResultProcessor) queueLogger() {
	for {
		select {
		case <-s.logTicker.C:
			qLen := len(s.queue)
			if qLen > 0 {
				s.queueHadItems.Store(true)
				s.logger.Debug("job detail extraction queue size", "size", qLen)
			} else if s.queueHadItems.Load() {
				s.logger.Info(Green("job detail extraction queue completely processed"))
				s.queueHadItems.Store(false)
			}
		case <-s.stopCh:
			return
		}
	}
}

// periodicRequeue runs every 4 hours to requeue unprocessed JobDetails.
func (s *SearchResultProcessor) periodicRequeue() {
	for {
		select {
		case <-s.requeueTicker.C:
			s.requeueUnprocessedJobDetails()
		case <-s.stopCh:
			return
		}
	}
}

// requeueUnprocessedJobDetails fetches JobDetails with nil ExtractedData and queues them.
func (s *SearchResultProcessor) requeueUnprocessedJobDetails() {
	ctx := context.Background()
	jobDetails, err := GetJobDetailsWithNoExtractedData(ctx, s.fs)
	if err != nil {
		s.logger.Error("failed to fetch unprocessed job details", "error", err)
		return
	}
	added := 0
	for _, job := range jobDetails {
		s.logger.Debug(Blue("requeueing job detail"), "jobID", job.ID)
		added++
		s.tryQueueExtractionTask(ExtractionTask{JobDetail: job})
	}
	if added > 0 {
		s.logger.Info("requeued unprocessed job details", "count", added)
	}
}

// tryQueueExtractionTask attempts to queue a JobDetail, returns false if queue is full.
func (s *SearchResultProcessor) tryQueueExtractionTask(task ExtractionTask) bool {
	select {
	case s.queue <- task:
		return true
	default:
		return false
	}
}

// OnJobDetail runs the transaction block synchronously, then queues extraction async.
func (s *SearchResultProcessor) OnJobDetail(jobSearch SearchTask, listing JobListing, jobDetail JobDetail, userID *string) {
	userIDStr := "[none]"
	if userID != nil {
		userIDStr = *userID
	}
	s.logger.Debug("OnJobDetail", "jobID", jobDetail.ProviderJobID, "userID", userIDStr)

	ctx := context.Background()

	s.logger.Debug("saving job detail and linking", "jobID", jobDetail.ID, "criteriaID", jobSearch.Criteria.ID, "userID", userIDStr)
	err := s.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := SaveJobDetailTx(ctx, s.fs, tx, &jobDetail); err != nil {
			s.logger.Error("failed to save job detail", "error", err, "jobID", jobDetail.ID)
			SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"jobID":      jobDetail.ID,
				"criteriaID": jobSearch.Criteria.ID,
			})
			return fmt.Errorf("failed to save job detail: %w", err)
		}

		if err := LinkJobDetailToSearchTaskTx(ctx, s.fs, tx, jobSearch.ID, jobDetail.ID); err != nil {
			s.logger.Error("failed to link job detail to search task", "error", err, "jobID", jobDetail.ID, "searchTaskID", jobSearch.ID)
			SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"jobID":        jobDetail.ID,
				"searchTaskID": jobSearch.ID,
				"criteriaID":   jobSearch.Criteria.ID,
			})
			return fmt.Errorf("failed to add job detail to search task: %w", err)
		}

		if err := LinkJobDetailToCriteriaTx(ctx, s.fs, tx, jobSearch.Criteria.ID, jobDetail.ID); err != nil {
			s.logger.Error("failed to link job detail to criteria", "error", err, "jobID", jobDetail.ID, "criteriaID", jobSearch.Criteria.ID)
			SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"jobID":      jobDetail.ID,
				"criteriaID": jobSearch.Criteria.ID,
			})
			return fmt.Errorf("failed to link job detail to criteria: %w", err)
		}

		return nil
	})

	if err != nil {
		s.logger.Error("failed to save job detail and link", "error", err, "jobID", jobDetail.ID, "criteriaID", jobSearch.Criteria.ID, "userID", userIDStr)
		return
	}

	_, err = UpsertJobDetailEmbeddings(ctx, jobSearch.Criteria, []JobDetail{jobDetail})
	if err != nil {
		s.logger.Error("failed to upsert job detail embeddings", "error", err, "jobID", jobDetail.ID, "criteriaID", jobSearch.Criteria.ID, "userID", userIDStr)
		SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"jobID": jobDetail.ID,
		})
	}

	// Only queue for extraction if a progress tracker is found for the search.
	if val, ok := s.progressTrackers.Load(jobSearch.ID); ok {
		progress := val.(*SearchProgress)
		task := ExtractionTask{
			SearchTaskID: jobSearch.ID,
			UserID:       userID,
			JobDetail:    jobDetail,
		}

		if s.tryQueueExtractionTask(task) {
			progress.TotalJobs.Add(1)
			fmt.Println(Green("PROGRESS_TRACKER_UPDATE_TOTALJOBS"), "searchTaskID:", jobSearch.ID, "TotalJobs:", progress.TotalJobs.Load(), "userID", userIDStr)
			fmt.Println(Yellow("EXTRACTION_TASK_QUEUED"), "userID:", userID, "jobID:", jobDetail.ID, "searchTaskID:", jobSearch.ID)
		} else {
			s.logger.Warn("job detail queue full, dropping job", "jobID", jobDetail.ID)
		}
	} else {
		s.logger.Warn("progress tracker not found, skipping extraction queue", "searchTaskID", jobSearch.ID, "jobID", jobDetail.ID)
	}
}

func (s *SearchResultProcessor) OnSearchStarted(jobSearch SearchTask, userID *string) {
	s.logger.Debug("OnSearchStarted", "criteria", jobSearch.Criteria.GetSearchHash())

	if userID != nil && *userID != "" {
		fmt.Println(Green("PROGRESS_TRACKER_CREATE"), "userID:", *userID, "searchTaskID:", jobSearch.ID)
		s.logger.Info("tracking search progress for user", "userID", *userID, "searchTaskID", jobSearch.ID)
		s.progressTrackers.Store(jobSearch.ID, &SearchProgress{
			UserID:       *userID,
			SearchTaskID: jobSearch.ID,
		})
		fmt.Println(Blue("PROGRESS_TRACKER_STORE"), "searchTaskID:", jobSearch.ID)
	}

	ctx := context.Background()
	if err := UpdateSearchTask(ctx, s.fs, &jobSearch); err != nil {
		s.logger.Error("failed to update search task", "error", err)
	}
}

// search.JobEventHandler implementation
func (s *SearchResultProcessor) OnJobListing(jobSearch SearchTask, job JobListing, userID *string) {
	s.logger.Debug("OnJobListing", "job", job.ProviderJobID)

	ctx := context.Background()
	if err := SaveJobListing(ctx, s.fs, jobSearch, &job); err != nil {
		s.logger.Error("failed to create job listing", "error", err)
	}
}

func (s *SearchResultProcessor) OnPageComplete(jobSearch SearchTask, page int, jobsFound int, errors []error, userID *string) {
	s.logger.Debug("OnPageComplete", "criteria", jobSearch.Criteria.ID, "page", page, "jobs_found", jobsFound, "errors", len(errors))

	// Log any errors for this page
	if len(errors) > 0 {
		for _, err := range errors {
			s.logger.Error("page processing error", "page", page, "error", err)
		}
	}

	if err := UpdateSearchTask(context.Background(), s.fs, &jobSearch); err != nil {
		s.logger.Error("failed to update search task", "error", err)
		SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
			"criteriaID": jobSearch.Criteria.ID,
			"page":       strconv.Itoa(page),
		})
		return
	}
}

func (s *SearchResultProcessor) OnSearchCompleted(jobSearch SearchTask, totalJobs int, errors []error, userID *string) {
	s.logger.Info("OnSearchCompleted", "criteria", jobSearch.Criteria.ID, "total_jobs", totalJobs, "errors", errors)

	if val, ok := s.progressTrackers.Load(jobSearch.ID); ok {
		progress := val.(*SearchProgress)
		progress.AllJobsQueued.Store(true)
		// It's possible OnJobDetail hasn't run for all jobs yet when OnSearchCompleted is called.
		// The TotalJobs is set in OnJobDetail. We check for completion in handleExtractionCompletion.
		s.logger.Debug("all jobs queued for search", "searchTaskID", jobSearch.ID, "totalJobsSoFar", progress.TotalJobs.Load())

		if progress.AllJobsQueued.Load() && progress.ProcessedJobs.Load() == progress.TotalJobs.Load() {
			s.logger.Info("all extractions complete for search at OnSearchCompleted", "searchTaskID", jobSearch.ID)
			s.eventDispatch.OnAllExtractionsComplete(jobSearch, int(progress.TotalJobs.Load()), userID)
			s.progressTrackers.Delete(jobSearch.ID)
		}
	}

	// TODO: Do this in a transaction
	jobSearch.Status = SearchTaskStatusCompleted
	// join all the errors into a single string
	if len(errors) > 0 {
		allErrors := ""
		for _, err := range errors {
			allErrors += err.Error() + "\n"
		}
		jobSearch.Error = &allErrors
	}

	ctx := context.Background()
	if err := UpdateSearchTask(ctx, s.fs, &jobSearch); err != nil {
		SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"criteriaID": jobSearch.Criteria.ID,
			"totalJobs":  strconv.Itoa(totalJobs),
			"errors":     strconv.Itoa(len(errors)),
		})
		s.logger.Error("failed to update search task", "error", err)
	}

	criteria := jobSearch.Criteria

	nowUTC := time.Now().UTC()
	criteria.LastSearchedAt = &nowUTC
	criteria.LastSearchJobID = jobSearch.ID

	// If a user is new, we bump the first search to have a higher max pages and listing age
	// so that we can get more results. Reset it back to default after the first search.
	criteria.MaxPages = DefaultMaxPages
	criteria.ListingAge = DefaultListingAge
	UpdateSearchCriteria(ctx, s.fs, &criteria)
}

func (s *SearchResultProcessor) OnJobDetailExtractionComplete(task SearchTask, jobDetail JobDetail, userID *string) {
	// Business logic is handled in the worker.
	// This is here to satisfy the interface.
	if userID != nil && *userID != "" {
		s.logger.Debug(Gray("OnJobDetailExtractionComplete"), "jobID", jobDetail.ID)
	}
}

func (s *SearchResultProcessor) OnAllExtractionsComplete(task SearchTask, totalJobs int, userID *string) {
	if userID != nil && *userID != "" {
		s.logger.Info(BGGreen("all extractions complete, sending redirect"), "searchTaskID", task.ID, "user", *userID)
		SendPusherRedirect(s.pusherClient, *userID, "/dashboard")
	}
}

func UpsertJobDetailEmbeddings(ctx context.Context, criteria SearchCriteria, jobDetails []JobDetail) (int, error) {
	client, err := NewPineconeClient()
	if err != nil {
		return 0, fmt.Errorf("failed to create pinecone client: %w", err)
	}

	denseIndex := os.Getenv("PINECONE_DENSE_INDEX_HOST")
	if denseIndex == "" {
		return 0, fmt.Errorf("PINECONE_DENSE_INDEX_HOST is not set, skipping job detail embedding")
	}

	sparseIndex := os.Getenv("PINECONE_SPARSE_INDEX_HOST")
	if sparseIndex == "" {
		return 0, fmt.Errorf("PINECONE_SPARSE_INDEX_HOST is not set, skipping job detail embedding")
	}

	if len(jobDetails) == 0 {
		return 0, nil
	}

	records := make([]*pinecone.IntegratedRecord, len(jobDetails))
	for i, detail := range jobDetails {
		records[i] = &pinecone.IntegratedRecord{
			"_id":             detail.ID,
			"provider":        detail.Provider,
			"providerJobID":   detail.ProviderJobID,
			"criteriaID":      criteria.ID,
			"text":            detail.Description, //  this is what gets embedded
			"postedDate":      detail.PostedDate,
			"title":           detail.Title,
			"company":         detail.Company,
			"location":        detail.Location,
			"workType":        criteria.WorkType,
			"experienceLevel": criteria.ExperienceLevel,
		}
	}

	denseCount, err := client.UpsertRecords(context.Background(), denseIndex, JobDetailNamespace, records, false)
	if err != nil {
		return 0, fmt.Errorf("failed to upsert records to Pinecone: %w", err)
	}

	sparseCount, err := client.UpsertRecords(context.Background(), sparseIndex, JobDetailNamespace, records, true)
	if err != nil {
		return 0, fmt.Errorf("failed to upsert records to Pinecone: %w", err)
	}

	count := denseCount + sparseCount

	return count, nil
}
