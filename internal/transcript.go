package internal

import (
	"context"
	"fmt"
	"strings"
	"talent-rodeo/internal/utils"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/openai"
)

// Transcript represents a conversation transcript
type Transcript struct {
	Messages []minds.Message `json:"messages"`
}

func (t *Transcript) Summarize(ctx context.Context) (string, error) {
	systemPrompt, err := utils.CreateTemplate("prompts/transcript-analysis.001.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create template: %w", err)
	}

	messages := []minds.Message{
		{
			Role:    "system",
			Content: systemPrompt.RawContent,
		},
		{
			Role:    "user",
			Content: t.String(),
		},
	}

	llm, err := openai.NewProvider(openai.WithModel("gpt-4o"))
	if err != nil {
		return "", fmt.Errorf("failed to create openai provider: %w", err)
	}

	req := minds.NewRequest(messages)

	response, err := llm.GenerateContent(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to chat: %w", err)
	}

	return response.String(), nil
}

func (t *Transcript) String() string {
	var sb strings.Builder
	for _, msg := range t.Messages {
		sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}
	return sb.String()
}
