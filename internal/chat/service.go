package chat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"talent-rodeo/internal"
	"time"

	"github.com/sashabaranov/go-openai"
)

// Dependencies interfaces for dependency injection
type FirestoreClient interface {
	// Add methods as needed - we'll expand this as we extract more logic
	RunTransaction(ctx context.Context, f func(context.Context, interface{}) error) error
}

type OpenAIClient interface {
	CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error)
}

// ServiceImpl implements the ChatService interface
type ServiceImpl struct {
	fs           *internal.FirestoreClient // For now, use concrete type until we can extract interface
	openAI       OpenAIClient
	logger       *slog.Logger
	toolRegistry ToolRegistry
}

// NewService creates a new ChatService with injected dependencies
func NewService(fs *internal.FirestoreClient, openAI OpenAIClient, logger *slog.Logger) ChatService {
	service := &ServiceImpl{
		fs:     fs,
		openAI: openAI,
		logger: logger,
	}

	// Initialize tool registry with reference to service
	service.toolRegistry = NewToolRegistry(service, logger)

	return service
}

// StartSession creates a new chat session and returns initial response
func (s *ServiceImpl) StartSession(ctx context.Context, userID string, req StartChatRequest) (*StartChatResponse, error) {
	s.logger.Debug("Starting chat session", "user_id", userID, "timezone", req.Timezone)

	// Get user
	user, found, err := internal.GetUser(ctx, s.fs, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	// Build assistant overrides to get the first message
	assistantOverrides, err := internal.BuildAssistantOverrides(ctx, user, s.fs, s.logger)
	if err != nil {
		s.logger.Error("Failed to build assistant overrides for chat start", "error", err, "user_id", userID)
		// Continue with default message
	}

	firstMessage := "Hi! I'm Shirely. I'm excited to learn about your career preferences. What kind of work environment do you prefer?"
	if assistantOverrides != nil && assistantOverrides.FirstMessage != nil {
		// Adapt the phone-oriented message for chat
		firstMessage = *assistantOverrides.FirstMessage
		firstMessage = strings.Replace(firstMessage, "calling back", "coming back to chat", -1)
		firstMessage = strings.Replace(firstMessage, "Am I speaking with", "Am I chatting with", -1)
	}

	// Create a new chat appointment
	appointment := &internal.Appointment{
		Name:          "initial-chat",
		Type:          internal.AppointmentTypeChat,
		ScheduledTime: time.Now(),
		Timezone:      req.Timezone,
		Status:        internal.AppointmentStatusRunning,
		ChatMessages: []internal.ChatMessage{
			{
				Role:      "assistant",
				Content:   firstMessage,
				Timestamp: time.Now(),
			},
		},
		ConversationChecklist: internal.InitializeConversationChecklist(),
	}

	if err := internal.CreateAppointment(ctx, s.fs, userID, appointment); err != nil {
		return nil, fmt.Errorf("failed to create chat appointment: %w", err)
	}

	// Update user status to indicate chat started
	if err := internal.UpdateUserStatus(ctx, s.fs, user, internal.UserStatusCallStarted); err != nil {
		s.logger.Error("Failed to update user status for chat", "error", err, "user_id", userID)
		// Continue despite error - chat session is already created
	}

	s.logger.Info("Chat session started", "user_id", userID, "appointment_id", appointment.ID)

	return &StartChatResponse{
		AppointmentID: appointment.ID,
		Status:        "started",
		FirstMessage:  firstMessage,
	}, nil
}

// ProcessUserMessage handles a user message and saves it to the appointment
func (s *ServiceImpl) ProcessUserMessage(ctx context.Context, userID, appointmentID, message string) error {
	s.logger.Debug("Processing user message", "user_id", userID, "appointment_id", appointmentID)

	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}

	// Get the appointment
	appointment, err := internal.GetAppointment(ctx, s.fs, userID, appointmentID)
	if err != nil {
		return fmt.Errorf("failed to get appointment: %w", err)
	}

	if appointment.Type != internal.AppointmentTypeChat {
		return fmt.Errorf("appointment is not a chat type")
	}

	// Ensure checklist is initialized
	if appointment.ConversationChecklist == nil {
		s.logger.Warn("Chat appointment missing checklist, initializing", "user_id", userID, "appointment_id", appointmentID)
		appointment.ConversationChecklist = internal.InitializeConversationChecklist()
	}

	// Save user message to appointment
	if err := internal.AddChatMessage(ctx, s.fs, appointment, "user", message); err != nil {
		return fmt.Errorf("failed to save user message: %w", err)
	}

	// Increment message count in checklist
	internal.IncrementMessageCount(appointment.ConversationChecklist)

	// Check conversation length limits
	if err := s.checkConversationLimits(appointment); err != nil {
		return fmt.Errorf("conversation limit check failed: %w", err)
	}

	return nil
}

