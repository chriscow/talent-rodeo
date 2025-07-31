package internal

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
)

// Mock implementations for testing

type MockLLMTool struct {
	name       string
	definition openai.Tool
	response   *LLMToolResponse
	err        error
}

func (m *MockLLMTool) GetDefinition() openai.Tool {
	return m.definition
}

func (m *MockLLMTool) Execute(ctx context.Context, args map[string]any, session LLMToolSession) (*LLMToolResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

type MockLLMToolSession struct {
	userID    string
	sessionID string
	logger    *slog.Logger
}

func (m *MockLLMToolSession) GetUserID() string {
	return m.userID
}

func (m *MockLLMToolSession) GetSessionID() string {
	return m.sessionID
}

func (m *MockLLMToolSession) GetLogger() *slog.Logger {
	return m.logger
}

func TestNewLLMToolRegistry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	registry := NewLLMToolRegistry("test", logger)

	if registry == nil {
		t.Fatal("Expected registry to be created, got nil")
	}

	// Initially should have no tools
	tools := registry.GetAllTools()
	if len(tools) != 0 {
		t.Errorf("Expected 0 tools, got %d", len(tools))
	}

	names := registry.ListToolNames()
	if len(names) != 0 {
		t.Errorf("Expected 0 tool names, got %d", len(names))
	}
}

func TestLLMToolRegistryRegisterTool(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	registry := NewLLMToolRegistry("test", logger)

	// Create a mock tool
	mockTool := &MockLLMTool{
		name: "test_tool",
		definition: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "test_tool",
				Description: "A test tool",
			},
		},
		response: &LLMToolResponse{
			Name:    "test_tool",
			Content: "Test response",
			Success: true,
		},
	}

	// Register the tool
	err := registry.RegisterTool("test_tool", mockTool)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	// Check that tool was registered
	tools := registry.GetAllTools()
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}

	names := registry.ListToolNames()
	if len(names) != 1 {
		t.Errorf("Expected 1 tool name, got %d", len(names))
	}
	if names[0] != "test_tool" {
		t.Errorf("Expected tool name 'test_tool', got '%s'", names[0])
	}
}

func TestLLMToolRegistryExecuteTool(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	registry := NewLLMToolRegistry("test", logger)

	// Create a mock tool
	expectedResponse := &LLMToolResponse{
		Name:    "test_tool",
		Content: "Test response",
		Success: true,
	}

	mockTool := &MockLLMTool{
		name:       "test_tool",
		definition: openai.Tool{},
		response:   expectedResponse,
	}

	// Register the tool
	err := registry.RegisterTool("test_tool", mockTool)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	// Create a mock session
	session := &MockLLMToolSession{
		userID:    "test-user",
		sessionID: "test-session",
		logger:    logger,
	}

	// Execute the tool
	ctx := context.Background()
	args := map[string]any{"test": "value"}
	response, err := registry.ExecuteTool(ctx, "test_tool", args, session)

	if err != nil {
		t.Fatalf("Failed to execute tool: %v", err)
	}

	if response.Name != expectedResponse.Name {
		t.Errorf("Expected response name '%s', got '%s'", expectedResponse.Name, response.Name)
	}
	if response.Content != expectedResponse.Content {
		t.Errorf("Expected response content '%s', got '%s'", expectedResponse.Content, response.Content)
	}
	if response.Success != expectedResponse.Success {
		t.Errorf("Expected response success %t, got %t", expectedResponse.Success, response.Success)
	}
}

func TestLLMToolRegistryExecuteNonexistentTool(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	registry := NewLLMToolRegistry("test", logger)

	session := &MockLLMToolSession{
		userID:    "test-user",
		sessionID: "test-session",
		logger:    logger,
	}

	ctx := context.Background()
	args := map[string]any{}
	response, err := registry.ExecuteTool(ctx, "nonexistent_tool", args, session)

	if err == nil {
		t.Fatal("Expected error for nonexistent tool, got nil")
	}

	if response == nil {
		t.Fatal("Expected response for nonexistent tool, got nil")
	}

	if response.Success {
		t.Error("Expected response.Success to be false for nonexistent tool")
	}
}

func TestBaseLLMToolExtractString(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	tool := NewBaseLLMTool(logger)

	args := map[string]any{
		"string_param": "test_value",
		"int_param":    42,
	}

	// Test required parameter that exists
	value, err := tool.ExtractString(args, "string_param", true)
	if err != nil {
		t.Fatalf("Failed to extract string parameter: %v", err)
	}
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}

	// Test optional parameter that doesn't exist
	value, err = tool.ExtractString(args, "missing_param", false)
	if err != nil {
		t.Fatalf("Unexpected error for optional missing parameter: %v", err)
	}
	if value != "" {
		t.Errorf("Expected empty string for missing optional parameter, got '%s'", value)
	}

	// Test required parameter that doesn't exist
	_, err = tool.ExtractString(args, "missing_param", true)
	if err == nil {
		t.Fatal("Expected error for required missing parameter, got nil")
	}

	// Test parameter with wrong type
	_, err = tool.ExtractString(args, "int_param", true)
	if err == nil {
		t.Fatal("Expected error for wrong parameter type, got nil")
	}
}

func TestRedactSensitiveInfo(t *testing.T) {
	args := map[string]any{
		"name":         "John Doe",
		"email":        "john@example.com",
		"salary_range": "100000-150000",
		"public_info":  "This is public",
		"notes":        "This is a very long note that should be truncated because it exceeds the maximum length allowed for display in logs and should show the truncation marker",
	}

	redacted := RedactSensitiveInfo(args)

	// Check that sensitive info is redacted
	if redacted["name"] != "[REDACTED]" {
		t.Errorf("Expected name to be redacted, got '%v'", redacted["name"])
	}
	if redacted["email"] != "[REDACTED]" {
		t.Errorf("Expected email to be redacted, got '%v'", redacted["email"])
	}
	if redacted["salary_range"] != "[REDACTED]" {
		t.Errorf("Expected salary_range to be redacted, got '%v'", redacted["salary_range"])
	}

	// Check that public info is preserved
	if redacted["public_info"] != "This is public" {
		t.Errorf("Expected public_info to be preserved, got '%v'", redacted["public_info"])
	}

	// Check that long notes are truncated
	if str, ok := redacted["notes"].(string); ok {
		if len(str) <= 100 {
			t.Errorf("Expected notes to be truncated, but got string of length %d", len(str))
		}
		if !strings.Contains(str, "[TRUNCATED]") {
			t.Errorf("Expected notes to contain truncation marker, got '%s'", str)
		}
	} else {
		t.Errorf("Expected notes to be a string, got %T", redacted["notes"])
	}
}
