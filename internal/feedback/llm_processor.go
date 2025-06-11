package feedback

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"talent-rodeo/internal"
)

// Supporting types and functions
type SemanticFeedbackAnalysis struct {
	LikedThemes      []SemanticTheme
	DislikedThemes   []SemanticTheme
	ConfidenceScore  float64
	TrendingPatterns []string
	Reasoning        string
}

type SemanticTheme struct {
	Theme      string
	Keywords   []string
	Confidence float64
	Frequency  int
}

type FeedbackData struct {
	LikedItems    []FeedbackItem
	DislikedItems []FeedbackItem
	TotalCount    int
}

type FeedbackItem struct {
	JobID    string
	ItemID   string
	ItemText string
	Date     string
}

type llmProcessor struct {
	store            FeedbackStore
	logger           *slog.Logger
	analysisChain    *analyzer
	queryUpdateChain *queryUpdater
	validationChain  *validator
}

func newLLMProcessor(store FeedbackStore, logger *slog.Logger, llm LLMClient) *llmProcessor {
	return &llmProcessor{
		store:            store,
		logger:           logger.With("component", "llm_feedback_processor"),
		analysisChain:    newAnalyzer(llm, logger),
		queryUpdateChain: newQueryUpdater(llm, logger),
		validationChain:  newValidator(llm, logger),
	}
}

// ProcessUserFeedbackEnhanced method replacing current ProcessUserFeedbackForSteeringQuery
func (p *llmProcessor) ProcessUserFeedback(ctx context.Context, userID string) error {
	// Get unprocessed feedback
	feedbacks, err := p.store.GetUnprocessedUserFeedback(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get unprocessed feedback: %w", err)
	}

	if len(feedbacks) == 0 {
		return nil // No feedback to process
	}

	// Get current steering query
	currentSQ, err := p.store.GetUserSteeringQuery(ctx, userID)
	if err != nil {
		return fmt.Errorf("no steering query found for user %s: %w", userID, err)
	}

	// Phase 1: Semantic analysis
	analysis, err := p.analysisChain.Run(ctx, feedbacks, *currentSQ)
	if err != nil {
		return fmt.Errorf("failed to analyze feedback semantics: %w", err)
	}

	// Phase 2: Update steering query
	updatedSQ, err := p.queryUpdateChain.Run(ctx, *currentSQ, analysis)
	if err != nil {
		return fmt.Errorf("failed to update steering query: %w", err)
	}

	// Phase 3: Validation
	if err := p.validationChain.Run(ctx, updatedSQ); err != nil {
		return fmt.Errorf("updated steering query validation failed: %w", err)
	}

	// Save updated steering query
	if err := p.store.SetUserSteeringQuery(ctx, userID, *updatedSQ); err != nil {
		return fmt.Errorf("failed to save updated steering query: %w", err)
	}

	// Mark feedback as processed
	feedbackIDs := make([]string, len(feedbacks))
	for i, feedback := range feedbacks {
		feedbackIDs[i] = feedback.ID
	}

	if err := p.store.MarkFeedbackAsProcessed(ctx, feedbackIDs, time.Now()); err != nil {
		return fmt.Errorf("failed to mark feedback as processed: %w", err)
	}

	p.logger.Info("enhanced feedback processing completed",
		"user_id", userID,
		"feedback_count", len(feedbacks),
		"confidence_score", analysis.ConfidenceScore,
	)

	return nil
}

func prepareFeedbackData(feedbacks []*internal.UserFeedback) FeedbackData {
	data := FeedbackData{
		LikedItems:    make([]FeedbackItem, 0),
		DislikedItems: make([]FeedbackItem, 0),
		TotalCount:    len(feedbacks),
	}

	for _, feedback := range feedbacks {
		item := FeedbackItem{
			JobID:    feedback.JobID,
			ItemID:   feedback.ItemID,
			ItemText: feedback.ItemText,
			Date:     feedback.CreatedAt.Format("2006-01-02"),
		}

		if feedback.Liked {
			data.LikedItems = append(data.LikedItems, item)
		} else {
			data.DislikedItems = append(data.DislikedItems, item)
		}
	}

	return data
}
