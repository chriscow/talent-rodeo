package chat

import (
	"context"
	"talent-rodeo/internal"

	"github.com/chriscow/vapi-go"
	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
)

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

// ChatSession represents a WebSocket chat session
type ChatSession struct {
	UserID        string
	AppointmentID string
	Connection    *websocket.Conn
}

// StreamMessage represents different types of messages sent via WebSocket during streaming
type StreamMessage struct {
	Type         string            `json:"type"`
	Content      string            `json:"content,omitempty"`
	Error        string            `json:"error,omitempty"`
	MessageID    string            `json:"message_id,omitempty"`
	ToolCall     *ToolCallData     `json:"tool_call,omitempty"`
	ToolResponse *ToolResponseData `json:"tool_response,omitempty"`
}

// ToolCallData represents a tool call made by the LLM
type ToolCallData struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolResponseData represents the response from a tool execution
type ToolResponseData struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Content   string     `json:"content"`
	Success   bool       `json:"success"`
	UIActions []UIAction `json:"ui_actions,omitempty"`
}

// UIAction represents an action for the client to perform
type UIAction struct {
	Type  string         `json:"type"`  // "redirect", "show_message", "update_status", etc.
	Data  map[string]any `json:"data"`  // Action-specific data
	Delay int            `json:"delay"` // Delay in milliseconds before executing
}

// Service defines the chat service interface
type Service interface {
	// StartSession creates a new chat session
	StartSession(ctx context.Context, userID string, req StartChatRequest) (*StartChatResponse, error)

	// SendMessage sends a message to a chat session
	SendMessage(ctx context.Context, userID, appointmentID, content string) error

	// HandleWebSocketSession handles a WebSocket chat session
	HandleWebSocketSession(ctx context.Context, session *ChatSession) error

	// EndSession ends a chat session
	EndSession(ctx context.Context, userID, appointmentID string) error

	// GetOrCreateSession gets an existing session or creates a new one
	GetOrCreateSession(ctx context.Context, userID string) (*StartChatResponse, error)

	// GetHistory returns the chat history for a session
	GetHistory(ctx context.Context, userID, appointmentID string, includeDebug bool) ([]internal.ChatMessage, error)

	// InjectTranscript injects a test transcript into a session (for testing)
	InjectTranscript(ctx context.Context, userID, appointmentID string, call vapi.Call) (int, error)
}

// Tool represents a chat tool that can be executed
type Tool struct {
	Definition openai.Tool
	Handler    func(ctx context.Context, session *ChatSession, args map[string]any) (*ToolResponseData, error)
}

// ToolRegistry manages available chat tools
type ToolRegistry interface {
	GetAllTools() []openai.Tool
	ExecuteTool(ctx context.Context, toolName string, args map[string]any, session *ChatSession) (*ToolResponseData, error)
}

// FirestoreClient interface for dependency injection and testing
type FirestoreClient interface {
	GetAppointment(ctx context.Context, userID, appointmentID string) (*internal.Appointment, error)
	UpdateAppointment(ctx context.Context, appointment *internal.Appointment) error
	CreateAppointment(ctx context.Context, userID string, appointment *internal.Appointment) error
	AddChatMessage(ctx context.Context, appointment *internal.Appointment, role, content string) error
	GetLatestRunningChatAppointment(ctx context.Context, userID string) (*internal.Appointment, bool, error)
	BuildAssistantOverrides(ctx context.Context, user *internal.User) (*vapi.Assistant, error)
	ProcessAppointmentStarted(ctx context.Context, user *internal.User, appointment *internal.Appointment) error
	GetUser(ctx context.Context, userID string) (*internal.User, bool, error)
	UpdateUserStatus(ctx context.Context, user *internal.User, status internal.UserStatus) error
}

// OpenAIClient interface for dependency injection and testing
type OpenAIClient interface {
	CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error)
}
