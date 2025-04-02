package internal

import (
	"talent-rodeo/internal/vapi"

	"github.com/sashabaranov/go-openai"
)

// Transcript represents a conversation transcript
type Transcript struct {
	Messages []openai.ChatCompletionMessage `json:"messages"`
}

// ConvertCallResultToTranscript converts a CallResult's messages to a Transcript
func ConvertCallResultToTranscript(callResult vapi.CallResult) *Transcript {
	messages := make([]openai.ChatCompletionMessage, 0, len(callResult.Messages))

	for _, msg := range callResult.Messages {
		// Map VAPI roles to OpenAI roles
		role := ""
		switch msg.Role {
		case "bot":
			role = openai.ChatMessageRoleAssistant
		case "system":
			role = openai.ChatMessageRoleSystem
		case "user":
			role = openai.ChatMessageRoleUser
		case "tool_calls":
			role = openai.ChatMessageRoleTool
		case "tool_call_result":
			role = openai.ChatMessageRoleFunction
		case "developer":
			role = "developer"
		default:
			continue
		}

		// Skip empty messages except for tool_calls and tool_call_result
		if msg.Message == "" &&
			role != openai.ChatMessageRoleTool &&
			role != openai.ChatMessageRoleFunction {
			continue
		}

		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: msg.Message,
		})
	}

	return &Transcript{
		Messages: messages,
	}
}
