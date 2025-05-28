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
	"talent-rodeo/internal/cloudflare"
	"time"

	"github.com/getsentry/sentry-go"
)

const (
	// DefaultTickInterval is the default interval for processing subscriptions (24 hours)
	DefaultTickInterval = 24 * time.Hour

	// FeatureFlag for subscription processor
	SubscriptionProcessorKey = "subscription-processor"
)

type ProcessorSettings struct {
	Interval          time.Duration // Interval for processing subscriptions
	Enabled           bool          // Whether the processor is enabled
	JobThresholdScore float64       // Minimum job score to consider for publishing
}

var DefaultSettings = ProcessorSettings{
	Interval:          DefaultTickInterval,
	Enabled:           true, // Default to enabled
	JobThresholdScore: 0.5,  // Default job score threshold
}

// Processor handles the periodic checking and publishing of search criteria subscriptions.
// It runs on a configurable interval, processes eligible search criteria, and publishes
// relevant job matches to subscribed users.
type Processor struct {
	fs         *internal.FirestoreClient // Firestore client for database operations
	logger     *slog.Logger              // Logger for recording processor activities
	ticker     *time.Ticker              // Ticker for scheduling periodic processing
	done       chan bool                 // Channel to signal shutdown
	wg         sync.WaitGroup            // WaitGroup to manage goroutines
	isRunning  bool                      // Flag indicating if the processor is currently running
	cancelFunc context.CancelFunc        // Function to cancel the processing context
	kv         *cloudflare.KV            // Cloudflare KV client for feature flags
	settings   ProcessorSettings         // Settings for the processor, including interval and enabled state
}

// NewProcessor creates a new subscription processor with the provided Firestore client and logger.
// If no logger is provided, it creates a default logger that writes to stdout.
// The Firestore client is required and the function will panic if nil is provided.
func NewProcessor(fs *internal.FirestoreClient, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	if fs == nil {
		panic("firestore client cannot be nil")
	}

	settings := DefaultSettings

	logger = logger.With("pkg", "subscription.Processor")
	return &Processor{
		fs:       fs,
		logger:   logger,
		done:     make(chan bool),
		settings: settings,
	}
}

