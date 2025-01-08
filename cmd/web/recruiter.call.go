package main

import (
	"fmt"
	"talent-rodeo/internal/vapi"
)

func (app *application) callCandidate(sessionID, systemPrompt string) error {
	app.sendStatus(sessionID, "info", "Calling candidate ...")

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
		app.sendStatus(sessionID, "error", "Failed to create call")
		return fmt.Errorf("callCandidate: failed to create call: %w", err)
	}

	app.sendStatus(sessionID, "success", "Call created!")

	return nil
}
