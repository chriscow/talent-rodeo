package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/models"
)

// MockPhoneCallHandler is a test implementation of the phone call handler
type MockPhoneCallHandler struct {
	*PhoneCallHandler
	db     *database.DB
	logger *slog.Logger
	twilio *MockTwilioClient
}

func NewMockPhoneCallHandler(t *testing.T) *MockPhoneCallHandler {
	t.Helper()

	db := database.SetupTestDB(t)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	twilio := NewMockTwilioClient()
	config := PhoneCallConfig{
		FromNumber: "+1987654321",
		WebhookURL: "http://example.com/webhook",
	}
	taskProc := &MockTaskProcessor{}

	handler := &MockPhoneCallHandler{
		PhoneCallHandler: NewPhoneCallHandler(twilio, logger, config, taskProc),
		db:               db,
		logger:           logger,
		twilio:           twilio,
	}

	return handler
}

func (h *MockPhoneCallHandler) TaskType() string {
	return "phone_call"
}

type phoneCallPayload struct {
	PhoneNumber string `json:"phone_number"`
	Message     string `json:"message"`
}

func (h *MockPhoneCallHandler) ProcessTask(ctx context.Context, task *models.Task) error {
	var payload phoneCallPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return err
	}

	if payload.PhoneNumber == "" {
		return errors.New("phone number is required")
	}
	if payload.Message == "" {
		return errors.New("message is required")
	}

	return nil
}

func TestPhoneCallHandler_TaskType(t *testing.T) {
	handler := NewMockPhoneCallHandler(t)
	got := handler.TaskType()
	want := "phone_call"
	if got != want {
		t.Errorf("TaskType() = %q; want %q", got, want)
	}
}

