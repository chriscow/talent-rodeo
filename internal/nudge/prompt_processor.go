package nudge

import (
	"context"
	"fmt"
	"log/slog"
	"talent-rodeo/internal"

	"github.com/chriscow/minds/tools"
	"github.com/sashabaranov/go-openai"
)

// LLMPromptProcessor implements PromptProcessor using the LLM tools
type LLMPromptProcessor struct{}

// NewLLMPromptProcessor creates a new prompt processor using LLM
func NewLLMPromptProcessor() *LLMPromptProcessor {
	return &LLMPromptProcessor{}
}

// GeneratePrompt generates a prompt from template and data
func (p *LLMPromptProcessor) GeneratePrompt(template []byte, data *NudgeEmailData) (string, error) {
	prompt, err := tools.GeneratePrompt(template, data)
	if err != nil {
		return "", fmt.Errorf("failed to generate prompt: %w", err)
	}
	return prompt, nil
}

// ProcessPrompt sends the prompt to LLM and returns the response
func (p *LLMPromptProcessor) ProcessPrompt(ctx context.Context, fs *internal.FirestoreClient, logger *slog.Logger, userID string, prompt string) (string, error) {
	const maxTokens = 2048
	resp := openai.ChatCompletionResponse{}
	response, err := tools.Ask(ctx, prompt, tools.WithModel(tools.DeepSeekChat), tools.WithMaxTokens(maxTokens), tools.WantCompletionResponse(&resp))
	if err != nil {
		return "", fmt.Errorf("failed to process prompt with LLM: %w", err)
	}

	// Record LLM cost for nudge email generation
	if err := internal.RecordLLMCost(ctx, fs, &userID, string(tools.DeepSeekChat), resp, "nudge_email_generation", nil); err != nil {
		logger.Error("failed to record LLM cost for nudge email generation", "error", err)
	}

	return response, nil
}
