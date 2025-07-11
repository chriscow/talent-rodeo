package inbox

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// mockMailer implements the Mailer interface for testing
type mockMailer struct {
	sentEmails []sentEmail
	err        error
}

type sentEmail struct {
	to        string
	subject   string
	plainBody string
	htmlBody  string
}

func (m *mockMailer) Send(to, subject, plainBody, htmlBody string) error {
	if m.err != nil {
		return m.err
	}
	m.sentEmails = append(m.sentEmails, sentEmail{
		to:        to,
		subject:   subject,
		plainBody: plainBody,
		htmlBody:  htmlBody,
	})
	return nil
}

func (m *mockMailer) SendFromTemplate(recipient string, data any, patterns ...string) error {
	if m.err != nil {
		return m.err
	}
	// For testing purposes, we'll just record a basic template send
	m.sentEmails = append(m.sentEmails, sentEmail{
		to:        recipient,
		subject:   "Template Email",
		plainBody: "Template content",
		htmlBody:  "Template HTML content",
	})
	return nil
}

func (m *mockMailer) getSentEmails() []sentEmail {
	return m.sentEmails
}

func (m *mockMailer) reset() {
	m.sentEmails = nil
	m.err = nil
}

func TestNewResponseSender(t *testing.T) {
	mailer := &mockMailer{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	sender := NewResponseSender(mailer, logger)
	
	if sender == nil {
		t.Error("NewResponseSender should not return nil")
	}
	
	if sender.logger != logger {
		t.Error("NewResponseSender should set the logger")
	}
}

func TestResponseSender_SendResponse(t *testing.T) {
	tests := []struct {
		name          string
		response      *EmailResponse
		recipient     string
		mailerError   error
		expectedError bool
	}{
		{
			name: "successful send",
			response: &EmailResponse{
				Type:        ResponseTypeConfirmation,
				Subject:     "Test Response",
				Body:        "Test Body",
				HTMLBody:    "<html><body>Test Body</body></html>",
				Summaries:   []string{"Action completed"},
				GeneratedAt: time.Now(),
			},
			recipient:     "user@example.com",
			expectedError: false,
		},
		{
			name: "successful send with name format",
			response: &EmailResponse{
				Type:        ResponseTypeConfirmation,
				Subject:     "Test Response",
				Body:        "Test Body",
				HTMLBody:    "<html><body>Test Body</body></html>",
				Summaries:   []string{"Action completed"},
				GeneratedAt: time.Now(),
			},
			recipient:     "John Doe <john@example.com>",
			expectedError: false,
		},
		{
			name:          "nil response",
			response:      nil,
			recipient:     "user@example.com",
			expectedError: true,
		},
		{
			name: "empty recipient",
			response: &EmailResponse{
				Type:        ResponseTypeConfirmation,
				Subject:     "Test Response",
				Body:        "Test Body",
				HTMLBody:    "<html><body>Test Body</body></html>",
				Summaries:   []string{"Action completed"},
				GeneratedAt: time.Now(),
			},
			recipient:     "",
			expectedError: true,
		},
		{
			name: "invalid response",
			response: &EmailResponse{
				Type:        ResponseTypeConfirmation,
				Subject:     "",
				Body:        "",
				HTMLBody:    "",
				GeneratedAt: time.Now(),
			},
			recipient:     "user@example.com",
			expectedError: true,
		},
		{
			name: "invalid recipient email",
			response: &EmailResponse{
				Type:        ResponseTypeConfirmation,
				Subject:     "Test Response",
				Body:        "Test Body",
				HTMLBody:    "<html><body>Test Body</body></html>",
				Summaries:   []string{"Action completed"},
				GeneratedAt: time.Now(),
			},
			recipient:     "invalid-email",
			expectedError: true,
		},
		{
			name: "mailer error",
			response: &EmailResponse{
				Type:        ResponseTypeConfirmation,
				Subject:     "Test Response",
				Body:        "Test Body",
				HTMLBody:    "<html><body>Test Body</body></html>",
				Summaries:   []string{"Action completed"},
				GeneratedAt: time.Now(),
			},
			recipient:     "user@example.com",
			mailerError:   errors.New("SMTP error"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mailer := &mockMailer{err: tt.mailerError}
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			sender := NewResponseSender(mailer, logger)

			err := sender.SendResponse(context.Background(), tt.response, tt.recipient)

			if (err != nil) != tt.expectedError {
				t.Errorf("SendResponse() error = %v, expectedError %v", err, tt.expectedError)
				return
			}

			if !tt.expectedError && tt.mailerError == nil {
				sentEmails := mailer.getSentEmails()
				if len(sentEmails) != 1 {
					t.Errorf("Expected 1 sent email, got %d", len(sentEmails))
					return
				}

				sent := sentEmails[0]
				expectedRecipient := tt.recipient
				if strings.Contains(tt.recipient, "<") {
					// Extract email from "Name <email>" format
					start := strings.Index(tt.recipient, "<")
					end := strings.Index(tt.recipient, ">")
					if start < end {
						expectedRecipient = strings.TrimSpace(tt.recipient[start+1 : end])
					}
				}

				if sent.to != expectedRecipient {
					t.Errorf("Expected recipient %q, got %q", expectedRecipient, sent.to)
				}

				if sent.subject != tt.response.Subject {
					t.Errorf("Expected subject %q, got %q", tt.response.Subject, sent.subject)
				}

				if sent.plainBody != tt.response.Body {
					t.Errorf("Expected body %q, got %q", tt.response.Body, sent.plainBody)
				}

				if sent.htmlBody != tt.response.HTMLBody {
					t.Errorf("Expected HTML body %q, got %q", tt.response.HTMLBody, sent.htmlBody)
				}
			}
		})
	}
}

func TestResponseSender_validateAndCleanEmail(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	sender := NewResponseSender(&mockMailer{}, logger)

	tests := []struct {
		name     string
		email    string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple email",
			email:    "user@example.com",
			expected: "user@example.com",
			wantErr:  false,
		},
		{
			name:     "email with whitespace",
			email:    "  user@example.com  ",
			expected: "user@example.com",
			wantErr:  false,
		},
		{
			name:     "email with name format",
			email:    "John Doe <john@example.com>",
			expected: "john@example.com",
			wantErr:  false,
		},
		{
			name:     "email with name format and whitespace",
			email:    "John Doe <  john@example.com  >",
			expected: "john@example.com",
			wantErr:  false,
		},
		{
			name:    "empty email",
			email:   "",
			wantErr: true,
		},
		{
			name:    "email without @",
			email:   "userexample.com",
			wantErr: true,
		},
		{
			name:    "email with multiple @",
			email:   "user@@example.com",
			wantErr: true,
		},
		{
			name:    "email with empty local part",
			email:   "@example.com",
			wantErr: true,
		},
		{
			name:    "email with empty domain",
			email:   "user@",
			wantErr: true,
		},
		{
			name:    "malformed name format",
			email:   "John Doe <john@example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sender.validateAndCleanEmail(tt.email)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateAndCleanEmail() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result != tt.expected {
				t.Errorf("validateAndCleanEmail() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestResponseSender_SendEscalation(t *testing.T) {
	tests := []struct {
		name            string
		originalEmail   *Email
		escalatedItems  []string
		adminEmail      string
		mailerError     error
		expectedError   bool
		expectedSubject string
	}{
		{
			name: "successful escalation",
			originalEmail: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Help needed",
				Body:    "I need help with something",
				Date:    time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			escalatedItems:  []string{"Complex billing issue", "Account access problem"},
			adminEmail:      "admin@example.com",
			expectedError:   false,
			expectedSubject: "Inbox Escalation: Help needed",
		},
		{
			name:           "nil original email",
			originalEmail:  nil,
			escalatedItems: []string{"Item 1"},
			adminEmail:     "admin@example.com",
			expectedError:  true,
		},
		{
			name: "empty admin email",
			originalEmail: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Test",
				Body:    "Test body",
				Date:    time.Now(),
			},
			escalatedItems: []string{"Item 1"},
			adminEmail:     "",
			expectedError:  true,
		},
		{
			name: "no escalated items",
			originalEmail: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Test",
				Body:    "Test body",
				Date:    time.Now(),
			},
			escalatedItems: []string{},
			adminEmail:     "admin@example.com",
			expectedError:  true,
		},
		{
			name: "invalid admin email",
			originalEmail: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Test",
				Body:    "Test body",
				Date:    time.Now(),
			},
			escalatedItems: []string{"Item 1"},
			adminEmail:     "invalid-email",
			expectedError:  true,
		},
		{
			name: "mailer error",
			originalEmail: &Email{
				ID:      "test-id",
				From:    "user@example.com",
				Subject: "Test",
				Body:    "Test body",
				Date:    time.Now(),
			},
			escalatedItems: []string{"Item 1"},
			adminEmail:     "admin@example.com",
			mailerError:    errors.New("SMTP error"),
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mailer := &mockMailer{err: tt.mailerError}
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			sender := NewResponseSender(mailer, logger)

			err := sender.SendEscalation(context.Background(), tt.originalEmail, tt.escalatedItems, tt.adminEmail)

			if (err != nil) != tt.expectedError {
				t.Errorf("SendEscalation() error = %v, expectedError %v", err, tt.expectedError)
				return
			}

			if !tt.expectedError && tt.mailerError == nil {
				sentEmails := mailer.getSentEmails()
				if len(sentEmails) != 1 {
					t.Errorf("Expected 1 sent email, got %d", len(sentEmails))
					return
				}

				sent := sentEmails[0]
				if sent.to != tt.adminEmail {
					t.Errorf("Expected recipient %q, got %q", tt.adminEmail, sent.to)
				}

				if sent.subject != tt.expectedSubject {
					t.Errorf("Expected subject %q, got %q", tt.expectedSubject, sent.subject)
				}

				// Verify escalation body contains expected content
				if !strings.Contains(sent.plainBody, tt.originalEmail.From) {
					t.Errorf("Escalation body should contain original sender %q", tt.originalEmail.From)
				}

				if !strings.Contains(sent.plainBody, tt.originalEmail.Subject) {
					t.Errorf("Escalation body should contain original subject %q", tt.originalEmail.Subject)
				}

				if !strings.Contains(sent.plainBody, tt.originalEmail.Body) {
					t.Errorf("Escalation body should contain original body %q", tt.originalEmail.Body)
				}

				for _, item := range tt.escalatedItems {
					if !strings.Contains(sent.plainBody, item) {
						t.Errorf("Escalation body should contain escalated item %q", item)
					}
				}
			}
		})
	}
}

