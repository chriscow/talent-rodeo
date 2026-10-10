package feedback

import (
	"context"
	"fmt"
	"talent-rodeo/internal"

	tools "talent-rodeo/internal/llmtools"
)

// LLMClient defines the interface for interacting with a Large Language Model
// for feedback processing. This allows for mocking in tests.
type LLMClient interface {
	AnalyzeFeedback(ctx context.Context, prompt string) (*SemanticFeedbackAnalysis, error)
	UpdateSteeringQuery(ctx context.Context, prompt string) (*internal.SteeringQuery, error)
	UpdateStatement(ctx context.Context, prompt string) (string, error)
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
		tools.WithModel(tools.DeepSeekFlash),
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
		tools.WithModel(tools.DeepSeekFlash),
		tools.WithMaxTokens(2048),
	)
	if err != nil {
		return nil, fmt.Errorf("llm failed to update steering query: %w", err)
	}
	return &updatedSQ, nil
}

// UpdateStatement uses an LLM call to update a user statement based on feedback.
func (c *mindsClient) UpdateStatement(ctx context.Context, prompt string) (string, error) {
	response, err := tools.Ask(ctx, prompt,
		tools.WithModel(tools.DeepSeekFlash),
		tools.WithMaxTokens(4096),
	)
	if err != nil {
		return "", fmt.Errorf("llm failed to update statement: %w", err)
	}
	return response, nil
}
