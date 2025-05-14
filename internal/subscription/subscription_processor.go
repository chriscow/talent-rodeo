// Package subscription provides functionality for handling search criteria subscriptions
// and publishing job matches to users based on their preferences.
package subscription

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"talent-rodeo/internal"
	"time"

	"github.com/getsentry/sentry-go"
)

const (
	// DefaultTickInterval is the default interval for processing subscriptions (24 hours)
	DefaultTickInterval = 24 * time.Hour

	// DefaultTopK is the number of top jobs to return from semantic search
	DefaultTopK = 100
)

// Processor handles the periodic checking and publishing of search criteria subscriptions.
// It runs on a configurable interval, processes eligible search criteria, and publishes
// relevant job matches to subscribed users.
type Processor struct {
	fs         *internal.FirestoreClient // Firestore client for database operations
	logger     *slog.Logger              // Logger for recording processor activities
	ticker     *time.Ticker              // Ticker for scheduling periodic processing
	done       chan bool                 // Channel to signal shutdown
	wg         sync.WaitGroup            // WaitGroup to manage goroutines
	interval   time.Duration             // Processing interval (defaults to DefaultTickInterval)
	isRunning  bool                      // Flag indicating if the processor is currently running
	cancelFunc context.CancelFunc        // Function to cancel the processing context
}

// NewProcessor creates a new subscription processor with the provided Firestore client and logger.
// If no logger is provided, it creates a default logger that writes to stdout.
// The Firestore client is required and the function will panic if nil is provided.
func NewProcessor(fs *internal.FirestoreClient, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	if fs == nil {
		panic("Firestore client cannot be nil")
	}

	logger = logger.With("component", "subscription.Processor")
	return &Processor{
		fs:       fs,
		logger:   logger,
		interval: DefaultTickInterval,
		done:     make(chan bool),
	}
}

// WithInterval sets a custom tick interval for the processor.
// This method allows for fluent configuration of the processor.
//
// Example:
//
//	processor := subscription.NewProcessor(fs, logger).WithInterval(12 * time.Hour)
func (p *Processor) WithInterval(interval time.Duration) *Processor {
	p.interval = interval
	return p
}

// Start begins the subscription processor's operation.
// It runs the processor immediately once and then on the configured interval.
// The processor will continue running until Stop is called or the provided context is canceled.
//
// Returns an error if the processor is already running.
func (p *Processor) Start(ctx context.Context) error {
	if p.isRunning {
		return fmt.Errorf("processor is already running")
	}

	p.logger.Info("Starting subscription processor", "interval", p.interval)

	// Create a cancelable context
	ctx, cancel := context.WithCancel(ctx)
	p.cancelFunc = cancel

	p.ticker = time.NewTicker(p.interval)
	p.isRunning = true

	// Run once at startup
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if err := p.process(ctx); err != nil {
			p.logger.Error("Error during initial subscription processing", "error", err)
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"source": "subscription_processor_initial",
			})
		}
	}()

	// Then run on ticker
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-p.ticker.C:
				if err := p.process(ctx); err != nil {
					p.logger.Error("Error during subscription processing", "error", err)
					internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
						"source": "subscription_processor_tick",
					})
				}
			case <-p.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Stop halts the subscription processor.
// It stops the ticker, signals the goroutines to stop, and waits for all processing to complete.
// This method is safe to call even if the processor is not running.
func (p *Processor) Stop() {
	if !p.isRunning {
		return
	}

	p.logger.Info("Stopping subscription processor")
	p.ticker.Stop()
	p.done <- true

	if p.cancelFunc != nil {
		p.cancelFunc()
	}

	p.wg.Wait()
	p.isRunning = false
	p.logger.Info("Subscription processor stopped")
}

// process handles the main logic of checking for eligible criteria and publishing to subscribers.
// It retrieves all search criteria, filters for eligible ones, and processes each eligible criterion.
//
// A criterion is eligible if:
// - It has subscribers (SubscriberCount > 0)
// - It has been searched recently (LastSearchedAt within the last 24 hours)
// - It hasn't been published recently (LastPublishedAt is nil or older than 24 hours)
//
// Errors during processing of individual criteria are logged and reported to Sentry,
// but do not stop the processing of other criteria.
func (p *Processor) process(ctx context.Context) error {
	p.logger.Info("Processing search criteria subscriptions...")

	// Get all criteria
	allCriteria, err := internal.ListSearchCriteria(ctx, p.fs)
	if err != nil {
		return fmt.Errorf("failed to list search criteria: %w", err)
	}

	// Filter for eligible criteria:
	// - LastPublishedAt older than 24 hours OR nil
	// - LastSearchedAt within the last 24 hours
	// - SubscriberCount > 0
	twentyFourHoursAgo := time.Now().Add(-24 * time.Hour)
	var eligibleCriteria []internal.SearchCriteria

	for _, criteria := range allCriteria {
		// Must have subscribers
		if criteria.SubscriberCount <= 0 {
			continue
		}

		// Must have been searched recently
		if criteria.LastSearchedAt == nil || criteria.LastSearchedAt.Before(twentyFourHoursAgo) {
			continue
		}

		// Must not have been published recently
		if criteria.LastPublishedAt != nil && criteria.LastPublishedAt.After(twentyFourHoursAgo) {
			continue
		}

		eligibleCriteria = append(eligibleCriteria, criteria)
	}

	if len(eligibleCriteria) > 0 {
		p.logger.Info("found eligible criteria for processing", "count", len(eligibleCriteria))
	}

	// Process each eligible criterion
	for _, criteria := range eligibleCriteria {
		if err := p.processCriteria(ctx, criteria); err != nil {
			// Log error but continue with other criteria
			p.logger.Error("processCriteria", "id", criteria.ID, "error", err)
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"source":      "subscription_processor",
				"criteria_id": criteria.ID,
			})
			continue
		}
	}

	return nil
}

