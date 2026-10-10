package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"talent-rodeo/assets"
	"talent-rodeo/internal"

	tools "talent-rodeo/internal/llmtools"
)

type queryUpdater struct {
	llm    LLMClient
	logger *slog.Logger
}

func newQueryUpdater(llm LLMClient, logger *slog.Logger) *queryUpdater {
	return &queryUpdater{
		llm:    llm,
		logger: logger.With("chain", "steering_query_update"),
	}
}

func (c *queryUpdater) Run(ctx context.Context, currentSQ internal.SteeringQuery, analysis *SemanticFeedbackAnalysis) (*internal.SteeringQuery, error) {
	promptTemplate, err := assets.EmbeddedFiles.ReadFile("prompts/feedback-query-update.v001.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to load query update prompt: %w", err)
	}

	analysisJSON, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal analysis: %w", err)
	}

	currentSQJSON, err := json.MarshalIndent(currentSQ, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal steering query: %w", err)
	}

	promptData := map[string]any{
		"CurrentQuery": string(currentSQJSON),
		"Analysis":     string(analysisJSON),
	}

	prompt, err := tools.GeneratePrompt(promptTemplate, promptData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate update prompt: %w", err)
	}

	// Use structured response for steering query
	updatedSQ, err := c.llm.UpdateSteeringQuery(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to update steering query: %w", err)
	}

	// Preserve technical fields
	updatedSQ.ForceAway = internal.DefaultForceAway

	return updatedSQ, nil
}
