package simulate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"talent-rodeo/internal"
	th "talent-rodeo/internal/tasks/handlers"
	"time"
)

// TaskProcessor defines the interface for processing tasks
type TaskProcessor interface {
	EnqueueTask(ctx context.Context, task *internal.Task) error
	CreateAndEnqueueTask(ctx context.Context, taskType string, name string, payload []byte, runAt time.Time) (*internal.Task, error)
}

// truncateJSON truncates a JSON string to maxLen bytes while trying to keep it valid JSON
func truncateJSON(data string, maxLen int) string {
	if len(data) <= maxLen {
		return data
	}
	// Try to find the last complete object/array closing
	truncated := data[:maxLen]
	lastBrace := max(strings.LastIndex(truncated, "}"), strings.LastIndex(truncated, "]"))
	if lastBrace > 0 {
		truncated = data[:lastBrace+1]
	}
	return truncated + "..."
}

// simulateEndOfCall schedules a simulated end-of-call webhook in test mode
func EndOfCall(ctx context.Context, processor TaskProcessor, userID, appointmentID string, logger *slog.Logger) error {
	if appointmentID == "" {
		return fmt.Errorf("appointment ID cannot be empty")
	}

	// Check if context is already canceled
	if err := ctx.Err(); err != nil {
		return err
	}

	// Load test data
	reportData, err := loadEndOfCallTestData()
	if err != nil {
		return fmt.Errorf("failed to load test data: %w", err)
	}

	logger.Debug("loaded end-of-call test data", "data", truncateJSON(string(reportData), 200))

	// Update report with appointment ID
	msg, err := updateEndOfCallReport(reportData, userID, appointmentID)
	if err != nil {
		return fmt.Errorf("failed to update end of call report: %w", err)
	}

	if msgBytes, err := json.Marshal(msg); err == nil {
		logger.Debug("updated end of call report", "message", truncateJSON(string(msgBytes), 200))
	}

	// Create webhook payload
	payload, err := createWebhookPayload(msg)
	if err != nil {
		return fmt.Errorf("failed to create webhook payload: %w", err)
	}

	logger.Debug("created webhook payload",
		"url", payload.URL,
		"method", payload.Method,
		"headers", payload.Headers,
		"body", truncateJSON(string(payload.Body), 200))

	// Launch goroutine to send webhook after delay
	go func() {
		// Sleep for 5 seconds
		time.Sleep(5 * time.Second)

		// Create HTTP client
		client := &http.Client{
			Timeout: 10 * time.Second,
		}

		// Create request
		req, err := http.NewRequest(payload.Method, payload.URL, bytes.NewReader(payload.Body))
		if err != nil {
			logger.Error("Failed to create webhook request", "error", err)
			return
		}

		// Add headers
		for k, v := range payload.Headers {
			req.Header.Set(k, v)
		}

		// Log request details before sending
		if body, err := json.Marshal(json.RawMessage(payload.Body)); err == nil {
			logger.Debug("sending webhook request",
				"url", req.URL.String(),
				"method", req.Method,
				"headers", req.Header,
				"body", truncateJSON(string(body), 200))
		}

		// Send request
		resp, err := client.Do(req)
		if err != nil {
			logger.Error("Failed to send webhook request", "error", err)
			return
		}
		defer resp.Body.Close()

		// Read and log response body
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error("Failed to read response body", "error", err)
		} else {
			logger.Debug("webhook response",
				"status_code", resp.StatusCode,
				"headers", resp.Header,
				"body", truncateJSON(string(respBody), 200))
		}

		if resp.StatusCode != http.StatusOK {
			logger.Error("Webhook request failed",
				"status_code", resp.StatusCode,
				"appointment_id", appointmentID)
			return
		}

		logger.Info("Successfully sent simulated end-of-call webhook",
			"appointment_id", appointmentID)
	}()

	logger.Info("scheduled simulated end-of-call webhook",
		"appointment_id", appointmentID)
	return nil
}

// loadEndOfCallTestData loads the test data from the file system
func loadEndOfCallTestData() ([]byte, error) {
	wd, err := os.Getwd()
	fmt.Printf("DEBUG: Working directory: %s\n", wd)

	reportData, err := os.ReadFile("testdata/vapi/webhooks/end-of-call-report/end-of-call-report_1743786911443605712.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read test data: %w", err)
	}
	return reportData, nil
}

// updateEndOfCallReport prepares a simulated end-of-call webhook message by:
// 1. Taking a template webhook message from test data
// 2. Injecting the provided appointment ID into the message
// 3. Handling different possible message structures:
//   - Messages with only type field (requires full restructuring)
//   - Full report structures (normal path)
//   - Nested data structures
//
// This function is used in testing to simulate VAPI webhook callbacks with
// real appointment IDs while maintaining the structure of a real webhook response.
func updateEndOfCallReport(reportData []byte, userID, appointmentID string) ([]byte, error) {
	// Parse the message data into a map for debugging
	var data map[string]any
	if err := json.Unmarshal(reportData, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message data: %w", err)
	}

	// Extract the message content
	messageData, ok := data["message"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid message structure")
	}

	// Initialize or update call information
	var call map[string]any
	if call, ok = messageData["call"].(map[string]any); !ok {
		return nil, fmt.Errorf("invalid call structure")
	}

	// Update call fields
	call["id"] = appointmentID
	call["name"] = appointmentID

	customer, ok := call["customer"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid customer structure")
	}

	customer["name"] = userID

	messageData["call"] = call

	// Ensure required fields are present
	messageData["type"] = "end-of-call-report"
	if _, ok := messageData["endedReason"]; !ok {
		messageData["endedReason"] = "completed"
	}

	data["message"] = messageData

	// Convert report to bytes
	return json.Marshal(data)
}

// createWebhookPayload creates a webhook payload from a webhook message
func createWebhookPayload(msg []byte) (*th.WebhookPayload, error) {
	webhookURL := os.Getenv("VAPI_WEBHOOK_URL")
	if webhookURL == "" {
		return nil, fmt.Errorf("VAPI_WEBHOOK_URL not set")
	}

	fmt.Printf("DEBUG: Final webhook structure: %s\n", truncateJSON(string(msg), 200))

	payload := &th.WebhookPayload{
		URL:    webhookURL,
		Method: "POST",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: msg,
	}

	return payload, nil
}
