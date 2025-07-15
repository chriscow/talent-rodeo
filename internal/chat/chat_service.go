package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"talent-rodeo/cmd/web/simulate"
	"talent-rodeo/internal"
	"time"

	"github.com/chriscow/vapi-go"
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
	toolRegistry internal.LLMToolRegistry // Use the generic LLM tool registry
	serviceDeps  *internal.ServiceDeps    // Dependencies for completion processing
}

// NewService creates a new ChatService with injected dependencies
func NewService(fs *internal.FirestoreClient, openAI OpenAIClient, serviceDeps *internal.ServiceDeps, logger *slog.Logger) ChatService {
	service := &ServiceImpl{
		fs:          fs,
		openAI:      openAI,
		logger:      logger,
		serviceDeps: serviceDeps,
	}

	// Initialize tool registry with chat-specific tools
	service.toolRegistry = internal.NewLLMToolRegistry("chat", logger)
	service.registerChatTools()

	return service
}

// registerChatTools registers chat-specific tools with the registry
func (s *ServiceImpl) registerChatTools() {
	// Register all the chat-specific tools
	s.toolRegistry.RegisterTool("complete_chat", NewCompleteChatTool(s.logger))
	s.toolRegistry.RegisterTool("update_user_preferences", NewUpdateUserPreferencesTool(s.logger))
	s.toolRegistry.RegisterTool("schedule_followup", NewScheduleFollowupTool(s.logger))
	s.toolRegistry.RegisterTool("get_checklist_status", NewGetChecklistStatusTool(s.logger))
	s.toolRegistry.RegisterTool("mark_topic_progress", NewMarkTopicProgressTool(s.logger))
	s.toolRegistry.RegisterTool("validate_information_quality", NewValidateInformationQualityTool(s.logger))
	s.toolRegistry.RegisterTool("check_completion_readiness", NewCheckCompletionReadinessTool(s.logger))
	s.toolRegistry.RegisterTool("override_completion", NewOverrideCompletionTool(s.logger))
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

	if user.Timezone != "" {
		internal.UpdateUserTimezone(ctx, s.fs, user, user.Timezone)
	}

	// Create a new chat appointment
	appointment := &internal.Appointment{
		Name:          "initial-chat",
		Type:          internal.AppointmentTypeChat,
		ScheduledTime: time.Now(),
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

// FastForwardChat injects test transcript data into a chat session for testing
func (s *ServiceImpl) FastForwardChat(ctx context.Context, userID, appointmentID string) error {
	// Get the appointment
	appointment, err := internal.GetAppointment(ctx, s.fs, userID, appointmentID)
	if err != nil {
		return fmt.Errorf("failed to get appointment for fast-forward: %w", err)
	}

	if appointment.Type != internal.AppointmentTypeChat {
		return fmt.Errorf("appointment is not a chat type")
	}

	// Load test call data using the TestingCallID
	s.logger.Info("Loading test call data for fast-forward", "appointment_id", appointmentID, "test_call_id", simulate.TestingCallID)

	call, err := vapi.GetCall(ctx, simulate.TestingCallID)
	if err != nil {
		return fmt.Errorf("failed to get test call data: %w", err)
	}

	// Extract transcript messages from the call
	var messages []internal.ChatMessage
	if call.Artifact != nil && call.Artifact.Messages != nil {
		s.logger.Info("Fast-forward: Processing call artifact", "artifact_messages_count", len(call.Artifact.Messages))

		tx := internal.NewTranscriptFromCallResult(*call)
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
			contentPreview := chatMessage.Content
			if len(contentPreview) > 50 {
				contentPreview = contentPreview[:50] + "..."
			}
			s.logger.Debug("Fast-forward: Added message", "index", i, "role", chatMessage.Role, "content_preview", contentPreview)
		}
	} else {
		return fmt.Errorf("no artifact or messages found in test call")
	}

	s.logger.Info("Fast-forward: Extracted messages", "count", len(messages))
	if len(messages) == 0 {
		return fmt.Errorf("no messages found in test call")
	}

	// Inject the messages into the current chat appointment
	s.logger.Info("Injecting transcript messages into chat", "appointment_id", appointmentID, "message_count", len(messages))

	// Clear existing messages first
	s.logger.Info("Fast-forward: Current chat messages before clearing", "count", len(appointment.ChatMessages))
	if len(appointment.ChatMessages) > 0 {
		appointment.ChatMessages = appointment.ChatMessages[:0] // Remove all messages
		s.logger.Info("Fast-forward: Cleared messages", "remaining_count", len(appointment.ChatMessages))
	}

	// Add the transcript messages
	s.logger.Info("Fast-forward: Adding transcript messages", "current_count", len(appointment.ChatMessages), "adding_count", len(messages))
	appointment.ChatMessages = append(appointment.ChatMessages, messages...)
	s.logger.Info("Fast-forward: Messages after adding transcript", "total_count", len(appointment.ChatMessages))

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
	s.logger.Info("Fast-forward: Saving appointment", "appointment_id", appointmentID, "final_message_count", len(appointment.ChatMessages))
	if err := internal.UpdateAppointment(ctx, s.fs, appointment); err != nil {
		return fmt.Errorf("failed to update appointment with transcript: %w", err)
	}

	// Verify the write was successful by reading it back
	s.logger.Info("Verifying appointment write...", "appointment_id", appointmentID)
	verifyAppointment, err := internal.GetAppointment(ctx, s.fs, userID, appointmentID)
	if err != nil {
		s.logger.Error("Failed to verify appointment write", "error", err, "appointment_id", appointmentID)
	} else {
		s.logger.Info("Appointment write verification",
			"expected_messages", len(appointment.ChatMessages),
			"actual_messages", len(verifyAppointment.ChatMessages),
			"write_successful", len(verifyAppointment.ChatMessages) == len(appointment.ChatMessages))

		if len(verifyAppointment.ChatMessages) != len(appointment.ChatMessages) {
			s.logger.Error("CRITICAL: Appointment write verification failed - data inconsistency detected",
				"expected", len(appointment.ChatMessages),
				"actual", len(verifyAppointment.ChatMessages))
		}
	}

	s.logger.Info("Chat fast-forwarded successfully", "appointment_id", appointmentID, "injected_messages", len(messages), "final_total_messages", len(appointment.ChatMessages))
	return nil
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

	var toolResponse *internal.LLMToolResponse
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
	s.logger.Debug("Generating AI response", "user_id", user.ID, "appointment_id", appointment.ID)

	// Build assistant overrides for personalized chat
	assistantOverrides, err := internal.BuildAssistantOverrides(ctx, user, s.fs, s.logger)
	if err != nil {
		s.logger.Error("Failed to build assistant overrides", "error", err, "user_id", user.ID)
		s.sendError(msgChan, "Failed to prepare assistant")
		return fmt.Errorf("failed to build assistant overrides: %w", err)
	}

	// Convert chat history to OpenAI format
	messages, err := s.buildOpenAIMessages(appointment, assistantOverrides)
	if err != nil {
		s.sendError(msgChan, "Failed to prepare conversation")
		return fmt.Errorf("failed to build OpenAI messages: %w", err)
	}

	// Create streaming request with tools
	req := openai.ChatCompletionRequest{
		Model:      openai.GPT4Dot1,
		Messages:   messages,
		Stream:     true,
		Tools:      s.toolRegistry.GetAllTools(),
		ToolChoice: "auto",
	}

	// Create OpenAI streaming response
	stream, err := s.openAI.CreateChatCompletionStream(ctx, req)
	if err != nil {
		s.logger.Error("Failed to create chat completion stream", "error", err)
		s.sendError(msgChan, "Failed to generate response")
		return fmt.Errorf("failed to create completion stream: %w", err)
	}
	defer stream.Close()

	// Stream the response and handle tool calls
	return s.processStreamingResponse(ctx, stream, msgChan, user, appointment)
}

// buildOpenAIMessages converts chat history to OpenAI format with system prompt
func (s *ServiceImpl) buildOpenAIMessages(appointment *internal.Appointment, assistantOverrides *vapi.Assistant) ([]openai.ChatCompletionMessage, error) {
	messages := []openai.ChatCompletionMessage{}

	// Add system message from assistant overrides
	systemPrompt := "You are Shirely, an AI career assistant helping job seekers find the right opportunities."
	if assistantOverrides != nil && assistantOverrides.Model != nil && assistantOverrides.Model.Messages != nil && len(assistantOverrides.Model.Messages) > 0 {
		systemPrompt = assistantOverrides.Model.Messages[0].Content
	}

	// Add progressive completion awareness to system prompt
	if appointment.ConversationChecklist != nil {
		completionStatus := internal.GetHybridCompletionStatus(appointment.ConversationChecklist)
		
		// Generate progressive prompting based on completion status
		systemPrompt += s.generateProgressivePrompting(completionStatus)
	}

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	})

	// Add conversation history
	chatHistory, err := internal.GetChatHistory(appointment)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat history: %w", err)
	}

	for _, msg := range chatHistory {
		// Skip messages with empty content
		if msg.Content == "" {
			continue
		}

		// Handle different message types according to OpenAI spec
		switch msg.Role {
		case openai.ChatMessageRoleUser:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: msg.Content,
			})
		case openai.ChatMessageRoleAssistant:
			// Check if this is a tool call message or regular assistant message
			if strings.HasPrefix(msg.Content, "TOOL_CALLS:") {
				// Parse the stored tool calls and add them as an assistant message with tool calls
				jsonStr := strings.TrimPrefix(msg.Content, "TOOL_CALLS:")
				var toolCalls []openai.ToolCall
				if err := json.Unmarshal([]byte(jsonStr), &toolCalls); err != nil {
					s.logger.Error("Failed to parse stored tool calls", "error", err, "content", msg.Content)
					continue
				}
				messages = append(messages, openai.ChatCompletionMessage{
					Role:      openai.ChatMessageRoleAssistant,
					ToolCalls: toolCalls,
					// Content is empty for tool call messages per OpenAI spec
				})
			} else {
				// Regular assistant message
				messages = append(messages, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: msg.Content,
				})
			}
		case openai.ChatMessageRoleTool:
			// Parse tool response and add as tool message
			if strings.HasPrefix(msg.Content, "TOOL_RESPONSE:") {
				jsonStr := strings.TrimPrefix(msg.Content, "TOOL_RESPONSE:")
				var toolData map[string]any
				if err := json.Unmarshal([]byte(jsonStr), &toolData); err != nil {
					s.logger.Error("Failed to parse stored tool response", "error", err, "content", msg.Content)
					continue
				}

				// Extract tool call ID and response content
				if toolCallID, ok := toolData["tool_call_id"].(string); ok {
					if content, ok := toolData["content"].(string); ok {
						messages = append(messages, openai.ChatCompletionMessage{
							Role:       openai.ChatMessageRoleTool,
							Content:    content,
							ToolCallID: toolCallID,
						})
					}
				}
			}
		case openai.ChatMessageRoleSystem:
			// System messages are typically handled separately, skip in main conversation
			continue
		default:
			// Skip unknown message types
			s.logger.Debug("Skipping unknown message type", "role", msg.Role)
			continue
		}
	}

	return messages, nil
}

