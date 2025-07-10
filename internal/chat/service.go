package chat

import (
	"context"
	"log/slog"
	"strings"
	"talent-rodeo/internal"
	"time"

	"github.com/chriscow/vapi-go"
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
	// Get user for assistant overrides
	user, found, err := s.fs.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, internal.ErrUserNotFound
	}

	// Build assistant overrides to get the first message
	assistantOverrides, err := s.fs.BuildAssistantOverrides(ctx, user)
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

	if err := s.fs.CreateAppointment(ctx, userID, appointment); err != nil {
		return nil, err
	}

	// Update user status to indicate chat started
	if err := s.fs.UpdateUserStatus(ctx, user, internal.UserStatusCallStarted); err != nil {
		s.logger.Error("Failed to update user status for chat", "error", err, "user_id", userID)
		// Continue despite error - chat session is already created
	}

	return &StartChatResponse{
		AppointmentID: appointment.ID,
		Status:        "started",
		FirstMessage:  firstMessage,
	}, nil
}

// SendMessage sends a message to a chat session
func (s *service) SendMessage(ctx context.Context, userID, appointmentID, content string) error {
	// Get the appointment
	appointment, err := s.fs.GetAppointment(ctx, userID, appointmentID)
	if err != nil {
		return err
	}

	if appointment.Type != internal.AppointmentTypeChat {
		return internal.ErrInvalidAppointmentType
	}

	// Add the user's message
	return s.fs.AddChatMessage(ctx, appointment, "user", content)
}

// HandleWebSocketSession handles a WebSocket chat session
func (s *service) HandleWebSocketSession(ctx context.Context, session *ChatSession) error {
	// Send connection confirmation
	confirmMsg := StreamMessage{
		Type:    "connected",
		Content: "WebSocket connection established",
	}
	if err := session.Connection.WriteJSON(confirmMsg); err != nil {
		s.logger.Error("Failed to send connection confirmation", "error", err)
		return err
	}

	// Handle WebSocket messages
	for {
		var msg StreamMessage
		err := session.Connection.ReadJSON(&msg)
		if err != nil {
			s.logger.Warn("WebSocket read error", "error", err)
			break
		}

		switch msg.Type {
		case "user_message":
			// Process user message
			go s.handleWebSocketUserMessage(ctx, session, msg.Content)
		case "ping":
			// Respond to ping with pong
			pong := StreamMessage{Type: "pong"}
			session.Connection.WriteJSON(pong)
		default:
			s.logger.Warn("Unknown WebSocket message type", "type", msg.Type, "user_id", session.UserID)
		}
	}

	return nil
}

// EndSession ends a chat session
func (s *service) EndSession(ctx context.Context, userID, appointmentID string) error {
	// Get the appointment
	appointment, err := s.fs.GetAppointment(ctx, userID, appointmentID)
	if err != nil {
		return err
	}

	if appointment.Type != internal.AppointmentTypeChat {
		return internal.ErrInvalidAppointmentType
	}

	// Get user for processing
	user, found, err := s.fs.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if !found {
		return internal.ErrUserNotFound
	}

	// Mark appointment as processing for transcript processing
	return s.fs.ProcessAppointmentStarted(ctx, user, appointment)
}

// GetOrCreateSession gets an existing session or creates a new one
func (s *service) GetOrCreateSession(ctx context.Context, userID string) (*StartChatResponse, error) {
	// Try to find the latest running chat appointment
	appointment, found, err := s.fs.GetLatestRunningChatAppointment(ctx, userID)
	if err != nil {
		return nil, err
	}

	if found {
		// Return existing session
		firstMessage := ""
		if len(appointment.ChatMessages) > 0 {
			firstMessage = appointment.ChatMessages[0].Content
		}

		return &StartChatResponse{
			AppointmentID: appointment.ID,
			Status:        "existing",
			FirstMessage:  firstMessage,
		}, nil
	}

	// No running session found, create a new one
	return s.StartSession(ctx, userID, StartChatRequest{
		Timezone: "UTC", // Default timezone, could be improved with user preferences
	})
}

// GetHistory returns the chat history for a session
func (s *service) GetHistory(ctx context.Context, userID, appointmentID string, includeDebug bool) ([]internal.ChatMessage, error) {
	// Get the appointment
	appointment, err := s.fs.GetAppointment(ctx, userID, appointmentID)
	if err != nil {
		return nil, err
	}

	if appointment.Type != internal.AppointmentTypeChat {
		return nil, internal.ErrInvalidAppointmentType
	}

	// Get chat history
	messages, err := internal.GetChatHistory(appointment)
	if err != nil {
		return nil, err
	}

	// Filter debug messages for non-debug users
	if !includeDebug {
		var filteredMessages []internal.ChatMessage
		for _, message := range messages {
			// Skip debug messages
			if isDebugMessage(message.Role) || isDebugMessageContent(message.Role, message.Content) {
				continue
			}
			filteredMessages = append(filteredMessages, message)
		}
		return filteredMessages, nil
	}

	return messages, nil
}

