package vapi

import (
	"fmt"
	"os"
	"talent-rodeo/internal/utils"
)

func GetDefaultVapiCallConfig(customerNumber, customerName, resume string) (*Call, error) {
	requiredEnvVars := []string{
		"VAPI_WEBHOOK_URL",
		"VAPI_ASSISTANT_ID",
		"VAPI_OUTBOUND_PHONE_ID",
	}

	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			return nil, fmt.Errorf("%s is not set", envVar)
		}
	}

	tmpl, err := utils.CreateTemplate("prompts/shirely.001.txt")
	if err != nil {
		return nil, fmt.Errorf("createCall: failed to create template: %w", err)
	}

	systemPrompt, err := tmpl.Execute(map[string]any{
		"FullName": customerName,
		"Resume":   resume,
	})
	if err != nil {
		return nil, fmt.Errorf("createCall: failed to render template: %w", err)
	}

	assistantID := os.Getenv("VAPI_ASSISTANT_ID")
	outboundPhoneID := os.Getenv("VAPI_OUTBOUND_PHONE_ID")

	modelConfig := DefaultModelConfig
	modelConfig.Messages = []ModelMessage{{
		Role:    "system",
		Content: systemPrompt,
	}}

	return &Call{
		AssistantID:   &assistantID,
		PhoneNumberID: &outboundPhoneID,
		AssistantOverrides: &Assistant{
			Model: &modelConfig,
			Server: &ServerConfig{
				URL:            os.Getenv("VAPI_WEBHOOK_URL"),
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
		Customer: &Customer{
			Number: customerNumber,
			Name:   customerName,
		},
	}, nil
}