// processStreamingResponse handles the OpenAI streaming response and tool execution
func (s *ServiceImpl) processStreamingResponse(ctx context.Context, stream *openai.ChatCompletionStream, msgChan chan<- StreamMessage, user *internal.User, appointment *internal.Appointment) error {
	var fullResponse strings.Builder
	var toolCalls []openai.ToolCall

	// Stream the response
	for {
		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			s.logger.Error("Error receiving stream", "error", err)
			s.sendError(msgChan, "Stream error")
			return fmt.Errorf("stream receive error: %w", err)
		}

		if len(response.Choices) > 0 {
			choice := response.Choices[0]

			// Handle regular content streaming
			if choice.Delta.Content != "" {
				content := choice.Delta.Content
				fullResponse.WriteString(content)

				// Send the content chunk via channel
				msgChan <- StreamMessage{
					Type:    "content",
					Content: content,
				}
			}

			// Handle tool calls
			if choice.Delta.ToolCalls != nil {
				for _, toolCall := range choice.Delta.ToolCalls {
					// Initialize tool call if it's the first delta for this tool call
					if toolCall.Index != nil && int(*toolCall.Index) >= len(toolCalls) {
						// Extend the toolCalls slice if necessary
						for len(toolCalls) <= int(*toolCall.Index) {
							toolCalls = append(toolCalls, openai.ToolCall{})
						}
					}

					if toolCall.Index != nil {
						idx := int(*toolCall.Index)

						// Accumulate tool call data
						if toolCall.ID != "" {
							toolCalls[idx].ID = toolCall.ID
						}
						if toolCall.Type != "" {
							toolCalls[idx].Type = toolCall.Type
						}
						if toolCall.Function.Name != "" {
							toolCalls[idx].Function.Name = toolCall.Function.Name
						}
						if toolCall.Function.Arguments != "" {
							toolCalls[idx].Function.Arguments += toolCall.Function.Arguments
						}
					}
				}
			}
		}
	}

	// Save the complete assistant response if there was text content
	if fullResponse.Len() > 0 {
		if err := s.saveAssistantMessage(ctx, appointment, fullResponse.String()); err != nil {
			s.logger.Error("Failed to save assistant message", "error", err)
		}
	}

	// Handle tool calls if any were made
	if len(toolCalls) > 0 {
		return s.processToolCalls(ctx, toolCalls, msgChan, user, appointment)
	}

	// Send completion event
	msgChan <- StreamMessage{Type: "done"}
	return nil
}

