package feedback

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"talent-rodeo/internal"
	"talent-rodeo/internal/cloudflare"
	"time"

	"log/slog"
)

const (
	DefaultTickInterval  = 24 * time.Hour
	FeedbackProcessorKey = "feedback-processor"
)

type ProcessorSettings struct {
	Paused    bool
	Interval  time.Duration
	StartHour int
	EndHour   int
}

var DefaultSettings = ProcessorSettings{
	Paused:    false,
	Interval:  DefaultTickInterval,
	StartHour: 17,
	EndHour:   0,
}

type Processor struct {
	fs         *internal.FirestoreClient
	logger     *slog.Logger
	ticker     *time.Ticker
	done       chan bool
	wg         sync.WaitGroup
	kv         *cloudflare.KV
	settings   ProcessorSettings
	isRunning  bool
	cancelFunc context.CancelFunc
}

func NewProcessor(fs *internal.FirestoreClient, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	if fs == nil {
		panic("firestore client cannot be nil")
	}
	settings := DefaultSettings
	logger = logger.With("component", "FeedbackProcessor", "pkg", "feedback")
	return &Processor{
		fs:       fs,
		logger:   logger,
		done:     make(chan bool),
		settings: settings,
	}
}

func (p *Processor) Start(ctx context.Context) error {
	if p.isRunning {
		return fmt.Errorf("processor is already running")
	}
	p.logger.Info("starting feedback processor", "interval", p.settings.Interval)
	if err := p.initCloudflareKV(); err != nil {
		p.logger.Error("failed to initialize Cloudflare KV client", "error", err)
	}
	if err := p.RefreshSettings(); err != nil {
		p.logger.Error("failed to refresh feedback processor settings", "error", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	p.cancelFunc = cancel
	p.ticker = time.NewTicker(p.settings.Interval)
	p.isRunning = true
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if err := p.process(ctx); err != nil {
			p.logger.Error("error during initial feedback processing", "error", err)
		}
	}()
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-p.ticker.C:
				if err := p.process(ctx); err != nil {
					p.logger.Error("error during feedback processing", "error", err)
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

func (p *Processor) Stop() {
	if !p.isRunning {
		return
	}
	p.logger.Info("stopping feedback processor")
	p.ticker.Stop()
	p.done <- true
	if p.cancelFunc != nil {
		p.cancelFunc()
	}
	p.wg.Wait()
	p.isRunning = false
	p.logger.Info("feedback processor stopped")
}

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
	if p.kv != nil {
		if err := p.kv.Get(FeedbackProcessorKey, &settings); err != nil {
			settings = DefaultSettings
			p.logger.Info("No feedback processor settings found, using defaults")
			if err := p.kv.Set(FeedbackProcessorKey, settings, 0); err != nil {
				return fmt.Errorf("failed to set default feedback processor settings: %w", err)
			}
			p.logger.Info("Default feedback processor settings saved to Cloudflare KV", "settings", settings)
		}
	} else {
		p.logger.Info("Cloudflare KV not initialized, using default settings")
	}
	p.settings = settings
	if p.settings.Interval <= 0 {
		p.settings.Interval = DefaultTickInterval
	}
	if p.settings.StartHour == 0 && p.settings.EndHour == 0 {
		p.settings.StartHour = DefaultSettings.StartHour
		p.settings.EndHour = DefaultSettings.EndHour
	}
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.ticker = time.NewTicker(p.settings.Interval)
	return nil
}

func (p *Processor) IsWithinAllowedHours() bool {
	if p.settings.StartHour == 0 && p.settings.EndHour == 0 {
		return true
	}
	now := time.Now().UTC()
	currentHour := now.Hour()
	if p.settings.StartHour > p.settings.EndHour {
		return currentHour >= p.settings.StartHour || currentHour < p.settings.EndHour
	} else {
		return currentHour >= p.settings.StartHour && currentHour < p.settings.EndHour
	}
}

func (p *Processor) process(ctx context.Context) error {
	p.RefreshSettings()
	if p.settings.Paused {
		p.logger.Info("feedback processor is paused, skipping processing")
		return nil
	}
	if !p.IsWithinAllowedHours() {
		p.logger.Debug("feedback processor outside allowed hours",
			"start_hour", p.settings.StartHour,
			"end_hour", p.settings.EndHour,
			"current_time_utc", time.Now().UTC().Format("15:04"))
		return nil
	}
	p.logger.Info("processing user feedback for all users with enhanced processor...")
	return ProcessAllUsersFeedback(ctx, p.fs, p.logger)
}

func ProcessUserFeedback(ctx context.Context, fs *internal.FirestoreClient, logger *slog.Logger, userID string) error {
	llmProcessor := newFeedbackProcessor(newFeedbackStore(fs), fs, logger, NewMindsClient())
	return llmProcessor.ProcessUserFeedback(ctx, userID)
}

// CleanupOrphanedFeedback removes feedback for users that no longer exist
// This can be run as a maintenance task to clean up orphaned data
func CleanupOrphanedFeedback(ctx context.Context, fs *internal.FirestoreClient, logger *slog.Logger) error {
	// Get all unprocessed feedback
	allFeedback, err := internal.GetAllUnprocessedFeedback(ctx, fs, 0) // No limit for cleanup
	if err != nil {
		return fmt.Errorf("failed to get unprocessed feedback: %w", err)
	}

	// Group feedback by user ID
	userFeedbackMap := make(map[string][]*internal.UserFeedback)
	for _, feedback := range allFeedback {
		userFeedbackMap[feedback.UserID] = append(userFeedbackMap[feedback.UserID], feedback)
	}

	cleanedCount := 0
	for userID, feedbacks := range userFeedbackMap {
		// Check if user exists
		_, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			logger.Error("error checking user existence during cleanup",
				"user_id", userID,
				"error", err)
			continue
		}

		if !found {
			// User doesn't exist, mark their feedback as processed
			feedbackIDs := make([]string, 0, len(feedbacks))
			for _, feedback := range feedbacks {
				feedbackIDs = append(feedbackIDs, feedback.ID)
			}

			if err := internal.MarkFeedbackAsProcessed(ctx, fs, feedbackIDs, time.Now()); err != nil {
				logger.Error("failed to mark orphaned feedback as processed during cleanup",
					"user_id", userID,
					"feedback_count", len(feedbackIDs),
					"error", err)
			} else {
				logger.Info("cleaned up orphaned feedback for deleted user",
					"user_id", userID,
					"feedback_count", len(feedbackIDs))
				cleanedCount += len(feedbackIDs)
			}
		}
	}

	logger.Info("orphaned feedback cleanup completed",
		"total_feedback_items", len(allFeedback),
		"cleaned_items", cleanedCount)

	return nil
}

// ProcessAllUsersFeedback processes feedback for all users who have unprocessed feedback
// using the enhanced LLM processor with metrics collection.
func ProcessAllUsersFeedback(ctx context.Context, fs *internal.FirestoreClient, logger *slog.Logger) error {
	llmProcessor := newFeedbackProcessor(newFeedbackStore(fs), fs, logger, NewMindsClient())
	metricsCollector := NewMetricsCollector(logger)

	startTime := time.Now()

	// Get all unprocessed feedback
	allFeedback, err := internal.GetAllUnprocessedFeedback(ctx, fs, 1000) // Limit to prevent memory issues
	if err != nil {
		return fmt.Errorf("failed to get unprocessed feedback: %w", err)
	}

	if len(allFeedback) == 0 {
		logger.Info("no unprocessed feedback found")
		return nil // No feedback to process
	}

	// Group feedback by user ID
	userFeedbackMap := make(map[string][]*internal.UserFeedback)
	for _, feedback := range allFeedback {
		userFeedbackMap[feedback.UserID] = append(userFeedbackMap[feedback.UserID], feedback)
	}

	// Process each user's feedback
	var errors []error
	processedUsers := 0
	successfulUsers := 0
	totalFeedbackItems := len(allFeedback)

	for userID := range userFeedbackMap {
		if err := llmProcessor.ProcessUserFeedback(ctx, userID); err != nil {
			// Check if this is a deleted user error
			if strings.Contains(err.Error(), "user deleted") {
				// Mark feedback as processed to prevent infinite retries for deleted users
				feedbackIDs := make([]string, 0, len(userFeedbackMap[userID]))
				for _, feedback := range userFeedbackMap[userID] {
					feedbackIDs = append(feedbackIDs, feedback.ID)
				}
				if markErr := internal.MarkFeedbackAsProcessed(ctx, fs, feedbackIDs, time.Now()); markErr != nil {
					logger.Error("failed to mark orphaned feedback as processed",
						"user_id", userID,
						"feedback_count", len(feedbackIDs),
						"error", markErr)
				} else {
					logger.Info("marked orphaned feedback as processed for deleted user",
						"user_id", userID,
						"feedback_count", len(feedbackIDs))
				}
				// Count as processed but not successful
				processedUsers++
			} else {
				// Log error but continue processing other users
				errors = append(errors, fmt.Errorf("failed to process feedback for user %s: %w", userID, err))
			}
			continue
		}
		processedUsers++
		successfulUsers++
	}

	// Log daily processing summary
	avgProcessingTime := time.Since(startTime) / time.Duration(len(userFeedbackMap))
	metricsCollector.LogDailyProcessingSummary(ctx, len(userFeedbackMap), successfulUsers, len(errors), totalFeedbackItems, avgProcessingTime)

	// Return aggregated error if any processing failed
	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		return fmt.Errorf("processed %d users with %d errors: %s",
			processedUsers, len(errors), strings.Join(errorMessages, "; "))
	}

	logger.Info("all user feedback processed successfully", "processed_users", processedUsers)
	return nil
}
