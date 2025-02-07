package match

import (
	"context"
	"errors"

	"github.com/chriscow/minds"
)

// mockLLMProvider implements the LLM provider interface for testing
type mockLLMProvider struct {
	normalizeResponse    string
	requirementsResponse string
	shouldError          bool
}

func (m *mockLLMProvider) GenerateContent(ctx context.Context, req minds.Request) (minds.Response, error) {
	if m.shouldError {
		return nil, errors.New("mock error")
	}

	// Return different responses based on the prompt
	if req.Messages[0].Content == NormalizePrompt {
		return &mockResponse{content: m.normalizeResponse}, nil
	} else if req.Messages[0].Content == RequirementsPrompt {
		return &mockResponse{content: m.requirementsResponse}, nil
	}
	return nil, errors.New("unknown prompt")
}

func (m *mockLLMProvider) ModelName() string {
	return "mock"
}

func (m *mockLLMProvider) Close() {
	return
}

type mockResponse struct {
	content string
}

func (m *mockResponse) String() string {
	return m.content
}

func (m *mockResponse) ToolCalls() []minds.ToolCall {
	return nil
}
