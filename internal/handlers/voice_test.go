package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockTwilioClient implements TwilioClient for testing
type mockTwilioClient struct {
	calls []struct {
		fromNumber string
		toNumber   string
		twiml      string
	}
	callSID string
	err     error
}

func (m *mockTwilioClient) MakeCall(ctx context.Context, fromNumber, toNumber, twiml string) (string, error) {
	m.calls = append(m.calls, struct {
		fromNumber string
		toNumber   string
		twiml      string
	}{fromNumber, toNumber, twiml})
	return m.callSID, m.err
}

func TestVoiceHandleMessage(t *testing.T) {
	tests := []struct {
		name        string
		input       CallRequest
		want        CallResult
		twilioError error
		wantErr     bool
	}{
		{
			name: "successful call",
			input: CallRequest{
				PhoneNumber: "+1234567890",
			},
			want: CallResult{
				CallSID:     "test-call-sid",
				PhoneNumber: "+1234567890",
				Status:      "initiated",
			},
			twilioError: nil,
			wantErr:     false,
		},
		{
			name: "failed call",
			input: CallRequest{
				PhoneNumber: "+1234567890",
			},
			want: CallResult{
				PhoneNumber: "+1234567890",
				Status:      "failed",
			},
			twilioError: fmt.Errorf("twilio error"),
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			defer env.cleanup()

			// Create mock Twilio client
			twilio := &mockTwilioClient{
				callSID: tt.want.CallSID,
				err:     tt.twilioError,
			}

			// Create handler config
			config := CallConfig{
				FromNumber: "+19876543210",
				Twiml:      "<Response><Say>Hello</Say></Response>",
			}

			// Create handler with a background context since we don't need to control its lifecycle.
			// The handler is required even though we don't explicitly use it because it registers
			// the message handler with the GenericHandler during creation.
			_, err := NewVoiceHandler(context.Background(), env.js, twilio, env.logger, config)
			if err != nil {
				t.Fatalf("Failed to create handler: %v", err)
			}

			// Send test message and verify response
			got, err := SendAndReceiveMessage[CallRequest, CallResult](
				t,
				context.Background(),
				env.js,
				rodeoStream,
				TopicVoiceInput,
				TopicVoiceResult,
				tt.input,
				1*time.Second,
			)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SendAndReceiveMessage error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify Twilio call was made with correct parameters
			if !tt.wantErr && tt.twilioError == nil {
				if len(twilio.calls) != 1 {
					t.Errorf("Expected 1 Twilio call, got %d", len(twilio.calls))
				} else {
					call := twilio.calls[0]
					if call.fromNumber != config.FromNumber {
						t.Errorf("Wrong from number: got %s, want %s", call.fromNumber, config.FromNumber)
					}
					if call.toNumber != tt.input.PhoneNumber {
						t.Errorf("Wrong to number: got %s, want %s", call.toNumber, tt.input.PhoneNumber)
					}
					if call.twiml != config.Twiml {
						t.Errorf("Wrong TwiML: got %s, want %s", call.twiml, config.Twiml)
					}
				}
			}

			// Verify response
			if got.CallSID != tt.want.CallSID {
				t.Errorf("Wrong CallSID: got %s, want %s", got.CallSID, tt.want.CallSID)
			}
			if got.PhoneNumber != tt.want.PhoneNumber {
				t.Errorf("Wrong PhoneNumber: got %s, want %s", got.PhoneNumber, tt.want.PhoneNumber)
			}
			if got.Status != tt.want.Status {
				t.Errorf("Wrong Status: got %s, want %s", got.Status, tt.want.Status)
			}
		})
	}
}
