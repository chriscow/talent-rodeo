package internal

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal/inbox"
)

// mockEmailParser implements EmailParser for testing
type mockEmailParser struct {
	structure *EmailStructure
	err       error
}

func (m *mockEmailParser) ParseEmail(ctx context.Context, email *inbox.Email) (*EmailStructure, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.structure, nil
}

func TestNewOpenAIEmailParser(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	apiKey := "test-api-key"
	fs := &FirestoreClient{}

	parser := NewOpenAIEmailParser(apiKey, logger, fs)

	if parser == nil {
		t.Error("NewOpenAIEmailParser returned nil")
	}

	// Verify it implements the interface
	var _ EmailParser = parser
}

func TestEmailParser_ParseEmail_NilEmail(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fs := &FirestoreClient{}
	parser := &openAIEmailParser{logger: logger, fs: fs}

	structure, err := parser.ParseEmail(context.Background(), nil)

	if structure != nil {
		t.Error("ParseEmail should return nil structure for nil email")
	}

	if err == nil {
		t.Error("ParseEmail should return error for nil email")
	}

	if !strings.Contains(err.Error(), "email cannot be nil") {
		t.Errorf("ParseEmail error = %q, want error containing 'email cannot be nil'", err.Error())
	}
}

func TestEmailParser_ParseEmail_EmptyBody(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fs := &FirestoreClient{}
	parser := &openAIEmailParser{logger: logger, fs: fs}

	email := &inbox.Email{
		ID:      "test-123",
		From:    "test@example.com",
		Subject: "Test Subject",
		Body:    "",
		Date:    time.Now(),
	}

	structure, err := parser.ParseEmail(context.Background(), email)

	if err != nil {
		t.Errorf("ParseEmail returned error for empty body: %v", err)
	}

	if structure == nil {
		t.Error("ParseEmail should return empty structure for empty body, not nil")
	}

	if len(structure.Requests) != 0 || len(structure.GeneralFeedback) != 0 || len(structure.NonRequests) != 0 {
		t.Error("ParseEmail should return empty structure for empty body")
	}
}

func TestEmailParser_ParseEmail_WhitespaceBody(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fs := &FirestoreClient{}
	parser := &openAIEmailParser{logger: logger, fs: fs}

	email := &inbox.Email{
		ID:      "test-123",
		From:    "test@example.com",
		Subject: "Test Subject",
		Body:    "   \n\t  \r\n  ",
		Date:    time.Now(),
	}

	structure, err := parser.ParseEmail(context.Background(), email)

	if err != nil {
		t.Errorf("ParseEmail returned error for whitespace body: %v", err)
	}

	if structure == nil {
		t.Error("ParseEmail should return empty structure for whitespace body, not nil")
	}

	if len(structure.Requests) != 0 || len(structure.GeneralFeedback) != 0 || len(structure.NonRequests) != 0 {
		t.Error("ParseEmail should return empty structure for whitespace body")
	}
}

func TestEmailParser_buildParsingPrompt(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fs := &FirestoreClient{}
	parser := &openAIEmailParser{logger: logger, fs: fs}

	email := &inbox.Email{
		From:    "user@example.com",
		Subject: "Job Search Update",
		Body:    "I would like to add remote work options to my search.",
	}

	prompt := parser.buildParsingPrompt(email)

	// Verify prompt contains email details
	if !strings.Contains(prompt, email.From) {
		t.Error("Prompt should contain email from address")
	}

	if !strings.Contains(prompt, email.Subject) {
		t.Error("Prompt should contain email subject")
	}

	if !strings.Contains(prompt, email.Body) {
		t.Error("Prompt should contain email body")
	}

	// Verify prompt contains instructions
	if !strings.Contains(prompt, "REQUESTS") {
		t.Error("Prompt should contain REQUESTS instruction")
	}

	if !strings.Contains(prompt, "GENERAL_FEEDBACK") {
		t.Error("Prompt should contain GENERAL_FEEDBACK instruction")
	}

	if !strings.Contains(prompt, "NON_REQUESTS") {
		t.Error("Prompt should contain NON_REQUESTS instruction")
	}

	if !strings.Contains(prompt, "output ONLY valid JSON") {
		t.Error("Prompt should contain JSON output instruction")
	}
}

