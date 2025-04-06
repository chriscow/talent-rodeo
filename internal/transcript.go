package internal

import (
	"context"
	"fmt"
	"strings"
	"talent-rodeo/internal/utils"
	"talent-rodeo/internal/vapi"

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

// ConvertCallResultToTranscript converts a CallResult's messages to a Transcript
func ConvertCallResultToTranscript(callResult vapi.Call) *Transcript {
	messages := make([]minds.Message, 0, len(callResult.Messages))

	for _, msg := range callResult.Messages {
		// Map VAPI roles to OpenAI roles
		role := ""
		switch msg.Role {
		case "bot":
			role = string(minds.RoleAssistant)
		case "system":
			role = string(minds.RoleSystem)
		case "user":
			role = string(minds.RoleUser)
		case "tool_calls":
			role = string(minds.RoleTool)
		case "tool_call_result":
			role = string(minds.RoleFunction)
		case "developer":
			role = "developer"
		default:
			continue
		}

		// Skip empty messages except for tool_calls and tool_call_result
		if msg.Message == "" &&
			role != string(minds.RoleTool) &&
			role != string(minds.RoleFunction) {
			continue
		}

		messages = append(messages, minds.Message{
			Role:    minds.Role(role),
			Content: msg.Message,
		})
	}

	return &Transcript{
		Messages: messages,
	}
}
