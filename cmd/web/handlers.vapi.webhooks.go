package main

import (
	"bytes"
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
)

// Message types
const (
	MsgTypeAssistantRequest   = "assistant-request"
	MsgTypeToolCalls          = "tool-calls"
	MsgTypeTransferDestReq    = "transfer-destination-request"
	MsgTypeConversationUpdate = "conversation-update"
	MsgTypeEndOfCallReport    = "end-of-call-report"
	MsgTypeFunctionCall       = "function-call"
	MsgTypeHang               = "hang"
	MsgTypeSpeechUpdate       = "speech-update"
	MsgTypeStatusUpdate       = "status-update"
	MsgTypeUserInterrupted    = "user-interrupted"
)

// WebhookMessage represents the incoming message from VAPI webhooks
type WebhookMessage struct {
	Message WebhookMessageContent `json:"message"`
}

// WebhookMessageContent contains the actual content of the webhook message
type WebhookMessageContent struct {
	Type string `json:"type"`
	// Additional fields depending on the message type
	// We're using a map for flexibility as there are many different message formats
	Data map[string]interface{} `json:"-"`
}

// UnmarshalJSON custom unmarshaler to capture all fields in the Data map
func (m *WebhookMessageContent) UnmarshalJSON(data []byte) error {
	// First, extract the type field
	type TypeOnly struct {
		Type string `json:"type"`
	}

	var typeInfo TypeOnly
	if err := json.Unmarshal(data, &typeInfo); err != nil {
		return err
	}
	m.Type = typeInfo.Type

	// Then put all data into the map
	return json.Unmarshal(data, &m.Data)
}

// MessageResponse represents the response to VAPI webhook messages
type MessageResponse struct {
	MessageResponse interface{} `json:"messageResponse,omitempty"`
	Error           string      `json:"error,omitempty"`
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
	ToolCallID string      `json:"toolCallId"`
	Status     string      `json:"status"`
	Content    interface{} `json:"content,omitempty"`
}

// HandleWebhook processes webhook requests from VAPI
func (app *application) handleVapiWebhook(w http.ResponseWriter, r *http.Request) {

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

	if os.Getenv("SAVE_WEBHOOK_PAYLOADS") == "true" {
		saveWebhookPayload(r.Method, body, app.logger)
	}

	// Parse the request body
	var msg WebhookMessage
	err = json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		app.logger.Error("failed to decode webhook message", "error", err)
		response.JSON(w, http.StatusBadRequest, MessageResponse{
			Error: "Invalid request body",
		})
		return
	}

	app.logger.Info("received webhook message", "type", msg.Message.Type)

	// Process based on message type
	switch msg.Message.Type {
	case MsgTypeAssistantRequest:
		app.handleAssistantRequest(w, r, msg)
	case MsgTypeToolCalls:
		app.handleToolCalls(w, r, msg)
	case MsgTypeTransferDestReq:
		app.handleTransferDestinationRequest(w, r, msg)
	case MsgTypeEndOfCallReport:
		app.handleEndOfCallReport(r, msg)
	default:
		// For other message types, we don't need to send a response
		app.logger.Debug("no response needed for message type", "type", msg.Message.Type)
		w.WriteHeader(http.StatusOK)
	}
}

func saveWebhookPayload(msgType string, body []byte, logger *slog.Logger) {
	// Save webhook payload to file for testing

	// Ensure the webhooks directory exists
	webhooksDir := "testdata/webhooks"
	if err := os.MkdirAll(webhooksDir, 0755); err != nil {
		logger.Error("Failed to create webhooks directory", "error", err)
		// Continue execution since this is just for testing purposes
	}

	// Create a filename based on message type and timestamp
	filename := fmt.Sprintf("%s/%s_%d.json",
		webhooksDir,
		msgType,
		time.Now().UnixNano(),
	)

	// Save the raw body instead of re-marshaling
	if err := os.WriteFile(filename, body, 0644); err != nil {
		logger.Error("Failed to save webhook payload", "error", err)
	} else {
		logger.Info("Saved webhook payload", "filename", filename)
	}

}

// handleAssistantRequest handles assistant-request messages
func (h *application) handleAssistantRequest(w http.ResponseWriter, r *http.Request, msg WebhookMessage) {
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
func (h *application) handleToolCalls(w http.ResponseWriter, r *http.Request, msg WebhookMessage) {
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
func (h *application) handleTransferDestinationRequest(w http.ResponseWriter, r *http.Request, msg WebhookMessage) {
	// In a real implementation, you would determine the transfer destination
	// For now, we'll just return a simple response

	resp := MessageResponse{
		MessageResponse: map[string]interface{}{
			"destination": map[string]interface{}{
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

// handleEndOfCallReport processes the end-of-call-report message
// handleEndOfCallReport processes the end-of-call-report message
func (app *application) handleEndOfCallReport(r *http.Request, webhookMsg WebhookMessage) error {
	// Parse the end of call report
	var report vapi.EndOfCallReport
	data, err := json.Marshal(webhookMsg.Message.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal message data: %w", err)
	}

	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("failed to unmarshal end of call report: %w", err)
	}

	// Get the call result
	callResult, err := vapi.GetCall(r.Context(), *report.Call.ID)
	if err != nil {
		return fmt.Errorf("failed to get call result: %w", err)
	}

	// Find the appointment by call ID
	appointments, err := internal.GetAppointmentsByCallID(r.Context(), app.fs, *report.Call.ID)
	if err != nil {
		return fmt.Errorf("failed to get appointment by call ID: %w", err)
	}

	if len(appointments) == 0 {
		return fmt.Errorf("no appointment found for call ID: %s", *report.Call.ID)
	}

	// Update the appointment with the call result and report
	appointment := appointments[0]
	appointment.EndOfCallReport = &report
	appointment.CallResult = callResult
	appointment.UpdatedAt = time.Now()

	// Update appointment status based on call result
	status, shouldStoreReason := internal.MapVapiEndReasonToAppointmentStatus(report.EndedReason)
	appointment.Status = status
	if shouldStoreReason {
		appointment.OtherStatus = &report.EndedReason
	}

	if err := internal.UpdateAppointment(r.Context(), app.fs, appointment); err != nil {
		return fmt.Errorf("failed to update appointment: %w", err)
	}

	app.logger.Info("Successfully processed end of call report",
		"call_id", *report.Call.ID,
		"appointment_id", appointment.ID,
		"ended_reason", report.EndedReason,
		"status", appointment.Status,
	)

	// Naaughty: We don't actually need a user object, we can just use the appointment user ID
	user := &internal.User{
		ID: appointment.UserID,
	}

	if err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusIntroCallCompleted); err != nil {
		app.logger.Error("Failed to update user", "error", err)
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}
