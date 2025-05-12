package internal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/pinecone-io/go-pinecone/v3/pinecone"
)

const (
	JobDetailNamespace = "job-details"
)

// JobProcessor is an interface that defines the methods for processing new job details
// returned from a search. It is used to enqueue jobs for post processing, such as
// normalizing and embedding the job details.
type JobProcessor interface {
	ProcessJob(ctx context.Context, jobID string) error
}

// JobSink is a struct that implements the search.JobEventHandler interface.
// It is responsible for handling job listings and job details returned from a search.
// It saves the job listings and job details to Firestore and links them to the search criteria.
type JobSink struct {
	fs        *FirestoreClient
	logger    *slog.Logger
	processor JobProcessor
}

func NewJobSink(fs *FirestoreClient, logger *slog.Logger, processor JobProcessor) *JobSink {
	return &JobSink{
		fs:        fs,
		logger:    logger,
		processor: processor,
	}
}

func (s *JobSink) OnSearchStarted(ctx context.Context, jobSearch SearchTask) {
	s.logger.Info("JobSink: on search started", "criteria", jobSearch.Criteria.GetSearchHash())
	if err := UpdateSearchTask(ctx, s.fs, &jobSearch); err != nil {
		s.logger.Error("JobSink: failed to update search task", "error", err)
	}
}

// search.JobEventHandler implementation
func (s *JobSink) OnJobListing(ctx context.Context, jobSearch SearchTask, job JobListing) {
	s.logger.Info("JobSink: on job listing", "job", job.ProviderJobID)

	if err := SaveJobListing(ctx, s.fs, jobSearch, &job); err != nil {
		s.logger.Error("JobSink: failed to create job listing", "error", err)
	}
}

func (s *JobSink) OnJobDetail(ctx context.Context, jobSearch SearchTask, job JobDetail) {
	s.logger.Info("JobSink: on job detail", "job", job.ProviderJobID)

	//
	// TODO: Do we care if the same job detail was found multiple times?
	//

	existingJob, found, err := GetJobDetail(ctx, s.fs, job.ID)
	if err != nil {
		s.logger.Error("JobSink: failed to get job detail", "error", err)
		return
	}

	if found {
		//
		// Job already exists, update the existing job with new details
		//
		job.CreatedAt = existingJob.CreatedAt
		job.SearchJobIDs = existingJob.SearchJobIDs
		job.SearchJobIDs = append(job.SearchJobIDs, jobSearch.ID)
		job.ID = existingJob.ID
		nowUTC := time.Now().UTC()
		job.UpdatedAt = &nowUTC
	}

	// Check if link already exists before the transaction
	linkID := fmt.Sprintf("%s_%s", jobSearch.Criteria.ID, job.ID)
	linkRef := s.fs.Collection(JobCriteriaLinkCollection).Doc(linkID)
	linkDoc, err := linkRef.Get(ctx)
	linkExists := err == nil && linkDoc.Exists()

	// Transaction for data operations only
	err = s.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// 1. Save the job detail
		if err := SaveJobDetailTx(ctx, s.fs, tx, &job); err != nil {
			return fmt.Errorf("failed to save job detail: %w", err)
		}

		if err := AddJobDetailToSearchTaskTx(ctx, s.fs, tx, jobSearch.ID, &job); err != nil {
			return fmt.Errorf("failed to add job detail to search task: %w", err)
		}

		// 2. Link the job detail to the criteria (only if it doesn't exist)
		if !linkExists {
			// Create the link directly without reading inside the transaction
			link := JobDetailCriteriaLink{
				CriteriaID: jobSearch.Criteria.ID,
				JobID:      job.ID,
				CreatedAt:  time.Now(),
			}
			if err := tx.Create(linkRef, &link); err != nil {
				return fmt.Errorf("failed to create link: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		s.logger.Error("JobSink: failed to save job detail and link", "error", err)
		return
	}
}

func (s *JobSink) OnSearchCompleted(ctx context.Context, jobSearch SearchTask, totalJobs int, errors []error) {
	s.logger.Info("JobSink: on search completed", "criteria", jobSearch.Criteria.GetSearchHash(), "total_jobs", totalJobs, "errors", errors)

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

	if err := UpdateSearchTask(ctx, s.fs, &jobSearch); err != nil {
		s.logger.Error("JobSink: failed to update search task", "error", err)
	}

	criteria := jobSearch.Criteria
	nowUTC := time.Now().UTC()
	criteria.LastSearchedAt = &nowUTC
	criteria.LastSearchJobID = jobSearch.ID
	UpdateSearchCriteria(s.fs, &criteria)

	jobDetails, err := GetJobDetailsForSearchTask(ctx, s.fs, jobSearch.ID)
	if err != nil {
		s.logger.Error("JobSink: failed to get job details", "error", err)
		return
	}

	count, err := s.EmbedJobDetails(ctx, jobSearch.Criteria.ID, jobDetails)
	if err != nil {
		s.logger.Error("JobSink: failed to embed job details", "error", err)
	}
	s.logger.Info("JobSink: embedded job details", "count", count)

	// s.processSubscriptions(ctx, jobSearch)
}

func (s *JobSink) EmbedJobDetails(ctx context.Context, criteriaID string, jobDetails []JobDetail) (int, error) {
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
			"_id":        detail.ID,
			"criteriaID": criteriaID,
			"text":       detail.Description, //  this is what gets embedded
			"postedDate": detail.PostedDate,
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

// func (s *JobSink) processSubscriptions(ctx context.Context, jobSearch SearchJob) {
// 	subscriptions, err := GetCriteriaSubscribers(s.fs, jobSearch.Criteria.ID)
// 	if err != nil {
// 		s.logger.Error("JobSink: failed to get users subscribed to criteria", "error", err)
// 		return
// 	}

// 	for _, subscription := range subscriptions {
// 		if err := s.processor.EnqueueAnalysis(ctx, subscription.UserID, jobSearch.ID); err != nil {
// 			s.logger.Error("JobSink: failed to queue job processing for user", "error", err, "user_id", subscription.UserID)
// 		}
// 	}
// }
