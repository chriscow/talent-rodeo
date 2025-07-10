package main

import (
	"context"
	"encoding/json"
	"net/http"
	"talent-rodeo/internal"
	"talent-rodeo/internal/chat"
	"talent-rodeo/internal/response"

	"github.com/gorilla/websocket"
)

// New service-based chat handlers that delegate to the chat service

// startChatService handles starting a new chat session using the service
func (app *application) startChatService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		response.JSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	var req struct {
		CSRFToken string `json:"csrf_token"`
		Timezone  string `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.logger.Error("Failed to decode chat start request", "error", err)
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Use service to start chat session
	startReq := chat.StartChatRequest{
		Timezone: req.Timezone,
	}

	resp, err := app.chatService.StartSession(r.Context(), user.ID, startReq)
	if err != nil {
		app.logger.Error("Failed to start chat session via service", "error", err, "user_id", user.ID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create chat session"})
		return
	}

	app.logger.Info("Chat session started via service", "user_id", user.ID, "appointment_id", resp.AppointmentID)

	response.JSON(w, http.StatusOK, map[string]any{
		"appointment_id": resp.AppointmentID,
		"status":         resp.Status,
		"first_message":  resp.FirstMessage,
	})
}

// streamChatResponseService handles WebSocket connections using the service
func (app *application) streamChatResponseService(w http.ResponseWriter, r *http.Request) {
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

	// Verify appointment exists and belongs to user (service will do this)
	_, err := app.chatService.GetHistory(r.Context(), user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to verify appointment for WebSocket", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		http.Error(w, "Appointment not found", http.StatusNotFound)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.logger.Error("Failed to upgrade to WebSocket", "error", err)
		return
	}
	defer conn.Close()

	app.logger.Info("WebSocket connection established via service", "user_id", user.ID, "appointment_id", appointmentID)

	// Send connection confirmation
	confirmMsg := map[string]any{
		"type":    "connected",
		"content": "WebSocket connection established",
	}
	if err := conn.WriteJSON(confirmMsg); err != nil {
		app.logger.Error("Failed to send connection confirmation", "error", err)
		return
	}

	// Handle WebSocket messages
	for {
		var msg map[string]any
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				app.logger.Warn("WebSocket unexpected close", "error", err)
			}
			break
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			app.logger.Warn("WebSocket message missing type", "user_id", user.ID)
			continue
		}

		switch msgType {
		case "user_message":
			// Handle user message via service
			go app.handleWebSocketUserMessageService(conn, user, appointmentID, msg)
		case "ping":
			// Respond to ping with pong
			pong := map[string]any{"type": "pong"}
			conn.WriteJSON(pong)
		default:
			app.logger.Warn("Unknown WebSocket message type", "type", msgType, "user_id", user.ID)
		}
	}
}

// handleWebSocketUserMessageService processes user messages via the service
func (app *application) handleWebSocketUserMessageService(conn *websocket.Conn, user *internal.User, appointmentID string, msg map[string]any) {
	ctx := context.Background()

	content, ok := msg["content"].(string)
	if !ok {
		app.logger.Error("WebSocket message missing content", "user_id", user.ID)
		errorMsg := map[string]any{
			"type":  "error",
			"error": "Message content missing",
		}
		conn.WriteJSON(errorMsg)
		return
	}

	// Process user message via service
	err := app.chatService.ProcessUserMessage(ctx, user.ID, appointmentID, content)
	if err != nil {
		app.logger.Error("Failed to process user message via service", "error", err, "user_id", user.ID)
		errorMsg := map[string]any{
			"type":  "error",
			"error": "Failed to save your message",
		}
		conn.WriteJSON(errorMsg)
		return
	}

	// Create a channel for streaming AI response
	msgChan := make(chan chat.StreamMessage, 100) // Buffered channel

	// Start AI response generation via service
	go func() {
		if err := app.chatService.StreamAIResponse(ctx, user.ID, appointmentID, msgChan); err != nil {
			app.logger.Error("Failed to stream AI response via service", "error", err, "user_id", user.ID)
			// The service should have sent an error message via the channel
		}
	}()

	// Forward stream messages to WebSocket
	for streamMsg := range msgChan {
		wsMsg := map[string]any{
			"type": streamMsg.Type,
		}

		if streamMsg.Content != "" {
			wsMsg["content"] = streamMsg.Content
		}
		if streamMsg.Error != "" {
			wsMsg["error"] = streamMsg.Error
		}
		if streamMsg.ToolResponse != nil {
			wsMsg["tool_response"] = streamMsg.ToolResponse
		}

		if err := conn.WriteJSON(wsMsg); err != nil {
			app.logger.Error("Failed to send WebSocket message", "error", err)
			return
		}
	}
}

// endChatService handles ending a chat session using the service
func (app *application) endChatService(w http.ResponseWriter, r *http.Request) {
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

	// End chat session via service
	if err := app.chatService.EndSession(r.Context(), user.ID, appointmentID); err != nil {
		app.logger.Error("Failed to end chat session via service", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to process chat session"})
		return
	}

	app.logger.Info("Chat session ended via service", "user_id", user.ID, "appointment_id", appointmentID)

	response.JSON(w, http.StatusOK, map[string]string{
		"status":         "ended",
		"appointment_id": appointmentID,
	})
}

// getChatHistoryService returns chat history using the service
func (app *application) getChatHistoryService(w http.ResponseWriter, r *http.Request) {
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

	// Get chat history via service
	messages, err := app.chatService.GetHistory(r.Context(), user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get chat history via service", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusNotFound, map[string]string{"error": "appointment not found"})
		return
	}

	// Filter debug messages for non-super users
	var filteredMessages []internal.ChatMessage
	for _, message := range messages {
		// Skip debug messages for non-super users
		if !user.IsSuperUser && (isDebugMessage(message.Role) || isDebugMessageContent(message.Role, message.Content)) {
			continue
		}
		filteredMessages = append(filteredMessages, message)
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"messages":       filteredMessages,
		"appointment_id": appointmentID,
		"debug_mode":     user.IsSuperUser, // Let client know if debug mode is enabled
	})
}

// fastForwardChatService handles fast-forward testing using the service layer
func (app *application) fastForwardChatService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		response.JSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	// Only allow super users to use this feature
	if !user.IsSuperUser {
		response.JSON(w, http.StatusForbidden, map[string]string{"error": "forbidden - super user required"})
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	if appointmentID == "" {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "missing appointment_id"})
		return
	}

	// Use the service layer to fast-forward the chat
	if err := app.chatService.FastForwardChat(r.Context(), user.ID, appointmentID); err != nil {
		app.logger.Error("Failed to fast-forward chat", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	app.logger.Info("Chat fast-forwarded successfully", "user_id", user.ID, "appointment_id", appointmentID)

	response.JSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"appointment_id": appointmentID,
	})
}

// Helper functions are shared with existing handlers.chat.go file
