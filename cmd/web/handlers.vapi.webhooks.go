package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/vapi"
	"time"

	"cloud.google.com/go/firestore"
)

// MessageResponse represents the response to VAPI webhook messages
type MessageResponse struct {
	MessageResponse any    `json:"messageResponse,omitempty"`
	Error           string `json:"error,omitempty"`
}

// AssistantRequestResponse is returned for assistant-request message types
type AssistantRequestResponse struct {
	// This could contain destination, assistant config, etc.
	// For simplicity, we'll just include the basic required fields
	AssistantID string `json:"assistantId"`
}

// ToolCallsResponse is returned for tool-calls message types
type ToolCallsResponse struct {
	Results []ToolCallResult `json:"results"`
}

// ToolCallResult represents a single tool call result
type ToolCallResult struct {
	ToolCallID string `json:"toolCallId"`
	Status     string `json:"status"`
	Content    any    `json:"content,omitempty"`
}

// handleVapiWebhook processes webhook requests from VAPI
func (app *application) handleVapiWebhook(w http.ResponseWriter, r *http.Request) {
	app.logger.Debug("received webhook request",
		"method", r.Method,
		"url", r.URL.String(),
		"headers", r.Header,
		"test_mode", os.Getenv("TESTING_MODE") == "true")

	// Read the raw request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		app.logger.Error("Failed to read request body", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to read request body: %w", err))
		response.JSON(w, http.StatusBadRequest, map[string]string{
			"error": "failed to read request body",
		})
		return
	}
	// Restore the body for later use
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	app.logger.Debug("read request body", "size_bytes", len(body))

	// Parse the request body
	var msg vapi.WebhookMessage
	err = json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		app.logger.Error("failed to decode webhook message",
			"error", err,
			"body", string(body)) // Log the raw body to help debug parsing issues
		response.JSON(w, http.StatusBadRequest, MessageResponse{
			Error: "Invalid request body",
		})
		return
	}

	app.logger.Info("received webhook message",
		"type", msg.Message.Type,
		"data_keys", getMapKeys(msg.Message.Data))

	// Always try to save the webhook payload - the save function will handle all conditions
	saveWebhookPayload(msg.Message.Type, body, app.logger, app.fs)

	// Process based on message type
	switch msg.Message.Type {
	case vapi.MsgTypeAssistantRequest:
		app.handleAssistantRequest(w, r, msg)
	case vapi.MsgTypeToolCalls:
		app.handleToolCalls(w, r, msg)
	case vapi.MsgTypeTransferDestReq:
		app.handleTransferDestinationRequest(w, r, msg)
	case vapi.MsgTypeEndOfCallReport:
		app.logger.Debug("processing end-of-call report")
		app.handleEndOfCallReport(r, msg)
	default:
		// For other message types, we don't need to send a response
		app.logger.Debug("no response needed for message type", "type", msg.Message.Type)
		w.WriteHeader(http.StatusOK)
	}
}

func saveWebhookPayload(msgType string, body []byte, logger *slog.Logger, fs *firestore.Client) {
	// Skip if saving is not enabled or if in testing mode
	if os.Getenv("SAVE_WEBHOOK_PAYLOADS") != "true" {
		logger.Debug("skipping webhook payload save - saving not enabled")
		return
	}

	if os.Getenv("TESTING_MODE") == "true" {
		logger.Debug("skipping webhook payload save - in testing mode")
		return
	}

	// Generate a unique ID based on message type and timestamp
	docID := fmt.Sprintf("%s_%d", msgType, time.Now().UnixNano())

	// Try to save to Firestore
	if os.Getenv("SAVE_WEBHOOK_TO_FIRESTORE") == "true" {
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			logger.Error("Failed to parse webhook payload for Firestore", "error", err)
		} else {
			// Add metadata
			data["_metadata"] = map[string]any{
				"timestamp": time.Now(),
				"type":      msgType,
			}

			// Save to Firestore
			docRef := fs.Collection("webhook-data").Doc("vapi").Collection("webhooks").Doc(docID)
			if _, err := docRef.Set(context.Background(), data); err != nil {
				logger.Error("Failed to save webhook payload to Firestore", "error", err)
			} else {
				logger.Info("Saved webhook payload to Firestore", "doc_id", docID)
			}
		}
	}

	// Save to file system
	webhooksDir := "testdata/webhooks"
	if err := os.MkdirAll(webhooksDir, 0755); err != nil {
		logger.Error("Failed to create webhooks directory", "error", err)
		return
	}

	filename := fmt.Sprintf("%s/%s.json", webhooksDir, docID)
	if err := os.WriteFile(filename, body, 0644); err != nil {
		logger.Error("Failed to save webhook payload to file", "error", err)
	} else {
		logger.Info("Saved webhook payload to file", "filename", filename)
	}
}

