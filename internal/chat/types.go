package chat

import (
	"context"
	"talent-rodeo/internal"
	"time"

	"github.com/chriscow/vapi-go"
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

// StreamMessage represents different types of messages sent via WebSocket during streaming
type StreamMessage struct {
	Type         string        `json:"type"`
	Content      string        `json:"content,omitempty"`
	Error        string        `json:"error,omitempty"`
	MessageID    string        `json:"message_id,omitempty"`
	ToolCall     *ToolCallData `json:"tool_call,omitempty"`
	ToolResponse *ToolResponse `json:"tool_response,omitempty"`
}

// ToolCallData represents a tool call made by the LLM
type ToolCallData struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolResponse represents the response from a tool execution
type ToolResponse struct {
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

// ChatSession represents the runtime state of a chat session
type ChatSession struct {
	User        *internal.User
	Appointment *internal.Appointment
	UpdatedAt   time.Time
}

// Service defines the interface for chat business logic
type Service interface {
	// StartSession creates a new chat session
	StartSession(ctx context.Context, userID string, req StartChatRequest) (*StartChatResponse, error)

	// ProcessUserMessage handles a user message and prepares for AI response
	ProcessUserMessage(ctx context.Context, userID, appointmentID, message string) (*ChatSession, error)

	// StreamAIResponse generates and streams AI response
	StreamAIResponse(ctx context.Context, session *ChatSession, msgChan chan<- StreamMessage) error

	// EndSession ends a chat session
	EndSession(ctx context.Context, userID, appointmentID string) error

	// GetHistory retrieves chat history
	GetHistory(ctx context.Context, userID, appointmentID string, includeDebug bool) ([]internal.ChatMessage, error)

	// FastForward injects test transcript (admin only)
	FastForward(ctx context.Context, userID, appointmentID string) error
}

// ToolRegistry defines the interface for managing chat tools
type ToolRegistry interface {
	// GetAllTools returns all available tools for OpenAI
	GetAllTools() []openai.Tool

	// ExecuteTool executes a specific tool with given arguments
	ExecuteTool(ctx context.Context, toolName string, args map[string]any, session *ChatSession) (*ToolResponse, error)

	// RegisterTool adds a new tool to the registry
	RegisterTool(name string, tool Tool)
}

// Tool represents a single chat tool
type Tool struct {
	Definition openai.Tool
	Handler    ToolHandler
}

// ToolHandler is the function signature for tool execution
type ToolHandler func(ctx context.Context, session *ChatSession, args map[string]any) (*ToolResponse, error)

// OpenAIClient interface for dependency injection and testing
type OpenAIClient interface {
	CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error)
}

// FirestoreClient interface for dependency injection and testing
type FirestoreClient interface {
	GetAppointment(ctx context.Context, userID, appointmentID string) (*internal.Appointment, error)
	UpdateAppointment(ctx context.Context, appointment *internal.Appointment) error
	AddChatMessage(ctx context.Context, appointment *internal.Appointment, role, content string) error
	BuildAssistantOverrides(ctx context.Context, user *internal.User) (*vapi.Assistant, error)
	ProcessAppointmentStarted(ctx context.Context, user *internal.User, appointment *internal.Appointment) error
}
