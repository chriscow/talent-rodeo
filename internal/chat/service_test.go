package chat

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/sashabaranov/go-openai"
)

// MockOpenAIClient for testing
type MockOpenAIClient struct{}

func (m *MockOpenAIClient) CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	// Return a mock stream for testing
	return nil, nil
}

func TestNewService(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockOpenAI := &MockOpenAIClient{}

	// For now, pass nil for firestore client since we haven't fully extracted the interface yet
	// This test verifies our package structure compiles correctly
	service := NewService(nil, mockOpenAI, logger)

	if service == nil {
		t.Fatal("Expected service to be created, got nil")
	}

	// Verify we can access the service interface
	var _ ChatService = service
}

func TestServiceInterface(t *testing.T) {
	// This test verifies that our concrete type implements the interface correctly
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockOpenAI := &MockOpenAIClient{}

	service := NewService(nil, mockOpenAI, logger)

	// Verify we can access the service interface without calling methods that need real dependencies
	var _ ChatService = service

	// Test that our types can be created
	req := StartChatRequest{Timezone: "UTC"}
	if req.Timezone != "UTC" {
		t.Errorf("Expected timezone to be UTC, got %s", req.Timezone)
	}
}

func TestStreamMessage(t *testing.T) {
	// Test that our StreamMessage type can be created and used
	msg := StreamMessage{
		Type:    "content",
		Content: "test content",
	}

	if msg.Type != "content" {
		t.Errorf("Expected Type to be 'content', got %s", msg.Type)
	}

	if msg.Content != "test content" {
		t.Errorf("Expected Content to be 'test content', got %s", msg.Content)
	}
}

func TestToolResponseType(t *testing.T) {
	// Test that our ToolResponse type can be created
	response := ToolResponse{
		Name:    "test_tool",
		Content: "test response",
		Success: true,
	}

	if response.Name != "test_tool" {
		t.Errorf("Expected Name to be 'test_tool', got %s", response.Name)
	}

	if !response.Success {
		t.Error("Expected Success to be true, got false")
	}
}
