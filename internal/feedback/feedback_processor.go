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
	logger = logger.With("pkg", "feedback.Processor")
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
	p.logger.Info("processing user feedback for all users...")
	return ProcessAllUsersFeedback(ctx, p.fs)
}

// ProcessUserFeedbackForSteeringQuery analyzes a user's unprocessed feedback
// and updates their SteeringQuery based on patterns in their likes/dislikes
func ProcessUserFeedbackForSteeringQuery(ctx context.Context, fs *internal.FirestoreClient, userID string) error {
	// Get unprocessed feedback for the user
	feedbacks, err := internal.GetUnprocessedUserFeedback(ctx, fs, userID)
	if err != nil {
		return fmt.Errorf("failed to get unprocessed feedback: %w", err)
	}

	// If no unprocessed feedback, nothing to do
	if len(feedbacks) == 0 {
		return nil
	}

	// Get current steering query
	currentSQ, err := internal.GetUserSteeringQuery(ctx, fs, userID)
	if err != nil {
		// If no steering query exists, we can't update it
		// This is expected for new users
		return fmt.Errorf("no steering query found for user %s: %w", userID, err)
	}

	// Analyze feedback patterns
	analysis := analyzeFeedbackPatterns(feedbacks)

	// Update steering query based on analysis
	updatedSQ, changed := updateSteeringQueryFromFeedback(*currentSQ, analysis)

	// Only update if something actually changed
	if changed {
		if err := internal.SetUserSteeringQuery(ctx, fs, userID, updatedSQ); err != nil {
			return fmt.Errorf("failed to update steering query: %w", err)
		}
	}

	// Mark feedback as processed
	feedbackIDs := make([]string, len(feedbacks))
	for i, feedback := range feedbacks {
		feedbackIDs[i] = feedback.ID
	}

	if err := internal.MarkFeedbackAsProcessed(ctx, fs, feedbackIDs, time.Now()); err != nil {
		return fmt.Errorf("failed to mark feedback as processed: %w", err)
	}

	return nil
}

// ProcessAllUsersFeedback processes feedback for all users who have unprocessed feedback
// This function is designed to be run as a daily batch job
func ProcessAllUsersFeedback(ctx context.Context, fs *internal.FirestoreClient) error {
	// Get all unprocessed feedback
	allFeedback, err := internal.GetAllUnprocessedFeedback(ctx, fs, 1000) // Limit to prevent memory issues
	if err != nil {
		return fmt.Errorf("failed to get unprocessed feedback: %w", err)
	}

	if len(allFeedback) == 0 {
		return nil // No feedback to process
	}

	// Group feedback by user ID
	userFeedbackMap := make(map[string][]*internal.UserFeedback)
	for _, feedback := range allFeedback {
		userFeedbackMap[feedback.UserID] = append(userFeedbackMap[feedback.UserID], feedback)
	}

	// Process each user's feedback
	processedUsers := 0
	errors := make([]error, 0)

	for userID := range userFeedbackMap {
		if err := ProcessUserFeedbackForSteeringQuery(ctx, fs, userID); err != nil {
			// Log error but continue processing other users
			errors = append(errors, fmt.Errorf("failed to process feedback for user %s: %w", userID, err))
			continue
		}
		processedUsers++
	}

	// Return aggregated error if any processing failed
	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		return fmt.Errorf("processed %d users, %d errors: %s",
			processedUsers, len(errors), strings.Join(errorMessages, "; "))
	}

	return nil
}

// FeedbackAnalysis represents patterns found in user feedback
type FeedbackAnalysis struct {
	TotalFeedback    int
	LikedCount       int
	DislikedCount    int
	LikedPatterns    []string       // Common words/phrases in liked items
	DislikedPatterns []string       // Common words/phrases in disliked items
	JobPreferences   map[string]int // Job-level preferences (job_id -> like count)
}

// analyzeFeedbackPatterns analyzes feedback to extract patterns
func analyzeFeedbackPatterns(feedbacks []*internal.UserFeedback) FeedbackAnalysis {
	analysis := FeedbackAnalysis{
		TotalFeedback:    len(feedbacks),
		LikedPatterns:    make([]string, 0),
		DislikedPatterns: make([]string, 0),
		JobPreferences:   make(map[string]int),
	}

	likedTexts := make([]string, 0)
	dislikedTexts := make([]string, 0)

	for _, feedback := range feedbacks {
		if feedback.Liked {
			analysis.LikedCount++
			likedTexts = append(likedTexts, feedback.ItemText)
			analysis.JobPreferences[feedback.JobID]++
		} else {
			analysis.DislikedCount++
			dislikedTexts = append(dislikedTexts, feedback.ItemText)
			analysis.JobPreferences[feedback.JobID]--
		}
	}

	// Extract common patterns (simplified - in practice you might use NLP)
	analysis.LikedPatterns = extractKeywords(likedTexts)
	analysis.DislikedPatterns = extractKeywords(dislikedTexts)

	return analysis
}

