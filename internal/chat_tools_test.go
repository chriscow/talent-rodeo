package internal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"
)

// application struct for testing (mimics the main application struct)
type application struct {
	fs     *FirestoreClient
	logger *slog.Logger
}

// ChatTool represents a tool that can be called by the LLM (for testing)
type ChatTool struct {
	Handler func(ctx context.Context, app *application, user *User, appointment *Appointment, args map[string]interface{}) (*ToolResponse, error)
}

// ChatToolRegistry holds all available tools for the chat system (for testing)
type ChatToolRegistry struct {
	tools map[string]ChatTool
}

// ToolResponse represents the response from a tool execution (for testing)
type ToolResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Success bool   `json:"success"`
}

// Handler methods for ChatToolRegistry (simplified for testing)
func (r *ChatToolRegistry) handleGetChecklistStatus(ctx context.Context, app *application, user *User, appointment *Appointment, args map[string]interface{}) (*ToolResponse, error) {
	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = InitializeConversationChecklist()
		if err := UpdateAppointment(ctx, app.fs, appointment); err != nil {
			return &ToolResponse{
				Name:    "get_checklist_status",
				Content: "Failed to initialize checklist",
				Success: false,
			}, err
		}
	}

	summary := GetChecklistSummary(appointment.ConversationChecklist)

	// Create a readable summary for the LLM
	content := "Checklist Status: " + summary["overall_status"].(string)

	return &ToolResponse{
		Name:    "get_checklist_status",
		Content: content,
		Success: true,
	}, nil
}

func (r *ChatToolRegistry) handleMarkTopicProgress(ctx context.Context, app *application, user *User, appointment *Appointment, args map[string]interface{}) (*ToolResponse, error) {
	topicID, _ := args["topic_id"].(string)
	status, _ := args["status"].(string)
	quality, _ := args["quality"].(string)
	notes, _ := args["notes"].(string)

	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = InitializeConversationChecklist()
	}

	// Extract info from args
	var extractedInfo map[string]interface{}
	if info, ok := args["extracted_info"].(map[string]interface{}); ok {
		extractedInfo = info
	}

	// Update the checklist item
	err := UpdateChecklistItem(appointment.ConversationChecklist, topicID, status, quality, extractedInfo, notes)
	if err != nil {
		return &ToolResponse{
			Name:    "mark_topic_progress",
			Content: "Failed to update topic: " + err.Error(),
			Success: false,
		}, err
	}

	// Save the updated appointment
	if err := UpdateAppointment(ctx, app.fs, appointment); err != nil {
		return &ToolResponse{
			Name:    "mark_topic_progress",
			Content: "Failed to save progress",
			Success: false,
		}, err
	}

	return &ToolResponse{
		Name:    "mark_topic_progress",
		Content: "Successfully updated topic",
		Success: true,
	}, nil
}

func (r *ChatToolRegistry) handleValidateInformationQuality(ctx context.Context, app *application, user *User, appointment *Appointment, args map[string]interface{}) (*ToolResponse, error) {
	topicID, _ := args["topic_id"].(string)
	validationNotes, _ := args["validation_notes"].(string)

	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = InitializeConversationChecklist()
	}

	// Get the checklist item
	item, err := GetChecklistItem(appointment.ConversationChecklist, topicID)
	if err != nil {
		return &ToolResponse{
			Name:    "validate_information_quality",
			Content: "Topic not found",
			Success: false,
		}, err
	}

	// Update validation notes
	item.ValidationNotes = validationNotes
	item.LastUpdated = time.Now()
	appointment.ConversationChecklist.LastUpdated = time.Now()

	// Save the updated appointment
	if err := UpdateAppointment(ctx, app.fs, appointment); err != nil {
		return &ToolResponse{
			Name:    "validate_information_quality",
			Content: "Failed to save validation notes",
			Success: false,
		}, err
	}

	return &ToolResponse{
		Name:    "validate_information_quality",
		Content: "Validation notes updated",
		Success: true,
	}, nil
}

func (r *ChatToolRegistry) handleCheckCompletionReadiness(ctx context.Context, app *application, user *User, appointment *Appointment, args map[string]interface{}) (*ToolResponse, error) {
	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = InitializeConversationChecklist()
	}

	// Validate completion readiness
	canComplete, _, err := ValidateChecklistCompletion(appointment.ConversationChecklist)
	if err != nil {
		return &ToolResponse{
			Name:    "check_completion_readiness",
			Content: "Failed to validate completion readiness",
			Success: false,
		}, err
	}

	var responseContent string
	if canComplete {
		responseContent = "Conversation is ready for completion"
	} else {
		responseContent = "Conversation is not ready for completion"
	}

	return &ToolResponse{
		Name:    "check_completion_readiness",
		Content: responseContent,
		Success: canComplete,
	}, nil
}

