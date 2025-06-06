package feedback

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"talent-rodeo/internal"
)

type LLMFeedbackProcessor struct {
	store            FeedbackStore
	logger           *slog.Logger
	analysisChain    *FeedbackAnalysisChain
	queryUpdateChain *SteeringQueryUpdateChain
	validationChain  *ValidationChain
}

func NewLLMFeedbackProcessor(store FeedbackStore, logger *slog.Logger, llm LLMClient) *LLMFeedbackProcessor {
	return &LLMFeedbackProcessor{
		store:            store,
		logger:           logger.With("component", "llm_feedback_processor"),
		analysisChain:    NewFeedbackAnalysisChain(llm, logger),
		queryUpdateChain: NewSteeringQueryUpdateChain(llm, logger),
		validationChain:  NewValidationChain(llm, logger),
	}
}

// ProcessUserFeedbackEnhanced method replacing current ProcessUserFeedbackForSteeringQuery
func (p *LLMFeedbackProcessor) ProcessUserFeedbackEnhanced(ctx context.Context, userID string) error {
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

// Supporting types and functions
type SemanticFeedbackAnalysis struct {
	LikedThemes      []SemanticTheme `json:"liked_themes"`
	DislikedThemes   []SemanticTheme `json:"disliked_themes"`
	ConfidenceScore  float64         `json:"confidence_score"`
	TrendingPatterns []string        `json:"trending_patterns"`
	Reasoning        string          `json:"reasoning"`
}

type SemanticTheme struct {
	Theme      string   `json:"theme"`
	Keywords   []string `json:"keywords"`
	Confidence float64  `json:"confidence"`
	Frequency  int      `json:"frequency"`
}

type FeedbackData struct {
	LikedItems    []FeedbackItem `json:"liked_items"`
	DislikedItems []FeedbackItem `json:"disliked_items"`
	TotalCount    int            `json:"total_count"`
}

type FeedbackItem struct {
	JobID    string `json:"job_id"`
	ItemID   string `json:"item_id"`
	ItemText string `json:"item_text"`
	Date     string `json:"date"`
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
