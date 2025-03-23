package vapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"talent-rodeo/internal/response"
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

// EndOfCallReport represents the end-of-call-report message type
type EndOfCallReport struct {
	Type      string    `json:"type"`
	CallID    string    `json:"callId"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Duration  int       `json:"duration"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	Cost      struct {
		Total     float64 `json:"total"`
		Currency  string  `json:"currency"`
		Breakdown struct {
			Model       float64 `json:"model"`
			Voice       float64 `json:"voice"`
			Transcriber float64 `json:"transcriber"`
		} `json:"breakdown"`
	} `json:"cost"`
	Metrics struct {
		TotalTurns        int     `json:"totalTurns"`
		HumanWords        int     `json:"humanWords"`
		AssistantWords    int     `json:"assistantWords"`
		AverageLatencyMs  float64 `json:"averageLatencyMs"`
		TranscriptQuality float64 `json:"transcriptQuality"`
	} `json:"metrics"`
	Recording struct {
		URL      string `json:"url"`
		Duration int    `json:"duration"`
	} `json:"recording,omitempty"`
}

// WebhookHandler handles webhook requests from VAPI
type WebhookHandler struct {
	logger *slog.Logger
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{
		logger: logger,
	}
}

// HandleWebhook processes webhook requests from VAPI
func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Parse the request body
	var msg WebhookMessage
	err := json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		h.logger.Error("failed to decode webhook message", "error", err)
		response.JSON(w, http.StatusBadRequest, MessageResponse{
			Error: "Invalid request body",
		})
		return
	}

	h.logger.Info("received webhook message", "type", msg.Message.Type)

	// Process based on message type
	switch msg.Message.Type {
	case MsgTypeAssistantRequest:
		h.handleAssistantRequest(w, r, msg)
	case MsgTypeToolCalls:
		h.handleToolCalls(w, r, msg)
	case MsgTypeTransferDestReq:
		h.handleTransferDestinationRequest(w, r, msg)
	case MsgTypeEndOfCallReport:
		h.handleEndOfCallReport(w, r, msg)
	default:
		// For other message types, we don't need to send a response
		h.logger.Debug("no response needed for message type", "type", msg.Message.Type)
		w.WriteHeader(http.StatusOK)
	}
}

// handleAssistantRequest handles assistant-request messages
func (h *WebhookHandler) handleAssistantRequest(w http.ResponseWriter, r *http.Request, msg WebhookMessage) {
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
func (h *WebhookHandler) handleToolCalls(w http.ResponseWriter, r *http.Request, msg WebhookMessage) {
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
func (h *WebhookHandler) handleTransferDestinationRequest(w http.ResponseWriter, r *http.Request, msg WebhookMessage) {
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
func (h *WebhookHandler) handleEndOfCallReport(w http.ResponseWriter, r *http.Request, msg WebhookMessage) {
	var report EndOfCallReport
	data, err := json.Marshal(msg.Message.Data)
	if err != nil {
		h.logger.Error("failed to marshal message data", "error", err)
		w.WriteHeader(http.StatusOK) // Still return 200 as this is async
		return
	}

	if err := json.Unmarshal(data, &report); err != nil {
		h.logger.Error("failed to unmarshal end of call report", "error", err)
		w.WriteHeader(http.StatusOK) // Still return 200 as this is async
		return
	}

	// Log the report details
	h.logger.Info("received end of call report",
		"call_id", report.CallID,
		"duration", report.Duration,
		"status", report.Status,
		"total_cost", report.Cost.Total,
		"currency", report.Cost.Currency,
		"total_turns", report.Metrics.TotalTurns,
		"human_words", report.Metrics.HumanWords,
		"assistant_words", report.Metrics.AssistantWords,
		"avg_latency_ms", report.Metrics.AverageLatencyMs,
	)

	// No response needed for end-of-call-report
	w.WriteHeader(http.StatusOK)
}
