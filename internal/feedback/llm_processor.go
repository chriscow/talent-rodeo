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

type feedbackProcessor struct {
	store                FeedbackStore
	fs                   *internal.FirestoreClient
	logger               *slog.Logger
	analysisChain        *analyzer
	queryUpdateChain     *queryUpdater
	statementUpdateChain *StatementGenerator
	validationChain      *validator
	metrics              *MetricsCollector
}

func newFeedbackProcessor(store FeedbackStore, fs *internal.FirestoreClient, logger *slog.Logger, llm LLMClient) *feedbackProcessor {
	return &feedbackProcessor{
		store:                store,
		fs:                   fs,
		logger:               logger.With("component", "llm_feedback_processor"),
		analysisChain:        newAnalyzer(llm, logger),
		queryUpdateChain:     newQueryUpdater(llm, logger),
		statementUpdateChain: NewStatementGenerator(store, llm, logger),
		validationChain:      newValidator(llm, logger),
		metrics:              NewMetricsCollector(logger),
	}
}

// ProcessUserFeedback processes user feedback to update both statement and steering query.
// Statement updates are prioritized as the primary function.
func (p *feedbackProcessor) ProcessUserFeedback(ctx context.Context, userID string) error {
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

	// Create source data for versioning system
	feedbackData := prepareFeedbackData(feedbacks)
	feedbackSourceData := map[string]any{
		"semantic_analysis": analysis,
		"feedback_items":    convertFeedbackDataToMap(feedbackData),
		"confidence_score":  analysis.ConfidenceScore,
		"reasoning":         analysis.Reasoning,
		"processor_version": "v1.0",
		"liked_themes":      analysis.LikedThemes(),
		"disliked_themes":   analysis.DislikedThemes(),
	}

	// Use versioning system to create enhanced statement version
	newVersion, err := p.store.CreateStatementVersionFromFeedback(ctx, userID, feedbackSourceData, "statement-update.v001.txt")
	if err != nil {
		// Check if this is a "user not found" error (user was deleted)
		if strings.Contains(err.Error(), "user not found") || strings.Contains(err.Error(), "no existing statement") {
			p.logger.Warn("skipping statement update for deleted user or user without statement",
				"user_id", userID,
				"feedback_count", len(feedbacks),
				"error", err)
			// Mark feedback as processed even though user is deleted or has no statement
			// This prevents infinite retry loops for orphaned feedback
			finalErr = fmt.Errorf("user deleted or no statement: %w", err)
			return finalErr
		}
		finalErr = fmt.Errorf("failed to create statement version from feedback: %w", err)
		return finalErr
	}
	statementUpdated = true

	// Record the new version for logging
	processingLog.StatementAfter = newVersion.StatementText
	processingLog.StatementVersionID = newVersion.ID

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
		"statement_length", len(processingLog.StatementAfter),
		"statement_version_id", processingLog.StatementVersionID,
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

// convertFeedbackDataToMap converts FeedbackData struct to map format for source data storage
func convertFeedbackDataToMap(feedbackData FeedbackData) map[string]any {
	likedItems := make([]map[string]any, len(feedbackData.LikedItems))
	for i, item := range feedbackData.LikedItems {
		likedItems[i] = map[string]any{
			"job_id":    item.JobID,
			"item_id":   item.ItemID,
			"item_text": item.ItemText,
			"date":      item.Date,
		}
	}

	dislikedItems := make([]map[string]any, len(feedbackData.DislikedItems))
	for i, item := range feedbackData.DislikedItems {
		dislikedItems[i] = map[string]any{
			"job_id":    item.JobID,
			"item_id":   item.ItemID,
			"item_text": item.ItemText,
			"date":      item.Date,
		}
	}

	return map[string]any{
		"liked_items":    likedItems,
		"disliked_items": dislikedItems,
		"total_count":    feedbackData.TotalCount,
	}
}
