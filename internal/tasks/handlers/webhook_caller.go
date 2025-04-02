package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"talent-rodeo/internal"

	"cloud.google.com/go/firestore"
)

const (
	// TaskTypeWebhookCall is the task type for webhook calls
	TaskTypeWebhookCall = "webhook_call"
	// MaxResponseSize is the maximum size of response body (1MB)
	MaxResponseSize = 1024 * 1024
)

// WebhookPayload represents the data needed to make a webhook call
type WebhookPayload struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
}

// WebhookScheduler handles scheduling and executing webhook calls
type WebhookScheduler struct {
	logger *slog.Logger
	fs     *firestore.Client
}

// NewWebhookScheduler creates a new webhook scheduler
func NewWebhookScheduler(logger *slog.Logger, fs *firestore.Client) *WebhookScheduler {
	return &WebhookScheduler{
		logger: logger,
		fs:     fs,
	}
}

// TaskType returns the type of task this handler processes
func (h *WebhookScheduler) TaskType() string {
	return TaskTypeWebhookCall
}

// ProcessTask handles the execution of a webhook call task
func (h *WebhookScheduler) ProcessTask(ctx context.Context, task *internal.Task) error {
	var payload WebhookPayload
	if err := task.GetPayload(&payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// If this is a VAPI call, try to update the appointment status
	if payload.Headers != nil && payload.Headers["X-VAPI-Call-ID"] != "" {
		callID := payload.Headers["X-VAPI-Call-ID"]
		appointments, err := internal.GetAppointmentsByCallID(ctx, h.fs, callID)
		if err != nil {
			h.logger.Error("Failed to get appointment by call ID", "error", err, "call_id", callID)
		} else if len(appointments) > 0 {
			appointment := appointments[0]
			appointment.Status = internal.AppointmentStatusFailed
			appointment.UpdatedAt = time.Now()
			if err := internal.UpdateAppointment(ctx, h.fs, appointment); err != nil {
				h.logger.Error("Failed to update appointment status", "error", err, "appointment_id", appointment.ID)
			}
		}
	}

	// Make the HTTP request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, payload.Method, payload.URL, bytes.NewReader(payload.Body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for k, v := range payload.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return &internal.TaskError{
			Err:         fmt.Errorf("webhook call failed: %w", err),
			ShouldRetry: true,
		}
	}
	defer resp.Body.Close()

	// Read response body (limited to prevent memory issues)
	body := make([]byte, MaxResponseSize)
	n, _ := io.ReadFull(resp.Body, body)
	body = body[:n]

	// Check response status
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		task.Status = internal.TaskStatusRetrying
		task.Attempts++
		task.RunAt = time.Now().Add(time.Second * time.Duration(task.Attempts*2)) // Simple exponential backoff
		return &internal.TaskError{
			Err:         fmt.Errorf("received retryable status code %d", resp.StatusCode),
			ShouldRetry: true,
		}
	}

	if resp.StatusCode >= 400 {
		task.Status = internal.TaskStatusFailed
		task.Error = fmt.Sprintf("webhook call failed with status %d", resp.StatusCode)
		return fmt.Errorf("webhook call failed with status %d", resp.StatusCode)
	}

	task.Status = internal.TaskStatusComplete
	return nil
}
