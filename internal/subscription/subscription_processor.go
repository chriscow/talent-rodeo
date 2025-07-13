// Package subscription provides functionality for handling search criteria subscriptions
// and publishing job matches to users based on their preferences.
package subscription

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"talent-rodeo/internal"
	"talent-rodeo/internal/cloudflare"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/pusher/pusher-http-go/v5"
)

const (
	// DefaultTickInterval is the default interval for processing subscriptions
	DefaultTickInterval = 5 * time.Minute
	// FeatureFlag for subscription processor
	SubscriptionProcessorKey = "subscription-processor"

	subscriptionConcurrency = 5
)

type ProcessorSettings struct {
	Interval          time.Duration // Interval for processing subscriptions
	Paused            bool          // Whether the processor is paused
	JobThresholdScore float64       // Minimum job score to consider for publishing
	StartHour         int           // Hour to start processing (24-hour format, UTC)
	EndHour           int           // Hour to end processing (24-hour format, UTC)
}

var DefaultSettings = ProcessorSettings{
	Interval:          DefaultTickInterval,
	Paused:            false, // Default to not paused (active)
	JobThresholdScore: 0.5,   // Default job score threshold
	StartHour:         17,    // 17:00 UTC
	EndHour:           0,     // 00:00 UTC (crosses midnight)
}