// saveAssistantMessage saves an assistant message and updates checklist
func (s *ServiceImpl) saveAssistantMessage(ctx context.Context, appointment *internal.Appointment, content string) error {
	if err := internal.AddChatMessage(ctx, s.fs, appointment, "assistant", content); err != nil {
		return fmt.Errorf("failed to add chat message: %w", err)
	}

	// Ensure checklist is initialized
	if appointment.ConversationChecklist == nil {
		s.logger.Warn("Chat appointment missing checklist during AI response, initializing", "appointment_id", appointment.ID)
		appointment.ConversationChecklist = internal.InitializeConversationChecklist()
	}

	// Increment message count in checklist
	internal.IncrementMessageCount(appointment.ConversationChecklist)

	// Save the updated appointment with incremented message count
	if err := internal.UpdateAppointment(ctx, s.fs, appointment); err != nil {
		return fmt.Errorf("failed to save appointment with updated message count: %w", err)
	}

	return nil
}

// processToolCalls handles execution of tool calls and follow-up responses
func (s *ServiceImpl) processToolCalls(ctx context.Context, toolCalls []openai.ToolCall, msgChan chan<- StreamMessage, user *internal.User, appointment *internal.Appointment) error {
	s.logger.Info("Processing tool calls", "count", len(toolCalls), "user_id", user.ID)

	// Save the assistant's tool call message to chat history
	if err := s.saveToolCallsMessage(ctx, appointment, toolCalls); err != nil {
		s.logger.Error("Failed to save tool calls message", "error", err)
	}

	session := &ChatSession{
		User:        user,
		Appointment: appointment,
		Service:     s,
	}

	// Execute each tool call
	for _, toolCall := range toolCalls {
		if toolCall.Function.Name == "" {
			s.logger.Warn("Tool call missing function name", "tool_call_id", toolCall.ID)
			continue
		}

		// Parse the function arguments
		var args map[string]any
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			s.logger.Error("Failed to parse tool call arguments", "error", err, "tool_call_id", toolCall.ID)

			// Send error response for this tool call
			errorResponse := &internal.LLMToolResponse{
				ID:      toolCall.ID,
				Name:    toolCall.Function.Name,
				Content: "Failed to parse tool arguments",
				Success: false,
			}
			msgChan <- StreamMessage{
				Type:         "tool_response",
				ToolResponse: errorResponse,
			}
			continue
		}

		s.logger.Info("Executing tool call",
			"tool_name", toolCall.Function.Name,
			"tool_id", toolCall.ID,
			"args", internal.RedactSensitiveInfo(args),
		)

		// Execute the tool
		toolResponse, err := s.toolRegistry.ExecuteTool(ctx, toolCall.Function.Name, args, session)
		if err != nil {
			s.logger.Error("Tool execution failed", "error", err, "tool_name", toolCall.Function.Name)
			toolResponse = &internal.LLMToolResponse{
				ID:      toolCall.ID,
				Name:    toolCall.Function.Name,
				Content: "Tool execution failed: " + err.Error(),
				Success: false,
			}
		}

		// Set the tool call ID in the response
		toolResponse.ID = toolCall.ID

		// Send tool response via channel
		msgChan <- StreamMessage{
			Type:         "tool_response",
			ToolResponse: toolResponse,
		}

		// Save tool response to chat history
		if err := s.saveToolResponseMessage(ctx, appointment, toolCall.ID, toolResponse.Content); err != nil {
			s.logger.Error("Failed to save tool response message", "error", err)
		}

		s.logger.Info("Tool call completed", "tool_name", toolCall.Function.Name, "success", toolResponse.Success)
	}

	// Generate follow-up response after tool execution
	return s.generateFollowUpResponse(ctx, msgChan, user, appointment)
}

