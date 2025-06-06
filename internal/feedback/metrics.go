package feedback

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ProcessingMetrics tracks feedback processing statistics
type ProcessingMetrics struct {
	UserID               string
	FeedbackCount        int
	AnalysisConfidence   float64
	StatementUpdated     bool
	SteeringQueryUpdated bool
	ProcessingDuration   time.Duration
	ErrorMessage         string
	Timestamp            time.Time
}

// MetricsCollector handles collection and logging of processing metrics
type MetricsCollector struct {
	logger *slog.Logger
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(logger *slog.Logger) *MetricsCollector {
	return &MetricsCollector{
		logger: logger.With("component", "feedback_metrics"),
	}
}

// RecordProcessingStart returns a function to complete metrics recording
func (mc *MetricsCollector) RecordProcessingStart(userID string, feedbackCount int) func(analysis *SemanticFeedbackAnalysis, statementUpdated, queryUpdated bool, err error) {
	startTime := time.Now()

	return func(analysis *SemanticFeedbackAnalysis, statementUpdated, queryUpdated bool, err error) {
		metrics := ProcessingMetrics{
			UserID:               userID,
			FeedbackCount:        feedbackCount,
			StatementUpdated:     statementUpdated,
			SteeringQueryUpdated: queryUpdated,
			ProcessingDuration:   time.Since(startTime),
			Timestamp:            time.Now(),
		}

		if analysis != nil {
			metrics.AnalysisConfidence = analysis.ConfidenceScore
		}

		if err != nil {
			metrics.ErrorMessage = err.Error()
		}

		mc.logMetrics(metrics)
	}
}

// logMetrics logs the processing metrics in a structured format
func (mc *MetricsCollector) logMetrics(metrics ProcessingMetrics) {
	logAttrs := []any{
		"user_id", metrics.UserID,
		"feedback_count", metrics.FeedbackCount,
		"analysis_confidence", metrics.AnalysisConfidence,
		"statement_updated", metrics.StatementUpdated,
		"steering_query_updated", metrics.SteeringQueryUpdated,
		"processing_duration_ms", metrics.ProcessingDuration.Milliseconds(),
		"timestamp", metrics.Timestamp.Format(time.RFC3339),
	}

	if metrics.ErrorMessage != "" {
		logAttrs = append(logAttrs, "error", metrics.ErrorMessage)
		mc.logger.Error("feedback processing completed with error", logAttrs...)
	} else {
		mc.logger.Info("feedback processing completed successfully", logAttrs...)
	}
}

// LogDailyProcessingSummary logs a summary of daily processing
func (mc *MetricsCollector) LogDailyProcessingSummary(ctx context.Context, totalUsers, successfulUsers, failedUsers int, totalFeedback int, avgProcessingTime time.Duration) {
	mc.logger.Info("daily feedback processing summary",
		"total_users", totalUsers,
		"successful_users", successfulUsers,
		"failed_users", failedUsers,
		"total_feedback_processed", totalFeedback,
		"avg_processing_time_ms", avgProcessingTime.Milliseconds(),
		"success_rate", fmt.Sprintf("%.2f%%", float64(successfulUsers)/float64(totalUsers)*100),
		"date", time.Now().Format("2006-01-02"),
	)
}
