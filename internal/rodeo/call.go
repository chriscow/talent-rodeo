package rodeo

import (
	"context"
	"fmt"
	"talent-rodeo/internal/vapi"

	"github.com/chriscow/minds"
)

func callCandidate(ctx context.Context, sessionID, systemPrompt string, status StatusSender) error {
	status.SendStatus(sessionID, "info", "Calling candidate ...")

	assistantID := "assistant-id"
	phoneID := "phone-id"

	if err := vapi.CreateCall(vapi.CallConfig{
		AssistantID:   assistantID,
		PhoneNumberID: phoneID,
		AssistantOverrides: vapi.Assistant{
			Model: vapi.ModelConfig{
				Model:     "gpt-4o",
				MaxTokens: 300,
				Messages: []vapi.Message{{
					Role:    "system",
					Content: systemPrompt,
				}},
			},
		},
	}); err != nil {
		status.SendStatus(sessionID, "error", "Failed to create call")
		return fmt.Errorf("callCandidate: failed to create call: %w", err)
	}

	status.SendStatus(sessionID, "success", "Call created!")

	return nil
}

type EndCallArgs struct {
	CallID      string `json:"call_id"`
	Disposition string `json:"disposition" enum:"completed,missed,failed,other"`
	Notes       string `json:"notes,omitempty"`
}

type endCallTool struct {
	cancel context.CancelFunc
}

func (t *endCallTool) Type() string {
	return "end_call"
}

func (t *endCallTool) Name() string {
	return "end_call"
}

func (t *endCallTool) Description() string {
	return "End the current phone call and record its disposition"
}

func (t *endCallTool) Parameters() minds.Definition {
	definition, _ := minds.GenerateSchema(EndCallArgs{})
	return *definition
}

func (t *endCallTool) Call(ctx context.Context, input []byte) ([]byte, error) {
	t.cancel()
	return nil, nil
}

func NewEndCallTool(cancel context.CancelFunc) (minds.Tool, error) {
	return &endCallTool{cancel: cancel}, nil
}
