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

type analyzer struct {
	llm    LLMClient
	logger *slog.Logger
}

func newAnalyzer(llm LLMClient, logger *slog.Logger) *analyzer {
	return &analyzer{
		llm:    llm,
		logger: logger.With("chain", "feedback_analysis"),
	}
}

func (c *analyzer) Run(ctx context.Context, feedbacks []*internal.UserFeedback, currentSQ internal.SteeringQuery) (*SemanticFeedbackAnalysis, error) {
	// Load analysis prompt
	promptTemplate, err := assets.EmbeddedFiles.ReadFile("prompts/feedback-analysis-semantic.v001.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to load feedback analysis prompt: %w", err)
	}

	// Prepare feedback data for analysis
	feedbackData := prepareFeedbackData(feedbacks)
	feedbackJSON, err := json.MarshalIndent(feedbackData, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal feedback data: %w", err)
	}

	currentSQJSON, err := json.MarshalIndent(currentSQ, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal steering query: %w", err)
	}

	promptData := map[string]any{
		"SteeringQuery": string(currentSQJSON),
		"Feedback":      string(feedbackJSON),
	}

	prompt, err := tools.GeneratePrompt(promptTemplate, promptData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate analysis prompt: %w", err)
	}

	// Get LLM analysis
	analysis, err := c.llm.AnalyzeFeedback(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze feedback: %w", err)
	}

	return analysis, nil
}
