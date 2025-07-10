package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
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
}

// NewService creates a new ChatService with injected dependencies
func NewService(fs *internal.FirestoreClient, openAI OpenAIClient, logger *slog.Logger) ChatService {
	service := &ServiceImpl{
		fs:     fs,
		openAI: openAI,
		logger: logger,
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

	// Add checklist status awareness to system prompt (compact JSON format)
	if appointment.ConversationChecklist != nil {
		checklistSummary := internal.GetChecklistSummary(appointment.ConversationChecklist)
		systemPrompt += fmt.Sprintf(`

CHECKLIST: {"status":"%s","attempts":%d/%d,"messages":%d/%d,"progress":%d/%d,"required":%d,"ready":%v}

YOUR TASK: Complete this checklist by calling 'mark_topic_progress' after each user response. Before asking questions, check if the slot is already high quality using 'get_checklist_status'. Ask clarifying questions only for low-quality slots.
`,
			checklistSummary["overall_status"],
			checklistSummary["completion_attempts"],
			internal.MaxCompletionAttempts,
			checklistSummary["message_count"],
			internal.MaxConversationLength,
			checklistSummary["completed_items"],
			checklistSummary["total_items"],
			checklistSummary["required_items"],
			checklistSummary["can_complete"],
		)
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
