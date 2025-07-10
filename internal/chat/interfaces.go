package chat

import (
	"context"
	"log/slog"
	"talent-rodeo/internal"
)

// ChatService defines the main business logic interface for chat functionality
type ChatService interface {
	// StartSession creates a new chat session and returns initial response
	StartSession(ctx context.Context, userID string, req StartChatRequest) (*StartChatResponse, error)

	// ProcessUserMessage handles a user message and returns whether AI response should be generated
	ProcessUserMessage(ctx context.Context, userID, appointmentID, message string) error

	// StreamAIResponse generates and streams AI response, sending chunks via the provided channel
	StreamAIResponse(ctx context.Context, userID, appointmentID string, msgChan chan<- StreamMessage) error

	// EndSession ends a chat session and processes completion
	EndSession(ctx context.Context, userID, appointmentID string) error

	// GetHistory retrieves chat message history
	GetHistory(ctx context.Context, userID, appointmentID string) ([]internal.ChatMessage, error)

	// FastForwardChat injects test transcript for testing (super user only)
	FastForwardChat(ctx context.Context, userID, appointmentID string) error
}

// ChatSession represents the context for a chat session, implementing LLMToolSession
type ChatSession struct {
	User        *internal.User
	Appointment *internal.Appointment
	Service     ChatService // Reference back to service for database operations
}

// Implement LLMToolSession interface
func (s *ChatSession) GetUserID() string {
	return s.User.ID
}

func (s *ChatSession) GetSessionID() string {
	return s.Appointment.ID
}

func (s *ChatSession) GetLogger() *slog.Logger {
	// We'll need to get this from the service or pass it in
	// For now, we'll use a placeholder
	return slog.Default()
}

// StreamMessage represents a message chunk sent via WebSocket streaming
type StreamMessage struct {
	Type         string                    `json:"type"`    // "content", "tool_response", "done", "error"
	Content      string                    `json:"content"` // Text content for streaming
	Error        string                    `json:"error,omitempty"`
	ToolResponse *internal.LLMToolResponse `json:"tool_response,omitempty"`
}

// StartChatRequest represents the request to start a new chat session
type StartChatRequest struct {
	CSRFToken string `json:"csrf_token"`
	Timezone  string `json:"timezone"`
}

// StartChatResponse represents the response when starting a chat
type StartChatResponse struct {
	AppointmentID string `json:"appointment_id"`
	Status        string `json:"status"`
	FirstMessage  string `json:"first_message"`
}

// Legacy type aliases for backward compatibility during transition
// These will be removed once all code is updated
type ToolRegistry = internal.LLMToolRegistry
type Tool = internal.LLMTool
type ToolResponse = internal.LLMToolResponse
type UIAction = internal.LLMToolUIAction
