package chat

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

// toolRegistry implements the ToolRegistry interface
type toolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() ToolRegistry {
	registry := &toolRegistry{
		tools: make(map[string]Tool),
	}

	// Register default tools (TODO: implement tool registration)

	return registry
}

// GetAllTools returns all available tools for OpenAI
func (r *toolRegistry) GetAllTools() []openai.Tool {
	tools := make([]openai.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool.Definition)
	}
	return tools
}

// ExecuteTool executes a specific tool with given arguments
func (r *toolRegistry) ExecuteTool(ctx context.Context, toolName string, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	tool, exists := r.tools[toolName]
	if !exists {
		return &ToolResponse{
			Name:    toolName,
			Content: "Tool not found",
			Success: false,
		}, nil
	}

	return tool.Handler(ctx, session, args)
}

// RegisterTool adds a new tool to the registry
func (r *toolRegistry) RegisterTool(name string, tool Tool) {
	r.tools[name] = tool
}
