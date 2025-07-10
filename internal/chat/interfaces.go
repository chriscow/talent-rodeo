package chat

import (
	"context"
	"talent-rodeo/internal"

	"github.com/sashabaranov/go-openai"
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

// ToolRegistry defines the interface for managing and executing chat tools
type ToolRegistry interface {
	// GetAllTools returns all registered tools as OpenAI tool definitions
	GetAllTools() []openai.Tool

	// ExecuteTool executes a tool by name with the given arguments
	ExecuteTool(ctx context.Context, toolName string, args map[string]any, session *ChatSession) (*ToolResponse, error)

	// RegisterTool adds a new tool to the registry
	RegisterTool(name string, tool Tool) error
}

// Tool represents a single chat tool that can be called by the LLM
type Tool interface {
	// GetDefinition returns the OpenAI tool definition
	GetDefinition() openai.Tool

	// Execute runs the tool with the given arguments and session context
	Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error)
}

// ChatSession represents the context for a chat session, passed to tools
type ChatSession struct {
	User        *internal.User
	Appointment *internal.Appointment
	Service     ChatService // Reference back to service for database operations
}

// StreamMessage represents a message chunk sent via WebSocket streaming
type StreamMessage struct {
	Type         string        `json:"type"`    // "content", "tool_response", "done", "error"
	Content      string        `json:"content"` // Text content for streaming
	Error        string        `json:"error,omitempty"`
	ToolResponse *ToolResponse `json:"tool_response,omitempty"`
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
	Type  string                 `json:"type"`  // "redirect", "show_message", "update_status", etc.
	Data  map[string]interface{} `json:"data"`  // Action-specific data
	Delay int                    `json:"delay"` // Delay in milliseconds before executing
}
