package internal

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type FeedbackType string

const (
	userFeedbackCollection              = "user_feedback"
	LikeFeedbackType       FeedbackType = "like"
	DislikeFeedbackType    FeedbackType = "dislike"
)

// UserFeedback represents a user's feedback on a specific job item
type UserFeedback struct {
	ID           string
	UserID       string
	JobID        string
	ItemID       string // e.g., "resp-1", "skill-2", "req-0"
	ItemText     string
	Liked        bool
	FeedbackType FeedbackType // "like" or "dislike"
	CreatedAt    time.Time
	ProcessedAt  *time.Time // When it was used to update SteeringQuery
}

// UpsertUserFeedback creates or updates user feedback document in Firestore
func UpsertUserFeedback(ctx context.Context, fs *FirestoreClient, feedback *UserFeedback) error {
	if feedback.UserID == "" {
		return fmt.Errorf("user ID is required")
	}
	if feedback.JobID == "" {
		return fmt.Errorf("job ID is required")
	}
	if feedback.ItemID == "" {
		return fmt.Errorf("item ID is required")
	}
	if feedback.ItemText == "" {
		return fmt.Errorf("item text is required")
	}
	if feedback.FeedbackType != LikeFeedbackType && feedback.FeedbackType != DislikeFeedbackType {
		return fmt.Errorf("feedback type must be 'like' or 'dislike'")
	}

	feedback.ID = feedback.UserID + "-" + feedback.JobID + "-" + feedback.ItemID

	// Check if feedback already exists to preserve CreatedAt
	existingFeedback, exists, err := GetUserFeedback(ctx, fs, feedback.ID)
	if err != nil {
		return fmt.Errorf("failed to check existing feedback: %w", err)
	}

	if exists {
		// Preserve original CreatedAt timestamp
		feedback.CreatedAt = existingFeedback.CreatedAt
	} else {
		// Set CreatedAt for new feedback
		feedback.CreatedAt = time.Now()
	}

	feedback.Liked = feedback.FeedbackType == LikeFeedbackType

	_, err = fs.Collection(userFeedbackCollection).Doc(feedback.ID).Set(ctx, feedback)
	if err != nil {
		return fmt.Errorf("failed to upsert user feedback: %w", err)
	}

	return nil
}

// GetUserFeedback retrieves a specific feedback by ID
func GetUserFeedback(ctx context.Context, fs *FirestoreClient, id string) (*UserFeedback, bool, error) {
	doc, err := fs.Collection(userFeedbackCollection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get user feedback: %w", err)
	}

	var feedback UserFeedback
	if err := doc.DataTo(&feedback); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal user feedback data: %w", err)
	}

	return &feedback, true, nil
}

// GetUnprocessedUserFeedback retrieves all unprocessed feedback for a specific user
func GetUnprocessedUserFeedback(ctx context.Context, fs *FirestoreClient, userID string) ([]*UserFeedback, error) {
	iter := fs.Collection(userFeedbackCollection).
		Where("UserID", "==", userID).
		Where("ProcessedAt", "==", nil).
		OrderBy("CreatedAt", firestore.Asc).
		Documents(ctx)

	var feedbacks []*UserFeedback
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate user feedback: %w", err)
		}

		var feedback UserFeedback
		if err := doc.DataTo(&feedback); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user feedback data: %w", err)
		}
		feedbacks = append(feedbacks, &feedback)
	}

	return feedbacks, nil
}

// GetUserFeedbackByDateRange retrieves feedback for a user within a date range
func GetUserFeedbackByDateRange(ctx context.Context, fs *FirestoreClient, userID string, startDate, endDate time.Time) ([]*UserFeedback, error) {
	iter := fs.Collection(userFeedbackCollection).
		Where("UserID", "==", userID).
		Where("CreatedAt", ">=", startDate).
		Where("CreatedAt", "<=", endDate).
		OrderBy("CreatedAt", firestore.Asc).
		Documents(ctx)

	var feedbacks []*UserFeedback
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate user feedback: %w", err)
		}

		var feedback UserFeedback
		if err := doc.DataTo(&feedback); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user feedback data: %w", err)
		}
		feedbacks = append(feedbacks, &feedback)
	}

	return feedbacks, nil
}

func GetUserFeedbackForJobIDs(ctx context.Context, fs *FirestoreClient, userID string, jobIDs []string) ([]*UserFeedback, error) {
	query := fs.Collection(userFeedbackCollection).
		Where("UserID", "==", userID).
		Where("JobID", "in", jobIDs).
		OrderBy("CreatedAt", firestore.Asc)

	iter := query.Documents(ctx)
	var feedbacks []*UserFeedback

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate user feedback: %w", err)
		}

		var feedback UserFeedback
		if err := doc.DataTo(&feedback); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user feedback data: %w", err)
		}
		feedbacks = append(feedbacks, &feedback)
	}

	return feedbacks, nil
}

