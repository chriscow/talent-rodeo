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
	twilio   TwilioClient
	logger   *slog.Logger
	config   PhoneCallConfig
	taskProc TaskProcessor
}

// PhoneCallConfig holds configuration for the phone call handler
type PhoneCallConfig struct {
	FromNumber string
	Twiml      string // TwiML template for the call
}

// PhoneCallPayload represents the data needed to make a phone call
type PhoneCallPayload struct {
	PhoneNumber string `json:"phone_number"` // The phone number to call
}

// ScheduleCallParams contains parameters for scheduling a call
type ScheduleCallParams struct {
	PhoneNumber string    // The phone number to call
	ScheduledAt time.Time // When to make the call (in UTC)
	TimeZone    string    // User's timezone
}

// TaskProcessor defines the interface for task processing
type TaskProcessor interface {
	EnqueueTask(ctx context.Context, task *models.Task) error
}

// NewPhoneCallHandler creates a new phone call handler
func NewPhoneCallHandler(twilio TwilioClient, logger *slog.Logger, config PhoneCallConfig, taskProc TaskProcessor) *PhoneCallHandler {
	return &PhoneCallHandler{
		twilio:   twilio,
		logger:   logger,
		config:   config,
		taskProc: taskProc,
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

	h.logger.Debug("Making call with TwiML",
		"phone_number", payload.PhoneNumber,
		"from_number", h.config.FromNumber,
		"twiml", h.config.Twiml,
	)

	// Make the call
	callSID, err := h.twilio.MakeCall(ctx, h.config.FromNumber, payload.PhoneNumber, h.config.Twiml)
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

// ScheduleCall schedules a new phone call task
func (h *PhoneCallHandler) ScheduleCall(ctx context.Context, params ScheduleCallParams) error {
	h.logger.Debug("scheduling phone call",
		"phone_number", params.PhoneNumber,
		"scheduled_at", params.ScheduledAt,
		"timezone", params.TimeZone,
	)

	// Validate parameters
	if params.PhoneNumber == "" {
		return fmt.Errorf("phone number is required")
	}
	if params.ScheduledAt.IsZero() {
		return fmt.Errorf("scheduled time is required")
	}
	if params.TimeZone == "" {
		params.TimeZone = "UTC"
	}

	// Convert scheduled time to UTC if it's not already
	location, err := time.LoadLocation(params.TimeZone)
	if err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	scheduledUTC := params.ScheduledAt.In(location).UTC()

	// Don't schedule calls in the past
	if scheduledUTC.Before(time.Now().UTC()) {
		return fmt.Errorf("cannot schedule calls in the past")
	}

	// Create task payload
	payload := PhoneCallPayload{
		PhoneNumber: params.PhoneNumber,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create unique task ID
	taskID := time.Now().UnixNano()

	// Create the task
	task := &models.Task{
		ID:          taskID,
		Type:        h.TaskType(),
		Payload:     payloadBytes,
		Status:      models.TaskStatusPending,
		MaxAttempts: 3, // Allow up to 3 retry attempts
		RunAt:       scheduledUTC,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	h.logger.Debug("creating phone call task",
		"task_id", task.ID,
		"run_at", task.RunAt,
		"max_attempts", task.MaxAttempts,
	)

	// Enqueue the task
	if err := h.taskProc.EnqueueTask(ctx, task); err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	h.logger.Info("phone call scheduled",
		"task_id", task.ID,
		"phone_number", params.PhoneNumber,
		"scheduled_at", scheduledUTC,
	)

	return nil
}