// saveToolCallsMessage saves tool calls to chat history
func (s *ServiceImpl) saveToolCallsMessage(ctx context.Context, appointment *internal.Appointment, toolCalls []openai.ToolCall) error {
	toolCallsJSON, err := json.Marshal(toolCalls)
	if err != nil {
		return fmt.Errorf("failed to marshal tool calls: %w", err)
	}

	toolCallContent := "TOOL_CALLS:" + string(toolCallsJSON)
	return internal.AddChatMessage(ctx, s.fs, appointment, openai.ChatMessageRoleAssistant, toolCallContent)
}

// saveToolResponseMessage saves a tool response to chat history
func (s *ServiceImpl) saveToolResponseMessage(ctx context.Context, appointment *internal.Appointment, toolCallID, content string) error {
	toolCallData := map[string]any{
		"tool_call_id": toolCallID,
		"content":      content,
	}
	toolCallJSON, _ := json.Marshal(toolCallData)
	toolResponseContent := "TOOL_RESPONSE:" + string(toolCallJSON)
	return internal.AddChatMessage(ctx, s.fs, appointment, openai.ChatMessageRoleTool, toolResponseContent)
}

// SaveAppointment saves an appointment to the database
// This method is exposed for tools that need to persist appointment changes
func (s *ServiceImpl) SaveAppointment(ctx context.Context, appointment *internal.Appointment) error {
	return internal.UpdateAppointment(ctx, s.fs, appointment)
}