// Processor handles the periodic checking and publishing of search criteria subscriptions.
// It runs on a configurable interval, processes eligible search criteria, and publishes
// relevant job matches to subscribed users.
type Processor struct {
	fs         *internal.FirestoreClient // Firestore client for database operations
	logger     *slog.Logger              // Logger for recording processor activities
	psh        *pusher.Client            // Pusher client for real-time notifications
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
func NewProcessor(fs *internal.FirestoreClient, psh *pusher.Client, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	if fs == nil {
		internal.SentryReportError(context.Background(), errors.New("firestore client cannot be nil"), sentry.LevelError, map[string]string{
			"action": "NewProcessor: firestore client cannot be nil",
		})
		panic("firestore client cannot be nil")
	}

	settings := DefaultSettings

	logger = logger.With("component", "SubscriptionProcessor", "pkg", "subscription")
	return &Processor{
		fs:       fs,
		logger:   logger,
		psh:      psh,
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
		startErr := fmt.Errorf("processor is already running")
		internal.SentryReportError(ctx, startErr, sentry.LevelError, map[string]string{
			"action": "Processor::Start: processor is already running",
		})
		return startErr
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
		internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
			"action": "Processor::initCloudflareKV: failed to create Cloudflare KV client",
		})
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
			internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
				"action": "Processor::RefreshSettings: failed to get feature flag",
			})
			return fmt.Errorf("failed to get feature flag: %w", err)
		}
		// If not found, use default settings
		settings = DefaultSettings
		if err := p.kv.Set(SubscriptionProcessorKey, DefaultSettings, 0); err != nil {
			p.logger.Error("failed to set default subscription processor settings in Cloudflare KV", "key", SubscriptionProcessorKey, "error", err)
			internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
				"action": "Processor::RefreshSettings: failed to set default feature flag",
			})
			return fmt.Errorf("failed to set default feature flag: %w", err)
		}
	}

	p.settings = settings
	if p.settings.Interval <= 0 {
		p.settings.Interval = DefaultTickInterval
	}
	// Set default time window if not configured
	if p.settings.StartHour == 0 && p.settings.EndHour == 0 {
		// Default to 24/7 operation if no hours are set
	}
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.ticker = time.NewTicker(p.settings.Interval)

	p.logger.Info("refreshed subscription processor settings", "interval", p.settings.Interval, "paused", p.settings.Paused, "job_threshold_score", p.settings.JobThresholdScore)
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
	if p.settings.Paused {
		p.logger.Info("subscription processor is paused, skipping processing")
		return nil
	}

	if !p.IsWithinAllowedHours() {
		p.logger.Debug("subscription processor outside allowed hours",
			"start_hour", p.settings.StartHour,
			"end_hour", p.settings.EndHour,
			"current_time_utc", time.Now().UTC().Format("15:04"))
		return nil
	}

	p.logger.Info("processing search criteria subscriptions...")

	// Get all criteria
	allCriteria, err := internal.ListSearchCriteria(ctx, p.fs)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "Processor::process: failed to list search criteria",
		})
		return fmt.Errorf("failed to list search criteria: %w", err)
	}

	// Filter for eligible criteria:
	// - LastPublishedAt older than 24 hours OR nil
	// - LastSearchedAt within the last 24 hours
	// - SubscriberCount > 0
	twentyFourHoursAgo := time.Now().Add(-24 * time.Hour)
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	var eligibleCriteria []internal.SearchCriteria

	for _, criteria := range allCriteria {

		// Must have been searched recently
		if criteria.LastSearchedAt == nil || criteria.LastSearchedAt.Before(twentyFourHoursAgo) {
			continue
		}

		// Must not have been published recently
		if criteria.LastPublishedAt != nil && criteria.LastPublishedAt.After(twoHoursAgo) {
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
// SUBSCRIPTION-AWARE PROCESSING (Added 2025-06-30):
// As part of the "Freemium Trial" business model implementation, this function now:
// - Filters subscribers to only include users with active subscriptions (SubscriptionEnd > now)
// - Skips job analysis and publishing for users whose trial/subscription has expired
// - Updates LastPublishedAt even if no active subscribers exist to prevent repeated processing
//
// This ensures that expensive LLM-based job analysis is only performed for users who are
// within their trial period or have active paid subscriptions, aligning costs with revenue.
//
// The function updates the LastPublishedAt timestamp on the criteria after successful processing.
// Errors during processing for individual users are logged and reported to Sentry,
// but do not stop the processing for other users.
func (p *Processor) processCriteria(ctx context.Context, criteria internal.SearchCriteria) error {
	p.logger.Info("processing criteria", "id", criteria.ID)

	// Get all subscribers for this criteria
	subscribers, err := internal.GetCriteriaSubscribers(ctx, p.fs, criteria.ID)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "Processor::processCriteria: failed to get subscribers for criteria",
		})
		return fmt.Errorf("failed to get subscribers for criteria %s: %w", criteria.ID, err)
	}

	if len(subscribers) == 0 {
		p.logger.Info("No subscribers found for criteria, skipping", "id", criteria.ID)
		return nil
	}

	// Extract user IDs from subscriptions
	userIDs := make([]string, len(subscribers))
	for i, sub := range subscribers {
		userIDs[i] = sub.UserID
	}

	p.logger.Info("found new jobs for criteria", "id", criteria.ID, "user-count", len(userIDs))

	// Get all users in one call
	users, err := internal.GetUsersByIDs(ctx, p.fs, userIDs)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "Processor::processCriteria: failed to get users",
		})
		return fmt.Errorf("failed to get users: %w", err)
	}

	// Filter for users with active subscriptions
	var activeUsers []*internal.User
	nowForFilter := time.Now()
	for _, user := range users {
		if user.SubscriptionEnd != nil && user.SubscriptionEnd.After(nowForFilter) {
			activeUsers = append(activeUsers, user)
		} else {
			p.logger.Info("user subscription has lapsed, skipping for publication", "userID", user.ID)
		}
	}

	if len(activeUsers) == 0 {
		p.logger.Info("no active subscribers found for criteria, skipping publication", "id", criteria.ID)
		// Update LastPublishedAt for this criteria so we don't check it again immediately
		publishedTime := time.Now()
		criteria.LastPublishedAt = &publishedTime
		if err := internal.SetSearchCriteria(ctx, p.fs, &criteria); err != nil {
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action":     "Processor::processCriteria: failed to update LastPublishedAt",
				"criteriaID": criteria.ID,
			})
			return fmt.Errorf("failed to update LastPublishedAt for criteria %s: %w", criteria.ID, err)
		}
		return nil
	}

	// Use a semaphore to limit concurrency to 5
	sem := make(chan struct{}, subscriptionConcurrency)
	var wg sync.WaitGroup

	for _, user := range activeUsers {
		sem <- struct{}{} // acquire a slot
		wg.Add(1)
		go func(user *internal.User) {
			defer func() {
				<-sem // release the slot
				wg.Done()
			}()
			p.logger.Info("refreshing user jobs", "user-id", user.ID, "criteria-id", criteria.ID)
			if err := RefreshUserJobs(ctx, p.fs, p.psh, p.logger, *user, criteria); err != nil {
				// Log error but continue with other users
				p.logger.Error("Error processing jobs for user", "user-id", user.ID, "error", err)
				internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
					"source":      "subscription_processor",
					"criteria_id": criteria.ID,
					"user_id":     user.ID,
				})
			}
		}(user)
	}

	wg.Wait()

	// Update LastPublishedAt for this criteria
	now := time.Now()
	criteria.LastPublishedAt = &now
	if err := internal.SetSearchCriteria(ctx, p.fs, &criteria); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action":     "Processor::processCriteria: failed to update LastPublishedAt",
			"criteriaID": criteria.ID,
		})
		return fmt.Errorf("failed to update LastPublishedAt for criteria %s: %w", criteria.ID, err)
	}

	p.logger.Info("successfully processed criteria", "id", criteria.ID)
	return nil
}

func (p *Processor) Paused() bool {
	return p.settings.Paused
}

// IsWithinAllowedHours checks if the current time (UTC) is within the configured off-peak hours.
// The time window can span midnight (e.g., 17:00-00:00).
func (p *Processor) IsWithinAllowedHours() bool {
	// Special case: if both are 0, run 24/7
	if p.settings.StartHour == 0 && p.settings.EndHour == 0 {
		return true
	}

	now := time.Now().UTC()
	currentHour := now.Hour()

	// Handle the case where the time window crosses midnight
	if p.settings.StartHour > p.settings.EndHour {
		// Window crosses midnight (e.g., 17:00-00:00)
		return currentHour >= p.settings.StartHour || currentHour < p.settings.EndHour
	} else {
		// Window doesn't cross midnight (e.g., 09:00-17:00)
		return currentHour >= p.settings.StartHour && currentHour < p.settings.EndHour
	}
}