func TestResponseSender_buildEscalationBody(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	sender := NewResponseSender(&mockMailer{}, logger)

	originalEmail := &Email{
		ID:      "test-id",
		From:    "user@example.com",
		Subject: "Help needed",
		Body:    "I need help with something",
		Date:    time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	escalatedItems := []string{"Complex billing issue", "Account access problem"}

	body := sender.buildEscalationBody(originalEmail, escalatedItems)

	// Check that the body contains all expected elements
	expectedElements := []string{
		"An email from a user could not be fully processed",
		"ORIGINAL EMAIL:",
		"From: user@example.com",
		"Subject: Help needed",
		"Date: 2023-01-01 12:00:00",
		"Body:",
		"I need help with something",
		"ESCALATED ITEMS:",
		"1. Complex billing issue",
		"2. Account access problem",
		"Please review this email",
		"Talent Rodeo Inbox System",
	}

	for _, element := range expectedElements {
		if !strings.Contains(body, element) {
			t.Errorf("Escalation body should contain %q", element)
		}
	}
}

func TestResponseSender_convertToHTML(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	sender := NewResponseSender(&mockMailer{}, logger)

	tests := []struct {
		name      string
		plainText string
		expected  string
	}{
		{
			name:      "simple text",
			plainText: "Hello world",
			expected:  "<html><body><pre>Hello world</pre></body></html>",
		},
		{
			name:      "text with newlines",
			plainText: "Line 1\nLine 2\nLine 3",
			expected:  "<html><body><pre>Line 1<br>\nLine 2<br>\nLine 3</pre></body></html>",
		},
		{
			name:      "empty text",
			plainText: "",
			expected:  "",
		},
		{
			name:      "text with multiple newlines",
			plainText: "Line 1\n\nLine 2\n\n\nLine 3",
			expected:  "<html><body><pre>Line 1<br>\n<br>\nLine 2<br>\n<br>\n<br>\nLine 3</pre></body></html>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sender.convertToHTML(tt.plainText)
			if result != tt.expected {
				t.Errorf("convertToHTML() = %v, want %v", result, tt.expected)
			}
		})
	}
}