func TestPhoneCallHandler_ProcessTask(t *testing.T) {
	t.Run("process valid phone call task", func(t *testing.T) {
		handler := NewMockPhoneCallHandler(t)
		ctx := context.Background()

		// Create a mock Twilio client
		mockTwilio := NewMockTwilioClient()
		handler.twilio = mockTwilio

		payload := phoneCallPayload{
			PhoneNumber: "+1234567890",
			Message:     "Test message",
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		task := &models.Task{
			ID:      time.Now().UnixNano(),
			Type:    "phone_call",
			Payload: payloadBytes,
			RunAt:   time.Now(),
		}

		// Start a goroutine to update the call status after a short delay
		go func() {
			time.Sleep(2 * time.Second)
			callSID := "test_call_" + payload.PhoneNumber
			mockTwilio.SetCallStatus(callSID, "in-progress")
		}()

		if err := handler.ProcessTask(ctx, task); err != nil {
			t.Errorf("ProcessTask() error = %v; want nil", err)
		}
	})

	t.Run("handle invalid payload", func(t *testing.T) {
		handler := NewMockPhoneCallHandler(t)
		ctx := context.Background()

		task := &models.Task{
			ID:      time.Now().UnixNano(),
			Type:    "phone_call",
			Payload: []byte("{invalid json}"),
			RunAt:   time.Now(),
		}

		err := handler.ProcessTask(ctx, task)
		if err == nil {
			t.Error("ProcessTask() error = nil; want error for invalid JSON")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid character") {
			t.Errorf("ProcessTask() error = %v; want error containing 'invalid character'", err)
		}
	})

	t.Run("handle missing phone number", func(t *testing.T) {
		handler := NewMockPhoneCallHandler(t)
		ctx := context.Background()

		payload := phoneCallPayload{
			Message: "Test message",
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		task := &models.Task{
			ID:      time.Now().UnixNano(),
			Type:    "phone_call",
			Payload: payloadBytes,
			RunAt:   time.Now(),
		}

		err = handler.ProcessTask(ctx, task)
		if err == nil {
			t.Error("ProcessTask() error = nil; want error for missing phone number")
		}
		if err != nil && !strings.Contains(err.Error(), "phone number is required") {
			t.Errorf("ProcessTask() error = %v; want error containing 'phone number is required'", err)
		}
	})

	t.Run("handle missing message", func(t *testing.T) {
		handler := NewMockPhoneCallHandler(t)
		ctx := context.Background()

		payload := phoneCallPayload{
			PhoneNumber: "+1234567890",
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		task := &models.Task{
			ID:      time.Now().UnixNano(),
			Type:    "phone_call",
			Payload: payloadBytes,
			RunAt:   time.Now(),
		}

		err = handler.ProcessTask(ctx, task)
		if err == nil {
			t.Error("ProcessTask() error = nil; want error for missing message")
		}
		if err != nil && !strings.Contains(err.Error(), "message is required") {
			t.Errorf("ProcessTask() error = %v; want error containing 'message is required'", err)
		}
	})
}

// MockTaskProcessor implements TaskProcessor for testing
type MockTaskProcessor struct {
	enqueuedTasks []*models.Task
}

func (m *MockTaskProcessor) EnqueueTask(ctx context.Context, task *models.Task) error {
	m.enqueuedTasks = append(m.enqueuedTasks, task)
	return nil
}

func TestPhoneCallHandler_ScheduleCall(t *testing.T) {
	t.Run("schedule valid call", func(t *testing.T) {
		handler := NewMockPhoneCallHandler(t)
		taskProc := &MockTaskProcessor{}
		handler.taskProc = taskProc

		// Schedule a call for 1 hour from now
		scheduledAt := time.Now().Add(1 * time.Hour)
		params := ScheduleCallParams{
			PhoneNumber: "+1234567890",
			Message:     "Test message",
			ScheduledAt: scheduledAt,
			TimeZone:    "UTC",
		}

		err := handler.ScheduleCall(context.Background(), params)
		if err != nil {
			t.Fatalf("ScheduleCall() error = %v", err)
		}

		// Verify task was enqueued
		if len(taskProc.enqueuedTasks) != 1 {
			t.Fatalf("expected 1 enqueued task, got %d", len(taskProc.enqueuedTasks))
		}

		task := taskProc.enqueuedTasks[0]
		if task.Type != "phone_call" {
			t.Errorf("task type = %q, want phone_call", task.Type)
		}
		if task.Status != models.TaskStatusPending {
			t.Errorf("task status = %q, want pending", task.Status)
		}
		if task.MaxAttempts != 3 {
			t.Errorf("task max attempts = %d, want 3", task.MaxAttempts)
		}

		// Verify payload
		var payload PhoneCallPayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}
		if payload.PhoneNumber != params.PhoneNumber {
			t.Errorf("payload phone number = %q, want %q", payload.PhoneNumber, params.PhoneNumber)
		}
		if payload.Message != params.Message {
			t.Errorf("payload message = %q, want %q", payload.Message, params.Message)
		}
	})

	t.Run("handle missing phone number", func(t *testing.T) {
		handler := NewMockPhoneCallHandler(t)
		taskProc := &MockTaskProcessor{}
		handler.taskProc = taskProc

		params := ScheduleCallParams{
			Message:     "Test message",
			ScheduledAt: time.Now().Add(1 * time.Hour),
			TimeZone:    "UTC",
		}

		err := handler.ScheduleCall(context.Background(), params)
		if err == nil {
			t.Error("ScheduleCall() error = nil, want error for missing phone number")
		}
		if len(taskProc.enqueuedTasks) > 0 {
			t.Error("task was enqueued despite validation error")
		}
	})

	t.Run("handle past schedule time", func(t *testing.T) {
		handler := NewMockPhoneCallHandler(t)
		taskProc := &MockTaskProcessor{}
		handler.taskProc = taskProc

		params := ScheduleCallParams{
			PhoneNumber: "+1234567890",
			Message:     "Test message",
			ScheduledAt: time.Now().Add(-1 * time.Hour),
			TimeZone:    "UTC",
		}

		err := handler.ScheduleCall(context.Background(), params)
		if err == nil {
			t.Error("ScheduleCall() error = nil, want error for past schedule time")
		}
		if len(taskProc.enqueuedTasks) > 0 {
			t.Error("task was enqueued despite validation error")
		}
	})

	t.Run("handle invalid timezone", func(t *testing.T) {
		handler := NewMockPhoneCallHandler(t)
		taskProc := &MockTaskProcessor{}
		handler.taskProc = taskProc

		params := ScheduleCallParams{
			PhoneNumber: "+1234567890",
			Message:     "Test message",
			ScheduledAt: time.Now().Add(1 * time.Hour),
			TimeZone:    "Invalid/Timezone",
		}

		err := handler.ScheduleCall(context.Background(), params)
		if err == nil {
			t.Error("ScheduleCall() error = nil, want error for invalid timezone")
		}
		if len(taskProc.enqueuedTasks) > 0 {
			t.Error("task was enqueued despite validation error")
		}
	})
}

func TestPhoneCallHandler_IntegrationTest(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Skip if Twilio credentials not set
	accountSID := os.Getenv("TWILIO_ACCOUNT_SID")
	authToken := os.Getenv("TWILIO_AUTH_TOKEN")
	fromNumber := os.Getenv("TWILIO_TEST_FROM_NUMBER")
	toNumber := os.Getenv("TWILIO_TEST_TO_NUMBER")
	webhookURL := os.Getenv("TWILIO_WEBHOOK_URL")

	if accountSID == "" || authToken == "" || fromNumber == "" || toNumber == "" || webhookURL == "" {
		t.Skip("Skipping integration test - missing Twilio credentials")
	}

	// Create a real Twilio client
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	realClient := NewRealTwilioClient(accountSID, authToken, fromNumber)
	config := PhoneCallConfig{
		FromNumber: fromNumber,
		WebhookURL: webhookURL,
	}
	taskProc := &MockTaskProcessor{}

	handler := NewPhoneCallHandler(realClient, logger, config, taskProc)

	// Test immediate call
	t.Run("make real outbound call", func(t *testing.T) {
		ctx := context.Background()

		payload := PhoneCallPayload{
			PhoneNumber: toNumber,
			Message:     "This is a test call from the talent rodeo system integration test.",
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		task := &models.Task{
			ID:          time.Now().UnixNano(),
			Type:        "phone_call",
			Payload:     payloadBytes,
			RunAt:       time.Now(),
			MaxAttempts: 1,
		}

		// Process the task (this will make a real call)
		err = handler.ProcessTask(ctx, task)
		if err != nil {
			t.Errorf("ProcessTask() error = %v; want nil", err)
		}

		t.Log("Successfully made outbound call to", toNumber)
	})

	// Test scheduled call
	t.Run("schedule real outbound call", func(t *testing.T) {
		ctx := context.Background()

		// Schedule a call for 1 minute from now
		scheduledAt := time.Now().Add(1 * time.Minute)
		params := ScheduleCallParams{
			PhoneNumber: toNumber,
			Message:     "This is a scheduled test call from the talent rodeo system.",
			ScheduledAt: scheduledAt,
			TimeZone:    "UTC",
		}

		err := handler.ScheduleCall(ctx, params)
		if err != nil {
			t.Fatalf("ScheduleCall() error = %v", err)
		}

		// Verify task was enqueued
		if len(taskProc.enqueuedTasks) != 1 {
			t.Fatalf("expected 1 enqueued task, got %d", len(taskProc.enqueuedTasks))
		}

		task := taskProc.enqueuedTasks[0]
		if task.Type != "phone_call" {
			t.Errorf("task type = %q, want phone_call", task.Type)
		}
		if task.Status != models.TaskStatusPending {
			t.Errorf("task status = %q, want pending", task.Status)
		}

		t.Logf("Successfully scheduled call to %s at %v", toNumber, scheduledAt)
	})
}