func (r *ChatToolRegistry) handleCompleteChat(ctx context.Context, app *application, user *User, appointment *Appointment, args map[string]interface{}) (*ToolResponse, error) {
	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = InitializeConversationChecklist()
	}

	// Increment completion attempt counter
	IncrementCompletionAttempt(appointment.ConversationChecklist)

	// Validate checklist completion
	canComplete, _, err := ValidateChecklistCompletion(appointment.ConversationChecklist)
	if err != nil {
		return &ToolResponse{
			Name:    "complete_chat",
			Content: "Failed to validate completion requirements",
			Success: false,
		}, err
	}

	if !canComplete {
		// Save the updated appointment with incremented attempt counter
		if err := UpdateAppointment(ctx, app.fs, appointment); err != nil {
			app.logger.Error("Failed to save appointment with completion attempt", "error", err)
		}

		return &ToolResponse{
			Name:    "complete_chat",
			Content: "Cannot complete chat yet",
			Success: false,
		}, nil
	}

	// Mark checklist as completed
	appointment.ConversationChecklist.OverallStatus = ChecklistStatusCompleted
	appointment.ConversationChecklist.LastUpdated = time.Now()

	// Save the updated appointment
	if err := UpdateAppointment(ctx, app.fs, appointment); err != nil {
		return &ToolResponse{
			Name:    "complete_chat",
			Content: "Failed to complete chat session",
			Success: false,
		}, err
	}

	return &ToolResponse{
		Name:    "complete_chat",
		Content: "Chat session completed successfully",
		Success: true,
	}, nil
}

func (r *ChatToolRegistry) handleOverrideCompletion(ctx context.Context, app *application, user *User, appointment *Appointment, args map[string]interface{}) (*ToolResponse, error) {
	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = InitializeConversationChecklist()
	}

	// Mark as completed with override status
	appointment.ConversationChecklist.OverallStatus = ChecklistStatusCompleted
	appointment.ConversationChecklist.LastUpdated = time.Now()

	// Save the updated appointment
	if err := UpdateAppointment(ctx, app.fs, appointment); err != nil {
		return &ToolResponse{
			Name:    "override_completion",
			Content: "Failed to complete chat session",
			Success: false,
		}, err
	}

	return &ToolResponse{
		Name:    "override_completion",
		Content: "Chat session completed with override",
		Success: true,
	}, nil
}

func setupMockApp(t *testing.T) *MockApplication {
	// Use the test Firestore client
	fs, err := NewTestFirestoreClient(context.Background())
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	return &MockApplication{
		fs:     fs,
		logger: logger,
	}
}