// MarkFeedbackAsProcessed marks multiple feedback items as processed
func MarkFeedbackAsProcessed(ctx context.Context, fs *FirestoreClient, feedbackIDs []string, processedAt time.Time) error {
	bulkWriter := fs.BulkWriter(ctx)

	for _, id := range feedbackIDs {
		docRef := fs.Collection(userFeedbackCollection).Doc(id)
		_, err := bulkWriter.Update(docRef, []firestore.Update{
			{Path: "ProcessedAt", Value: processedAt},
		})
		if err != nil {
			return fmt.Errorf("failed to update feedback %s: %w", id, err)
		}
	}

	bulkWriter.End()
	return nil
}

// GetAllUnprocessedFeedback retrieves all unprocessed feedback across all users
// This is useful for batch processing operations
func GetAllUnprocessedFeedback(ctx context.Context, fs *FirestoreClient, limit int) ([]*UserFeedback, error) {
	query := fs.Collection(userFeedbackCollection).
		Where("ProcessedAt", "==", nil).
		OrderBy("CreatedAt", firestore.Asc)

	if limit > 0 {
		query = query.Limit(limit)
	}

	iter := query.Documents(ctx)
	var feedbacks []*UserFeedback

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate unprocessed feedback: %w", err)
		}

		var feedback UserFeedback
		if err := doc.DataTo(&feedback); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user feedback data: %w", err)
		}
		feedbacks = append(feedbacks, &feedback)
	}

	return feedbacks, nil
}

// GetUserFeedbackStats returns statistics about a user's feedback
type UserFeedbackStats struct {
	TotalFeedback    int
	LikedItems       int
	DislikedItems    int
	UnprocessedCount int
	LastFeedbackAt   *time.Time
	MostRecentJobID  string
}

func GetUserFeedbackStats(ctx context.Context, fs *FirestoreClient, userID string) (*UserFeedbackStats, error) {
	iter := fs.Collection(userFeedbackCollection).
		Where("UserID", "==", userID).
		OrderBy("CreatedAt", firestore.Desc).
		Documents(ctx)

	stats := &UserFeedbackStats{}
	var mostRecentTime time.Time

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate user feedback for stats: %w", err)
		}

		var feedback UserFeedback
		if err := doc.DataTo(&feedback); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user feedback data: %w", err)
		}

		stats.TotalFeedback++

		if feedback.Liked {
			stats.LikedItems++
		} else {
			stats.DislikedItems++
		}

		if feedback.ProcessedAt == nil {
			stats.UnprocessedCount++
		}

		// Track most recent feedback
		if stats.LastFeedbackAt == nil || feedback.CreatedAt.After(mostRecentTime) {
			stats.LastFeedbackAt = &feedback.CreatedAt
			stats.MostRecentJobID = feedback.JobID
			mostRecentTime = feedback.CreatedAt
		}
	}

	return stats, nil
}

// DeleteUserFeedback deletes all feedback for a specific user
// This should be called when a user account is deleted
func DeleteUserFeedback(ctx context.Context, fs *FirestoreClient, userID string) error {
	// Get all feedback documents for the user
	iter := fs.Collection(userFeedbackCollection).
		Where("UserID", "==", userID).
		Documents(ctx)

	// Use batch delete for efficiency
	batch := fs.Batch()
	count := 0

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate user feedback for deletion: %w", err)
		}

		batch.Delete(doc.Ref)
		count++

		// Firestore batch limit is 500 operations
		if count >= 450 {
			if _, err := batch.Commit(ctx); err != nil {
				return fmt.Errorf("failed to delete user feedback batch: %w", err)
			}
			batch = fs.Batch()
			count = 0
		}
	}

	// Commit remaining deletions
	if count > 0 {
		if _, err := batch.Commit(ctx); err != nil {
			return fmt.Errorf("failed to delete remaining user feedback: %w", err)
		}
	}

	return nil
}

// DeleteUserFeedbackByID deletes a specific feedback by ID
func DeleteUserFeedbackByID(ctx context.Context, fs *FirestoreClient, id string) error {
	_, err := fs.Collection(userFeedbackCollection).Doc(id).Delete(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil // Already deleted, no error
		}
		return fmt.Errorf("failed to delete user feedback: %w", err)
	}
	return nil
}
