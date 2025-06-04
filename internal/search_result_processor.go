package internal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/getsentry/sentry-go"
	"github.com/pinecone-io/go-pinecone/v3/pinecone"
)

const (
	JobDetailNamespace         = "job-details" // Pinecone namespace for job details
	MaxConcurrentJobProcessing = 10            // Maximum number of concurrent job detail processing goroutines
)

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
	fs                    *FirestoreClient
	logger                *slog.Logger
	semaphore             chan struct{} // Semaphore to limit concurrent job detail processing
	outstandingGoroutines int64         // Counter for outstanding goroutines (atomic)
}

func NewSearchResultProcessor(fs *FirestoreClient, logger *slog.Logger) *SearchResultProcessor {
	logger = logger.With("pkg", "SearchResultProcessor")
	return &SearchResultProcessor{
		fs:                    fs,
		logger:                logger,
		semaphore:             make(chan struct{}, MaxConcurrentJobProcessing),
		outstandingGoroutines: 0,
	}
}

func (s *SearchResultProcessor) OnSearchStarted(jobSearch SearchTask) {
	s.logger.Debug("OnSearchStarted", "criteria", jobSearch.Criteria.GetSearchHash())
	ctx := context.Background()
	if err := UpdateSearchTask(ctx, s.fs, &jobSearch); err != nil {
		s.logger.Error("failed to update search task", "error", err)
	}
}

// search.JobEventHandler implementation
func (s *SearchResultProcessor) OnJobListing(jobSearch SearchTask, job JobListing) {
	s.logger.Debug("OnJobListing", "job", job.ProviderJobID)

	ctx := context.Background()
	if err := SaveJobListing(ctx, s.fs, jobSearch, &job); err != nil {
		s.logger.Error("failed to create job listing", "error", err)
	}
}

func (s *SearchResultProcessor) OnJobDetail(jobSearch SearchTask, job JobDetail) {
	s.logger.Debug("OnJobDetail", "jobID", job.ProviderJobID)

	go func() {
		// Acquire semaphore slot
		s.semaphore <- struct{}{}

		// Increment counter and log
		count := atomic.AddInt64(&s.outstandingGoroutines, 1)
		s.logger.Debug("goroutine started", "jobID", job.ID, "outstanding", count)

		defer func() {
			// Decrement counter and log
			count := atomic.AddInt64(&s.outstandingGoroutines, -1)
			s.logger.Debug("goroutine finished", "jobID", job.ID, "outstanding", count)

			// Release semaphore slot
			<-s.semaphore
		}()

		ctx := context.Background()

		start := time.Now()
		s.logger.Debug("extracting job detail data", "jobID", job.ID, "criteriaID", jobSearch.Criteria.ID)
		if err := ExtractJobDetailData(ctx, &job); err != nil {
			s.logger.Error("failed to extract job detail data", "error", err, "jobID", job.ID)
			SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"jobID":      job.ID,
				"criteriaID": jobSearch.Criteria.ID,
			})

			return
		}

		s.logger.Debug(Green("extracted job detail data"), "jobID", job.ID, "duration", time.Since(start).String())

		// Transaction for data operations only
		err := s.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			if err := SaveJobDetailTx(ctx, s.fs, tx, &job); err != nil {
				s.logger.Error("failed to save job detail", "error", err, "jobID", job.ID)
				SentryReportError(ctx, err, sentry.LevelError, map[string]string{
					"jobID":      job.ID,
					"criteriaID": jobSearch.Criteria.ID,
				})
				return fmt.Errorf("failed to save job detail: %w", err)
			}

			if err := LinkJobDetailToSearchTaskTx(ctx, s.fs, tx, jobSearch.ID, job.ID); err != nil {
				s.logger.Error("failed to link job detail to search task", "error", err, "jobID", job.ID, "searchTaskID", jobSearch.ID)
				SentryReportError(ctx, err, sentry.LevelError, map[string]string{
					"jobID":        job.ID,
					"searchTaskID": jobSearch.ID,
					"criteriaID":   jobSearch.Criteria.ID,
				})
				return fmt.Errorf("failed to add job detail to search task: %w", err)
			}

			if err := LinkJobDetailToCriteriaTx(ctx, s.fs, tx, jobSearch.Criteria.ID, job.ID); err != nil {
				s.logger.Error("failed to link job detail to criteria", "error", err, "jobID", job.ID, "criteriaID", jobSearch.Criteria.ID)
				SentryReportError(ctx, err, sentry.LevelError, map[string]string{
					"jobID":      job.ID,
					"criteriaID": jobSearch.Criteria.ID,
				})
				return fmt.Errorf("failed to link job detail to criteria: %w", err)
			}

			return nil
		})

		if err != nil {
			s.logger.Error("failed to save job detail and link", "error", err)
			return
		}

		if _, err := UpsertJobDetailEmbeddings(ctx, jobSearch.Criteria, []JobDetail{job}); err != nil {
			s.logger.Error("failed to upsert job detail embeddings", "error", err, "jobID", job.ID)
			SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"jobID":      job.ID,
				"criteriaID": jobSearch.Criteria.ID,
			})
			return
		}
	}()
}

func (s *SearchResultProcessor) OnPageComplete(jobSearch SearchTask, page int, jobsFound int, errors []error) {
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

func (s *SearchResultProcessor) OnSearchCompleted(jobSearch SearchTask, totalJobs int, errors []error) {
	s.logger.Info("OnSearchCompleted", "criteria", jobSearch.Criteria.ID, "total_jobs", totalJobs, "errors", errors)

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
	UpdateSearchCriteria(ctx, s.fs, &criteria)
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

// GetOutstandingGoroutines returns the current number of outstanding job detail processing goroutines
func (s *SearchResultProcessor) GetOutstandingGoroutines() int64 {
	return atomic.LoadInt64(&s.outstandingGoroutines)
}
