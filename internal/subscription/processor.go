package subscription

import (
	"context"
	"fmt"
	"log"
	"sync"
	"talent-rodeo/internal"
	"time"

	"github.com/getsentry/sentry-go"
)

const (
	// DefaultTickInterval is the default interval for processing subscriptions
	DefaultTickInterval = 24 * time.Hour

	// DefaultTopK is the number of top jobs to return from semantic search
	DefaultTopK = 100
)

// Processor handles the periodic checking and publishing of search criteria subscriptions
type Processor struct {
	fs         *internal.FirestoreClient
	ticker     *time.Ticker
	done       chan bool
	wg         sync.WaitGroup
	interval   time.Duration
	isRunning  bool
	cancelFunc context.CancelFunc
}

// NewProcessor creates a new subscription processor
func NewProcessor(fs *internal.FirestoreClient) *Processor {
	return &Processor{
		fs:       fs,
		interval: DefaultTickInterval,
		done:     make(chan bool),
	}
}

// WithInterval sets a custom tick interval
func (p *Processor) WithInterval(interval time.Duration) *Processor {
	p.interval = interval
	return p
}

// Start begins the subscription processor
func (p *Processor) Start(ctx context.Context) error {
	if p.isRunning {
		return fmt.Errorf("processor is already running")
	}

	log.Printf("Starting subscription processor with interval %v", p.interval)

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
			log.Printf("Error during initial subscription processing: %v", err)
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
					log.Printf("Error during subscription processing: %v", err)
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

// Stop halts the subscription processor
func (p *Processor) Stop() {
	if !p.isRunning {
		return
	}

	log.Println("Stopping subscription processor")
	p.ticker.Stop()
	p.done <- true

	if p.cancelFunc != nil {
		p.cancelFunc()
	}

	p.wg.Wait()
	p.isRunning = false
	log.Println("Subscription processor stopped")
}

// process handles the main logic of checking for eligible criteria and publishing to subscribers
func (p *Processor) process(ctx context.Context) error {
	log.Println("Processing search criteria subscriptions...")

	// Get all criteria
	allCriteria, err := internal.ListSearchCriteria(p.fs)
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

	log.Printf("Found %d eligible criteria for processing", len(eligibleCriteria))

	// Process each eligible criterion
	for _, criteria := range eligibleCriteria {
		if err := p.processCriteria(ctx, criteria); err != nil {
			// Log error but continue with other criteria
			log.Printf("Error processing criteria %s: %v", criteria.ID, err)
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"source":      "subscription_processor",
				"criteria_id": criteria.ID,
			})
			continue
		}
	}

	return nil
}

// processCriteria handles processing a single search criteria
func (p *Processor) processCriteria(ctx context.Context, criteria internal.SearchCriteria) error {
	log.Printf("Processing criteria: %s", criteria.ID)

	// Get all subscribers for this criteria
	subscribers, err := internal.GetCriteriaSubscribers(p.fs, criteria.ID)
	if err != nil {
		return fmt.Errorf("failed to get subscribers for criteria %s: %w", criteria.ID, err)
	}

	if len(subscribers) == 0 {
		log.Printf("No subscribers found for criteria %s, skipping", criteria.ID)
		return nil
	}

	// Get all jobs found in the last 24 hours for this criteria
	twentyFourHoursAgo := time.Now().Add(-24 * time.Hour)
	jobIDs, err := internal.GetJobIDsForCriteriaCreatedAfter(p.fs, criteria.ID, twentyFourHoursAgo)
	if err != nil {
		return fmt.Errorf("failed to get recent jobs for criteria %s: %w", criteria.ID, err)
	}

	if len(jobIDs) == 0 {
		log.Printf("No new jobs found for criteria %s, skipping", criteria.ID)
		return nil
	}

	log.Printf("Found %d new jobs for criteria %s", len(jobIDs), criteria.ID)

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
			log.Printf("Error processing jobs for user %s: %v", user.ID, err)
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
	if err := internal.UpdateSearchCriteria(p.fs, &criteria); err != nil {
		return fmt.Errorf("failed to update LastPublishedAt for criteria %s: %w", criteria.ID, err)
	}

	log.Printf("Successfully processed criteria %s", criteria.ID)
	return nil
}

// processUserJobs handles the semantic search and job creation for a single user
func (p *Processor) processUserJobs(ctx context.Context, user *internal.User, criteria internal.SearchCriteria) error {
	// Skip users without a steering query
	if user.SteeringQuery == nil {
		log.Printf("User %s has no steering query, skipping", user.ID)
		return nil
	}

	// Search for jobs using the user's steering query and the criteria ID
	scoredJobs, err := internal.SemanticSearchJobs(ctx, user.SteeringQuery, criteria.ID, DefaultTopK)
	if err != nil {
		return fmt.Errorf("failed to search jobs for user %s: %w", user.ID, err)
	}

	if len(scoredJobs) == 0 {
		log.Printf("No matching jobs found for user %s and criteria %s", user.ID, criteria.ID)
		return nil
	}

	log.Printf("Found %d matching jobs for user %s", len(scoredJobs), user.ID)

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

		log.Printf("Created job %s for user %s", jobID, user.ID)
	}

	return nil
}
