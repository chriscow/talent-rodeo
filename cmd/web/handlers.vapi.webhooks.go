package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"time"

	"github.com/chriscow/vapi-go"

	"github.com/getsentry/sentry-go"
)

// MessageResponse represents the response to VAPI webhook messages
type MessageResponse struct {
	MessageResponse any    `json:"messageResponse,omitempty"`
	Error           string `json:"error,omitempty"`
}

// AssistantRequestResponse is returned for assistant-request message types
type AssistantRequestResponse struct {
	// This is the error if the call shouldn’t be accepted. This is spoken to the customer.
	// If this is sent, assistantId, assistant, squadId, squad, and destination are ignored.
	Error string `json:"error,omitempty"`
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
		"type", msg.Message.Type)

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
		if err := handleEndOfCallReport(r, app, body); err != nil {
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"webhook_type": "end-of-call-report",
				"webhook_body": string(body),
			})
			app.logger.Error("failed to handle end-of-call report", "error", err)
			response.JSON(w, http.StatusInternalServerError, MessageResponse{
				Error: "failed to handle end-of-call report",
			})
		}
	default:
		// For other message types, we don't need to send a response
		app.logger.Debug("no response needed for message type", "type", msg.Message.Type)
		w.WriteHeader(http.StatusOK)
	}
}

func saveWebhookPayload(msgType string, body []byte, logger *slog.Logger, _ *internal.FirestoreClient) {
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
		logger.Info("saved webhook payload to file", "filename", filename)
	}
}

// handleAssistantRequest handles assistant-request messages
func (h *application) handleAssistantRequest(w http.ResponseWriter, r *http.Request, msg vapi.WebhookMessage) {

	// user, ok, err := internal.GetUserByPhone(r.Context(), h.fs, msg.Message.Data["phoneNumber"].(string))
	// if err != nil {
	// 	h.logger.Error("failed to get user by phone number", "error", err)
	// 	response.JSON(w, http.StatusInternalServerError, MessageResponse{
	// 		Error: "failed to get user by phone number",
	// 	})
	// 	return
	// }

	resp := MessageResponse{
		MessageResponse: AssistantRequestResponse{
			Error: "This is the error if the call shouldn’t be accepted. This is spoken to the customer.",
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

// handleEndOfCallReport processes an end-of-call report webhook
func handleEndOfCallReport(r *http.Request, app *application, msg []byte) error {
	envelope, err := validateEndOfCallReport(r, msg)
	if err != nil {
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":       "failed_to_validate_end_of_call_report",
			"webhook_body": string(msg),
		})
		return fmt.Errorf("failed to validate end of call report: %w", err)
	}

	report := envelope.EndOfCallReport
	appointmentID := *report.Call.Name
	customerID := report.Call.Customer.Name

	app.processCompletedAppointment(r, customerID, appointmentID, *envelope)

	app.logger.Debug("processed end-of-call report", "appointmentID", appointmentID, "customerID", customerID)

	return nil
}

func validateEndOfCallReport(r *http.Request, webhookMsg []byte) (*vapi.EndOfCallReportEnvelope, error) {
	var env vapi.EndOfCallReportEnvelope
	if err := json.Unmarshal(webhookMsg, &env); err != nil {
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":       "failed_to_unmarshal_end_of_call_report",
			"webhook_body": string(webhookMsg),
		})
		return nil, fmt.Errorf("missing call object in end of call report: %w", err)
	}

	// call.name is the appointment ID
	// call.customer.name is the customer ID

	// Validate required fields
	report := env.EndOfCallReport
	if report.Call == nil {
		err := errors.New("missing call object in end of call report")
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":       "missing_call_object_in_end_of_call_report",
			"webhook_body": string(webhookMsg),
		})
		return nil, fmt.Errorf("missing `Call` object in end of call report: %w", err)
	}

	if report.Call.ID == nil {
		err := errors.New("missing call ID in end of call report")
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":       "missing_call_id_in_end_of_call_report",
			"webhook_body": string(webhookMsg),
		})
		return nil, fmt.Errorf("missing `Call.ID` in end of call report: %w", err)
	}

	// The appointment ID is stored in the call name
	if report.Call.Name == nil {
		err := errors.New("missing call name in end of call report. call name stores the appointment ID")
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":       "missing_call_name_in_end_of_call_report",
			"webhook_body": string(webhookMsg),
		})
		return nil, fmt.Errorf("missing `Call.Name` in end of call report: %w", err)
	}

	if report.Call.Customer == nil {
		err := errors.New("missing customer object in end of call report")
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":       "missing_customer_object_in_end_of_call_report",
			"webhook_body": string(webhookMsg),
		})
		return nil, fmt.Errorf("missing `Call.Customer` object in end of call report: %w", err)
	}

	// The customer ID is required for getting the appointment
	if report.Call.Customer.Name == "" {
		err := errors.New("missing customer name in end of call report. The customer name stores the customer ID")
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":       "missing_customer_name_in_end_of_call_report",
			"webhook_body": string(webhookMsg),
		})
		return nil, fmt.Errorf("missing `Call.Customer.Name` in end of call report: %w", err)
	}

	return &env, nil
}