func createTestAppointmentWithChecklist(t *testing.T, app *MockApplication, userID string) *Appointment {
	appointment := &Appointment{
		ID:                    "test-chat-" + userID,
		Name:                  "test-chat",
		UserID:                userID,
		Type:                  AppointmentTypeChat,
		ScheduledTime:         time.Now(),
		Status:                AppointmentStatusRunning,
		ChatMessages:          []ChatMessage{},
		ConversationChecklist: InitializeConversationChecklist(),
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err := CreateAppointment(context.Background(), app.fs, userID, appointment)
	if err != nil {
		t.Fatalf("Failed to create test appointment: %v", err)
	}

	return appointment
}

func createTestUser(prefix string) *User {
	now := time.Now()
	return &User{
		ID:              fmt.Sprintf("%s-%d", prefix, now.UnixNano()),
		CreatedAt:       now,
		Email:           fmt.Sprintf("%s@example.com", prefix),
		Name:            fmt.Sprintf("Test User %s", prefix),
		IsSuperUser:     false,
		Phone:           StringPtr("+14251234567"),
		Status:          UserStatusPendingResumeUpload,
		StatusUpdatedAt: now,
	}
}

func TestChatToolsIntegration(t *testing.T) {
	app := setupMockApp(t)
	defer app.fs.Close()

	// Create test user
	user := createTestUser("chat-tools-test")
	err := CreateUser(context.Background(), app.fs, user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	defer DeleteUser(context.Background(), app.fs, user.ID)

	// Create test appointment
	appointment := createTestAppointmentWithChecklist(t, app, user.ID)

	// Test the chat tool registry and handlers
	t.Run("GetChecklistStatus", func(t *testing.T) {
		testGetChecklistStatus(t, app, user, appointment)
	})

	t.Run("MarkTopicProgress", func(t *testing.T) {
		testMarkTopicProgress(t, app, user, appointment)
	})

	t.Run("ValidateInformationQuality", func(t *testing.T) {
		testValidateInformationQuality(t, app, user, appointment)
	})

	t.Run("CheckCompletionReadiness", func(t *testing.T) {
		testCheckCompletionReadiness(t, app, user, appointment)
	})

	t.Run("CompleteChatWithValidation", func(t *testing.T) {
		testCompleteChatWithValidation(t, app, user, appointment)
	})

	t.Run("OverrideCompletion", func(t *testing.T) {
		testOverrideCompletion(t, app, user, appointment)
	})
}

func testGetChecklistStatus(t *testing.T, app *MockApplication, user *User, appointment *Appointment) {
	// Create a mock tool registry for testing
	registry := &ChatToolRegistry{
		tools: make(map[string]ChatTool),
	}

	// Test get_checklist_status handler
	response, err := registry.handleGetChecklistStatus(context.Background(),
		&application{fs: app.fs, logger: app.logger},
		user,
		appointment,
		map[string]interface{}{})

	if err != nil {
		t.Fatalf("handleGetChecklistStatus failed: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success, got failure: %s", response.Content)
	}

	if response.Content == "" {
		t.Error("Expected content in response")
	}

	// Check that response contains expected status information
	expectedPhrases := []string{"Checklist Status"}
	for _, phrase := range expectedPhrases {
		if !contains(response.Content, phrase) {
			t.Errorf("Expected response to contain '%s', got: %s", phrase, response.Content)
		}
	}
}

func testMarkTopicProgress(t *testing.T, app *MockApplication, user *User, appointment *Appointment) {
	registry := &ChatToolRegistry{
		tools: make(map[string]ChatTool),
	}

	// Test marking topic progress
	args := map[string]interface{}{
		"topic_id": "introduction",
		"status":   ChecklistItemStatusCompleted,
		"quality":  ChecklistItemQualityHigh,
		"extracted_info": map[string]interface{}{
			"name_confirmed":    true,
			"good_time_to_chat": true,
		},
		"notes": "Successfully gathered introduction information",
	}

	response, err := registry.handleMarkTopicProgress(context.Background(),
		&application{fs: app.fs, logger: app.logger},
		user,
		appointment,
		args)

	if err != nil {
		t.Fatalf("handleMarkTopicProgress failed: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success, got failure: %s", response.Content)
	}

	// Verify the checklist was updated
	updatedAppointment, err := GetAppointment(context.Background(), app.fs, user.ID, appointment.ID)
	if err != nil {
		t.Fatalf("Failed to get updated appointment: %v", err)
	}

	item, err := GetChecklistItem(updatedAppointment.ConversationChecklist, "introduction")
	if err != nil {
		t.Fatalf("Failed to get checklist item: %v", err)
	}

	if item.Status != ChecklistItemStatusCompleted {
		t.Errorf("Expected status %s, got %s", ChecklistItemStatusCompleted, item.Status)
	}

	if item.Quality != ChecklistItemQualityHigh {
		t.Errorf("Expected quality %s, got %s", ChecklistItemQualityHigh, item.Quality)
	}

	if !item.ExtractedInfo["name_confirmed"].(bool) {
		t.Error("Expected name_confirmed to be true")
	}
}

func testValidateInformationQuality(t *testing.T, app *MockApplication, user *User, appointment *Appointment) {
	registry := &ChatToolRegistry{
		tools: make(map[string]ChatTool),
	}

	args := map[string]interface{}{
		"topic_id":         "introduction",
		"validation_notes": "Good quality information gathered",
	}

	response, err := registry.handleValidateInformationQuality(context.Background(),
		&application{fs: app.fs, logger: app.logger},
		user,
		appointment,
		args)

	if err != nil {
		t.Fatalf("handleValidateInformationQuality failed: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success, got failure: %s", response.Content)
	}

	// Verify the validation notes were updated
	updatedAppointment, err := GetAppointment(context.Background(), app.fs, user.ID, appointment.ID)
	if err != nil {
		t.Fatalf("Failed to get updated appointment: %v", err)
	}

	item, err := GetChecklistItem(updatedAppointment.ConversationChecklist, "introduction")
	if err != nil {
		t.Fatalf("Failed to get checklist item: %v", err)
	}

	if item.ValidationNotes != "Good quality information gathered" {
		t.Errorf("Expected validation notes to be updated, got: %s", item.ValidationNotes)
	}
}

func testCheckCompletionReadiness(t *testing.T, app *MockApplication, user *User, appointment *Appointment) {
	registry := &ChatToolRegistry{
		tools: make(map[string]ChatTool),
	}

	// Test with incomplete checklist
	response, err := registry.handleCheckCompletionReadiness(context.Background(),
		&application{fs: app.fs, logger: app.logger},
		user,
		appointment,
		map[string]interface{}{})

	if err != nil {
		t.Fatalf("handleCheckCompletionReadiness failed: %v", err)
	}

	if response.Success {
		t.Error("Expected completion readiness to fail with incomplete checklist")
	}

	// Complete all required items
	requiredItems := []string{"introduction", "warm_up", "deal_breakers", "search_strategy", "work_environment", "location_preferences", "final_confirmation"}

	for _, itemID := range requiredItems {
		err := UpdateChecklistItem(appointment.ConversationChecklist, itemID, ChecklistItemStatusCompleted, ChecklistItemQualityHigh, nil, "")
		if err != nil {
			t.Fatalf("Failed to update item %s: %v", itemID, err)
		}
	}

	// Save the updated appointment
	err = UpdateAppointment(context.Background(), app.fs, appointment)
	if err != nil {
		t.Fatalf("Failed to update appointment: %v", err)
	}

	// Test with complete checklist
	response, err = registry.handleCheckCompletionReadiness(context.Background(),
		&application{fs: app.fs, logger: app.logger},
		user,
		appointment,
		map[string]interface{}{})

	if err != nil {
		t.Fatalf("handleCheckCompletionReadiness failed: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected completion readiness to succeed with complete checklist, got: %s", response.Content)
	}
}

func testCompleteChatWithValidation(t *testing.T, app *MockApplication, user *User, appointment *Appointment) {
	registry := &ChatToolRegistry{
		tools: make(map[string]ChatTool),
	}

	// Test complete_chat with valid checklist (should succeed)
	args := map[string]interface{}{
		"summary":                  "Successfully gathered all required information",
		"confidence_level":         "high",
		"next_step_recommendation": "proceed_to_job_search",
	}

	response, err := registry.handleCompleteChat(context.Background(),
		&application{fs: app.fs, logger: app.logger},
		user,
		appointment,
		args)

	if err != nil {
		t.Fatalf("handleCompleteChat failed: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected complete_chat to succeed with valid checklist, got: %s", response.Content)
	}

	// Verify the appointment was updated
	updatedAppointment, err := GetAppointment(context.Background(), app.fs, user.ID, appointment.ID)
	if err != nil {
		t.Fatalf("Failed to get updated appointment: %v", err)
	}

	if updatedAppointment.ConversationChecklist.OverallStatus != ChecklistStatusCompleted {
		t.Errorf("Expected checklist status to be %s, got %s", ChecklistStatusCompleted, updatedAppointment.ConversationChecklist.OverallStatus)
	}
}

func testOverrideCompletion(t *testing.T, app *MockApplication, user *User, appointment *Appointment) {
	// Create a new appointment for override testing
	overrideAppointment := createTestAppointmentWithChecklist(t, app, user.ID)
	overrideAppointment.ID = "test-override-" + user.ID

	err := CreateAppointment(context.Background(), app.fs, user.ID, overrideAppointment)
	if err != nil {
		t.Fatalf("Failed to create override test appointment: %v", err)
	}

	registry := &ChatToolRegistry{
		tools: make(map[string]ChatTool),
	}

	// Test override completion
	args := map[string]interface{}{
		"reason":  "User needs to leave urgently",
		"summary": "Gathered partial information about preferences",
	}

	response, err := registry.handleOverrideCompletion(context.Background(),
		&application{fs: app.fs, logger: app.logger},
		user,
		overrideAppointment,
		args)

	if err != nil {
		t.Fatalf("handleOverrideCompletion failed: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected override completion to succeed, got: %s", response.Content)
	}

	// Verify the appointment was marked as completed
	updatedAppointment, err := GetAppointment(context.Background(), app.fs, user.ID, overrideAppointment.ID)
	if err != nil {
		t.Fatalf("Failed to get updated appointment: %v", err)
	}

	if updatedAppointment.ConversationChecklist.OverallStatus != ChecklistStatusCompleted {
		t.Errorf("Expected checklist status to be %s after override, got %s", ChecklistStatusCompleted, updatedAppointment.ConversationChecklist.OverallStatus)
	}
}
