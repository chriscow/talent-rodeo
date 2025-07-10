package chat

import (
	"context"
	"log/slog"
	"talent-rodeo/internal"
)

// ChatService defines the interface for chat functionality
type ChatService interface {
	// Session management
	StartSession(ctx context.Context, userID string, req StartChatRequest) (*StartChatResponse, error)
	EndSession(ctx context.Context, userID, appointmentID string) error

	// Message handling
	ProcessUserMessage(ctx context.Context, userID, appointmentID, message string) error
	StreamAIResponse(ctx context.Context, userID, appointmentID string, msgChan chan<- StreamMessage) error

	// History and status
	GetHistory(ctx context.Context, userID, appointmentID string) ([]internal.ChatMessage, error)

	// Testing and admin functions
	FastForwardChat(ctx context.Context, userID, appointmentID string) error

	// Internal functionality for tools
	SaveAppointment(ctx context.Context, appointment *internal.Appointment) error
}

// ChatSession represents an active chat session with context for tool execution
type ChatSession struct {
	User        *internal.User
	Appointment *internal.Appointment
	Service     ChatService // Reference to the service for saving appointments
}

// Implement LLMToolSession interface
func (s *ChatSession) GetUserID() string {
	return s.User.ID
}

func (s *ChatSession) GetSessionID() string {
	return s.Appointment.ID
}

// GetLogger returns the logger from the session
func (s *ChatSession) GetLogger() *slog.Logger {
	if serviceImpl, ok := s.Service.(*ServiceImpl); ok {
		return serviceImpl.logger
	}
	// Fallback - this shouldn't happen in normal usage
	return slog.Default()
}

// SaveAppointment saves the appointment through the service
func (s *ChatSession) SaveAppointment(ctx context.Context) error {
	return s.Service.SaveAppointment(ctx, s.Appointment)
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