// StreamAIResponse generates and streams AI response via the provided channel
func (s *ServiceImpl) StreamAIResponse(ctx context.Context, userID, appointmentID string, msgChan chan<- StreamMessage) error {
	defer close(msgChan)

	s.logger.Debug("Streaming AI response", "user_id", userID, "appointment_id", appointmentID)

	// Get fresh appointment data
	appointment, err := s.getFreshAppointment(ctx, userID, appointmentID)
	if err != nil {
		s.sendError(msgChan, "Failed to get appointment data")
		return fmt.Errorf("failed to get appointment: %w", err)
	}

	// Get user
	user, found, err := internal.GetUser(ctx, s.fs, userID)
	if err != nil {
		s.sendError(msgChan, "Failed to get user data")
		return fmt.Errorf("failed to get user: %w", err)
	}
	if !found {
		s.sendError(msgChan, "User not found")
		return fmt.Errorf("user not found: %s", userID)
	}

	// Check for hard stop conditions
	if s.shouldForceCompletion(appointment) {
		return s.handleForcedCompletion(ctx, msgChan, user, appointment)
	}

	// Generate AI response
	return s.generateAIResponse(ctx, msgChan, user, appointment)
}

// EndSession ends a chat session and processes completion
func (s *ServiceImpl) EndSession(ctx context.Context, userID, appointmentID string) error {
	s.logger.Debug("Ending chat session", "user_id", userID, "appointment_id", appointmentID)

	// Get user and appointment
	user, found, err := internal.GetUser(ctx, s.fs, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if !found {
		return fmt.Errorf("user not found: %s", userID)
	}

	appointment, err := internal.GetAppointment(ctx, s.fs, userID, appointmentID)
	if err != nil {
		return fmt.Errorf("failed to get appointment: %w", err)
	}

	if appointment.Type != internal.AppointmentTypeChat {
		return fmt.Errorf("appointment is not a chat type")
	}

	// Mark appointment as processing for transcript processing
	if err := internal.ProcessAppointmentStarted(ctx, s.fs, user, appointment); err != nil {
		return fmt.Errorf("failed to start appointment processing: %w", err)
	}

	s.logger.Info("Chat session ended", "user_id", userID, "appointment_id", appointmentID)
	return nil
}

// GetHistory retrieves chat message history
func (s *ServiceImpl) GetHistory(ctx context.Context, userID, appointmentID string) ([]internal.ChatMessage, error) {
	s.logger.Debug("Getting chat history", "user_id", userID, "appointment_id", appointmentID)

	appointment, err := internal.GetAppointment(ctx, s.fs, userID, appointmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get appointment: %w", err)
	}

	if appointment.Type != internal.AppointmentTypeChat {
		return nil, fmt.Errorf("appointment is not a chat type")
	}

	return internal.GetChatHistory(appointment)
}

// FastForwardChat injects test transcript for testing (super user only)
func (s *ServiceImpl) FastForwardChat(ctx context.Context, userID, appointmentID string) error {
	// Implementation will be moved from handlers.chat.go
	// For now, return not implemented
	return fmt.Errorf("fast forward not yet implemented in service layer")
}

// Helper methods

// getFreshAppointment fetches appointment with retry logic for eventual consistency
func (s *ServiceImpl) getFreshAppointment(ctx context.Context, userID, appointmentID string) (*internal.Appointment, error) {
	maxRetries := 3
	for retry := 0; retry < maxRetries; retry++ {
		appointment, err := internal.GetAppointment(ctx, s.fs, userID, appointmentID)
		if err != nil {
			if retry < maxRetries-1 {
				time.Sleep(time.Duration(100*(1<<retry)) * time.Millisecond) // 100ms, 200ms, 400ms
				continue
			}
			return nil, err
		}
		return appointment, nil
	}
	return nil, fmt.Errorf("failed to get appointment after retries")
}

// checkConversationLimits validates conversation length and warns user
func (s *ServiceImpl) checkConversationLimits(appointment *internal.Appointment) error {
	if appointment.ConversationChecklist.MessageCount >= internal.MaxConversationLength-10 {
		s.logger.Warn("Conversation approaching length limit",
			"appointment_id", appointment.ID,
			"message_count", appointment.ConversationChecklist.MessageCount,
			"max_length", internal.MaxConversationLength)
	}
	return nil
}

// shouldForceCompletion checks if conversation should be forcibly completed
func (s *ServiceImpl) shouldForceCompletion(appointment *internal.Appointment) bool {
	return appointment.ConversationChecklist.MessageCount >= internal.MaxConversationLength
}

// handleForcedCompletion handles forced completion when limits are exceeded
func (s *ServiceImpl) handleForcedCompletion(ctx context.Context, msgChan chan<- StreamMessage, user *internal.User, appointment *internal.Appointment) error {
	s.logger.Warn("Conversation exceeded maximum length, forcing completion",
		"user_id", user.ID,
		"appointment_id", appointment.ID,
		"message_count", appointment.ConversationChecklist.MessageCount,
		"max_length", internal.MaxConversationLength)

	// Check if we can complete normally first
	canComplete, issues, err := internal.ValidateChecklistCompletion(appointment.ConversationChecklist)
	if err != nil {
		s.logger.Error("Failed to validate checklist during hard stop", "error", err)
	}

	session := &ChatSession{
		User:        user,
		Appointment: appointment,
		Service:     s,
	}

	var toolResponse *ToolResponse
	if canComplete {
		// Try normal completion
		args := map[string]any{
			"summary":                  "Conversation completed due to length limit",
			"confidence_level":         "medium",
			"next_step_recommendation": "review_profile",
		}
		toolResponse, err = s.toolRegistry.ExecuteTool(ctx, "complete_chat", args, session)
	} else {
		// Use override completion
		args := map[string]any{
			"reason":  fmt.Sprintf("Conversation exceeded maximum length (%d messages)", internal.MaxConversationLength),
			"summary": fmt.Sprintf("Conversation ended due to length limit. Issues: %s", strings.Join(issues, ", ")),
		}
		toolResponse, err = s.toolRegistry.ExecuteTool(ctx, "override_completion", args, session)
	}

	if err != nil {
		s.sendError(msgChan, "Failed to complete conversation")
		return fmt.Errorf("failed to execute completion tool: %w", err)
	}

	// Send tool response
	msgChan <- StreamMessage{
		Type:         "tool_response",
		ToolResponse: toolResponse,
	}

	return nil
}

// generateAIResponse handles the main AI response generation logic
func (s *ServiceImpl) generateAIResponse(ctx context.Context, msgChan chan<- StreamMessage, user *internal.User, appointment *internal.Appointment) error {
	// This will contain the main AI generation logic moved from the handlers
	// For now, return not implemented
	msgChan <- StreamMessage{
		Type:  "error",
		Error: "AI response generation not yet implemented in service layer",
	}
	return fmt.Errorf("AI response generation not yet implemented")
}

// sendError sends an error message via the channel
func (s *ServiceImpl) sendError(msgChan chan<- StreamMessage, message string) {
	msgChan <- StreamMessage{
		Type:  "error",
		Error: message,
	}
}
