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
	SemanticThemes struct {
		Liked    []SemanticTheme `json:"Liked"`
		Disliked []SemanticTheme `json:"Disliked"`
	} `json:"SemanticThemes"`
	ConfidenceScore  float64 `json:"ConfidenceScore"`
	TrendingPatterns string  `json:"TrendingPatterns"`
	Reasoning        string  `json:"Reasoning"`
}

// LikedThemes returns the liked themes for backward compatibility
func (s *SemanticFeedbackAnalysis) LikedThemes() []SemanticTheme {
	return s.SemanticThemes.Liked
}

// DislikedThemes returns the disliked themes for backward compatibility
func (s *SemanticFeedbackAnalysis) DislikedThemes() []SemanticTheme {
	return s.SemanticThemes.Disliked
}

type SemanticTheme struct {
	Theme           string   `json:"Theme"`
	Keywords        []string `json:"Keywords"`
	ConfidenceScore float64  `json:"ConfidenceScore"`
	Frequency       int      `json:"Frequency"`
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
	store                FeedbackStore
	logger               *slog.Logger
	analysisChain        *analyzer
	queryUpdateChain     *queryUpdater
	statementUpdateChain *StatementUpdateChain
	validationChain      *validator
	metrics              *MetricsCollector
}

func newLLMProcessor(store FeedbackStore, logger *slog.Logger, llm LLMClient) *llmProcessor {
	return &llmProcessor{
		store:                store,
		logger:               logger.With("component", "llm_feedback_processor"),
		analysisChain:        newAnalyzer(llm, logger),
		queryUpdateChain:     newQueryUpdater(llm, logger),
		statementUpdateChain: newStatementUpdater(llm, logger),
		validationChain:      newValidator(llm, logger),
		metrics:              NewMetricsCollector(logger),
	}
}

// ProcessUserFeedback processes user feedback to update both statement and steering query.
// Statement updates are prioritized as the primary function.
func (p *llmProcessor) ProcessUserFeedback(ctx context.Context, userID string) error {
	// Get unprocessed feedback
	feedbacks, err := p.store.GetUnprocessedUserFeedback(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get unprocessed feedback: %w", err)
	}

	if len(feedbacks) == 0 {
		return nil // No feedback to process
	}

	// Start metrics collection
	recordMetrics := p.metrics.RecordProcessingStart(userID, len(feedbacks))
	var analysis *SemanticFeedbackAnalysis
	var statementUpdated, queryUpdated bool
	var finalErr error

	defer func() {
		recordMetrics(analysis, statementUpdated, queryUpdated, finalErr)
	}()

	// Get current steering query for analysis context
	currentSQ, err := p.store.GetUserSteeringQuery(ctx, userID)
	if err != nil {
		p.logger.Warn("no steering query found for user, using empty one", "user_id", userID)
		currentSQ = &internal.SteeringQuery{}
	}

	// Phase 1: Semantic analysis
	analysis, err = p.analysisChain.Run(ctx, feedbacks, *currentSQ)
	if err != nil {
		finalErr = fmt.Errorf("failed to analyze feedback semantics: %w", err)
		return finalErr
	}

	// Phase 2a: Update user statement (primary function)
	currentStatement, err := p.store.GetUserStatement(ctx, userID)
	if err != nil {
		p.logger.Warn("failed to get current statement", "user_id", userID, "error", err)
		currentStatement = nil
	}

	updatedStatement, err := p.statementUpdateChain.Run(ctx, currentStatement, analysis)
	if err != nil {
		finalErr = fmt.Errorf("failed to update user statement: %w", err)
		return finalErr
	}

	// Save updated statement
	if err := p.store.SetUserStatement(ctx, userID, updatedStatement); err != nil {
		finalErr = fmt.Errorf("failed to save updated statement: %w", err)
		return finalErr
	}
	statementUpdated = true

	// Phase 2b: Update steering query (secondary function for backwards compatibility)
	/*
		updatedSQ, err := p.queryUpdateChain.Run(ctx, *currentSQ, analysis)
		if err != nil {
			p.logger.Warn("failed to update steering query", "user_id", userID, "error", err)
		} else {
			// Phase 3: Validation (only for steering query)
			if err := p.validationChain.Run(ctx, updatedSQ); err != nil {
				p.logger.Warn("steering query validation failed", "user_id", userID, "error", err)
			} else {
				// Save updated steering query
				if err := p.store.SetUserSteeringQuery(ctx, userID, *updatedSQ); err != nil {
					p.logger.Warn("failed to save updated steering query", "user_id", userID, "error", err)
				} else {
					queryUpdated = true
				}
			}
		}
	*/

	// Mark feedback as processed
	feedbackIDs := make([]string, len(feedbacks))
	for i, feedback := range feedbacks {
		feedbackIDs[i] = feedback.ID
	}

	if err := p.store.MarkFeedbackAsProcessed(ctx, feedbackIDs, time.Now()); err != nil {
		finalErr = fmt.Errorf("failed to mark feedback as processed: %w", err)
		return finalErr
	}

	p.logger.Info("feedback processing completed",
		"user_id", userID,
		"feedback_count", len(feedbacks),
		"confidence_score", analysis.ConfidenceScore,
		"statement_updated", statementUpdated,
		"statement_length", len(updatedStatement),
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