// WithInterval sets a custom tick interval for the processor.
// This method allows for fluent configuration of the processor.
//
// Example:
//
//	processor := subscription.NewProcessor(fs, logger).WithInterval(12 * time.Hour)
func (p *Processor) WithInterval(interval time.Duration) *Processor {
	p.settings.Interval = interval
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

	p.logger.Info("starting subscription processor", "interval", p.settings.Interval)

	// Initialize Cloudflare KV client for feature flags
	if err := p.initCloudflareKV(); err != nil {
		p.logger.Error("failed to initialize Cloudflare KV client", "error", err)
		// Continue without feature flags
	}

	if err := p.RefreshSettings(); err != nil {
		p.logger.Error("failed to refresh subscription processor settings", "error", err)
		// Continue with default settings
	}

	// Create a cancelable context
	ctx, cancel := context.WithCancel(ctx)
	p.cancelFunc = cancel

	p.ticker = time.NewTicker(p.settings.Interval)
	p.isRunning = true

	// Run once at startup
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if err := p.process(ctx); err != nil {
			p.logger.Error("error during initial subscription processing", "error", err)
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
					p.logger.Error("error during subscription processing", "error", err)
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

// initCloudflareKV initializes the Cloudflare KV client
func (p *Processor) initCloudflareKV() error {
	kv, err := cloudflare.NewKV()
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare KV client: %w", err)
	}

	p.kv = kv
	return nil
}

func (p *Processor) RefreshSettings() error {
	var settings ProcessorSettings
	if err := p.kv.Get(SubscriptionProcessorKey, &settings); err != nil {
		if err != cloudflare.ErrNotFound {
			p.logger.Error("failed to get subscription processor settings from Cloudflare KV", "key", SubscriptionProcessorKey, "error", err)
			return fmt.Errorf("failed to get feature flag: %w", err)
		}
		// If not found, use default settings
		settings = DefaultSettings
		if err := p.kv.Set(SubscriptionProcessorKey, DefaultSettings, 0); err != nil {
			p.logger.Error("failed to set default subscription processor settings in Cloudflare KV", "key", SubscriptionProcessorKey, "error", err)
			return fmt.Errorf("failed to set default feature flag: %w", err)
		}
	}

	p.settings = settings
	if p.settings.Interval <= 0 {
		p.settings.Interval = DefaultTickInterval
	}
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.ticker = time.NewTicker(p.settings.Interval)

	p.logger.Info("refreshed subscription processor settings", "interval", p.settings.Interval, "enabled", p.settings.Enabled, "job_threshold_score", p.settings.JobThresholdScore)
	return nil
}

// Stop halts the subscription processor.
// It stops the ticker, signals the goroutines to stop, and waits for all processing to complete.
// This method is safe to call even if the processor is not running.
func (p *Processor) Stop() {
	if !p.isRunning {
		return
	}

	p.logger.Info("stopping subscription processor")
	p.ticker.Stop()
	p.done <- true

	if p.cancelFunc != nil {
		p.cancelFunc()
	}

	p.wg.Wait()
	p.isRunning = false

	p.logger.Info("subscription processor stopped")
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
	p.RefreshSettings()
	if !p.settings.Enabled {
		p.logger.Info("subscription processor is disabled, skipping processing")
		return nil
	}

	p.logger.Info("processing search criteria subscriptions...")

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

		// Must have been searched recently
		if criteria.LastSearchedAt == nil || criteria.LastSearchedAt.Before(twentyFourHoursAgo) {
			continue
		}

		// Must not have been published recently
		if criteria.LastPublishedAt != nil && criteria.LastPublishedAt.After(twentyFourHoursAgo) {
			continue
		}

		if criteria.SubscriberCount <= 0 {
			p.logger.Warn("criteria has no subscribers", "id", criteria.ID)
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
	p.logger.Info("processing criteria", "id", criteria.ID)

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

	// Extract user IDs from subscriptions
	userIDs := make([]string, len(subscribers))
	for i, sub := range subscribers {
		userIDs[i] = sub.UserID
	}

	p.logger.Info("found new jobs for criteria", "jobs-count", len(jobIDs), "id", criteria.ID, "user-count", len(userIDs))

	// Get all users in one call
	users, err := internal.GetUsersByIDs(ctx, p.fs, userIDs)
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	for _, user := range users {
		p.logger.Info("refreshing user jobs", "user-id", user.ID, "criteria-id", criteria.ID)
		if err := internal.RefreshUserJobs(ctx, p.fs, user, criteria, p.settings.JobThresholdScore); err != nil {
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

	p.logger.Info("successfully processed criteria", "id", criteria.ID)
	return nil
}

/*
func (app *application) saveUploadedFile(file multipart.File, filepath string) error {
	// Read the entire file into a byte slice
	b, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("couldn't read file: %v", err)
	}

	fileSize := int64(len(b))

	// Convert the byte slice back into a reader
	fileReader := bytes.NewReader(b)

	// Create the S3 Bucket object
	bucket := os.Getenv("SPACES_BUCKET")

	app.logger.Info("Uploading file to DigitalOcean Spaces", "bucket", bucket, "key", filepath, "size", fileSize)

	// Create the parameters for the upload
	params := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(filepath),
		Body:          fileReader,
		ACL:           aws.String("public-read"), // or "public-read" if you want the file to be publicly accessible
		ContentLength: aws.Int64(fileSize),
	}

	// Upload the file to S3
	_, err = app.s3.PutObject(params)
	if err != nil {
		return fmt.Errorf("couldn't upload file: %v", err)
	}

	return nil
}

func (app *application) deleteImageByName(imageName string) error {
	// Create the S3 Bucket object
	bucket := os.Getenv("SPACES_BUCKET")

	app.logger.Info("Deleting file from DigitalOcean Spaces", "bucket", bucket, "key", imageName)

	// Create the parameters for the delete operation
	params := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(imageName),
	}

	// Delete the object from S3
	_, err := app.s3.DeleteObject(params)
	if err != nil {
		return fmt.Errorf("couldn't delete file: %v", err)
	}

	app.logger.Info("File deleted successfully", "bucket", bucket, "key", imageName)

	return nil
}
*/
