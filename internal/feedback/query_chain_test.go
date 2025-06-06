package feedback

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"testing"

	"talent-rodeo/internal"
)

func TestSteeringQueryUpdateChain_Run(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	t.Run("successful update", func(t *testing.T) {
		mockSQ := &internal.SteeringQuery{
			MoveToward: "I want to work with Go.",
		}

		mockClient := &mockLLMClient{
			updateSteeringQueryFunc: func(ctx context.Context, prompt string) (*internal.SteeringQuery, error) {
				return mockSQ, nil
			},
		}

		chain := NewSteeringQueryUpdateChain(mockClient, logger)
		currentSQ := internal.SteeringQuery{}
		analysis := &SemanticFeedbackAnalysis{}
		updatedSQ, err := chain.Run(ctx, currentSQ, analysis)
		if err != nil {
			t.Fatalf("Run() unexpected error = %v", err)
		}

		// The function modifies the ForceAway field after the call
		mockSQ.ForceAway = internal.DefaultForceAway

		if !reflect.DeepEqual(updatedSQ, mockSQ) {
			t.Errorf("Run() got = %v, want %v", updatedSQ, mockSQ)
		}
	})

	t.Run("LLM returns an error", func(t *testing.T) {
		mockErr := errors.New("llm failed")
		mockClient := &mockLLMClient{
			updateSteeringQueryFunc: func(ctx context.Context, prompt string) (*internal.SteeringQuery, error) {
				return nil, mockErr
			},
		}

		chain := NewSteeringQueryUpdateChain(mockClient, logger)
		_, err := chain.Run(ctx, internal.SteeringQuery{}, &SemanticFeedbackAnalysis{})
		if err == nil {
			t.Fatalf("Run() expected an error, got nil")
		}
	})
}