// processCriteria handles processing a single search criteria.
// It retrieves all subscribers for the criteria, finds recent jobs matching the criteria,
// and processes jobs for each subscribed user.
//
// The function updates the LastPublishedAt timestamp on the criteria after successful processing.
// Errors during processing for individual users are logged and reported to Sentry,
// but do not stop the processing for other users.
func (p *Processor) processCriteria(ctx context.Context, criteria internal.SearchCriteria) error {
	p.logger.Info("Processing criteria", "id", criteria.ID)

	// Get all subscribers for this criteria
	subscribers, err := internal.GetCriteriaSubscribers(ctx, p.fs, criteria.ID)
	if err != nil {
		return fmt.Errorf("failed to get subscribers for criteria %s: %w", criteria.ID, err)
	}

	if len(subscribers) == 0 {
		p.logger.Info("No subscribers found for criteria, skipping", "id", criteria.ID)
		return nil
	}

	// Get all jobs found in the last 24 hours for this criteria
	twentyFourHoursAgo := time.Now().Add(-24 * time.Hour)
	jobIDs, err := internal.GetJobIDsForCriteriaCreatedAfter(ctx, p.fs, criteria.ID, twentyFourHoursAgo)
	if err != nil {
		return fmt.Errorf("failed to get recent jobs for criteria %s: %w", criteria.ID, err)
	}

	if len(jobIDs) == 0 {
		p.logger.Info("No new jobs found for criteria, skipping", "id", criteria.ID)
		return nil
	}

	p.logger.Info("Found new jobs for criteria", "jobs-count", len(jobIDs), "id", criteria.ID)

	// Extract user IDs from subscriptions
	userIDs := make([]string, len(subscribers))
	for i, sub := range subscribers {
		userIDs[i] = sub.UserID
	}

	// Get all users in one call
	users, err := internal.GetUsersByIDs(ctx, p.fs, userIDs)
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	for _, user := range users {
		if err := p.processUserJobs(ctx, user, criteria); err != nil {
			// Log error but continue with other users
			p.logger.Error("Error processing jobs for user", "user-id", user.ID, "error", err)
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"source":      "subscription_processor",
				"criteria_id": criteria.ID,
				"user_id":     user.ID,
			})
			continue
		}
	}

	// Update LastPublishedAt for this criteria
	now := time.Now()
	criteria.LastPublishedAt = &now
	if err := internal.UpdateSearchCriteria(ctx, p.fs, &criteria); err != nil {
		return fmt.Errorf("failed to update LastPublishedAt for criteria %s: %w", criteria.ID, err)
	}

	p.logger.Info("Successfully processed criteria", "id", criteria.ID)
	return nil
}

// processUserJobs handles the semantic search and job creation for a single user.
// It performs semantic search using the user's steering query against jobs matching the criteria,
// and creates user-job links for jobs that the user doesn't already have.
//
// The function skips users without a steering query.
// If no matching jobs are found for the user, it logs this and returns without error.
func (p *Processor) processUserJobs(ctx context.Context, user *internal.User, criteria internal.SearchCriteria) error {
	// Skip users without a steering query
	if user.SteeringQuery == nil {
		p.logger.Info("User has no steering query, skipping", "user-id", user.ID)
		return nil
	}

	// Search for jobs using the user's steering query and the criteria ID
	scoredJobs, err := internal.SemanticSearchJobs(ctx, user.SteeringQuery, criteria.ID, DefaultTopK)
	if err != nil {
		return fmt.Errorf("failed to search jobs for user %s: %w", user.ID, err)
	}

	if len(scoredJobs) == 0 {
		p.logger.Info("No matching jobs found for user %s and criteria %s", user.ID, criteria.ID)
		return nil
	}

	p.logger.Info("Found matching jobs for user", "jobs-count", len(scoredJobs), "user-id", user.ID)

	// Create a user job for each matching job
	for _, scoredJob := range scoredJobs {
		jobID := scoredJob.JobDetail.ID

		// Check if the user already has this job
		_, exists, err := internal.GetUserJob(ctx, p.fs, user.ID, jobID)
		if err != nil {
			return fmt.Errorf("failed to check if user already has job %s: %w", jobID, err)
		}

		if exists {
			// Skip this job if the user already has it
			continue
		}

		// Create the user job
		_, err = internal.CreateUserJob(ctx, p.fs, user.ID, jobID)
		if err != nil {
			return fmt.Errorf("failed to create user job for user %s and job %s: %w", user.ID, jobID, err)
		}

		p.logger.Info("Created job for user", "job-id", jobID, "user-id", user.ID)
	}

	return nil
}
