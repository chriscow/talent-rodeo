package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockTwilioClient implements TwilioClient for testing
type mockTwilioClient struct {
	callSID string
	err     error
}

func (m *mockTwilioClient) MakeCall(ctx context.Context, fromNumber, toNumber, twiml string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.callSID, nil
}

func TestMakeCall(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	tests := []struct {
		name        string
		phoneNumber string
		mockCallSID string
		mockErr     error
		want        *CallResult
		wantErr     bool
	}{
		{
			name:        "successful call",
			phoneNumber: "+1234567890",
			mockCallSID: "call-123",
			want: &CallResult{
				CallSID:     "call-123",
				PhoneNumber: "+1234567890",
				Status:      "initiated",
			},
		},
		{
			name:        "twilio error",
			phoneNumber: "+1234567890",
			mockErr:     fmt.Errorf("twilio error"),
			wantErr:     true,
		},
		{
			name:        "invalid phone number",
			phoneNumber: "1234567890", // missing plus
			mockErr:     fmt.Errorf("Invalid phone number: The 'To' number 1234567890 is not a valid phone number"),
			wantErr:     true,
		},
		{
			name:        "empty phone number",
			phoneNumber: "",
			mockErr:     fmt.Errorf("Invalid phone number: The 'To' number is required"),
			wantErr:     true,
		},
	}

	config := CallConfig{
		FromNumber: "+1987654321",
		Twiml:      "<Response><Say>Test call</Say></Response>",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock Twilio client for this test
			twilio := &mockTwilioClient{
				callSID: tt.mockCallSID,
				err:     tt.mockErr,
			}

			// Create handler
			handler, err := NewVoiceHandler(context.Background(), env.js, twilio, env.logger, config)
			if err != nil {
				t.Fatalf("Failed to create handler: %v", err)
			}

			// Test makeCall
			got, err := handler.makeCall(context.Background(), CallRequest{PhoneNumber: tt.phoneNumber})
			if (err != nil) != tt.wantErr {
				t.Errorf("makeCall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got.CallSID != tt.want.CallSID {
				t.Errorf("CallSID = %v, want %v", got.CallSID, tt.want.CallSID)
			}
			if got.PhoneNumber != tt.want.PhoneNumber {
				t.Errorf("PhoneNumber = %v, want %v", got.PhoneNumber, tt.want.PhoneNumber)
			}
			if got.Status != tt.want.Status {
				t.Errorf("Status = %v, want %v", got.Status, tt.want.Status)
			}
		})
	}
}

func TestHandleMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	config := CallConfig{
		FromNumber: "+1987654321",
		Twiml:      "<Response><Say>Test call</Say></Response>",
	}

	tests := []struct {
		name        string
		input       CallRequest
		mockCallSID string
		mockErr     error
		want        *CallResult
		wantErr     bool
	}{
		{
			name: "successful call",
			input: CallRequest{
				PhoneNumber: "+1234567890",
			},
			mockCallSID: "call-123",
			want: &CallResult{
				CallSID:     "call-123",
				PhoneNumber: "+1234567890",
				Status:      "initiated",
			},
		},
		{
			name: "failed call",
			input: CallRequest{
				PhoneNumber: "+1234567890",
			},
			mockErr: fmt.Errorf("twilio error"),
			want: &CallResult{
				PhoneNumber: "+1234567890",
				Status:      "failed",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock Twilio client for this test
			twilio := &mockTwilioClient{
				callSID: tt.mockCallSID,
				err:     tt.mockErr,
			}

			// Create handler
			handler, err := NewVoiceHandler(context.Background(), env.js, twilio, env.logger, config)
			if err != nil {
				t.Fatalf("Failed to create handler: %v", err)
			}

			// Start the handler
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // Increase timeout duration
			defer cancel()
			go handler.Start(ctx)
			time.Sleep(100 * time.Millisecond) // Allow handler to initialize

			// Send and receive message
			got, err := SendAndReceiveMessage[CallRequest, CallResult](
				t,
				context.Background(),
				env.js,
				rodeoStream,
				TopicVoiceInput,
				TopicVoiceResult,
				tt.input,
				5*time.Second, // Increase wait duration
			)
			if err != nil && !tt.wantErr {
				t.Fatalf("Failed to send and receive message: %v", err)
			}

			// Verify results
			if got.CallSID != tt.want.CallSID {
				t.Errorf("CallSID = %v, want %v", got.CallSID, tt.want.CallSID)
			}
			if got.PhoneNumber != tt.want.PhoneNumber {
				t.Errorf("PhoneNumber = %v, want %v", got.PhoneNumber, tt.want.PhoneNumber)
			}
			if got.Status != tt.want.Status {
				t.Errorf("Status = %v, want %v", got.Status, tt.want.Status)
			}
		})
	}
}
