package handlers

import (
	"context"
	"errors"
)

// Common errors
var (
	ErrCallNotFound = errors.New("call not found")
)

// TwilioClient defines the interface for making phone calls
type TwilioClient interface {
	// MakeCall initiates a phone call and returns a call SID
	MakeCall(ctx context.Context, from, to, twiml string) (string, error)

	// GetCallStatus retrieves the current status of a call
	GetCallStatus(ctx context.Context, callSID string) (string, error)

	// EndCall terminates an active call
	EndCall(ctx context.Context, callSID string) error
}

// MockTwilioClient implements TwilioClient for testing
type MockTwilioClient struct {
	calls map[string]string // callSID -> status
}

// NewMockTwilioClient creates a new mock Twilio client
func NewMockTwilioClient() *MockTwilioClient {
	return &MockTwilioClient{
		calls: make(map[string]string),
	}
}

func (m *MockTwilioClient) MakeCall(ctx context.Context, from, to, twiml string) (string, error) {
	// Generate a predictable test call SID
	callSID := "test_call_" + to
	m.calls[callSID] = "queued"
	return callSID, nil
}

func (m *MockTwilioClient) GetCallStatus(ctx context.Context, callSID string) (string, error) {
	if status, exists := m.calls[callSID]; exists {
		return status, nil
	}
	return "", ErrCallNotFound
}

func (m *MockTwilioClient) EndCall(ctx context.Context, callSID string) error {
	if _, exists := m.calls[callSID]; exists {
		m.calls[callSID] = "completed"
		return nil
	}
	return ErrCallNotFound
}

// SetCallStatus allows tests to manipulate call status
func (m *MockTwilioClient) SetCallStatus(callSID, status string) {
	m.calls[callSID] = status
}
