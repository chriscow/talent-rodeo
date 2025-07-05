package internal

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	// "talent-rodeo/internal/search"

	"cloud.google.com/go/firestore"
	"github.com/getsentry/sentry-go"
)

// ProcessRelatedJobs processes SimilarJobs and PeopleAlsoViewed for a high-rated job.
// For each related job, it:
// 1. Extracts the LinkedIn job ID from the URL
// 2. Checks if the job already exists in our database
// 3. If exists, creates a UserJob entry and links it to the search criteria
// 4. If not exists, fetches it via the job provider and processes it
func ProcessRelatedJobs(ctx context.Context, fs *FirestoreClient, logger *slog.Logger, userID string,
	jobDetail JobDetail, searchCriteria SearchCriteria, jobProvider JobProvider) error {

	if fs == nil {
		return fmt.Errorf("firestore client cannot be nil")
	}
	if logger == nil {
		return fmt.Errorf("logger cannot be nil")
	}
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}
	if jobProvider == nil {
		return fmt.Errorf("jobProvider cannot be nil")
	}

	logger = logger.With("jobID", jobDetail.ID, "userID", userID, "function", "ProcessRelatedJobs")
	logger.Debug("processing related jobs for high-rated job")

	// Extract all related job IDs
	relatedJobIDs := ExtractRelatedJobIDs(jobDetail.SimilarJobs, jobDetail.PeopleAlsoViewed)
	if len(relatedJobIDs) == 0 {
		logger.Debug("no related jobs found")
		return nil
	}

	logger.Info("found related jobs to process", "count", len(relatedJobIDs))

	var existingJobs []JobDetail
	var missingJobIDs []string

	// Check which jobs already exist in our database
	for _, jobID := range relatedJobIDs {
		// LinkedIn job IDs are stored as "scrapingdog:{jobID}" in our system
		internalJobID := fmt.Sprintf("scrapingdog:%s", jobID)

		existingJob, found, err := GetJobDetail(ctx, fs, internalJobID)
		if err != nil {
			logger.Error("failed to check if related job exists", "relatedJobID", jobID, "error", err)
			SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action":       "ProcessRelatedJobs: failed to check job existence",
				"relatedJobID": jobID,
				"userID":       userID,
				"sourceJobID":  jobDetail.ID,
			})
			continue
		}

		if found {
			existingJobs = append(existingJobs, *existingJob)
			logger.Debug("related job already exists in database", "relatedJobID", jobID)
		} else {
			missingJobIDs = append(missingJobIDs, jobID)
			logger.Debug("related job needs to be fetched", "relatedJobID", jobID)
		}
	}

	// Process existing jobs - add them to user's collection and link to criteria
	if len(existingJobs) > 0 {
		logger.Info("processing existing related jobs", "count", len(existingJobs))
		if err := processExistingRelatedJobs(ctx, fs, logger, userID, existingJobs, searchCriteria); err != nil {
			logger.Error("failed to process existing related jobs", "error", err)
			SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action":      "ProcessRelatedJobs: failed to process existing jobs",
				"userID":      userID,
				"sourceJobID": jobDetail.ID,
				"jobCount":    fmt.Sprintf("%d", len(existingJobs)),
			})
			// Continue with missing jobs even if existing job processing fails
		}
	}

	// Fetch and process missing jobs
	if len(missingJobIDs) > 0 {
		logger.Info("fetching missing related jobs", "count", len(missingJobIDs))
		if err := fetchAndProcessMissingJobs(ctx, fs, logger, userID, missingJobIDs, searchCriteria, jobProvider); err != nil {
			logger.Error("failed to fetch missing related jobs", "error", err)
			SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action":      "ProcessRelatedJobs: failed to fetch missing jobs",
				"userID":      userID,
				"sourceJobID": jobDetail.ID,
				"jobCount":    fmt.Sprintf("%d", len(missingJobIDs)),
			})
			// Don't return error here - we want to continue even if some jobs fail to fetch
		}
	}

	logger.Info("completed processing related jobs",
		"totalFound", len(relatedJobIDs),
		"existingProcessed", len(existingJobs),
		"missingQueued", len(missingJobIDs))

	return nil
}

