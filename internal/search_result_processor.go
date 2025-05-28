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
	JobDetailNamespace = "job-details" // Pinecone namespace for job details
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
	fs     *FirestoreClient
	logger *slog.Logger
}

func NewSearchResultProcessor(fs *FirestoreClient, logger *slog.Logger) *SearchResultProcessor {
	logger = logger.With("pkg", "SearchResultProcessor")
	return &SearchResultProcessor{
		fs:     fs,
		logger: logger,
	}
}

func (s *SearchResultProcessor) OnSearchStarted(jobSearch SearchTask) {
	s.logger.Info("OnSearchStarted", "criteria", jobSearch.Criteria.GetSearchHash())
	ctx := context.Background()
	if err := UpdateSearchTask(ctx, s.fs, &jobSearch); err != nil {
		s.logger.Error("failed to update search task", "error", err)
	}
}

// search.JobEventHandler implementation
func (s *SearchResultProcessor) OnJobListing(jobSearch SearchTask, job JobListing) {
	s.logger.Info("OnJobListing", "job", job.ProviderJobID)

	ctx := context.Background()
	if err := SaveJobListing(ctx, s.fs, jobSearch, &job); err != nil {
		s.logger.Error("failed to create job listing", "error", err)
	}
}

func (s *SearchResultProcessor) OnJobDetail(jobSearch SearchTask, job JobDetail) {
	s.logger.Info("OnJobDetail", "jobID", job.ProviderJobID)

	//
	// TODO: Do we care if the same job detail was found multiple times?
	//
	go func() {
		// Check if link already exists before the transaction
		ctx := context.Background()
		linkID := fmt.Sprintf("%s_%s", jobSearch.Criteria.ID, job.ID)
		linkRef := s.fs.Collection(JobCriteriaLinkCollection).Doc(linkID)
		linkDoc, err := linkRef.Get(ctx)
		linkExists := err == nil && linkDoc.Exists()

		if err := ExtractJobDetailData(ctx, &job); err != nil {
			s.logger.Error("failed to extract job detail data", "error", err, "jobID", job.ID)
			return
		}

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
			s.logger.Error("failed to save job detail and link", "error", err)
			return
		}
	}()
}

func (s *SearchResultProcessor) OnSearchCompleted(jobSearch SearchTask, totalJobs int, errors []error) {
	s.logger.Info("SearnSearchCompleted", "criteria", jobSearch.Criteria.ID, "total_jobs", totalJobs, "errors", errors)

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
		s.logger.Error("failed to update search task", "error", err)
	}

	criteria := jobSearch.Criteria
	nowUTC := time.Now().UTC()
	criteria.LastSearchedAt = &nowUTC
	criteria.LastSearchJobID = jobSearch.ID
	UpdateSearchCriteria(ctx, s.fs, &criteria)

	jobDetails, err := GetJobDetailsForSearchTask(ctx, s.fs, jobSearch.ID)
	if err != nil {
		s.logger.Error("failed to get job details", "error", err)
		return
	}

	count, err := s.UpsertJobDetailEmbeddings(ctx, jobSearch.Criteria.ID, jobDetails)
	if err != nil {
		s.logger.Error("failed to embed job details", "error", err)
	}
	s.logger.Info("embedded job details", "count", count)

	// s.processSubscriptions(ctx, jobSearch)
}

func (s *SearchResultProcessor) UpsertJobDetailEmbeddings(ctx context.Context, criteriaID string, jobDetails []JobDetail) (int, error) {
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
			"_id":           detail.ID,
			"provider":      detail.Provider,
			"providerJobID": detail.ProviderJobID,
			"criteriaID":    criteriaID,
			"text":          detail.Description, //  this is what gets embedded
			"postedDate":    detail.PostedDate,
			"title":         detail.Title,
			"company":       detail.Company,
			"location":      detail.Location,
			"url":           detail.URL,
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

// func (s *processSubscriptions(ctx context.Context, jobSearch SearchJob) {
// 	subscriptions, err := GetCriteriaSubscribers(s.fs, jobSearch.Criteria.ID)
// 	if err != nil {
// 		s.logger.Error("failed to get users subscribed to criteria", "error", err)
// 		return
// 	}

// 	for _, subscription := range subscriptions {
// 		if err := s.processor.EnqueueAnalysis(ctx, subscription.UserID, jobSearch.ID); err != nil {
// 			s.logger.Error("failed to queue job processing for user", "error", err, "user_id", subscription.UserID)
// 		}
// 	}
// }
