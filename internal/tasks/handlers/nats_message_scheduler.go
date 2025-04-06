package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"talent-rodeo/internal"
	"talent-rodeo/internal/tasks"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	TaskTypeNATSMessageScheduler = "nats_message_scheduler"
)

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
	return TaskTypeNATSMessageScheduler
}

// ProcessTask handles the execution of a task
func (h *NATSMessageScheduler) ProcessTask(ctx context.Context, task *internal.Task) error {
	h.logger.Debug("processing NATS message scheduler task",
		"task_id", task.ID,
		"run_at", task.RunAt,
	)

	// Parse payload
	var payload tasks.NATSMessagePayload
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
	payload.Metadata["task_id"] = task.ID

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
