package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"talent-rodeo/cmd/web/simulate"
	"talent-rodeo/internal"
	"talent-rodeo/internal/chat"
	"talent-rodeo/internal/response"

	"log/slog"

	"github.com/chriscow/vapi-go"
	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
)

// WebSocket upgrader with CORS support
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for WebSocket connections (you may want to restrict this in production)
		return true
	},
}

// Global chat service instance - will be initialized in main
var chatService chat.Service

// initializeChatService sets up the chat service with dependencies
func (app *application) initializeChatService() {
	// Create OpenAI client wrapper
	openAIClient := &realOpenAIClient{
		client: openai.NewClient(os.Getenv("OPENAI_API_KEY")),
	}

	// Create Firestore client adapter
	firestoreAdapter := &firestoreClientAdapter{
		client: app.fs,
		logger: app.logger,
	}

	// Initialize the chat service
	chatService = chat.NewService(firestoreAdapter, openAIClient, app.logger)
}

// realOpenAIClient implements the OpenAIClient interface
type realOpenAIClient struct {
	client *openai.Client
}

// CreateChatCompletionStream implements the OpenAIClient interface
func (c *realOpenAIClient) CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	return c.client.CreateChatCompletionStream(ctx, request)
}

// firestoreClientAdapter adapts the existing FirestoreClient to the chat.FirestoreClient interface
type firestoreClientAdapter struct {
	client *internal.FirestoreClient
	logger *slog.Logger
}

func (f *firestoreClientAdapter) GetAppointment(ctx context.Context, userID, appointmentID string) (*internal.Appointment, error) {
	return internal.GetAppointment(ctx, f.client, userID, appointmentID)
}

func (f *firestoreClientAdapter) UpdateAppointment(ctx context.Context, appointment *internal.Appointment) error {
	return internal.UpdateAppointment(ctx, f.client, appointment)
}

func (f *firestoreClientAdapter) CreateAppointment(ctx context.Context, userID string, appointment *internal.Appointment) error {
	return internal.CreateAppointment(ctx, f.client, userID, appointment)
}

func (f *firestoreClientAdapter) AddChatMessage(ctx context.Context, appointment *internal.Appointment, role, content string) error {
	return internal.AddChatMessage(ctx, f.client, appointment, role, content)
}

func (f *firestoreClientAdapter) GetLatestRunningChatAppointment(ctx context.Context, userID string) (*internal.Appointment, bool, error) {
	return internal.GetLatestRunningChatAppointment(ctx, f.client, userID)
}

func (f *firestoreClientAdapter) BuildAssistantOverrides(ctx context.Context, user *internal.User) (*vapi.Assistant, error) {
	return internal.BuildAssistantOverrides(ctx, user, f.client, f.logger)
}

func (f *firestoreClientAdapter) ProcessAppointmentStarted(ctx context.Context, user *internal.User, appointment *internal.Appointment) error {
	return internal.ProcessAppointmentStarted(ctx, f.client, user, appointment)
}

func (f *firestoreClientAdapter) GetUser(ctx context.Context, userID string) (*internal.User, bool, error) {
	return internal.GetUser(ctx, f.client, userID)
}

func (f *firestoreClientAdapter) UpdateUserStatus(ctx context.Context, user *internal.User, status internal.UserStatus) error {
	return internal.UpdateUserStatus(ctx, f.client, user, status)
}

// startChat handles starting a new chat session
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

	var req chat.StartChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.logger.Error("Failed to decode chat start request", "error", err)
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Use the chat service to start the session
	chatResponse, err := chatService.StartSession(r.Context(), user.ID, req)
	if err != nil {
		app.logger.Error("Failed to start chat session", "error", err, "user_id", user.ID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create chat session"})
		return
	}

	app.logger.Info("Chat session started", "user_id", user.ID, "appointment_id", chatResponse.AppointmentID)

	response.JSON(w, http.StatusOK, chatResponse)
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

	var req struct {
		Content   string `json:"content"`
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.logger.Error("Failed to decode chat message request", "error", err)
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Use the chat service to send the message
	err := chatService.SendMessage(r.Context(), user.ID, appointmentID, req.Content)
	if err != nil {
		app.logger.Error("Failed to send chat message", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
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

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.logger.Error("Failed to upgrade to WebSocket", "error", err)
		return
	}
	defer conn.Close()

	app.logger.Info("WebSocket connection established", "user_id", user.ID, "appointment_id", appointmentID)

	// Use the chat service to handle the WebSocket session
	session := &chat.ChatSession{
		UserID:        user.ID,
		AppointmentID: appointmentID,
		Connection:    conn,
	}

	if err := chatService.HandleWebSocketSession(r.Context(), session); err != nil {
		app.logger.Error("WebSocket session failed", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
	}
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

	// Use the chat service to end the session
	err := chatService.EndSession(r.Context(), user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to end chat session", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
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
	var appointment *internal.Appointment
	var err error

	if appointmentID == "" {
		// Use the chat service to find or create a session
		chatResponse, err := chatService.GetOrCreateSession(r.Context(), user.ID)
		if err != nil {
			app.logger.Error("Failed to get or create chat session", "error", err, "user_id", user.ID)
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}
		appointmentID = chatResponse.AppointmentID

		// Get the appointment for template data
		appointment, err = internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
		if err != nil {
			app.logger.Error("Failed to get appointment after creation", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}
	} else {
		// Verify the appointment exists and belongs to this user
		appointment, err = internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
		if err != nil {
			app.logger.Error("Failed to get appointment for chat page", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}
	}

	if appointment.Type != internal.AppointmentTypeChat {
		app.logger.Error("Appointment is not a chat type", "user_id", user.ID, "appointment_id", appointmentID, "type", appointment.Type)
		http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
		return
	}

	data := app.newTemplateData(r)
	data["AppointmentID"] = appointmentID
	data["Appointment"] = appointment
	data["User"] = user

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

	// Use the chat service to get history
	history, err := chatService.GetHistory(r.Context(), user.ID, appointmentID, user.IsSuperUser)
	if err != nil {
		app.logger.Error("Failed to get chat history", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get chat history"})
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"messages":       history,
		"appointment_id": appointmentID,
		"debug_mode":     user.IsSuperUser,
	})
}

// fastForwardChat injects a test transcript into the current chat session for testing
func (app *application) fastForwardChat(w http.ResponseWriter, r *http.Request) {
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

	// Load test call data using the TestingCallID
	app.logger.Info("Loading test call data for fast-forward", "appointment_id", appointmentID, "test_call_id", simulate.TestingCallID)

	call, err := vapi.GetCall(r.Context(), simulate.TestingCallID)
	if err != nil {
		app.logger.Error("Failed to get test call data", "error", err, "test_call_id", simulate.TestingCallID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load test call data"})
		return
	}

	// Use the chat service to inject the transcript
	injectedCount, err := chatService.InjectTranscript(r.Context(), user.ID, appointmentID, *call)
	if err != nil {
		app.logger.Error("Failed to inject transcript", "error", err, "appointment_id", appointmentID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to inject transcript"})
		return
	}

	app.logger.Info("Chat fast-forwarded successfully", "appointment_id", appointmentID, "injected_messages", injectedCount)

	response.JSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"appointment_id":    appointmentID,
		"injected_messages": injectedCount,
	})
}