// extractKeywords is a simplified keyword extraction function
// In a real implementation, you might use NLP libraries or LLM analysis
func extractKeywords(texts []string) []string {
	wordCount := make(map[string]int)

	for _, text := range texts {
		// Simple word extraction (you'd want more sophisticated NLP here)
		words := strings.Fields(strings.ToLower(text))
		for _, word := range words {
			// Filter out common words and short words
			if len(word) > 3 && !isCommonWord(word) {
				wordCount[word]++
			}
		}
	}

	// Return words that appear in multiple texts
	keywords := make([]string, 0)
	for word, count := range wordCount {
		if count > 1 || (len(texts) == 1 && count >= 1) {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// isCommonWord filters out common English words
func isCommonWord(word string) bool {
	commonWords := map[string]bool{
		"the": true, "and": true, "for": true, "are": true, "but": true,
		"not": true, "you": true, "all": true, "can": true, "had": true,
		"her": true, "was": true, "one": true, "our": true, "out": true,
		"day": true, "get": true, "has": true, "him": true, "his": true,
		"how": true, "its": true, "may": true, "new": true, "now": true,
		"old": true, "see": true, "two": true, "who": true, "boy": true,
		"did": true, "use": true, "way": true, "she": true, "many": true,
		"oil": true, "sit": true, "set": true, "run": true, "eat": true,
		"far": true, "sea": true, "eye": true, "ask": true, "own": true,
		"under": true, "think": true, "also": true, "back": true,
		"after": true, "first": true, "well": true, "water": true,
		"even": true, "want": true, "because": true, "been": true,
		"any": true, "these": true, "give": true, "most": true,
		"us": true, "is": true, "call": true, "find": true,
		"long": true, "down": true, "come": true, "made": true, "part": true,
	}
	return commonWords[word]
}

// updateSteeringQueryFromFeedback updates a steering query based on feedback analysis
// Returns the updated steering query and whether any changes were made
func updateSteeringQueryFromFeedback(currentSQ internal.SteeringQuery, analysis FeedbackAnalysis) (internal.SteeringQuery, bool) {
	updated := currentSQ
	changed := false

	// If user consistently likes certain patterns, strengthen MoveToward
	if len(analysis.LikedPatterns) > 0 && analysis.LikedCount > analysis.DislikedCount {
		// Build a stronger MoveToward statement based on liked patterns
		likedPhrase := strings.Join(analysis.LikedPatterns[:min(3, len(analysis.LikedPatterns))], ", ")
		newMoveToward := fmt.Sprintf("I want roles that involve %s", likedPhrase)

		// Only update if significantly different from current
		if !strings.Contains(updated.MoveToward, likedPhrase) {
			if updated.MoveToward != "" {
				updated.MoveToward = fmt.Sprintf("%s and %s", updated.MoveToward, newMoveToward)
			} else {
				updated.MoveToward = newMoveToward
			}
			// Strengthen the force if user is consistently liking things
			if updated.ForceToward < 0.9 {
				updated.ForceToward = min(0.9, updated.ForceToward+0.1)
			}
			changed = true
		}
	}

	// If user consistently dislikes certain patterns, strengthen MoveAway
	if len(analysis.DislikedPatterns) > 0 && analysis.DislikedCount > analysis.LikedCount {
		// Build a stronger MoveAway statement based on disliked patterns
		dislikedPhrase := strings.Join(analysis.DislikedPatterns[:min(3, len(analysis.DislikedPatterns))], ", ")
		newMoveAway := fmt.Sprintf("I don't want roles that involve %s", dislikedPhrase)

		// Only update if significantly different from current
		if !strings.Contains(updated.MoveAway, dislikedPhrase) {
			if updated.MoveAway != "" {
				updated.MoveAway = fmt.Sprintf("%s and %s", updated.MoveAway, newMoveAway)
			} else {
				updated.MoveAway = newMoveAway
			}
			// Strengthen the force if user is consistently disliking things
			if updated.ForceAway < 0.9 {
				updated.ForceAway = min(0.9, updated.ForceAway+0.1)
			}
			changed = true
		}
	}

	// Update RerankQuery to incorporate feedback patterns
	if changed {
		// Rebuild the rerank query to incorporate the updated MoveToward and MoveAway
		updated.RerankQuery = fmt.Sprintf("I am looking for %s positions. %s. %s",
			updated.Query, updated.MoveToward, updated.MoveAway)
	}

	return updated, changed
}
