//go:build integration
// +build integration

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"talent-rodeo/internal/tasks/handlers"
	"testing"
	"time"

	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
)

// realTwilioClient implements TwilioClient for integration tests
type realTwilioClient struct {
	client     *twilio.RestClient
	logger     *slog.Logger
	fromNumber string
}

func newRealTwilioClient(accountSID, authToken, fromNumber string, logger *slog.Logger) *realTwilioClient {
	logger.Debug("Creating Twilio client",
		"account_sid", accountSID,
		"from_number", fromNumber)

	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: accountSID,
		Password: authToken,
	})

	return &realTwilioClient{
		client:     client,
		fromNumber: fromNumber,
		logger:     logger,
	}
}

func (c *realTwilioClient) MakeCall(ctx context.Context, from, to, twiml string) (string, error) {
	if from == "" {
		c.logger.Debug("No 'from' number specified, using default",
			"default_from", c.fromNumber)
		from = c.fromNumber
	}

	c.logger.Debug("Making Twilio call",
		"from", from,
		"to", to,
		"twiml", twiml)

	params := &twilioApi.CreateCallParams{}
	params.SetTo(to)
	params.SetFrom(from)
	params.SetTwiml(twiml)

	resp, err := c.client.Api.CreateCall(params)
	if err != nil {
		return "", fmt.Errorf("failed to create call: %w", err)
	}

	c.logger.Info("Call initiated",
		"call_sid", *resp.Sid,
		"status", *resp.Status)

	return *resp.Sid, nil
}

func TestIntegrationVoiceCalls(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create test environment
	env := newTestEnv(t)
	defer env.cleanup()

	// Validate required environment variables
	testToNumber := os.Getenv("TWILIO_TEST_TO_NUMBER")
	if testToNumber == "" {
		t.Skip("TWILIO_TEST_TO_NUMBER environment variable not set")
	}

	// Create real Twilio client
	twilio := newRealTwilioClient(
		os.Getenv("TWILIO_ACCOUNT_SID"),
		os.Getenv("TWILIO_AUTH_TOKEN"),
		os.Getenv("TWILIO_FROM_NUMBER"),
		env.logger,
	)

	config := CallConfig{
		FromNumber: os.Getenv("TWILIO_FROM_NUMBER"),
		Twiml:      "<Response><Say>This is an integration test call from the talent rodeo system. Please ignore.</Say></Response>",
	}

	tests := []struct {
		name        string
		phoneNumber string
		wantStatus  string
		wantErr     bool
	}{
		{
			name:        "real call to test number",
			phoneNumber: testToNumber,
			wantStatus:  "initiated",
		},
		{
			name:        "invalid phone number",
			phoneNumber: "+1invalid",
			wantStatus:  "failed",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env.logger.Info("Starting integration test",
				"test_name", tt.name,
				"phone_number", tt.phoneNumber)

			// Create handler
			handler, err := NewVoiceHandler(context.Background(), env.js, twilio, env.logger, config)
			if err != nil {
				t.Fatalf("Failed to create handler: %v", err)
			}

			// Register the handler
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := handler.generic.Register(ctx, rodeoStream, TopicVoiceInput, "voice-test", handler.handleMessage); err != nil {
				t.Fatalf("Failed to register handler: %v", err)
			}
			time.Sleep(100 * time.Millisecond) // Allow handler to initialize

			// Send and receive message
			callReq := CallRequest{PhoneNumber: tt.phoneNumber}
			callReqJSON, err := json.Marshal(callReq)
			if err != nil {
				t.Fatalf("Failed to marshal call request: %v", err)
			}

			msgData := handlers.MessageData{
				Data: callReqJSON,
				Metadata: map[string]string{
					"test": "true",
				},
			}

			got, err := SendAndReceiveMessage[handlers.MessageData, CallResult](
				t,
				context.Background(),
				env.js,
				rodeoStream,
				TopicVoiceInput,
				TopicVoiceResult,
				msgData,
				5*time.Second,
				fmt.Sprintf("voice-test-%s", strings.ReplaceAll(tt.name, " ", "-")),
			)
			if err != nil {
				t.Fatalf("Failed to send and receive message: %v", err)
			}

			// Verify results
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", got.Status, tt.wantStatus)
			}
			if got.PhoneNumber != tt.phoneNumber {
				t.Errorf("PhoneNumber = %v, want %v", got.PhoneNumber, tt.phoneNumber)
			}
			if !tt.wantErr && got.CallSID == "" {
				t.Error("Expected CallSID, got empty string")
			}

			// Add delay between tests to avoid rate limiting
			time.Sleep(2 * time.Second)

			env.logger.Info("Integration test completed",
				"test_name", tt.name,
				"success", true)
		})
	}
}
