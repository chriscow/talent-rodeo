package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"talent-rodeo/internal"
	"testing"
	"time"

	"github.com/chriscow/vapi-go"
)

// TestProcessCompletedAppointmentIntegration tests the full flow for both successful and failed calls
func TestProcessCompletedAppointmentIntegration(t *testing.T) {
	testCases := []struct {
		name                  string
		webhookFile           string
		expectedStatus        internal.AppointmentStatus
		expectedEmailType     string
		expectedFailureReason string
	}{
		{
			name:                  "Voicemail Call - Should Fail",
			webhookFile:           "end-of-call-report_d61bb2a0-260f-454e-9b5d-93a51c848b2a.json",
			expectedStatus:        internal.AppointmentStatusFailed,
			expectedEmailType:     "failure",
			expectedFailureReason: "the call went to voicemail",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up test server
			ts := newTestServer(t, "appointment-integration")
			defer ts.Cleanup()

			// Create test user and appointment using test server helpers
			user := ts.user1
			appointment := &internal.Appointment{
				ID:            "test-appointment-" + generateRandomID(),
				Name:          "initial-call",
				UserID:        user.ID,
				ScheduledTime: time.Now(),
				Timezone:      "UTC",
				Status:        internal.AppointmentStatusScheduled,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}

			err := internal.CreateAppointment(context.Background(), ts.app.fs, user.ID, appointment)
			if err != nil {
				t.Fatalf("Failed to create test appointment: %v", err)
			}

			// Load webhook data
			webhookPath := filepath.Join("../../testdata/webhooks", tc.webhookFile)
			webhookData, err := os.ReadFile(webhookPath)
			if err != nil {
				t.Fatalf("Failed to read webhook file %s: %v", webhookPath, err)
			}

			// Parse webhook to get the end-of-call report
			var envelope vapi.EndOfCallReportEnvelope
			if err := json.Unmarshal(webhookData, &envelope); err != nil {
				t.Fatalf("Failed to unmarshal webhook: %v", err)
			}

			// Update the webhook data to reference our test appointment and user
			envelope.EndOfCallReport.Call.Name = &appointment.ID
			envelope.EndOfCallReport.Call.Customer = &vapi.Customer{Name: user.ID}

			// Add a mock call ID
			callID := "test-call-" + generateRandomID()
			envelope.EndOfCallReport.Call.ID = &callID

			// Create HTTP request with the webhook data
			updatedWebhookData, err := json.Marshal(envelope)
			if err != nil {
				t.Fatalf("Failed to marshal updated webhook: %v", err)
			}

			req := httptest.NewRequest("POST", "/webhooks/vapi", bytes.NewReader(updatedWebhookData))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			// Process the webhook
			ts.app.handleVapiWebhook(rr, req)

			// Check HTTP response
			if rr.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d. Response: %s", rr.Code, rr.Body.String())
			}

			// Wait a moment for async processing
			time.Sleep(2 * time.Second)

			// Verify appointment status
			updatedAppointment, err := internal.GetAppointment(context.Background(), ts.app.fs, user.ID, appointment.ID)
			if err != nil {
				t.Fatalf("Failed to get updated appointment: %v", err)
			}

			if updatedAppointment.Status != tc.expectedStatus {
				t.Errorf("Expected appointment status %s, got %s", tc.expectedStatus, updatedAppointment.Status)
			}

			// Verify end reason is set
			if updatedAppointment.EndReason == nil {
				t.Error("Expected EndReason to be set")
			} else if *updatedAppointment.EndReason != envelope.EndOfCallReport.EndedReason {
				t.Errorf("Expected EndReason %s, got %s", envelope.EndOfCallReport.EndedReason, *updatedAppointment.EndReason)
			}

			// Verify call ID is set
			if updatedAppointment.CallID == nil {
				t.Error("Expected CallID to be set")
			} else if *updatedAppointment.CallID != callID {
				t.Errorf("Expected CallID %s, got %s", callID, *updatedAppointment.CallID)
			}

			// Verify summary is set
			if updatedAppointment.Summary == nil {
				t.Error("Expected Summary to be set")
			}

			// Verify appropriate timestamp is set
			if tc.expectedStatus == internal.AppointmentStatusFailed {
				if updatedAppointment.FailedAt == nil {
					t.Error("Expected FailedAt to be set for failed appointment")
				}
				if updatedAppointment.CompletedAt != nil {
					t.Error("Expected CompletedAt to be nil for failed appointment")
				}
			} else {
				if updatedAppointment.CompletedAt == nil {
					t.Error("Expected CompletedAt to be set for completed appointment")
				}
				if updatedAppointment.FailedAt != nil {
					t.Error("Expected FailedAt to be nil for completed appointment")
				}
			}

			// Verify user status
			updatedUser, found, err := internal.GetUser(context.Background(), ts.app.fs, user.ID)
			if err != nil || !found {
				t.Fatalf("Failed to get updated user: %v", err)
			}

			if tc.expectedStatus == internal.AppointmentStatusFailed {
				// For failed appointments, user should be reset to allow retry
				if updatedUser.Status != internal.UserStatusResumeUploaded {
					t.Errorf("Expected user status %s for failed appointment, got %s", internal.UserStatusResumeUploaded, updatedUser.Status)
				}
			}

			// Log the complete webhook processing result for manual inspection
			t.Logf("\n=== Webhook Processing Result ===")
			t.Logf("Webhook File: %s", tc.webhookFile)
			t.Logf("End Reason: %s", envelope.EndOfCallReport.EndedReason)
			t.Logf("User-Friendly Reason: %s", tc.expectedFailureReason)
			t.Logf("Appointment Status: %s", updatedAppointment.Status)
			t.Logf("User Status: %s", updatedUser.Status)
			if updatedAppointment.Summary != nil {
				t.Logf("Summary: %s", *updatedAppointment.Summary)
			}
			t.Logf("================================\n")
		})
	}
}

// TestFailureReasonDescriptions tests the user-friendly failure reason mapping
func TestFailureReasonDescriptions(t *testing.T) {
	app := &application{}

	testCases := []struct {
		endReason   string
		description string
	}{
		{"voicemail", "the call went to voicemail"},
		{"busy", "your phone was busy"},
		{"no-answer", "there was no answer"},
		{"failed", "the call failed to connect"},
		{"cancelled", "the call was cancelled"},
		{"dial-busy", "we couldn't connect the call"},
		{"dial-failed", "we couldn't connect the call"},
		{"pipeline-error-openai", "there was a technical issue with our AI system"},
		{"pipeline-error-deepgram", "there was a technical issue with our AI system"},
		{"some-random-error", "there was a technical issue"},
		{"unknown-failure", "the call did not complete successfully"},
		{"", "there was an issue with the call"},
		{"customer-ended-call", "the call did not complete successfully"}, // This would normally be successful, but testing fallback
	}

	for _, tc := range testCases {
		t.Run(tc.endReason, func(t *testing.T) {
			description := app.getFailureReasonDescription(tc.endReason)
			if description != tc.description {
				t.Errorf("Expected description '%s' for endReason '%s', got '%s'", tc.description, tc.endReason, description)
			}
		})
	}
}

// Helper functions

func generateRandomID() string {
	return strings.ReplaceAll(time.Now().Format("20060102150405.000"), ".", "")
}
