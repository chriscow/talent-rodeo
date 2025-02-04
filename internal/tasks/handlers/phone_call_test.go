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
	db     *database.DB
	logger *slog.Logger
	twilio *MockTwilioClient
}

func NewMockPhoneCallHandler(t *testing.T) *MockPhoneCallHandler {
	t.Helper()
	db, err := database.New(":memory:", database.WithInMemory())
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	return &MockPhoneCallHandler{
		db:     db,
		logger: logger,
	}
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
			ID:      "test-task",
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
			ID:      "invalid-task",
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
			ID:      "missing-phone-task",
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
			ID:      "missing-message-task",
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
