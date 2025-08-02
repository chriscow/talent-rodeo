package feedback

import (
	"context"
	"talent-rodeo/internal"
	"time"
)

// FeedbackStore defines the interface for database operations related to feedback processing.
// This allows for mocking in tests.
type FeedbackStore interface {
	GetUnprocessedUserFeedback(ctx context.Context, userID string) ([]*internal.UserFeedback, error)
	GetUserSteeringQuery(ctx context.Context, userID string) (*internal.SteeringQuery, error)
	SetUserSteeringQuery(ctx context.Context, userID string, sq internal.SteeringQuery) error
	GetUserStatement(ctx context.Context, userID string) (*string, error)
	CreateStatementVersionFromFeedback(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error)
	MarkFeedbackAsProcessed(ctx context.Context, feedbackIDs []string, processedAt time.Time) error
}

// firestoreStore is an implementation of FeedbackStore that uses Firestore.
type firestoreStore struct {
	fs *internal.FirestoreClient
}

// newFeedbackStore creates a new firestoreStore (internal use).
func newFeedbackStore(fs *internal.FirestoreClient) FeedbackStore {
	return &firestoreStore{fs: fs}
}

// NewFeedbackStore creates a new firestoreStore (exported for external use).
func NewFeedbackStore(fs *internal.FirestoreClient) FeedbackStore {
	return &firestoreStore{fs: fs}
}

func (s *firestoreStore) GetUnprocessedUserFeedback(ctx context.Context, userID string) ([]*internal.UserFeedback, error) {
	return internal.GetUnprocessedUserFeedback(ctx, s.fs, userID)
}

func (s *firestoreStore) GetUserSteeringQuery(ctx context.Context, userID string) (*internal.SteeringQuery, error) {
	return internal.GetUserSteeringQuery(ctx, s.fs, userID)
}

func (s *firestoreStore) SetUserSteeringQuery(ctx context.Context, userID string, sq internal.SteeringQuery) error {
	return internal.SetUserSteeringQuery(ctx, s.fs, userID, sq)
}

func (s *firestoreStore) GetUserStatement(ctx context.Context, userID string) (*string, error) {
	user, found, err := internal.GetUser(ctx, s.fs, userID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return user.Statement, nil
}

func (s *firestoreStore) CreateStatementVersionFromFeedback(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error) {
	return internal.EnhanceStatementWithAppFeedback(ctx, s.fs, userID, feedbackData, promptFile)
}

func (s *firestoreStore) MarkFeedbackAsProcessed(ctx context.Context, feedbackIDs []string, processedAt time.Time) error {
	return internal.MarkFeedbackAsProcessed(ctx, s.fs, feedbackIDs, processedAt)
}
