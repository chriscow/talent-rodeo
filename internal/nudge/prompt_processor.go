package nudge

import (
	"context"
	"fmt"

	"github.com/chriscow/minds/tools"
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
func (p *LLMPromptProcessor) ProcessPrompt(ctx context.Context, prompt string) (string, error) {
	const maxTokens = 2048
	response, err := tools.Ask(ctx, prompt, tools.WithModel(tools.DeepSeekChat), tools.WithMaxTokens(maxTokens))
	if err != nil {
		return "", fmt.Errorf("failed to process prompt with LLM: %w", err)
	}
	return response, nil
}
