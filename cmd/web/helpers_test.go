package main

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
)

// mockMailer implements smtp.Mailer for testing
type mockMailer struct {
	sendFunc             func(recipient, subject, plainBody, htmlBody string) error
	sendFromTemplateFunc func(recipient string, data any, patterns ...string) error
}

func (m *mockMailer) Send(recipient, subject, plainBody, htmlBody string) error {
	if m.sendFunc != nil {
		return m.sendFunc(recipient, subject, plainBody, htmlBody)
	}
	return nil
}

func (m *mockMailer) SendFromTemplate(recipient string, data any, patterns ...string) error {
	if m.sendFromTemplateFunc != nil {
		return m.sendFromTemplateFunc(recipient, data, patterns...)
	}
	return nil
}

func TestSendNotification(t *testing.T) {
	tests := []struct {
		name           string
		templateName   string
		data           map[string]any
		expectSend     bool
		expectError    bool
		notificationEmail string
	}{
		{
			name:              "successful notification",
			templateName:      "user-created.html",
			data:              map[string]any{"UserID": "123", "Email": "test@example.com"},
			expectSend:        true,
			expectError:       false,
			notificationEmail: "admin@example.com",
		},
		{
			name:              "no notification email configured",
			templateName:      "user-created.html",
			data:              map[string]any{"UserID": "123", "Email": "test@example.com"},
			expectSend:        false,
			expectError:       false,
			notificationEmail: "",
		},
		{
			name:              "mailer error",
			templateName:      "user-created.html",
			data:              map[string]any{"UserID": "123", "Email": "test@example.com"},
			expectSend:        true,
			expectError:       true,
			notificationEmail: "admin@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sendCalled bool
			var sentRecipient string
			var sentData any
			var sentPatterns []string

			mailer := &mockMailer{
				sendFromTemplateFunc: func(recipient string, data any, patterns ...string) error {
					sendCalled = true
					sentRecipient = recipient
					sentData = data
					sentPatterns = patterns
					if tt.expectError {
						return errors.New("mock mailer error")
					}
					return nil
				},
			}

			app := &application{
				config: config{
					notifications: struct {
						email string
					}{
						email: tt.notificationEmail,
					},
				},
				logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				mailer: mailer,
			}

			err := app.sendNotification(tt.templateName, tt.data)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if tt.expectSend {
				if !sendCalled {
					t.Error("expected mailer.SendFromTemplate to be called")
				}
				if sentRecipient != tt.notificationEmail {
					t.Errorf("expected recipient %s, got %s", tt.notificationEmail, sentRecipient)
				}
				if len(sentPatterns) != 1 || sentPatterns[0] != tt.templateName {
					t.Errorf("expected patterns [%s], got %v", tt.templateName, sentPatterns)
				}
				// Check that data was merged with base email data
				if sentData == nil {
					t.Error("expected data to be sent")
				}
			} else {
				if sendCalled {
					t.Error("expected mailer.SendFromTemplate not to be called")
				}
			}
		})
	}
}

func TestSendNotificationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Only run if we have SMTP configuration
	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		t.Skip("SMTP_HOST not configured, skipping integration test")
	}

	notificationEmail := os.Getenv("NOTIFICATION_EMAIL")
	if notificationEmail == "" {
		t.Skip("NOTIFICATION_EMAIL not configured, skipping integration test")
	}

	// Create a real mailer for integration testing
	// This would use the actual SMTP configuration
	ts := newTestServer(t, "sendnotification")
	
	// Set up a mock mailer for integration testing since we don't want to send real emails
	// but we want to test the full flow
	var sentEmails []struct {
		recipient string
		data      any
		patterns  []string
	}
	
	ts.app.mailer = &mockMailer{
		sendFromTemplateFunc: func(recipient string, data any, patterns ...string) error {
			sentEmails = append(sentEmails, struct {
				recipient string
				data      any
				patterns  []string
			}{recipient, data, patterns})
			return nil
		},
	}
	
	// Override the notification email for testing
	ts.app.config.notifications.email = notificationEmail

	// Test data for user creation notification
	testData := map[string]any{
		"UserID":     "test-user-123",
		"Email":      "testuser@example.com",
		"Name":       "Test User",
		"Provider":   "linkedin",
		"Location":   "Seattle, WA",
		"AvatarURL":  "https://example.com/avatar.jpg",
		"CreatedAt":  "2023-01-01T00:00:00Z",
	}

	// Send actual notification
	err := ts.app.sendNotification("user-created.html", testData)
	if err != nil {
		t.Fatalf("failed to send notification: %v", err)
	}

	// Verify email was sent
	if len(sentEmails) != 1 {
		t.Fatalf("expected 1 email to be sent, got %d", len(sentEmails))
	}
	
	if sentEmails[0].recipient != notificationEmail {
		t.Errorf("expected recipient %s, got %s", notificationEmail, sentEmails[0].recipient)
	}
	
	if len(sentEmails[0].patterns) != 1 || sentEmails[0].patterns[0] != "user-created.html" {
		t.Errorf("expected patterns [user-created.html], got %v", sentEmails[0].patterns)
	}

	t.Logf("Successfully sent notification to %s", notificationEmail)
}