// generateFollowUpResponse generates AI response after tool execution
func (s *ServiceImpl) generateFollowUpResponse(ctx context.Context, msgChan chan<- StreamMessage, user *internal.User, appointment *internal.Appointment) error {
	s.logger.Debug("Generating follow-up response after tool calls", "user_id", user.ID, "appointment_id", appointment.ID)

	// Build assistant overrides
	assistantOverrides, err := internal.BuildAssistantOverrides(ctx, user, s.fs, s.logger)
	if err != nil {
		s.logger.Error("Failed to build assistant overrides for follow-up", "error", err)
	}

	// Build messages including the recent tool calls
	messages, err := s.buildOpenAIMessages(appointment, assistantOverrides)
	if err != nil {
		s.sendError(msgChan, "Failed to prepare follow-up conversation")
		return fmt.Errorf("failed to build follow-up messages: %w", err)
	}

	// Create follow-up request (prevent cascading tool calls)
	req := openai.ChatCompletionRequest{
		Model:      openai.GPT4Dot1,
		Messages:   messages,
		Stream:     true,
		Tools:      s.toolRegistry.GetAllTools(),
		ToolChoice: "none", // Prevent cascading tool calls after processing results
	}

	// Stream the follow-up response
	stream, err := s.openAI.CreateChatCompletionStream(ctx, req)
	if err != nil {
		s.logger.Error("Failed to create follow-up stream", "error", err)
		s.sendError(msgChan, "Failed to generate follow-up response")
		return fmt.Errorf("failed to create follow-up stream: %w", err)
	}
	defer stream.Close()

	var followUpResponse strings.Builder

	// Stream the follow-up response
	for {
		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			s.logger.Error("Error receiving follow-up stream", "error", err)
			s.sendError(msgChan, "Follow-up stream error")
			return fmt.Errorf("follow-up stream error: %w", err)
		}

		if len(response.Choices) > 0 {
			choice := response.Choices[0]

			// Handle content streaming
			if choice.Delta.Content != "" {
				content := choice.Delta.Content
				followUpResponse.WriteString(content)

				// Send the content chunk via channel
				msgChan <- StreamMessage{
					Type:    "content",
					Content: content,
				}
			}
		}
	}

	// Save the follow-up response
	if followUpResponse.Len() > 0 {
		if err := s.saveAssistantMessage(ctx, appointment, followUpResponse.String()); err != nil {
			s.logger.Error("Failed to save follow-up assistant message", "error", err)
		}
	}

	// Send completion event
	msgChan <- StreamMessage{Type: "done"}
	return nil
}

