package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/models"
	"talent-rodeo/internal/onboarding"
)

// PhoneCallHandler processes phone call tasks
type PhoneCallHandler struct {
	twilio   TwilioClient
	logger   *slog.Logger
	config   PhoneCallConfig
	taskProc TaskProcessor
	db       UserDatabase
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

// UserDatabase defines the database operations needed by the phone call handler
type UserDatabase interface {
	GetUserByPhone(phone string) (*database.User, error)
	UpdateOnboardingStep(userID int, step onboarding.Step) error
}

// NewPhoneCallHandler creates a new phone call handler
func NewPhoneCallHandler(twilio TwilioClient, logger *slog.Logger, config PhoneCallConfig, taskProc TaskProcessor, db UserDatabase) *PhoneCallHandler {
	return &PhoneCallHandler{
		twilio:   twilio,
		logger:   logger,
		config:   config,
		taskProc: taskProc,
		db:       db,
	}
}

func (h *PhoneCallHandler) TaskType() string {
	return "phone_call"
}

func (h *PhoneCallHandler) ProcessTask(ctx context.Context, task *models.Task) error {
	h.logger.Debug("Processing phone call task",
		"task_id", task.ID,
		"attempt", task.Attempts,
		"max_attempts", task.MaxAttempts)

	var payload PhoneCallPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// If this is the last attempt, update the user's onboarding state back to resume uploaded
	if task.Attempts >= task.MaxAttempts-1 {
		h.logger.Warn("Phone call task exhausted all retries, updating onboarding state",
			"task_id", task.ID,
			"attempts", task.Attempts,
			"max_attempts", task.MaxAttempts)

		// Find the user by phone number
		user, err := h.db.GetUserByPhone(payload.PhoneNumber)
		if err != nil {
			h.logger.Error("Failed to find user by phone number",
				"error", err,
				"phone", payload.PhoneNumber)
			return fmt.Errorf("failed to find user: %w", err)
		}

		// Update onboarding state back to resume uploaded
		if err := h.db.UpdateOnboardingStep(user.ID, onboarding.StepResumeUploaded); err != nil {
			h.logger.Error("Failed to update onboarding state",
				"error", err,
				"user_id", user.ID)
			return fmt.Errorf("failed to update onboarding state: %w", err)
		}

		h.logger.Info("Updated user onboarding state back to resume uploaded",
			"user_id", user.ID,
			"phone", payload.PhoneNumber)
	}

	// Make the call
	h.logger.Debug("Making call with Twilio",
		"from_number", h.config.FromNumber,
		"to_number", payload.PhoneNumber,
		"twiml", h.config.Twiml)

	callSID, err := h.twilio.MakeCall(ctx, h.config.FromNumber, payload.PhoneNumber, h.config.Twiml)
	if err != nil {
		h.logger.Error("Failed to make call",
			"error", err,
			"from_number", h.config.FromNumber,
			"to_number", payload.PhoneNumber)
		return fmt.Errorf("failed to make call: %w", err)
	}

	h.logger.Debug("Call initiated",
		"call_sid", callSID,
		"phone_number", payload.PhoneNumber)

	return nil
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