func TestEmailParser_assignRequestIDs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fs := &FirestoreClient{}
	parser := &openAIEmailParser{logger: logger, fs: fs}

	structure := &EmailStructure{
		Requests: []EmailRequest{
			{Type: "job_search", Description: "Test 1"},
			{Type: "account", Description: "Test 2"},
		},
		GeneralFeedback: []EmailRequest{
			{Type: "feedback", Description: "Feedback 1"},
		},
		NonRequests: []EmailRequest{
			{Type: "greeting", Description: "Hello"},
		},
	}

	parser.assignRequestIDs(structure)

	// Verify all IDs are assigned and unique
	ids := make(map[string]bool)

	for _, req := range structure.Requests {
		if req.ID == "" {
			t.Error("Request ID should not be empty")
		}
		if ids[req.ID] {
			t.Errorf("Duplicate ID found: %s", req.ID)
		}
		ids[req.ID] = true
		if !strings.HasPrefix(req.ID, "req_") {
			t.Errorf("Request ID should start with 'req_', got: %s", req.ID)
		}
	}

	for _, feedback := range structure.GeneralFeedback {
		if feedback.ID == "" {
			t.Error("Feedback ID should not be empty")
		}
		if ids[feedback.ID] {
			t.Errorf("Duplicate ID found: %s", feedback.ID)
		}
		ids[feedback.ID] = true
		if !strings.HasPrefix(feedback.ID, "feedback_") {
			t.Errorf("Feedback ID should start with 'feedback_', got: %s", feedback.ID)
		}
	}

	for _, nonReq := range structure.NonRequests {
		if nonReq.ID == "" {
			t.Error("NonRequest ID should not be empty")
		}
		if ids[nonReq.ID] {
			t.Errorf("Duplicate ID found: %s", nonReq.ID)
		}
		ids[nonReq.ID] = true
		if !strings.HasPrefix(nonReq.ID, "non_req_") {
			t.Errorf("NonRequest ID should start with 'non_req_', got: %s", nonReq.ID)
		}
	}
}

func TestEmailParser_assignRequestIDs_EmptyStructure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fs := &FirestoreClient{}
	parser := &openAIEmailParser{logger: logger, fs: fs}

	structure := &EmailStructure{
		Requests:        []EmailRequest{},
		GeneralFeedback: []EmailRequest{},
		NonRequests:     []EmailRequest{},
	}

	// Should not panic with empty structure
	parser.assignRequestIDs(structure)

	if len(structure.Requests) != 0 || len(structure.GeneralFeedback) != 0 || len(structure.NonRequests) != 0 {
		t.Error("assignRequestIDs should not modify empty structure")
	}
}

// Test mock EmailParser
func TestMockEmailParser(t *testing.T) {
	tests := []struct {
		name      string
		mock      *mockEmailParser
		wantErr   bool
		wantNil   bool
		errSubstr string
	}{
		{
			name: "successful parsing",
			mock: &mockEmailParser{
				structure: &EmailStructure{
					Requests: []EmailRequest{{ID: "1", Type: "test"}},
				},
			},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "parsing error",
			mock: &mockEmailParser{
				err: errors.New("parsing failed"),
			},
			wantErr:   true,
			wantNil:   true,
			errSubstr: "parsing failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email := &inbox.Email{ID: "test"}
			structure, err := tt.mock.ParseEmail(context.Background(), email)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.wantNil && structure != nil {
				t.Error("Expected nil structure but got non-nil")
			}

			if !tt.wantNil && structure == nil {
				t.Error("Expected non-nil structure but got nil")
			}

			if tt.errSubstr != "" && (err == nil || !strings.Contains(err.Error(), tt.errSubstr)) {
				t.Errorf("Expected error containing %q, got: %v", tt.errSubstr, err)
			}
		})
	}
}
