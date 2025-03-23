package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"talent-rodeo/internal"
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

// WebhookScheduler handles webhook call tasks
type WebhookScheduler struct {
	client *http.Client
	logger *slog.Logger
	// List of status codes that should trigger a retry
	retryStatusCodes []int
}

// NewWebhookScheduler creates a new webhook handler
func NewWebhookScheduler(logger *slog.Logger) *WebhookScheduler {
	// Create a custom transport that doesn't follow redirects
	transport := &http.Transport{
		DisableKeepAlives: true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &WebhookScheduler{
		client: client,
		logger: logger,
		retryStatusCodes: []int{
			408, // Request Timeout
			429, // Too Many Requests
			500, // Internal Server Error
			502, // Bad Gateway
			503, // Service Unavailable
			504, // Gateway Timeout
		},
	}
}

// TaskType returns the type of task this handler processes
func (h *WebhookScheduler) TaskType() string {
	return TaskTypeWebhookCall
}

// ProcessTask handles the execution of a webhook call task
func (h *WebhookScheduler) ProcessTask(ctx context.Context, task *internal.Task) error {
	var payload WebhookPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal webhook payload: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, payload.Method, payload.URL, bytes.NewReader(payload.Body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for key, value := range payload.Headers {
		req.Header.Set(key, value)
	}

	// Make the request
	h.logger.Debug("making webhook request",
		"url", payload.URL,
		"method", payload.Method,
		"headers", payload.Headers,
		"body_size", len(payload.Body),
	)

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body with size limit
	limitedReader := io.LimitReader(resp.Body, MaxResponseSize)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	h.logger.Debug("webhook response received",
		"status", resp.StatusCode,
		"body_size", len(respBody),
	)

	// Check if we should retry based on status code
	for _, code := range h.retryStatusCodes {
		if resp.StatusCode == code {
			task.Status = internal.TaskStatusRetrying
			task.Error = fmt.Sprintf("received retryable status code %d", resp.StatusCode)
			return fmt.Errorf("received retryable status code %d", resp.StatusCode)
		}
	}

	// If status code is not in 2xx range and not in retry list, mark as permanent failure
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		task.Status = internal.TaskStatusFailed
		task.Error = fmt.Sprintf("webhook call failed with status %d", resp.StatusCode)
		return fmt.Errorf("webhook call failed with status %d", resp.StatusCode)
	}

	// Mark successful tasks as complete
	task.Status = internal.TaskStatusComplete
	task.Error = ""
	return nil
}
