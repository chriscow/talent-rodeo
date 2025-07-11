package internal

import (
	"context"
	"errors"
	"testing"
	"talent-rodeo/internal/inbox"
)

func TestHandlingResult(t *testing.T) {
	tests := []struct {
		name     string
		result   inbox.HandlingResult
		expected bool
	}{
		{
			name: "handled with summary",
			result: inbox.HandlingResult{
				Handled: true,
				Summary: "Updated user search preferences",
				Metadata: map[string]interface{}{
					"user_id": "123",
					"action":  "search_update",
				},
			},
			expected: true,
		},
		{
			name: "not handled",
			result: inbox.HandlingResult{
				Handled:  false,
				Summary:  "",
				Metadata: nil,
			},
			expected: false,
		},
		{
			name: "handled without summary",
			result: inbox.HandlingResult{
				Handled:  true,
				Summary:  "",
				Metadata: nil,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.Handled != tt.expected {
				t.Errorf("inbox.HandlingResult.Handled = %v, want %v", tt.result.Handled, tt.expected)
			}
		})
	}
}

func TestEmailHandlerFunc(t *testing.T) {
	// Test that EmailHandlerFunc implements the EmailHandler interface
	var handler EmailHandler = EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
		return &inbox.HandlingResult{
			Handled: true,
			Summary: "test summary",
		}, nil
	})

	email := &inbox.Email{
		ID:      "test-id",
		Subject: "test subject",
		Body:    "test body",
		From:    "test@example.com",
	}

	result, err := handler.HandleEmail(context.Background(), email)
	if err != nil {
		t.Errorf("EmailHandlerFunc returned error: %v", err)
	}

	if result == nil {
		t.Error("EmailHandlerFunc returned nil result")
		return
	}

	if !result.Handled {
		t.Error("EmailHandlerFunc result should be handled")
	}

	if result.Summary != "test summary" {
		t.Errorf("EmailHandlerFunc result summary = %q, want %q", result.Summary, "test summary")
	}
}

func TestInboxDispatch_Register(t *testing.T) {
	dispatch := NewInboxDispatch()

	// Test valid registration
	handler := EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
		return &inbox.HandlingResult{Handled: true}, nil
	})

	err := dispatch.Register("test-handler", handler)
	if err != nil {
		t.Errorf("Register returned error: %v", err)
	}

	if dispatch.GetHandlerCount() != 1 {
		t.Errorf("Handler count = %d, want 1", dispatch.GetHandlerCount())
	}

	// Test empty handler ID
	err = dispatch.Register("", handler)
	if err == nil {
		t.Error("Register should return error for empty handler ID")
	}

	// Test nil handler
	err = dispatch.Register("nil-handler", nil)
	if err == nil {
		t.Error("Register should return error for nil handler")
	}
}

func TestInboxDispatch_Deregister(t *testing.T) {
	dispatch := NewInboxDispatch()

	// Register a handler
	handler := EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
		return &inbox.HandlingResult{Handled: true}, nil
	})

	dispatch.Register("test-handler", handler)

	// Test successful deregistration
	removed := dispatch.Deregister("test-handler")
	if !removed {
		t.Error("Deregister should return true for existing handler")
	}

	if dispatch.GetHandlerCount() != 0 {
		t.Errorf("Handler count = %d, want 0", dispatch.GetHandlerCount())
	}

	// Test deregistering non-existent handler
	removed = dispatch.Deregister("non-existent")
	if removed {
		t.Error("Deregister should return false for non-existent handler")
	}
}

func TestInboxDispatch_HandleEmail(t *testing.T) {
	dispatch := NewInboxDispatch()

	// Test with no handlers
	email := &inbox.Email{
		ID:      "test-id",
		Subject: "test subject",
		Body:    "test body",
		From:    "test@example.com",
	}

	err := dispatch.HandleEmail(context.Background(), email)
	if err != nil {
		t.Errorf("HandleEmail should not return error with no handlers: %v", err)
	}

	// Test with successful handler
	successHandler := EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
		return &inbox.HandlingResult{
			Handled: true,
			Summary: "Successfully processed",
		}, nil
	})

	dispatch.Register("success-handler", successHandler)

	err = dispatch.HandleEmail(context.Background(), email)
	if err != nil {
		t.Errorf("HandleEmail returned error: %v", err)
	}

	// Test with failing handler
	failingHandler := EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) (*inbox.HandlingResult, error) {
		return nil, errors.New("handler failed")
	})

	dispatch.Register("failing-handler", failingHandler)

	err = dispatch.HandleEmail(context.Background(), email)
	if err == nil {
		t.Error("HandleEmail should return error when handler fails")
	}
}