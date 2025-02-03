package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"talent-rodeo/internal/models"
)

// PhoneCallHandler processes phone call tasks
type PhoneCallHandler struct {
	twilio TwilioClient
	logger *slog.Logger
	config PhoneCallConfig
}

// PhoneCallConfig holds configuration for the phone call handler
type PhoneCallConfig struct {
	FromNumber string
	WebhookURL string
}

// PhoneCallPayload represents the data needed to make a phone call
type PhoneCallPayload struct {
	PhoneNumber string `json:"phone_number"`
	Message     string `json:"message"`
}

// NewPhoneCallHandler creates a new phone call handler
func NewPhoneCallHandler(twilio TwilioClient, logger *slog.Logger, config PhoneCallConfig) *PhoneCallHandler {
	return &PhoneCallHandler{
		twilio: twilio,
		logger: logger,
		config: config,
	}
}

func (h *PhoneCallHandler) TaskType() string {
	return "phone_call"
}

func (h *PhoneCallHandler) ProcessTask(ctx context.Context, task *models.Task) error {
	var payload PhoneCallPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	if payload.PhoneNumber == "" {
		return fmt.Errorf("phone number is required")
	}
	if payload.Message == "" {
		return fmt.Errorf("message is required")
	}

	// Make the call
	callSID, err := h.twilio.MakeCall(ctx, h.config.FromNumber, payload.PhoneNumber, h.config.WebhookURL)
	if err != nil {
		return fmt.Errorf("failed to make call: %w", err)
	}

	// Wait for call to be initiated (up to 30 seconds)
	for i := 0; i < 30; i++ {
		status, err := h.twilio.GetCallStatus(ctx, callSID)
		if err != nil {
			h.logger.Error("failed to get call status",
				"error", err,
				"call_sid", callSID,
				"attempt", i+1,
			)
			continue
		}

		h.logger.Info("call status",
			"status", status,
			"call_sid", callSID,
			"attempt", i+1,
		)

		switch status {
		case "in-progress", "completed":
			return nil
		case "failed", "busy", "no-answer", "canceled":
			return fmt.Errorf("call failed with status: %s", status)
		}

		// Wait a second before checking again
		time.Sleep(time.Second)
	}

	// If we get here, the call didn't connect in time
	if err := h.twilio.EndCall(ctx, callSID); err != nil {
		h.logger.Error("failed to end call",
			"error", err,
			"call_sid", callSID,
		)
	}

	return fmt.Errorf("call did not connect within 30 seconds")
}