// handleAssistantRequest handles assistant-request messages
func (h *application) handleAssistantRequest(w http.ResponseWriter, r *http.Request, msg vapi.WebhookMessage) {
	resp := MessageResponse{
		MessageResponse: AssistantRequestResponse{
			AssistantID: "default-assistant-id", // In a real implementation, you would determine this dynamically
		},
	}

	err := response.JSON(w, http.StatusOK, resp)
	if err != nil {
		h.logger.Error("failed to write response", "error", err)
	}
}

// handleToolCalls handles tool-calls messages
func (h *application) handleToolCalls(w http.ResponseWriter, r *http.Request, msg vapi.WebhookMessage) {
	// In a real implementation, you would extract the tool calls from the message
	// and execute them, then return the results

	resp := MessageResponse{
		MessageResponse: ToolCallsResponse{
			Results: []ToolCallResult{
				{
					ToolCallID: "example-tool-call-id",
					Status:     "success",
					Content:    "Tool call executed successfully",
				},
			},
		},
	}

	err := response.JSON(w, http.StatusOK, resp)
	if err != nil {
		h.logger.Error("failed to write response", "error", err)
	}
}

// handleTransferDestinationRequest handles transfer-destination-request messages
func (h *application) handleTransferDestinationRequest(w http.ResponseWriter, r *http.Request, msg vapi.WebhookMessage) {
	// In a real implementation, you would determine the transfer destination
	// For now, we'll just return a simple response

	resp := MessageResponse{
		MessageResponse: map[string]any{
			"destination": map[string]any{
				"type":   "number",
				"number": "+12345678900",
			},
		},
	}

	err := response.JSON(w, http.StatusOK, resp)
	if err != nil {
		h.logger.Error("failed to write response", "error", err)
	}
}

// getMapKeys returns a sorted list of keys from a map for logging
func getMapKeys(data []byte) []string {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// handleEndOfCallReport processes an end-of-call report webhook
func (app *application) handleEndOfCallReport(r *http.Request, webhookMsg vapi.WebhookMessage) error {
	// Debug log the incoming message
	if msgBytes, err := json.Marshal(webhookMsg); err == nil {
		app.logger.Debug("received end-of-call report", "message", string(msgBytes))
	}

	// Parse the message data into a report
	var report vapi.EndOfCallReport
	if err := json.Unmarshal(webhookMsg.Message.Data, &report); err != nil {
		app.logger.Error("failed to unmarshal end-of-call report", "error", err)
		return fmt.Errorf("failed to unmarshal end-of-call report: %w", err)
	}

	// Debug log the parsed report
	if reportBytes, err := json.Marshal(report); err == nil {
		app.logger.Debug("parsed end-of-call report", "report", string(reportBytes))
	}

	// Validate required fields
	if report.Call == nil {
		app.logger.Error("missing call information in end of call report")
		return fmt.Errorf("missing call information in end of call report")
	}

	// Debug log call information
	if callBytes, err := json.Marshal(report.Call); err == nil {
		app.logger.Debug("call information", "call", string(callBytes))
	}

	if report.Call.ID == nil {
		app.logger.Error("missing call ID in end of call report")
		return fmt.Errorf("missing call ID in end of call report")
	}

	// Debug log the call ID
	app.logger.Debug("processing call", "id", *report.Call.ID)

	// Extract call ID
	callID := *report.Call.ID

	app.logger.Debug("parsed end-of-call report",
		"call_id", callID,
		"ended_reason", report.EndedReason)

	// The appointment ID is stored in the call name
	if report.Call.Name == nil {
		app.logger.Error("missing call name in end of call report")
		return fmt.Errorf("missing call name in end of call report")
	}

	appointmentID := *report.Call.Name

	// The customer ID is required for getting the appointment
	if report.Call.CustomerID == nil {
		app.logger.Error("missing customer ID in end of call report")
		return fmt.Errorf("missing customer ID in end of call report")
	}
	customerID := *report.Call.CustomerID

	summary := report.Summary

	// Get the appointment
	appointment, err := internal.GetAppointment(r.Context(), app.fs, customerID, appointmentID)
	if err != nil {
		app.logger.Error("failed to get appointment",
			"error", err,
			"customer_id", customerID,
			"appointment_id", appointmentID)
		return fmt.Errorf("failed to get appointment: %w", err)
	}

	// Update the appointment with the call result and report
	appointment.CallID = &callID
	appointment.TranscriptSummary = summary
	appointment.UpdatedAt = time.Now()
	endReason := report.EndedReason

	// Update appointment status based on call result
	status, shouldLog := internal.MapVapiEndReasonToAppointmentStatus(endReason)
	appointment.Status = status

	if shouldLog {
		app.logger.Info("appointment status updated",
			"appointment_id", appointmentID,
			"status", status,
			"end_reason", endReason)
	}

	if err := internal.UpdateAppointment(r.Context(), app.fs, appointment); err != nil {
		app.logger.Error("failed to update appointment",
			"error", err,
			"appointment_id", appointmentID)
		return fmt.Errorf("failed to update appointment: %w", err)
	}

	return nil
}
