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

func TestFeedbackAnalysisChain_Run(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	t.Run("successful analysis", func(t *testing.T) {
		mockAnalysis := &SemanticFeedbackAnalysis{
			LikedThemes: []SemanticTheme{
				{Theme: "Go programming", Keywords: []string{"golang", "go"}},
			},
			ConfidenceScore: 0.9,
			Reasoning:       "User likes Go.",
		}

		mockClient := &mockLLMClient{
			analyzeFeedbackFunc: func(ctx context.Context, prompt string) (*SemanticFeedbackAnalysis, error) {
				return mockAnalysis, nil
			},
		}

		chain := NewFeedbackAnalysisChain(mockClient, logger)
		analysis, err := chain.Run(ctx, []*internal.UserFeedback{}, internal.SteeringQuery{})
		if err != nil {
			t.Fatalf("Run() unexpected error = %v", err)
		}

		if !reflect.DeepEqual(analysis, mockAnalysis) {
			t.Errorf("Run() got = %v, want %v", analysis, mockAnalysis)
		}
	})

	t.Run("LLM returns an error", func(t *testing.T) {
		mockErr := errors.New("llm failed")
		mockClient := &mockLLMClient{
			analyzeFeedbackFunc: func(ctx context.Context, prompt string) (*SemanticFeedbackAnalysis, error) {
				return nil, mockErr
			},
		}

		chain := NewFeedbackAnalysisChain(mockClient, logger)
		_, err := chain.Run(ctx, []*internal.UserFeedback{}, internal.SteeringQuery{})
		if err == nil {
			t.Fatalf("Run() expected an error, got nil")
		}
	})
}
