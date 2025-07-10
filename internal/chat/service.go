package chat

import (
	"context"
	"log/slog"
	"talent-rodeo/internal"

	"github.com/sashabaranov/go-openai"
)

// service implements the Service interface
type service struct {
	fs           FirestoreClient
	openAI       OpenAIClient
	logger       *slog.Logger
	toolRegistry ToolRegistry
}

// NewService creates a new chat service with injected dependencies
func NewService(fs FirestoreClient, openAI OpenAIClient, logger *slog.Logger) Service {
	return &service{
		fs:           fs,
		openAI:       openAI,
		logger:       logger,
		toolRegistry: NewToolRegistry(),
	}
}

// StartSession creates a new chat session
func (s *service) StartSession(ctx context.Context, userID string, req StartChatRequest) (*StartChatResponse, error) {
	// TODO: Implement session start logic
	// This will be moved from handlers.chat.go
	return nil, nil
}

// ProcessUserMessage handles a user message and prepares for AI response
func (s *service) ProcessUserMessage(ctx context.Context, userID, appointmentID, message string) (*ChatSession, error) {
	// TODO: Implement user message processing logic
	// This will be moved from handlers.chat.go
	return nil, nil
}

// StreamAIResponse generates and streams AI response
func (s *service) StreamAIResponse(ctx context.Context, session *ChatSession, msgChan chan<- StreamMessage) error {
	// TODO: Implement AI response streaming logic
	// This will be moved from handlers.chat.go
	return nil
}

// EndSession ends a chat session
func (s *service) EndSession(ctx context.Context, userID, appointmentID string) error {
	// TODO: Implement session end logic
	// This will be moved from handlers.chat.go
	return nil
}

// GetHistory retrieves chat history
func (s *service) GetHistory(ctx context.Context, userID, appointmentID string, includeDebug bool) ([]internal.ChatMessage, error) {
	// TODO: Implement history retrieval logic
	// This will be moved from handlers.chat.go
	return nil, nil
}

// FastForward injects test transcript (admin only)
func (s *service) FastForward(ctx context.Context, userID, appointmentID string) error {
	// TODO: Implement fast forward logic
	// This will be moved from handlers.chat.go
	return nil
}

// realOpenAIClient is an adapter to make the real OpenAI client implement our interface
type realOpenAIClient struct {
	client *openai.Client
}

// NewRealOpenAIClient creates a wrapper around the real OpenAI client
func NewRealOpenAIClient(client *openai.Client) OpenAIClient {
	return &realOpenAIClient{client: client}
}

// CreateChatCompletionStream implements our OpenAIClient interface
func (c *realOpenAIClient) CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	return c.client.CreateChatCompletionStream(ctx, request)
}
