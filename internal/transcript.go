package internal

import (
	"context"
	"fmt"
	"strings"
	"talent-rodeo/assets"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/tools"
	"github.com/chriscow/vapi-go"
)

// Transcript represents a conversation transcript
type Transcript struct {
	Messages []minds.Message `json:"messages"`
}

func NewTranscriptFromCallResult(callResult vapi.Call) *Transcript {
	if callResult.Messages == nil {
		return &Transcript{
			Messages: []minds.Message{},
		}
	}

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

func (t *Transcript) Summarize(ctx context.Context) (string, error) {
	systemPrompt, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-analysis.001.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create template: %w", err)
	}

	msgs := minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: string(systemPrompt),
		},
	}

	return tools.Ask(ctx, t.String(), tools.WithMessages(msgs), tools.WithModel(tools.DeepSeekChat))
}

func (t *Transcript) String() string {
	var sb strings.Builder
	for _, msg := range t.Messages {
		sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}
	return sb.String()
}
