package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"time"

	"github.com/chriscow/vapi-go"
	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
)

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// WebSocket upgrader with CORS support
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for WebSocket connections (you may want to restrict this in production)
		return true
	},
}

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type      string `json:"type"`
	Content   string `json:"content"`
	Error     string `json:"error,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}

// chatStartRequest represents the request to start a new chat session
type chatStartRequest struct {
	CSRFToken string `json:"csrf_token"`
	Timezone  string `json:"timezone"`
}

// chatMessageRequest represents a message sent in the chat
type chatMessageRequest struct {
	Content   string `json:"content"`
	CSRFToken string `json:"csrf_token"`
}

// chatStartResponse represents the response when starting a chat
type chatStartResponse struct {
	AppointmentID string `json:"appointment_id"`
	Status        string `json:"status"`
	FirstMessage  string `json:"first_message"`
}

// startChat handles starting a new chat session (creates a chat appointment)
func (app *application) startChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		response.JSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	var req chatStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.logger.Error("Failed to decode chat start request", "error", err)
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Create a new chat appointment
	appointment := &internal.Appointment{
		Name:          "initial-chat",
		Type:          internal.AppointmentTypeChat,
		ScheduledTime: time.Now(),
		Timezone:      req.Timezone,
		Status:        internal.AppointmentStatusRunning,
		ChatMessages:  []internal.ChatMessage{},
	}

	if err := internal.CreateAppointment(r.Context(), app.fs, user.ID, appointment); err != nil {
		app.logger.Error("Failed to create chat appointment", "error", err, "user_id", user.ID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create chat session"})
		return
	}

	// Build assistant overrides to get the first message
	assistantOverrides, err := internal.BuildAssistantOverrides(r.Context(), user, app.fs, app.logger)
	if err != nil {
		app.logger.Error("Failed to build assistant overrides for chat start", "error", err, "user_id", user.ID)
		// Continue with default message
		assistantOverrides = &vapi.Assistant{
			FirstMessage: stringPtr("Hi! I'm Shirely. I'm excited to learn about your career preferences. What kind of work environment do you prefer?"),
		}
	}

	firstMessage := "Hi! I'm Shirely. I'm excited to learn about your career preferences. What kind of work environment do you prefer?"
	if assistantOverrides != nil && assistantOverrides.FirstMessage != nil {
		firstMessage = *assistantOverrides.FirstMessage
	}

	// Update user status to indicate chat started
	if err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusCallStarted); err != nil {
		app.logger.Error("Failed to update user status for chat", "error", err, "user_id", user.ID)
		// Continue despite error - chat session is already created
	}

	app.logger.Info("Chat session started", "user_id", user.ID, "appointment_id", appointment.ID)

	response.JSON(w, http.StatusOK, chatStartResponse{
		AppointmentID: appointment.ID,
		Status:        "started",
		FirstMessage:  firstMessage,
	})
}

// sendChatMessage is deprecated - now handled via WebSocket in streamChatResponse
// Keeping for backward compatibility during transition
func (app *application) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		response.JSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	if appointmentID == "" {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "missing appointment_id"})
		return
	}

	var req chatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.logger.Error("Failed to decode chat message request", "error", err)
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Get the appointment
	appointment, err := internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get appointment", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusNotFound, map[string]string{"error": "appointment not found"})
		return
	}

	if appointment.Type != internal.AppointmentTypeChat {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "not a chat appointment"})
		return
	}

	// Add the user's message
	if err := internal.AddChatMessage(r.Context(), app.fs, appointment, "user", req.Content); err != nil {
		app.logger.Error("Failed to add user message", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save message"})
		return
	}

	app.logger.Info("Chat message saved", "user_id", user.ID, "appointment_id", appointmentID)
	
	response.JSON(w, http.StatusOK, map[string]string{
		"status": "message_saved",
	})
}

// streamChatResponse handles WebSocket connections for real-time chat streaming
func (app *application) streamChatResponse(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	if appointmentID == "" {
		http.Error(w, "Missing appointment_id", http.StatusBadRequest)
		return
	}

	// Get the appointment
	appointment, err := internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get appointment for WebSocket", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		http.Error(w, "Appointment not found", http.StatusNotFound)
		return
	}

	if appointment.Type != internal.AppointmentTypeChat {
		http.Error(w, "Not a chat appointment", http.StatusBadRequest)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.logger.Error("Failed to upgrade to WebSocket", "error", err)
		return
	}
	defer conn.Close()

	app.logger.Info("WebSocket connection established", "user_id", user.ID, "appointment_id", appointmentID)

	// Send connection confirmation
	confirmMsg := WebSocketMessage{
		Type:    "connected",
		Content: "WebSocket connection established",
	}
	if err := conn.WriteJSON(confirmMsg); err != nil {
		app.logger.Error("Failed to send connection confirmation", "error", err)
		return
	}

	// Handle WebSocket messages
	for {
		var msg WebSocketMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				app.logger.Error("WebSocket unexpected close", "error", err)
			}
			break
		}

		app.logger.Debug("Received WebSocket message", "type", msg.Type, "user_id", user.ID)

		switch msg.Type {
		case "user_message":
			// Save user message and generate AI response
			go app.handleWebSocketUserMessage(conn, user, appointment, msg.Content)
		case "ping":
			// Respond to ping with pong
			pong := WebSocketMessage{Type: "pong"}
			conn.WriteJSON(pong)
		default:
			app.logger.Warn("Unknown WebSocket message type", "type", msg.Type, "user_id", user.ID)
		}
	}
}

// handleWebSocketUserMessage processes a user message and streams the AI response via WebSocket
func (app *application) handleWebSocketUserMessage(conn *websocket.Conn, user *internal.User, appointment *internal.Appointment, userMessage string) {
	ctx := context.Background()
	
	// Save user message to appointment
	if err := internal.AddChatMessage(ctx, app.fs, appointment, "user", userMessage); err != nil {
		app.logger.Error("Failed to save user message", "error", err, "user_id", user.ID)
		errorMsg := WebSocketMessage{
			Type:  "error",
			Error: "Failed to save your message",
		}
		conn.WriteJSON(errorMsg)
		return
	}

	// Stream AI response
	app.streamAIResponseViaWebSocket(ctx, conn, user, appointment)
}

// streamAIResponseViaWebSocket generates and streams the AI response via WebSocket
func (app *application) streamAIResponseViaWebSocket(ctx context.Context, conn *websocket.Conn, user *internal.User, appointment *internal.Appointment) {
	// Build assistant overrides for personalized chat
	assistantOverrides, err := internal.BuildAssistantOverrides(ctx, user, app.fs, app.logger)
	if err != nil {
		app.logger.Error("Failed to build assistant overrides", "error", err, "user_id", user.ID)
		errorMsg := WebSocketMessage{
			Type:  "error",
			Error: "Failed to prepare assistant",
		}
		conn.WriteJSON(errorMsg)
		return
	}

	// Convert chat history to OpenAI format
	messages := []openai.ChatCompletionMessage{}

	// Add system message from assistant overrides
	systemPrompt := "You are Shirely, an AI career assistant helping job seekers find the right opportunities."
	if assistantOverrides.Model != nil && assistantOverrides.Model.Messages != nil && len(assistantOverrides.Model.Messages) > 0 {
		// Use the system prompt from assistant overrides
		systemPrompt = assistantOverrides.Model.Messages[0].Content
	}
	
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	})

	// Add conversation history
	chatHistory, err := internal.GetChatHistory(appointment)
	if err != nil {
		app.logger.Error("Failed to get chat history", "error", err)
		errorMsg := WebSocketMessage{
			Type:  "error",
			Error: "Failed to get chat history",
		}
		conn.WriteJSON(errorMsg)
		return
	}

	for _, msg := range chatHistory {
		role := openai.ChatMessageRoleUser
		if msg.Role == "assistant" {
			role = openai.ChatMessageRoleAssistant
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	// Create streaming request
	req := openai.ChatCompletionRequest{
		Model:    openai.GPT4o,
		Messages: messages,
		Stream:   true,
	}

	// Create OpenAI client
	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		app.logger.Error("Failed to create chat completion stream", "error", err)
		errorMsg := WebSocketMessage{
			Type:  "error",
			Error: "Failed to generate response",
		}
		conn.WriteJSON(errorMsg)
		return
	}
	defer stream.Close()

	var fullResponse strings.Builder
	isFirstToken := true

	// Stream the response
	for {
		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			app.logger.Error("Error receiving stream", "error", err)
			errorMsg := WebSocketMessage{
				Type:  "error",
				Error: "Stream error",
			}
			conn.WriteJSON(errorMsg)
			return
		}

		if len(response.Choices) > 0 && response.Choices[0].Delta.Content != "" {
			content := response.Choices[0].Delta.Content
			fullResponse.WriteString(content)

			// Send the content chunk via WebSocket
			msg := WebSocketMessage{
				Type:    "content",
				Content: content,
			}

			// Add prefix for first token
			if isFirstToken {
				msg.Content = "AI: " + content
				isFirstToken = false
			}

			if err := conn.WriteJSON(msg); err != nil {
				app.logger.Error("Failed to send WebSocket message", "error", err)
				return
			}
		}
	}

	// Save the complete assistant response
	if fullResponse.Len() > 0 {
		if err := internal.AddChatMessage(ctx, app.fs, appointment, "assistant", fullResponse.String()); err != nil {
			app.logger.Error("Failed to save assistant message", "error", err)
		}
	}

	// Send completion event
	doneMsg := WebSocketMessage{
		Type: "done",
	}
	conn.WriteJSON(doneMsg)
}

// endChat handles ending a chat session
func (app *application) endChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		response.JSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	if appointmentID == "" {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "missing appointment_id"})
		return
	}

	// Get the appointment
	appointment, err := internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get appointment for ending", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusNotFound, map[string]string{"error": "appointment not found"})
		return
	}

	if appointment.Type != internal.AppointmentTypeChat {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "not a chat appointment"})
		return
	}

	// Mark appointment as processing for transcript processing
	if err := internal.ProcessAppointmentStarted(r.Context(), app.fs, user, appointment); err != nil {
		app.logger.Error("Failed to start appointment processing", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to process chat session"})
		return
	}

	app.logger.Info("Chat session ended", "user_id", user.ID, "appointment_id", appointmentID)

	response.JSON(w, http.StatusOK, map[string]string{
		"status":         "ended",
		"appointment_id": appointmentID,
	})
}

// showChatPage displays the dedicated chat page
func (app *application) showChatPage(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	if appointmentID == "" {
		app.logger.Error("No appointment ID provided for chat page")
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}

	// Verify the appointment exists and belongs to this user
	appointment, err := internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get appointment for chat page", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}

	if appointment.Type != internal.AppointmentTypeChat {
		app.logger.Error("Appointment is not a chat type", "user_id", user.ID, "appointment_id", appointmentID, "type", appointment.Type)
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}

	data := app.newTemplateData(r)
	data["AppointmentID"] = appointmentID
	data["Appointment"] = appointment

	err = response.Page(w, http.StatusOK, data, "pages/chat.html")
	if err != nil {
		app.logger.Error("Failed to render chat page", "error", err)
		app.serverError(w, r, err)
	}
}

// getChatHistory returns the chat history for an appointment  
func (app *application) getChatHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		response.JSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	if appointmentID == "" {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "missing appointment_id"})
		return
	}

	// Get the appointment
	appointment, err := internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get appointment for history", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusNotFound, map[string]string{"error": "appointment not found"})
		return
	}

	if appointment.Type != internal.AppointmentTypeChat {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "not a chat appointment"})
		return
	}

	// Get chat history
	messages, err := internal.GetChatHistory(appointment)
	if err != nil {
		app.logger.Error("Failed to get chat history", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get chat history"})
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"messages": messages,
		"appointment_id": appointmentID,
	})
}
