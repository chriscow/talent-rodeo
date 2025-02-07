package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"talent-rodeo/internal"

	"github.com/nats-io/nats.go/jetstream"
)

// NATSMessagePayload represents the data needed to publish a NATS message
type NATSMessagePayload struct {
	Subject  string            `json:"subject"`  // The subject to publish to
	Data     json.RawMessage   `json:"data"`     // The data to publish
	Metadata map[string]string `json:"metadata"` // Optional metadata for the message
}

// MessageData wraps the actual message data with metadata
type MessageData struct {
	Data     json.RawMessage   `json:"data"`
	Metadata map[string]string `json:"metadata"`
}

// NATSMessageScheduler processes scheduled NATS message tasks
type NATSMessageScheduler struct {
	js     jetstream.JetStream
	logger *slog.Logger
}

// NewNATSMessageScheduler creates a new NATS message scheduler
func NewNATSMessageScheduler(js jetstream.JetStream, logger *slog.Logger) *NATSMessageScheduler {
	return &NATSMessageScheduler{
		js:     js,
		logger: logger.With(slog.String("component", "nats_message_scheduler")),
	}
}

// TaskType returns the type of task this handler processes
func (h *NATSMessageScheduler) TaskType() string {
	return "nats_message_scheduler"
}

// ProcessTask handles the execution of a task
func (h *NATSMessageScheduler) ProcessTask(ctx context.Context, task *internal.Task) error {
	h.logger.Debug("processing NATS message scheduler task",
		"task_id", task.ID,
		"run_at", task.RunAt,
	)

	// Parse payload
	var payload NATSMessagePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Validate payload
	if payload.Subject == "" {
		return fmt.Errorf("subject is required")
	}
	if len(payload.Data) == 0 || string(payload.Data) == "null" {
		return fmt.Errorf("data is required")
	}

	// Add task ID to metadata for tracing
	if payload.Metadata == nil {
		payload.Metadata = make(map[string]string)
	}
	payload.Metadata["task_id"] = fmt.Sprintf("%d", task.ID)

	// Create message with metadata included in the payload
	msgData := MessageData{
		Data:     payload.Data,
		Metadata: payload.Metadata,
	}

	// Marshal the complete message
	msgBytes, err := json.Marshal(msgData)
	if err != nil {
		return fmt.Errorf("failed to marshal message data: %w", err)
	}

	h.logger.Debug("publishing message",
		"task_id", task.ID,
		"subject", payload.Subject,
		"data_size", len(msgBytes),
	)

	// Publish the message
	_, err = h.js.Publish(ctx, payload.Subject, msgBytes)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	h.logger.Info("message published successfully",
		"task_id", task.ID,
		"subject", payload.Subject,
	)

	return nil
}

// ScheduleMessage schedules a message to be published at the specified time
func (h *NATSMessageScheduler) ScheduleMessage(ctx context.Context, subject string, data interface{}, metadata map[string]string, runAt time.Time, taskProc TaskProcessor) error {
	h.logger.Debug("scheduling message",
		"subject", subject,
		"run_at", runAt,
	)

	// Validate parameters
	if subject == "" {
		return fmt.Errorf("subject is required")
	}
	if data == nil {
		return fmt.Errorf("data is required")
	}
	if runAt.IsZero() {
		return fmt.Errorf("run time is required")
	}

	// Don't schedule messages in the past
	if runAt.Before(time.Now().UTC()) {
		return fmt.Errorf("cannot schedule messages in the past")
	}

	// Marshal the message data
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Create task payload
	payload := NATSMessagePayload{
		Subject:  subject,
		Data:     dataBytes,
		Metadata: metadata,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create unique task ID
	taskID := time.Now().UnixNano()

	// Create the task
	task := &internal.Task{
		ID:          taskID,
		Type:        h.TaskType(),
		Payload:     payloadBytes,
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3, // Allow up to 3 retry attempts
		RunAt:       runAt.UTC(),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	h.logger.Debug("creating scheduler task",
		"task_id", task.ID,
		"subject", subject,
		"run_at", task.RunAt,
		"max_attempts", task.MaxAttempts,
	)

	// Enqueue the task
	if err := taskProc.EnqueueTask(ctx, task); err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	h.logger.Info("message scheduled",
		"task_id", task.ID,
		"subject", subject,
		"run_at", runAt.UTC(),
	)

	return nil
}
