package feedback

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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
	fs                   *internal.FirestoreClient
	logger               *slog.Logger
	analysisChain        *analyzer
	queryUpdateChain     *queryUpdater
	statementUpdateChain *StatementUpdateChain
	validationChain      *validator
	metrics              *MetricsCollector
}

func newLLMProcessor(store FeedbackStore, fs *internal.FirestoreClient, logger *slog.Logger, llm LLMClient) *llmProcessor {
	return &llmProcessor{
		store:                store,
		fs:                   fs,
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
	startTime := time.Now()

	// Get unprocessed feedback
	feedbacks, err := p.store.GetUnprocessedUserFeedback(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get unprocessed feedback: %w", err)
	}

	if len(feedbacks) == 0 {
		return nil // No feedback to process
	}

	// Get user info for logging
	user, found, userErr := internal.GetUser(ctx, p.fs, userID)
	if userErr != nil || !found {
		// Continue with processing but log the issue
		p.logger.Warn("could not get user details for logging", "user_id", userID, "error", userErr)
		user = &internal.User{ID: userID} // Create minimal user for logging
	}

	// Start metrics collection
	recordMetrics := p.metrics.RecordProcessingStart(userID, len(feedbacks))
	var analysis *SemanticFeedbackAnalysis
	var statementUpdated, queryUpdated bool
	var finalErr error

	// Initialize processing log
	processingLog := &internal.FeedbackProcessingLog{
		UserID:           userID,
		UserName:         user.Name,
		UserEmail:        user.Email,
		ProcessedAt:      time.Now(),
		FeedbackCount:    len(feedbacks),
		ProcessorVersion: "v1.0",
		LLMModel:         "deepseek-chat",
	}

	// Collect feedback IDs
	feedbackIDs := make([]string, len(feedbacks))
	for i, feedback := range feedbacks {
		feedbackIDs[i] = feedback.ID
	}
	processingLog.FeedbackItems = feedbackIDs

	// Get current statement for before/after comparison
	currentStatement, err := p.store.GetUserStatement(ctx, userID)
	if err != nil {
		p.logger.Warn("failed to get current user statement", "user_id", userID, "error", err)
		processingLog.StatementBefore = ""
	} else if currentStatement != nil {
		processingLog.StatementBefore = *currentStatement
	}

	defer func() {
		// Complete processing log
		processingLog.ProcessingDuration = time.Since(startTime).Milliseconds()
		processingLog.Success = (finalErr == nil)
		processingLog.StatementUpdated = statementUpdated
		processingLog.SteeringQueryUpdated = queryUpdated

		if analysis != nil {
			processingLog.AnalysisConfidence = analysis.ConfidenceScore
			// Extract theme names
			for _, theme := range analysis.SemanticThemes.Liked {
				processingLog.LikedThemes = append(processingLog.LikedThemes, theme.Theme)
			}
			for _, theme := range analysis.SemanticThemes.Disliked {
				processingLog.DislikedThemes = append(processingLog.DislikedThemes, theme.Theme)
			}
		}

		if finalErr != nil {
			processingLog.ErrorMessage = finalErr.Error()
		}

		// Store processing log
		if logErr := internal.CreateFeedbackProcessingLog(ctx, p.fs, processingLog); logErr != nil {
			p.logger.Error("failed to create processing log", "error", logErr, "user_id", userID)
		}

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
	// (currentStatement already retrieved above for logging)

	updatedStatement, err := p.statementUpdateChain.Run(ctx, currentStatement, analysis)
	if err != nil {
		finalErr = fmt.Errorf("failed to update user statement: %w", err)
		return finalErr
	}

	// Save updated statement
	if err := p.store.SetUserStatement(ctx, userID, updatedStatement); err != nil {
		// Check if this is a "user not found" error (user was deleted)
		if strings.Contains(err.Error(), "user not found") {
			p.logger.Warn("skipping statement update for deleted user",
				"user_id", userID,
				"feedback_count", len(feedbacks),
				"error", err)
			// Mark feedback as processed even though user is deleted
			// This prevents infinite retry loops for orphaned feedback
			finalErr = fmt.Errorf("user deleted: %w", err)
			return finalErr
		}
		finalErr = fmt.Errorf("failed to save updated statement: %w", err)
		return finalErr
	}
	statementUpdated = true

	// Record the updated statement for logging
	processingLog.StatementAfter = updatedStatement

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
	// (feedbackIDs already created above for logging)

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