// sendError sends an error message via the channel
func (s *ServiceImpl) sendError(msgChan chan<- StreamMessage, message string) {
	msgChan <- StreamMessage{
		Type:  "error",
		Error: message,
	}
}

// generateProgressivePrompting creates context-aware system prompts based on completion status
func (s *ServiceImpl) generateProgressivePrompting(completionStatus *internal.CompletionStatus) string {
	var prompt strings.Builder
	
	// Base checklist information
	prompt.WriteString(fmt.Sprintf(`

COMPLETION STATUS: {"score":%.1f%%,"hard":%v,"soft":%v,"can_complete":%v}
METRICS: {"attempts":%d/%d,"messages":%d/%d,"progress":%d/%d,"quality":%d}
`,
		completionStatus.CompletionScore*100,
		completionStatus.HardComplete,
		completionStatus.SoftComplete,
		completionStatus.CanComplete,
		completionStatus.QualityMetrics.AttemptNumber,
		internal.MaxCompletionAttempts,
		completionStatus.QualityMetrics.MessageCount,
		internal.MaxConversationLength,
		completionStatus.QualityMetrics.CompletedItems,
		completionStatus.QualityMetrics.RequiredItems,
		completionStatus.QualityMetrics.HighQualityItems,
	))
	
	// Progressive guidance based on completion status
	if completionStatus.HardComplete {
		prompt.WriteString(`
COMPLETION READY: All requirements met. You may complete the conversation naturally when it feels right.
FOCUS: Wrap up naturally and call 'complete_chat' when appropriate.
`)
	} else if completionStatus.SoftComplete {
		prompt.WriteString(`
SOFT COMPLETION: Sufficient information gathered. You can complete naturally but may continue if user is engaged.
FOCUS: Balance completion readiness with natural conversation flow.
`)
	} else if completionStatus.CanComplete {
		prompt.WriteString(`
ESCALATED MODE: Can complete due to limits/attempts. Prioritize user experience over perfect completion.
FOCUS: Find a natural stopping point and complete to avoid user frustration.
`)
	} else {
		// Progressive urgency based on attempt number
		if completionStatus.QualityMetrics.AttemptNumber == 0 {
			prompt.WriteString(`
GATHERING PHASE: Focus on collecting information naturally through conversation.
FOCUS: Use 'mark_topic_progress' to track what you learn, maintain conversational flow.
`)
		} else if completionStatus.QualityMetrics.AttemptNumber == 1 {
			prompt.WriteString(`
GUIDANCE PHASE: Some information still needed. Gently guide conversation toward gaps.
FOCUS: Address missing items while maintaining natural dialogue. Use 'check_completion_readiness' to assess.
`)
		} else {
			prompt.WriteString(`
CLOSURE PHASE: Multiple attempts made. Prioritize user experience over perfect completion.
FOCUS: Consider completing with available information. Avoid repetitive information gathering.
`)
		}
	}
	
	// Add critical missing items guidance
	if len(completionStatus.MissingCritical) > 0 {
		prompt.WriteString(fmt.Sprintf(`
CRITICAL MISSING: %s
APPROACH: Naturally work these topics into conversation without being mechanical.
`, strings.Join(completionStatus.MissingCritical, ", ")))
	}
	
	// Add specific guidance
	prompt.WriteString(fmt.Sprintf(`
GUIDANCE: %s

TOOLS: Always call 'mark_topic_progress' after learning something new. Use 'check_completion_readiness' to assess status.
`, completionStatus.Guidance))
	
	return prompt.String()
}
