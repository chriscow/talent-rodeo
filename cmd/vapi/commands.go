package main

import (
	"context"
	"fmt"
	"talent-rodeo/internal/utils"
	"talent-rodeo/internal/vapi"
)

func (app *application) createCall(ctx context.Context, customerNumber, customerName, resume string) error {

	tmpl, err := utils.CreateTemplate("prompts/shirely.001.txt")
	if err != nil {
		return fmt.Errorf("createCall: failed to create template: %w", err)
	}

	systemPrompt, err := tmpl.Execute(map[string]interface{}{
		"FullName": customerName,
		"Resume":   resume,
	})
	if err != nil {
		return fmt.Errorf("createCall: failed to render template: %w", err)
	}

	assistantID := "30709dea-4c26-457a-96a0-4ee7488f6b72"
	outboundPhoneID := "31e6bce0-24ee-4071-a70e-98936bccb9fe"

	modelConfig := vapi.DefaultModelConfig
	modelConfig.Messages = []vapi.ModelMessage{{
		Role:    "system",
		Content: systemPrompt,
	}}

	result, err := vapi.CreateCall(ctx, vapi.CallConfig{
		AssistantID:   &assistantID,
		PhoneNumberID: &outboundPhoneID,
		AssistantOverrides: &vapi.Assistant{
			Model: &modelConfig,
			Server: &vapi.ServerConfig{
				URL:            app.config.webhookURL,
				TimeoutSeconds: 20,
			},
			ServerMessages: []string{
				"conversation-update",
				"end-of-call-report",
				"function-call",
				"hang",
				"speech-update",
				"status-update",
				"tool-calls",
				"transfer-destination-request",
				"user-interrupted",
			},
		},
		Customer: &vapi.Customer{
			Number: customerNumber,
			Name:   customerName,
		},
	})

	if err != nil {
		return fmt.Errorf("createCall: failed to create call: %w", err)
	}

	app.logger.Info("call created successfully",
		"call_id", result.ID,
		"status", result.Status,
	)

	return nil
}
