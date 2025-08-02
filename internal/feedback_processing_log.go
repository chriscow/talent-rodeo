package internal

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

const feedbackProcessingLogCollection = "feedback_processing_logs"

// FeedbackProcessingLog stores detailed logs of each feedback processing run
type FeedbackProcessingLog struct {
	ID            string
	UserID        string
	UserName      string
	UserEmail     string
	ProcessedAt   time.Time
	FeedbackCount int
	FeedbackItems []string

	// Analysis Results
	AnalysisConfidence float64
	LikedThemes        []string
	DislikedThemes     []string

	// What Changed
	StatementBefore      string
	StatementAfter       string
	StatementVersionID   string // Version ID from statement versioning system
	StatementUpdated     bool
	SteeringQueryUpdated bool

	// Processing Details
	ProcessingDuration int64
	Success            bool
	ErrorMessage       string

	// Metadata
	ProcessorVersion string
	LLMModel         string
}

// CreateFeedbackProcessingLog stores a processing log in Firestore
func CreateFeedbackProcessingLog(ctx context.Context, fs *FirestoreClient, log *FeedbackProcessingLog) error {
	if fs == nil {
		return fmt.Errorf("firestore client is nil")
	}

	// Generate ID if not set
	if log.ID == "" {
		log.ID = fmt.Sprintf("%s-%d", log.UserID, log.ProcessedAt.Unix())
	}

	_, err := fs.Collection(feedbackProcessingLogCollection).Doc(log.ID).Set(ctx, log)
	if err != nil {
		return fmt.Errorf("failed to create processing log: %w", err)
	}

	return nil
}

// GetRecentProcessingLogs retrieves recent processing logs
func GetRecentProcessingLogs(ctx context.Context, fs *FirestoreClient, limit int) ([]*FeedbackProcessingLog, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}

	query := fs.Collection(feedbackProcessingLogCollection).
		OrderBy("processed_at", firestore.Desc)

	if limit > 0 {
		query = query.Limit(limit)
	}

	iter := query.Documents(ctx)
	var logs []*FeedbackProcessingLog

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate processing logs: %w", err)
		}

		var log FeedbackProcessingLog
		if err := doc.DataTo(&log); err != nil {
			return nil, fmt.Errorf("failed to unmarshal processing log: %w", err)
		}
		log.ID = doc.Ref.ID

		logs = append(logs, &log)
	}

	return logs, nil
}

// GetProcessingLogsByUser retrieves processing logs for a specific user
func GetProcessingLogsByUser(ctx context.Context, fs *FirestoreClient, userID string, limit int) ([]*FeedbackProcessingLog, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}

	query := fs.Collection(feedbackProcessingLogCollection).
		Where("user_id", "==", userID).
		OrderBy("processed_at", firestore.Desc)

	if limit > 0 {
		query = query.Limit(limit)
	}

	iter := query.Documents(ctx)
	var logs []*FeedbackProcessingLog

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate user processing logs: %w", err)
		}

		var log FeedbackProcessingLog
		if err := doc.DataTo(&log); err != nil {
			return nil, fmt.Errorf("failed to unmarshal processing log: %w", err)
		}
		log.ID = doc.Ref.ID

		logs = append(logs, &log)
	}

	return logs, nil
}

// GetProcessingLogsByDateRange retrieves processing logs within a date range
func GetProcessingLogsByDateRange(ctx context.Context, fs *FirestoreClient, startDate, endDate time.Time, limit int) ([]*FeedbackProcessingLog, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}

	query := fs.Collection(feedbackProcessingLogCollection).
		Where("processed_at", ">=", startDate).
		Where("processed_at", "<=", endDate).
		OrderBy("processed_at", firestore.Desc)

	if limit > 0 {
		query = query.Limit(limit)
	}

	iter := query.Documents(ctx)
	var logs []*FeedbackProcessingLog

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate processing logs: %w", err)
		}

		var log FeedbackProcessingLog
		if err := doc.DataTo(&log); err != nil {
			return nil, fmt.Errorf("failed to unmarshal processing log: %w", err)
		}
		log.ID = doc.Ref.ID

		logs = append(logs, &log)
	}

	return logs, nil
}

// GetProcessingStats calculates processing statistics
type ProcessingStats struct {
	TotalProcessingRuns int
	SuccessfulRuns      int
	FailedRuns          int
	TotalFeedbackItems  int
	AvgProcessingTime   time.Duration
	AvgConfidenceScore  float64
	StatementsUpdated   int
	QueriesUpdated      int
	LastProcessingTime  time.Time
}

func GetProcessingStats(ctx context.Context, fs *FirestoreClient, startDate, endDate time.Time) (*ProcessingStats, error) {
	logs, err := GetProcessingLogsByDateRange(ctx, fs, startDate, endDate, 0) // No limit
	if err != nil {
		return nil, fmt.Errorf("failed to get processing logs: %w", err)
	}

	stats := &ProcessingStats{}
	var totalDuration time.Duration
	var totalConfidence float64
	var confidenceCount int

	for _, log := range logs {
		stats.TotalProcessingRuns++
		stats.TotalFeedbackItems += log.FeedbackCount

		if log.Success {
			stats.SuccessfulRuns++
		} else {
			stats.FailedRuns++
		}

		if log.StatementUpdated {
			stats.StatementsUpdated++
		}

		if log.SteeringQueryUpdated {
			stats.QueriesUpdated++
		}

		totalDuration += time.Duration(log.ProcessingDuration) * time.Millisecond

		if log.AnalysisConfidence > 0 {
			totalConfidence += log.AnalysisConfidence
			confidenceCount++
		}

		if log.ProcessedAt.After(stats.LastProcessingTime) {
			stats.LastProcessingTime = log.ProcessedAt
		}
	}

	if stats.TotalProcessingRuns > 0 {
		stats.AvgProcessingTime = totalDuration / time.Duration(stats.TotalProcessingRuns)
	}

	if confidenceCount > 0 {
		stats.AvgConfidenceScore = totalConfidence / float64(confidenceCount)
	}

	return stats, nil
}
