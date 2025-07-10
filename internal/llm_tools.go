package internal

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/sashabaranov/go-openai"
)

// LLMToolRegistry defines the interface for managing and executing LLM tools
type LLMToolRegistry interface {
	// GetAllTools returns all registered tools as OpenAI tool definitions
	GetAllTools() []openai.Tool

	// ExecuteTool executes a tool by name with the given arguments and session context
	ExecuteTool(ctx context.Context, toolName string, args map[string]any, session LLMToolSession) (*LLMToolResponse, error)

	// RegisterTool adds a new tool to the registry
	RegisterTool(name string, tool LLMTool) error

	// ListToolNames returns the names of all registered tools
	ListToolNames() []string
}

// LLMTool represents a single tool that can be called by an LLM
type LLMTool interface {
	// GetDefinition returns the OpenAI tool definition
	GetDefinition() openai.Tool

	// Execute runs the tool with the given arguments and session context
	Execute(ctx context.Context, args map[string]any, session LLMToolSession) (*LLMToolResponse, error)
}

// LLMToolSession represents the context for an LLM tool execution
// This is a generic interface that specific implementations can extend
type LLMToolSession interface {
	// GetUserID returns the user ID for this session
	GetUserID() string

	// GetSessionID returns a unique session identifier
	GetSessionID() string

	// GetLogger returns the logger for this session
	GetLogger() *slog.Logger
}

// LLMToolResponse represents the response from a tool execution
type LLMToolResponse struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Content   string            `json:"content"`
	Success   bool              `json:"success"`
	UIActions []LLMToolUIAction `json:"ui_actions,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
}

// LLMToolUIAction represents an action for the client to perform
type LLMToolUIAction struct {
	Type  string         `json:"type"`  // "redirect", "show_message", "update_status", etc.
	Data  map[string]any `json:"data"`  // Action-specific data
	Delay int            `json:"delay"` // Delay in milliseconds before executing
}

// LLMToolRegistryImpl implements the LLMToolRegistry interface
type LLMToolRegistryImpl struct {
	tools  map[string]LLMTool
	logger *slog.Logger
	name   string // Registry name for logging/debugging
}

// NewLLMToolRegistry creates a new tool registry
func NewLLMToolRegistry(name string, logger *slog.Logger) LLMToolRegistry {
	return &LLMToolRegistryImpl{
		tools:  make(map[string]LLMTool),
		logger: logger,
		name:   name,
	}
}

// RegisterTool adds a new tool to the registry
func (r *LLMToolRegistryImpl) RegisterTool(name string, tool LLMTool) error {
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if tool == nil {
		return fmt.Errorf("tool cannot be nil")
	}

	r.logger.Debug("Registering LLM tool", "registry", r.name, "tool_name", name)
	r.tools[name] = tool
	return nil
}

// GetAllTools returns all registered tools as OpenAI tool definitions
func (r *LLMToolRegistryImpl) GetAllTools() []openai.Tool {
	tools := make([]openai.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool.GetDefinition())
	}
	return tools
}

// ExecuteTool executes a tool by name with the given arguments
func (r *LLMToolRegistryImpl) ExecuteTool(ctx context.Context, toolName string, args map[string]any, session LLMToolSession) (*LLMToolResponse, error) {
	tool, exists := r.tools[toolName]
	if !exists {
		return &LLMToolResponse{
			Name:    toolName,
			Content: "Tool not found",
			Success: false,
		}, fmt.Errorf("tool '%s' not found in registry '%s'", toolName, r.name)
	}

	r.logger.Debug("Executing LLM tool", "registry", r.name, "tool_name", toolName, "user_id", session.GetUserID())
	return tool.Execute(ctx, args, session)
}

// ListToolNames returns the names of all registered tools
func (r *LLMToolRegistryImpl) ListToolNames() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// BaseLLMTool provides common functionality for all LLM tools
type BaseLLMTool struct {
	logger *slog.Logger
}

// NewBaseLLMTool creates a new base tool with the given logger
func NewBaseLLMTool(logger *slog.Logger) BaseLLMTool {
	return BaseLLMTool{logger: logger}
}

// Helper function to safely extract string from args with validation
func (t *BaseLLMTool) ExtractString(args map[string]any, key string, required bool) (string, error) {
	value, exists := args[key]
	if !exists {
		if required {
			return "", fmt.Errorf("required parameter '%s' not found", key)
		}
		return "", nil
	}

	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("parameter '%s' must be a string, got %T", key, value)
	}

	return str, nil
}

// Helper function to safely extract map from args with validation
func (t *BaseLLMTool) ExtractMap(args map[string]any, key string, required bool) (map[string]any, error) {
	value, exists := args[key]
	if !exists {
		if required {
			return nil, fmt.Errorf("required parameter '%s' not found", key)
		}
		return nil, nil
	}

	mapVal, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parameter '%s' must be an object, got %T", key, value)
	}

	return mapVal, nil
}

// Helper function to safely extract array from args with validation
func (t *BaseLLMTool) ExtractArray(args map[string]any, key string, required bool) ([]any, error) {
	value, exists := args[key]
	if !exists {
		if required {
			return nil, fmt.Errorf("required parameter '%s' not found", key)
		}
		return nil, nil
	}

	arr, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("parameter '%s' must be an array, got %T", key, value)
	}

	return arr, nil
}

// RedactSensitiveInfo removes or masks sensitive information from tool arguments for logging
func RedactSensitiveInfo(args map[string]any) map[string]any {
	redacted := make(map[string]any)

	for key, value := range args {
		switch strings.ToLower(key) {
		case "salary_range", "salary", "compensation", "pay":
			redacted[key] = "[REDACTED]"
		case "location", "address", "city", "state", "zip", "zipcode":
			redacted[key] = "[REDACTED]"
		case "phone", "phone_number", "email", "email_address":
			redacted[key] = "[REDACTED]"
		case "name", "first_name", "last_name", "full_name":
			redacted[key] = "[REDACTED]"
		case "ssn", "social_security_number", "tax_id":
			redacted[key] = "[REDACTED]"
		case "notes", "summary", "extracted_info":
			// For notes and summaries, truncate if too long
			if str, ok := value.(string); ok && len(str) > 100 {
				redacted[key] = str[:100] + "...[TRUNCATED]"
			} else {
				redacted[key] = value
			}
		default:
			// Keep other values as-is
			redacted[key] = value
		}
	}

	return redacted
}

// Helper function to create standard UI actions
func CreateRedirectAction(url string, delay int) LLMToolUIAction {
	return LLMToolUIAction{
		Type: "redirect",
		Data: map[string]any{
			"url": url,
		},
		Delay: delay,
	}
}

func CreateMessageAction(message, messageType string, delay int) LLMToolUIAction {
	return LLMToolUIAction{
		Type: "show_message",
		Data: map[string]any{
			"message": message,
			"type":    messageType,
		},
		Delay: delay,
	}
}

func CreateStatusUpdateAction(status, statusType string, delay int) LLMToolUIAction {
	return LLMToolUIAction{
		Type: "update_status",
		Data: map[string]any{
			"status": status,
			"type":   statusType,
		},
		Delay: delay,
	}
}