// InjectTranscript injects a test transcript into a session (for testing)
func (s *service) InjectTranscript(ctx context.Context, userID, appointmentID string, call vapi.Call) (int, error) {
	// Get the appointment
	appointment, err := s.fs.GetAppointment(ctx, userID, appointmentID)
	if err != nil {
		return 0, err
	}

	if appointment.Type != internal.AppointmentTypeChat {
		return 0, internal.ErrInvalidAppointmentType
	}

	// Extract transcript messages from the call
	var messages []internal.ChatMessage
	if call.Artifact != nil && call.Artifact.Messages != nil {
		s.logger.Info("Fast-forward: Processing call artifact", "artifact_messages_count", len(call.Artifact.Messages))

		tx := internal.NewTranscriptFromCallResult(call)
		s.logger.Info("Fast-forward: Transcript processed", "transcript_messages_count", len(tx.Messages))

		for i, msg := range tx.Messages {
			s.logger.Debug("Fast-forward: Processing message", "index", i, "role", msg.Role, "content_length", len(msg.Content))

			// Skip messages with empty content
			if msg.Content == "" {
				s.logger.Debug("Fast-forward: Skipping empty message", "index", i)
				continue
			}

			chatMessage := internal.ChatMessage{
				Role:      string(msg.Role),
				Content:   msg.Content,
				Timestamp: time.Now(),
			}
			messages = append(messages, chatMessage)
		}
	} else {
		s.logger.Error("Fast-forward: No artifact or messages found", "has_artifact", call.Artifact != nil)
		return 0, internal.ErrNoMessagesFound
	}

	if len(messages) == 0 {
		return 0, internal.ErrNoMessagesFound
	}

	// Clear existing messages and inject the new ones
	appointment.ChatMessages = messages

	// Update the conversation checklist to reflect that topics have been covered
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = internal.InitializeConversationChecklist()
	}

	// Mark all major topics as completed since we're injecting a full transcript
	topicsToComplete := []string{
		"introduction", "warm_up", "deal_breakers", "search_strategy",
		"work_environment", "location_preferences", "final_confirmation",
	}

	for _, topicID := range topicsToComplete {
		extractedInfo := map[string]any{
			"source":         "transcript_injection",
			"auto_populated": true,
			"timestamp":      time.Now(),
		}

		err := internal.UpdateChecklistItem(
			appointment.ConversationChecklist,
			topicID,
			internal.ChecklistItemStatusCompleted,
			internal.ChecklistItemQualityMedium,
			extractedInfo,
			"Auto-populated from injected transcript for testing",
		)
		if err != nil {
			s.logger.Error("Failed to update checklist item", "error", err, "topic_id", topicID)
			// Continue with other topics even if one fails
		}
	}

	// Update the checklist message count to reflect the injected messages
	appointment.ConversationChecklist.MessageCount = len(appointment.ChatMessages)
	appointment.ConversationChecklist.LastUpdated = time.Now()

	// Mark the overall status as ready for completion since we've covered all topics
	appointment.ConversationChecklist.OverallStatus = internal.ChecklistStatusReadyForCompletion

	// Update the appointment with the new messages and checklist
	if err := s.fs.UpdateAppointment(ctx, appointment); err != nil {
		return 0, err
	}

	return len(messages), nil
}

// handleWebSocketUserMessage processes a user message from WebSocket
func (s *service) handleWebSocketUserMessage(ctx context.Context, session *ChatSession, userMessage string) {
	// TODO: Implement the actual WebSocket message handling logic
	// This would include:
	// 1. Saving the user message
	// 2. Generating AI response
	// 3. Streaming the response back via WebSocket
	// 4. Handling tool calls

	s.logger.Info("Processing WebSocket user message", "user_id", session.UserID, "appointment_id", session.AppointmentID, "message", userMessage)

	// For now, just acknowledge the message
	response := StreamMessage{
		Type:    "content",
		Content: "Message received: " + userMessage,
	}
	session.Connection.WriteJSON(response)

	// Send done message
	done := StreamMessage{
		Type: "done",
	}
	session.Connection.WriteJSON(done)
}

// Helper functions

// isDebugMessage checks if a message role is considered a debug message
func isDebugMessage(role string) bool {
	return role == openai.ChatMessageRoleTool || role == openai.ChatMessageRoleSystem
}

// isDebugMessageContent checks if message content indicates a debug message (tool calls)
func isDebugMessageContent(role, content string) bool {
	return role == openai.ChatMessageRoleAssistant && strings.HasPrefix(content, "TOOL_CALLS:")
}
