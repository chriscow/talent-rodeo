package feedback

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"
)

// mockLLMClient is a mock implementation of the LLMClient interface for testing.
type mockLLMClient struct {
	analyzeFeedbackFunc     func(ctx context.Context, prompt string) (*SemanticFeedbackAnalysis, error)
	updateSteeringQueryFunc func(ctx context.Context, prompt string) (*internal.SteeringQuery, error)
}

func (m *mockLLMClient) AnalyzeFeedback(ctx context.Context, prompt string) (*SemanticFeedbackAnalysis, error) {
	if m.analyzeFeedbackFunc != nil {
		return m.analyzeFeedbackFunc(ctx, prompt)
	}
	return nil, errors.New("analyzeFeedbackFunc not implemented")
}

func (m *mockLLMClient) UpdateSteeringQuery(ctx context.Context, prompt string) (*internal.SteeringQuery, error) {
	if m.updateSteeringQueryFunc != nil {
		return m.updateSteeringQueryFunc(ctx, prompt)
	}
	return nil, errors.New("updateSteeringQueryFunc not implemented")
}

// mockFeedbackStore is a mock implementation of the FeedbackStore interface for testing.
type mockFeedbackStore struct {
	getUnprocessedUserFeedbackFunc func(ctx context.Context, userID string) ([]*internal.UserFeedback, error)
	getUserSteeringQueryFunc       func(ctx context.Context, userID string) (*internal.SteeringQuery, error)
	setUserSteeringQueryFunc       func(ctx context.Context, userID string, sq internal.SteeringQuery) error
	markFeedbackAsProcessedFunc    func(ctx context.Context, feedbackIDs []string, processedAt time.Time) error
}

func (m *mockFeedbackStore) GetUnprocessedUserFeedback(ctx context.Context, userID string) ([]*internal.UserFeedback, error) {
	if m.getUnprocessedUserFeedbackFunc != nil {
		return m.getUnprocessedUserFeedbackFunc(ctx, userID)
	}
	return nil, errors.New("getUnprocessedUserFeedbackFunc not implemented")
}

func (m *mockFeedbackStore) GetUserSteeringQuery(ctx context.Context, userID string) (*internal.SteeringQuery, error) {
	if m.getUserSteeringQueryFunc != nil {
		return m.getUserSteeringQueryFunc(ctx, userID)
	}
	return nil, errors.New("getUserSteeringQueryFunc not implemented")
}

func (m *mockFeedbackStore) SetUserSteeringQuery(ctx context.Context, userID string, sq internal.SteeringQuery) error {
	if m.setUserSteeringQueryFunc != nil {
		return m.setUserSteeringQueryFunc(ctx, userID, sq)
	}
	return errors.New("setUserSteeringQueryFunc not implemented")
}

func (m *mockFeedbackStore) MarkFeedbackAsProcessed(ctx context.Context, feedbackIDs []string, processedAt time.Time) error {
	if m.markFeedbackAsProcessedFunc != nil {
		return m.markFeedbackAsProcessedFunc(ctx, feedbackIDs, processedAt)
	}
	return errors.New("markFeedbackAsProcessedFunc not implemented")
}

func TestLLMFeedbackProcessor_ProcessUserFeedbackEnhanced(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	userID := "test-user"

	t.Run("successful processing", func(t *testing.T) {
		store := &mockFeedbackStore{
			getUnprocessedUserFeedbackFunc: func(ctx context.Context, userID string) ([]*internal.UserFeedback, error) {
				return []*internal.UserFeedback{{ID: "fb1"}}, nil
			},
			getUserSteeringQueryFunc: func(ctx context.Context, userID string) (*internal.SteeringQuery, error) {
				return &internal.SteeringQuery{MoveToward: "Initial"}, nil
			},
			setUserSteeringQueryFunc: func(ctx context.Context, userID string, sq internal.SteeringQuery) error {
				return nil
			},
			markFeedbackAsProcessedFunc: func(ctx context.Context, feedbackIDs []string, processedAt time.Time) error {
				return nil
			},
		}

		llm := &mockLLMClient{
			analyzeFeedbackFunc: func(ctx context.Context, prompt string) (*SemanticFeedbackAnalysis, error) {
				return &SemanticFeedbackAnalysis{ConfidenceScore: 0.9}, nil
			},
			updateSteeringQueryFunc: func(ctx context.Context, prompt string) (*internal.SteeringQuery, error) {
				return &internal.SteeringQuery{
					Queries:         []string{"Go"},
					ExperienceLevel: internal.ExperienceLevelMidSeniorLevel,
					WorkTypes:       []internal.WorkType{internal.WorkTypeRemote},
					MoveToward:      "Updated",
					MoveAway:        "None",
					RerankQuery:     "Updated Rerank",
					ForceAway:       internal.DefaultForceAway,
				}, nil
			},
		}

		processor := newLLMProcessor(store, logger, llm)
		err := processor.ProcessUserFeedback(ctx, userID)
		if err != nil {
			t.Fatalf("ProcessUserFeedback() unexpected error = %v", err)
		}
	})

	t.Run("no feedback to process", func(t *testing.T) {
		store := &mockFeedbackStore{
			getUnprocessedUserFeedbackFunc: func(ctx context.Context, userID string) ([]*internal.UserFeedback, error) {
				return []*internal.UserFeedback{}, nil
			},
		}

		processor := newLLMProcessor(store, logger, nil) // llm client won't be called
		err := processor.ProcessUserFeedback(ctx, userID)
		if err != nil {
			t.Fatalf("ProcessUserFeedback() unexpected error = %v", err)
		}
	})

	t.Run("error getting feedback", func(t *testing.T) {
		mockErr := errors.New("db error")
		store := &mockFeedbackStore{
			getUnprocessedUserFeedbackFunc: func(ctx context.Context, userID string) ([]*internal.UserFeedback, error) {
				return nil, mockErr
			},
		}

		processor := newLLMProcessor(store, logger, nil)
		err := processor.ProcessUserFeedback(ctx, userID)
		if err == nil {
			t.Fatal("ProcessUserFeedback() expected an error, got nil")
		}
	})
}
