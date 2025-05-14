package internal

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chriscow/minds"
	"github.com/chriscow/vapi-go"
	"github.com/sashabaranov/go-openai"
)

func TestConvertCallResultToTranscript(t *testing.T) {
	// Read the test data file
	data, err := os.ReadFile("../testdata/vapi/create-call-response.json")
	if err != nil {
		t.Fatalf("Failed to read test data: %v", err)
	}

	// Parse the JSON into a struct that has the messages field
	var callResult vapi.Call
	if err := json.Unmarshal(data, &callResult); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}

	// Convert the messages to a transcript
	transcript := NewTranscriptFromCallResult(callResult)

	// Verify the conversion
	if len(transcript.Messages) == 0 {
		t.Error("Expected non-empty transcript messages")
	}

	// Define valid OpenAI roles
	validRoles := map[minds.Role]bool{
		minds.RoleSystem:    true,
		minds.RoleUser:      true,
		minds.RoleAssistant: true,
		minds.RoleFunction:  true,
		minds.RoleTool:      true,
		"developer":         true,
	}

	// Check role mapping and content
	for i, msg := range transcript.Messages {
		// Verify roles are correctly mapped to valid OpenAI roles
		if !validRoles[msg.Role] {
			t.Errorf("Message %d has invalid role: %s", i, msg.Role)
		}

		// Verify content is not empty (except for tool/function calls which might be empty)
		if msg.Content == "" && msg.Role != openai.ChatMessageRoleTool && msg.Role != openai.ChatMessageRoleFunction {
			t.Errorf("Message %d has empty content with role %s", i, msg.Role)
		}
	}

	// Test specific role mappings from the call response
	expectedMappings := []struct {
		vapiRole   string
		openAIRole string
		hasContent bool
		sampleText string
	}{
		{
			vapiRole:   "system",
			openAIRole: openai.ChatMessageRoleSystem,
			hasContent: true,
			sampleText: "You are Shirley, an AI job search specialist at Talent Rodeo",
		},
		{
			vapiRole:   "bot",
			openAIRole: openai.ChatMessageRoleAssistant,
			hasContent: true,
			sampleText: "Hi there. I'm Shirley from Talent Rodeo",
		},
		{
			vapiRole:   "user",
			openAIRole: openai.ChatMessageRoleUser,
			hasContent: true,
			sampleText: "Sure",
		},
		{
			vapiRole:   "tool_calls",
			openAIRole: openai.ChatMessageRoleTool,
			hasContent: false,
		},
		{
			vapiRole:   "tool_call_result",
			openAIRole: openai.ChatMessageRoleFunction,
			hasContent: false,
		},
	}

	// Verify each expected mapping exists in the transcript
	for _, mapping := range expectedMappings {
		found := false
		for _, msg := range transcript.Messages {
			if msg.Role == minds.Role(mapping.openAIRole) {
				if mapping.hasContent && mapping.sampleText != "" {
					if mapping.sampleText != "" && msg.Content != "" && strings.Contains(msg.Content, mapping.sampleText) {
						found = true
						break
					}
				} else {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("Failed to find message with role %s mapped from VAPI role %s",
				mapping.openAIRole, mapping.vapiRole)
		}
	}
}