// processExistingRelatedJobs creates UserJob entries for existing related jobs and links them to the search criteria
func processExistingRelatedJobs(ctx context.Context, fs *FirestoreClient, logger *slog.Logger,
	userID string, existingJobs []JobDetail, searchCriteria SearchCriteria) error {

	for _, job := range existingJobs {
		// Check if user already has this job
		_, exists, err := GetUserJob(ctx, fs, userID, job.ID)
		if err != nil {
			logger.Error("failed to check if user already has related job", "jobID", job.ID, "error", err)
			continue
		}

		if exists {
			logger.Debug("user already has this related job, skipping", "jobID", job.ID)
			continue
		}

		// Create UserJob entry
		userJob := UserJob{
			ID:        job.ID,
			UserID:    userID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Status:    JobStatusNew, // Related jobs start as "new" for user review
			JobRef:    fs.Collection(JobDetailsCollection).Doc(job.ID),
		}

		// Save the user job
		err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			// Create the user job
			userJobRef := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(job.ID)
			if err := tx.Set(userJobRef, userJob); err != nil {
				return fmt.Errorf("failed to create user job: %w", err)
			}

			// Link the job to the search criteria
			if err := LinkJobDetailToCriteriaTx(ctx, fs, tx, searchCriteria.ID, job.ID); err != nil {
				return fmt.Errorf("failed to link job to criteria: %w", err)
			}

			return nil
		})

		if err != nil {
			logger.Error("failed to save related user job", "jobID", job.ID, "error", err)
			return err
		}

		logger.Debug("successfully processed existing related job", "jobID", job.ID)
	}

	return nil
}

// fetchAndProcessMissingJobs fetches missing related jobs and processes them
func fetchAndProcessMissingJobs(ctx context.Context, fs *FirestoreClient, logger *slog.Logger,
	userID string, missingJobIDs []string, searchCriteria SearchCriteria, jobProvider JobProvider) error {

	if len(missingJobIDs) == 0 {
		return nil
	}

	var successCount, failCount int

	for _, linkedInJobID := range missingJobIDs {
		// Fetch the job details using the provider
		fetchedJob, err := jobProvider.GetJobDetails(ctx, &userID, linkedInJobID)
		if err != nil {
			logger.Error("failed to fetch related job", "jobID", linkedInJobID, "error", err)
			SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action":        "fetchAndProcessMissingJobs: failed to fetch job",
				"linkedInJobID": linkedInJobID,
				"userID":        userID,
			})
			failCount++
			continue
		}

		if fetchedJob == nil {
			logger.Warn("job provider returned nil job", "jobID", linkedInJobID)
			failCount++
			continue
		}

		// Process the fetched job (save it and create user job entry)
		err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			// Save the job detail
			if err := SaveJobDetailTx(ctx, fs, tx, fetchedJob); err != nil {
				return fmt.Errorf("failed to save fetched job detail: %w", err)
			}

			// Create user job entry
			userJob := UserJob{
				ID:        fetchedJob.ID,
				UserID:    userID,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Status:    JobStatusNew, // Related jobs start as "new" for user review
				JobRef:    fs.Collection(JobDetailsCollection).Doc(fetchedJob.ID),
			}

			// Save the user job
			userJobRef := fs.Collection(usersCollection).Doc(userID).Collection(userJobsCollection).Doc(fetchedJob.ID)
			if err := tx.Set(userJobRef, userJob); err != nil {
				return fmt.Errorf("failed to create user job: %w", err)
			}

			// Link the job to the search criteria
			if err := LinkJobDetailToCriteriaTx(ctx, fs, tx, searchCriteria.ID, fetchedJob.ID); err != nil {
				return fmt.Errorf("failed to link job to criteria: %w", err)
			}

			return nil
		})

		if err != nil {
			logger.Error("failed to save fetched related job", "jobID", linkedInJobID, "error", err)
			SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action":        "fetchAndProcessMissingJobs: failed to save job",
				"linkedInJobID": linkedInJobID,
				"userID":        userID,
			})
			failCount++
			continue
		}

		logger.Debug("successfully fetched and processed related job", "jobID", linkedInJobID)
		successCount++
	}

	logger.Info("completed fetching missing related jobs",
		"total", len(missingJobIDs),
		"success", successCount,
		"failed", failCount)

	// Return error only if all jobs failed to process
	if successCount == 0 && failCount > 0 {
		return fmt.Errorf("failed to fetch any of the %d related jobs", len(missingJobIDs))
	}

	return nil
}
