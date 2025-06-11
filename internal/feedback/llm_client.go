package feedback

import (
	"context"
	"fmt"
	"talent-rodeo/internal"

	"github.com/chriscow/minds/tools"
)

// LLMClient defines the interface for interacting with a Large Language Model
// for feedback processing. This allows for mocking in tests.
type LLMClient interface {
	AnalyzeFeedback(ctx context.Context, prompt string) (*SemanticFeedbackAnalysis, error)
	UpdateSteeringQuery(ctx context.Context, prompt string) (*internal.SteeringQuery, error)
}

// mindsClient is an implementation of LLMClient that uses the minds library.
type mindsClient struct{}

// NewMindsClient creates a new mindsClient.
func NewMindsClient() LLMClient {
	return &mindsClient{}
}

// AnalyzeFeedback uses a structured LLM call to analyze user feedback.
func (c *mindsClient) AnalyzeFeedback(ctx context.Context, prompt string) (*SemanticFeedbackAnalysis, error) {
	analysis, err := tools.StructuredAsk[SemanticFeedbackAnalysis](ctx, prompt,
		tools.WithModel(tools.DeepSeekChat),
		tools.WithMaxTokens(8192),
	)
	if err != nil {
		return nil, fmt.Errorf("llm failed to analyze feedback: %w", err)
	}
	return &analysis, nil
}

// UpdateSteeringQuery uses a structured LLM call to update a steering query.
func (c *mindsClient) UpdateSteeringQuery(ctx context.Context, prompt string) (*internal.SteeringQuery, error) {
	updatedSQ, err := tools.StructuredAsk[internal.SteeringQuery](ctx, prompt,
		tools.WithModel(tools.DeepSeekChat),
		tools.WithMaxTokens(2048),
	)
	if err != nil {
		return nil, fmt.Errorf("llm failed to update steering query: %w", err)
	}
	return &updatedSQ, nil
}
