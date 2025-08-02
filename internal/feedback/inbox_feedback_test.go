package feedback

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/inbox"
	"testing"
	"time"
)

// mockOpenAIClient implements OpenAIClient for testing.
type mockOpenAIClient struct {
	isSearchAdjustmentFunc func(ctx context.Context, email *inbox.Email) (bool, string, error)
	updateStatementFunc    func(ctx context.Context, currentStatement, emailContent, adjustmentRequest string) (string, error)
}

func (m *mockOpenAIClient) IsSearchAdjustmentRequest(ctx context.Context, email *inbox.Email) (bool, string, error) {
	if m.isSearchAdjustmentFunc != nil {
		return m.isSearchAdjustmentFunc(ctx, email)
	}
	return false, "", nil
}

func (m *mockOpenAIClient) UpdateStatement(ctx context.Context, currentStatement, emailContent, adjustmentRequest string) (string, error) {
	if m.updateStatementFunc != nil {
		return m.updateStatementFunc(ctx, currentStatement, emailContent, adjustmentRequest)
	}
	return currentStatement, nil
}

// mockUserService implements UserService for testing.
type mockUserService struct {
	getUserByEmailFunc                         func(ctx context.Context, email string) (*internal.User, bool, error)
	setUserStatementFunc                       func(ctx context.Context, userID, statement string) error
	createStatementVersionFromFeedbackFunc    func(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error)
	createStatementVersionFromInboxFeedbackFunc func(ctx context.Context, userID, emailContent, adjustmentRequest, userEmail, promptFile string) (*internal.UserStatementVersion, error)
}

func (m *mockUserService) GetUserByEmail(ctx context.Context, email string) (*internal.User, bool, error) {
	if m.getUserByEmailFunc != nil {
		return m.getUserByEmailFunc(ctx, email)
	}
	return nil, false, nil
}

func (m *mockUserService) SetUserStatement(ctx context.Context, userID, statement string) error {
	if m.setUserStatementFunc != nil {
		return m.setUserStatementFunc(ctx, userID, statement)
	}
	return nil
}

func (m *mockUserService) CreateStatementVersionFromFeedback(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error) {
	if m.createStatementVersionFromFeedbackFunc != nil {
		return m.createStatementVersionFromFeedbackFunc(ctx, userID, feedbackData, promptFile)
	}
	// Return a mock version for tests
	return &internal.UserStatementVersion{
		ID:            userID + "_v1",
		VersionNumber: 1,
		StatementText: "Updated statement from application feedback",
		GeneratedBy:   "feedback",
		PromptFile:    promptFile,
	}, nil
}

func (m *mockUserService) CreateStatementVersionFromInboxFeedback(ctx context.Context, userID, emailContent, adjustmentRequest, userEmail, promptFile string) (*internal.UserStatementVersion, error) {
	if m.createStatementVersionFromInboxFeedbackFunc != nil {
		return m.createStatementVersionFromInboxFeedbackFunc(ctx, userID, emailContent, adjustmentRequest, userEmail, promptFile)
	}
	// Return a mock version for tests
	return &internal.UserStatementVersion{
		ID:            userID + "_v1",
		VersionNumber: 1,
		StatementText: "Updated statement from inbox email feedback",
		GeneratedBy:   "feedback",
		PromptFile:    promptFile,
	}, nil
}

// mockMailer implements smtp.Mailer for testing.
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

func TestExtractEmailAddress(t *testing.T) {
	tests := []struct {
		name      string
		fromField string
		expected  string
	}{
		{
			name:      "name and email format",
			fromField: "John Doe <john@example.com>",
			expected:  "john@example.com",
		},
		{
			name:      "plain email address",
			fromField: "jane@example.com",
			expected:  "jane@example.com",
		},
		{
			name:      "email with extra spaces",
			fromField: "  test@example.com  ",
			expected:  "test@example.com",
		},
		{
			name:      "complex name format",
			fromField: "Dr. Jane Smith <jane.smith@university.edu>",
			expected:  "jane.smith@university.edu",
		},
		{
			name:      "malformed bracket",
			fromField: "John <john@example.com",
			expected:  "John <john@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractEmailAddress(tt.fromField)
			if result != tt.expected {
				t.Errorf("extractEmailAddress(%q) = %q, want %q", tt.fromField, result, tt.expected)
			}
		})
	}
}

func TestInboxFeedbackHandler_HandleEmail_NotSearchRequest(t *testing.T) {
	mockOpenAI := &mockOpenAIClient{
		isSearchAdjustmentFunc: func(ctx context.Context, email *inbox.Email) (bool, string, error) {
			return false, "", nil
		},
	}

	handler := NewInboxFeedbackHandler(mockOpenAI, nil, nil, "", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	email := &inbox.Email{
		ID:      "test-email-1",
		From:    "user@example.com",
		Subject: "Thank you for the update",
		Body:    "Just wanted to say thanks for the job recommendations!",
	}

	_, err := handler.HandleEmail(context.Background(), email)
	if err != nil {
		t.Errorf("HandleEmail() error = %v, wantErr false", err)
	}
}

func TestInboxFeedbackHandler_HandleEmail_SearchRequest(t *testing.T) {
	testUser := &internal.User{
		ID:        "test-user-1",
		Email:     "user@example.com",
		Statement: stringPtr("I am looking for software engineering roles in New York."),
	}

	mockOpenAI := &mockOpenAIClient{
		isSearchAdjustmentFunc: func(ctx context.Context, email *inbox.Email) (bool, string, error) {
			return true, "User wants to add remote work options", nil
		},
		updateStatementFunc: func(ctx context.Context, currentStatement, emailContent, adjustmentRequest string) (string, error) {
			return "I am looking for software engineering roles in New York or remote positions.", nil
		},
	}

	var capturedUserID string
	var capturedFeedbackData map[string]any
	var capturedPromptFile string
	mockUserSvc := &mockUserService{
		getUserByEmailFunc: func(ctx context.Context, email string) (*internal.User, bool, error) {
			if email == "user@example.com" {
				return testUser, true, nil
			}
			return nil, false, nil
		},
		createStatementVersionFromFeedbackFunc: func(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error) {
			capturedUserID = userID
			capturedFeedbackData = feedbackData
			capturedPromptFile = promptFile
			return &internal.UserStatementVersion{
				ID:            userID + "_v2",
				VersionNumber: 2,
				StatementText: "I am looking for software engineering roles in New York or remote positions.",
				GeneratedBy:   "feedback",
				PromptFile:    promptFile,
			}, nil
		},
	}

	handler := NewInboxFeedbackHandlerWithServices(mockOpenAI, mockUserSvc, &mockMailer{}, "admin@example.com", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	email := &inbox.Email{
		ID:      "test-email-2",
		From:    "user@example.com",
		Subject: "Update my job search",
		Body:    "I would like to include remote work options in my search.",
	}

	_, err := handler.HandleEmail(context.Background(), email)
	if err != nil {
		t.Errorf("HandleEmail() error = %v, wantErr false", err)
	}

	// Verify the versioning system was called correctly
	if capturedUserID != testUser.ID {
		t.Errorf("Expected user ID %s, got %s", testUser.ID, capturedUserID)
	}

	// Verify feedback data structure was created correctly
	if capturedFeedbackData == nil {
		t.Errorf("Expected feedback data to be captured, got nil")
	} else {
		if capturedFeedbackData["source_type"] != "email_feedback" {
			t.Errorf("Expected source_type 'email_feedback', got %v", capturedFeedbackData["source_type"])
		}
		if capturedFeedbackData["user_email"] != "user@example.com" {
			t.Errorf("Expected user_email 'user@example.com', got %v", capturedFeedbackData["user_email"])
		}
	}

	// Verify correct prompt file was used
	if capturedPromptFile != "inbox-feedback.v001.txt" {
		t.Errorf("Expected prompt file 'inbox-feedback.v001.txt', got %s", capturedPromptFile)
	}
}

func TestInboxFeedbackHandler_HandleEmail_UserNotFound(t *testing.T) {
	mockOpenAI := &mockOpenAIClient{
		isSearchAdjustmentFunc: func(ctx context.Context, email *inbox.Email) (bool, string, error) {
			return true, "User wants to add remote work options", nil
		},
	}

	mockUserSvc := &mockUserService{
		getUserByEmailFunc: func(ctx context.Context, email string) (*internal.User, bool, error) {
			return nil, false, nil
		},
	}

	handler := NewInboxFeedbackHandlerWithServices(mockOpenAI, mockUserSvc, &mockMailer{}, "admin@example.com", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	email := &inbox.Email{
		ID:      "test-email-3",
		From:    "unknown@example.com",
		Subject: "Update my job search",
		Body:    "I would like to include remote work options in my search.",
	}

	_, err := handler.HandleEmail(context.Background(), email)
	if err == nil {
		t.Error("HandleEmail() error = nil, wantErr true")
	}

	if !strings.Contains(err.Error(), "user not found") {
		t.Errorf("Expected error to contain 'user not found', got: %v", err)
	}
}

func TestInboxFeedbackHandler_HandleEmail_OpenAIError(t *testing.T) {
	mockOpenAI := &mockOpenAIClient{
		isSearchAdjustmentFunc: func(ctx context.Context, email *inbox.Email) (bool, string, error) {
			return false, "", errors.New("OpenAI API error")
		},
	}

	handler := NewInboxFeedbackHandler(mockOpenAI, nil, nil, "", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	email := &inbox.Email{
		ID:      "test-email-4",
		From:    "user@example.com",
		Subject: "Update my job search",
		Body:    "I would like to include remote work options in my search.",
	}

	_, err := handler.HandleEmail(context.Background(), email)
	if err == nil {
		t.Error("HandleEmail() error = nil, wantErr true")
	}

	if !strings.Contains(err.Error(), "failed to analyze email") {
		t.Errorf("Expected error to contain 'failed to analyze email', got: %v", err)
	}
}

func TestInboxFeedbackHandler_HandleEmail_StatementUpdateError(t *testing.T) {
	testUser := &internal.User{
		ID:        "test-user-1",
		Email:     "user@example.com",
		Statement: stringPtr("I am looking for software engineering roles in New York."),
	}

	mockOpenAI := &mockOpenAIClient{
		isSearchAdjustmentFunc: func(ctx context.Context, email *inbox.Email) (bool, string, error) {
			return true, "User wants to add remote work options", nil
		},
		updateStatementFunc: func(ctx context.Context, currentStatement, emailContent, adjustmentRequest string) (string, error) {
			return "", errors.New("OpenAI statement update failed")
		},
	}

	mockUserSvc := &mockUserService{
		getUserByEmailFunc: func(ctx context.Context, email string) (*internal.User, bool, error) {
			return testUser, true, nil
		},
		createStatementVersionFromFeedbackFunc: func(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error) {
			return nil, errors.New("failed to create statement version from feedback")
		},
	}

	handler := NewInboxFeedbackHandlerWithServices(mockOpenAI, mockUserSvc, &mockMailer{}, "admin@example.com", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	email := &inbox.Email{
		ID:      "test-email-5",
		From:    "user@example.com",
		Subject: "Update my job search",
		Body:    "I would like to include remote work options in my search.",
	}

	_, err := handler.HandleEmail(context.Background(), email)
	if err == nil {
		t.Error("HandleEmail() error = nil, wantErr true")
	}

	if !strings.Contains(err.Error(), "failed to create statement version from feedback") {
		t.Errorf("Expected error to contain 'failed to create statement version from feedback', got: %v", err)
	}
}

func TestInboxFeedbackHandler_HandleEmail_NilStatement(t *testing.T) {
	testUser := &internal.User{
		ID:        "test-user-1",
		Email:     "user@example.com",
		Statement: nil, // No existing statement
	}

	mockOpenAI := &mockOpenAIClient{
		isSearchAdjustmentFunc: func(ctx context.Context, email *inbox.Email) (bool, string, error) {
			return true, "User wants remote work options", nil
		},
		updateStatementFunc: func(ctx context.Context, currentStatement, emailContent, adjustmentRequest string) (string, error) {
			if currentStatement != "" {
				t.Errorf("Expected empty current statement, got: %s", currentStatement)
			}
			return "I am looking for remote work opportunities.", nil
		},
	}

	var capturedFeedbackData map[string]any
	mockUserSvc := &mockUserService{
		getUserByEmailFunc: func(ctx context.Context, email string) (*internal.User, bool, error) {
			return testUser, true, nil
		},
		createStatementVersionFromFeedbackFunc: func(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error) {
			capturedFeedbackData = feedbackData
			return &internal.UserStatementVersion{
				ID:            userID + "_v1",
				VersionNumber: 1,
				StatementText: "I am looking for remote work opportunities.",
				GeneratedBy:   "feedback",
				PromptFile:    promptFile,
			}, nil
		},
	}

	handler := NewInboxFeedbackHandlerWithServices(mockOpenAI, mockUserSvc, &mockMailer{}, "admin@example.com", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	email := &inbox.Email{
		ID:      "test-email-6",
		From:    "user@example.com",
		Subject: "Job search preferences",
		Body:    "I would like to find remote work opportunities.",
	}

	_, err := handler.HandleEmail(context.Background(), email)
	if err != nil {
		t.Errorf("HandleEmail() error = %v, wantErr false", err)
	}

	// Verify the versioning system was called
	if capturedFeedbackData == nil {
		t.Errorf("Expected feedback data to be captured, got nil")
	} else {
		if capturedFeedbackData["source_type"] != "email_feedback" {
			t.Errorf("Expected source_type 'email_feedback', got %v", capturedFeedbackData["source_type"])
		}
	}
}

func TestAnalyzeEmail(t *testing.T) {
	tests := []struct {
		name              string
		email             *inbox.Email
		expectedIsRequest bool
		expectedSummary   string
		expectedError     bool
		mockResponse      func(ctx context.Context, email *inbox.Email) (bool, string, error)
	}{
		{
			name: "search adjustment request",
			email: &inbox.Email{
				ID:      "test-1",
				From:    "user@example.com",
				Subject: "Update my job search",
				Body:    "I would like to add remote work options to my search.",
			},
			expectedIsRequest: true,
			expectedSummary:   "User wants to add remote work options",
			expectedError:     false,
			mockResponse: func(ctx context.Context, email *inbox.Email) (bool, string, error) {
				return true, "User wants to add remote work options", nil
			},
		},
		{
			name: "not a search request",
			email: &inbox.Email{
				ID:      "test-2",
				From:    "user@example.com",
				Subject: "Thank you",
				Body:    "Thanks for the job recommendations!",
			},
			expectedIsRequest: false,
			expectedSummary:   "",
			expectedError:     false,
			mockResponse: func(ctx context.Context, email *inbox.Email) (bool, string, error) {
				return false, "", nil
			},
		},
		{
			name: "OpenAI error",
			email: &inbox.Email{
				ID:      "test-3",
				From:    "user@example.com",
				Subject: "Update search",
				Body:    "Please update my search criteria.",
			},
			expectedIsRequest: false,
			expectedSummary:   "",
			expectedError:     true,
			mockResponse: func(ctx context.Context, email *inbox.Email) (bool, string, error) {
				return false, "", errors.New("API error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockOpenAI := &mockOpenAIClient{
				isSearchAdjustmentFunc: tt.mockResponse,
			}

			handler := NewInboxFeedbackHandler(mockOpenAI, nil, nil, "", slog.New(slog.NewTextHandler(os.Stdout, nil)))

			isRequest, summary, err := handler.analyzeEmail(context.Background(), tt.email)

			if tt.expectedError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if isRequest != tt.expectedIsRequest {
				t.Errorf("Expected isRequest=%v, got %v", tt.expectedIsRequest, isRequest)
			}
			if summary != tt.expectedSummary {
				t.Errorf("Expected summary=%q, got %q", tt.expectedSummary, summary)
			}
		})
	}
}

// Helper function to create string pointers for testing
func stringPtr(s string) *string {
	return &s
}

// Example test for the actual OpenAI client (would require API key)
func TestOpenAIClient_Integration(t *testing.T) {
	// Skip this test unless running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	client := NewOpenAIClient(apiKey, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	email := &inbox.Email{
		ID:      "integration-test",
		From:    "user@example.com",
		Subject: "Update my job search preferences",
		Body:    "I would like to focus on remote senior software engineering positions instead of on-site junior roles.",
		Date:    time.Now(),
	}

	ctx := context.Background()
	isRequest, summary, err := client.IsSearchAdjustmentRequest(ctx, email)
	if err != nil {
		t.Fatalf("IsSearchAdjustmentRequest() error = %v", err)
	}

	if !isRequest {
		t.Error("Expected this to be identified as a search adjustment request")
	}

	if summary == "" {
		t.Error("Expected non-empty summary for search adjustment request")
	}

	t.Logf("Summary: %s", summary)

	// Test statement update
	currentStatement := "I am looking for junior software engineering roles in the Bay Area."
	updatedStatement, err := client.UpdateStatement(ctx, currentStatement, email.Body, summary)
	if err != nil {
		t.Fatalf("UpdateStatement() error = %v", err)
	}

	if updatedStatement == "" {
		t.Error("Expected non-empty updated statement")
	}

	if !strings.Contains(updatedStatement, "remote") {
		t.Error("Expected updated statement to contain 'remote'")
	}

	t.Logf("Updated statement: %s", updatedStatement)
}

func TestInboxFeedbackHandler_EmailNotification(t *testing.T) {
	// Set environment variable to enable notifications
	originalEnv := os.Getenv("SEND_FEEDBACK_ADJUSTMENTS")
	os.Setenv("SEND_FEEDBACK_ADJUSTMENTS", "true")
	defer func() {
		if originalEnv == "" {
			os.Unsetenv("SEND_FEEDBACK_ADJUSTMENTS")
		} else {
			os.Setenv("SEND_FEEDBACK_ADJUSTMENTS", originalEnv)
		}
	}()

	testUser := &internal.User{
		ID:        "test-user-1",
		Email:     "user@example.com",
		Name:      "John Doe",
		FirstName: "John",
		LastName:  "Doe",
		Statement: stringPtr("I am looking for software engineering roles in New York."),
	}

	mockOpenAI := &mockOpenAIClient{
		isSearchAdjustmentFunc: func(ctx context.Context, email *inbox.Email) (bool, string, error) {
			return true, "User wants to add remote work options", nil
		},
		updateStatementFunc: func(ctx context.Context, currentStatement, emailContent, adjustmentRequest string) (string, error) {
			return "I am looking for software engineering roles in New York or remote positions.", nil
		},
	}

	var capturedRecipient string
	var capturedData SearchAdjustmentEmailData
	var capturedTemplate string
	mockMail := &mockMailer{
		sendFromTemplateFunc: func(recipient string, data any, patterns ...string) error {
			capturedRecipient = recipient
			if emailData, ok := data.(SearchAdjustmentEmailData); ok {
				capturedData = emailData
			}
			if len(patterns) > 0 {
				capturedTemplate = patterns[0]
			}
			return nil
		},
	}

	mockUserSvc := &mockUserService{
		getUserByEmailFunc: func(ctx context.Context, email string) (*internal.User, bool, error) {
			if email == "user@example.com" {
				return testUser, true, nil
			}
			return nil, false, nil
		},
		createStatementVersionFromFeedbackFunc: func(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error) {
			return &internal.UserStatementVersion{
				ID:            userID + "_v2",
				VersionNumber: 2,
				StatementText: "I am looking for software engineering roles in New York or remote positions.",
				GeneratedBy:   "feedback",
				PromptFile:    promptFile,
			}, nil
		},
	}

	handler := NewInboxFeedbackHandlerWithServices(mockOpenAI, mockUserSvc, mockMail, "admin@example.com", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	email := &inbox.Email{
		ID:      "test-email-notification",
		From:    "user@example.com",
		Subject: "Update my job search",
		Body:    "I would like to include remote work options in my search.",
	}

	_, err := handler.HandleEmail(context.Background(), email)
	if err != nil {
		t.Errorf("HandleEmail() error = %v, wantErr false", err)
	}

	// Verify the notification email was sent
	if capturedRecipient != "admin@example.com" {
		t.Errorf("Expected recipient admin@example.com, got %s", capturedRecipient)
	}

	if capturedTemplate != "search-adjustment-notification.tmpl" {
		t.Errorf("Expected template search-adjustment-notification.tmpl, got %s", capturedTemplate)
	}

	// Verify the email data
	if capturedData.UserID != testUser.ID {
		t.Errorf("Expected user ID %s, got %s", testUser.ID, capturedData.UserID)
	}

	if capturedData.UserEmail != testUser.Email {
		t.Errorf("Expected user email %s, got %s", testUser.Email, capturedData.UserEmail)
	}

	if capturedData.UserName != "John Doe" {
		t.Errorf("Expected user name 'John Doe', got %s", capturedData.UserName)
	}

	if capturedData.OriginalRequest != email.Body {
		t.Errorf("Expected original request %q, got %q", email.Body, capturedData.OriginalRequest)
	}

	if capturedData.AdjustmentSummary != "User wants to add remote work options" {
		t.Errorf("Expected adjustment summary 'User wants to add remote work options', got %q", capturedData.AdjustmentSummary)
	}
}

func TestInboxFeedbackHandler_EmailNotificationDisabled(t *testing.T) {
	// Ensure environment variable is not set
	originalEnv := os.Getenv("SEND_FEEDBACK_ADJUSTMENTS")
	os.Unsetenv("SEND_FEEDBACK_ADJUSTMENTS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("SEND_FEEDBACK_ADJUSTMENTS", originalEnv)
		}
	}()

	testUser := &internal.User{
		ID:        "test-user-1",
		Email:     "user@example.com",
		Name:      "John Doe",
		Statement: stringPtr("I am looking for software engineering roles in New York."),
	}

	mockOpenAI := &mockOpenAIClient{
		isSearchAdjustmentFunc: func(ctx context.Context, email *inbox.Email) (bool, string, error) {
			return true, "User wants to add remote work options", nil
		},
		updateStatementFunc: func(ctx context.Context, currentStatement, emailContent, adjustmentRequest string) (string, error) {
			return "I am looking for software engineering roles in New York or remote positions.", nil
		},
	}

	var emailSent bool
	mockMail := &mockMailer{
		sendFromTemplateFunc: func(recipient string, data any, patterns ...string) error {
			emailSent = true
			return nil
		},
	}

	mockUserSvc := &mockUserService{
		getUserByEmailFunc: func(ctx context.Context, email string) (*internal.User, bool, error) {
			return testUser, true, nil
		},
		createStatementVersionFromFeedbackFunc: func(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error) {
			return &internal.UserStatementVersion{
				ID:            userID + "_v2",
				VersionNumber: 2,
				StatementText: "Updated statement",
				GeneratedBy:   "feedback",
				PromptFile:    promptFile,
			}, nil
		},
	}

	handler := NewInboxFeedbackHandlerWithServices(mockOpenAI, mockUserSvc, mockMail, "admin@example.com", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	email := &inbox.Email{
		ID:      "test-email-no-notification",
		From:    "user@example.com",
		Subject: "Update my job search",
		Body:    "I would like to include remote work options in my search.",
	}

	_, err := handler.HandleEmail(context.Background(), email)
	if err != nil {
		t.Errorf("HandleEmail() error = %v, wantErr false", err)
	}

	// Verify no email was sent
	if emailSent {
		t.Error("Expected no email to be sent when notifications are disabled")
	}
}

func TestInboxFeedbackHandler_extractUserEmail(t *testing.T) {
	handler := NewInboxFeedbackHandler(nil, nil, nil, "", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	tests := []struct {
		name      string
		email     *inbox.Email
		expected  string
		expectErr bool
	}{
		{
			name:     "valid email with name",
			email:    &inbox.Email{From: "John Doe <john@example.com>"},
			expected: "john@example.com",
		},
		{
			name:     "plain email address",
			email:    &inbox.Email{From: "jane@example.com"},
			expected: "jane@example.com",
		},
		{
			name:      "empty from field",
			email:     &inbox.Email{From: ""},
			expectErr: true,
		},
		{
			name:     "malformed email",
			email:    &inbox.Email{From: "not-an-email"},
			expected: "not-an-email", // extractEmailAddress returns the input if no brackets found
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.extractUserEmail(tt.email)

			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestInboxFeedbackHandler_lookupUser(t *testing.T) {
	testUser := &internal.User{
		ID:    "test-user-1",
		Email: "user@example.com",
		Name:  "Test User",
	}

	tests := []struct {
		name      string
		email     string
		mockFunc  func(ctx context.Context, email string) (*internal.User, bool, error)
		expected  *internal.User
		expectErr bool
	}{
		{
			name:  "user found",
			email: "user@example.com",
			mockFunc: func(ctx context.Context, email string) (*internal.User, bool, error) {
				return testUser, true, nil
			},
			expected: testUser,
		},
		{
			name:  "user not found",
			email: "missing@example.com",
			mockFunc: func(ctx context.Context, email string) (*internal.User, bool, error) {
				return nil, false, nil
			},
			expectErr: true,
		},
		{
			name:  "database error",
			email: "error@example.com",
			mockFunc: func(ctx context.Context, email string) (*internal.User, bool, error) {
				return nil, false, errors.New("database error")
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUserSvc := &mockUserService{
				getUserByEmailFunc: tt.mockFunc,
			}

			handler := NewInboxFeedbackHandlerWithServices(nil, mockUserSvc, nil, "", slog.New(slog.NewTextHandler(os.Stdout, nil)))

			result, err := handler.lookupUser(context.Background(), tt.email)

			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestInboxFeedbackHandler_buildUserName(t *testing.T) {
	handler := NewInboxFeedbackHandler(nil, nil, nil, "", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	tests := []struct {
		name     string
		user     *internal.User
		expected string
	}{
		{
			name:     "user with name field",
			user:     &internal.User{Name: "John Doe", FirstName: "John", LastName: "Doe"},
			expected: "John Doe",
		},
		{
			name:     "user with first and last name",
			user:     &internal.User{FirstName: "Jane", LastName: "Smith"},
			expected: "Jane Smith",
		},
		{
			name:     "user with only first name",
			user:     &internal.User{FirstName: "Bob"},
			expected: "Bob",
		},
		{
			name:     "user with only last name",
			user:     &internal.User{LastName: "Wilson"},
			expected: "Wilson",
		},
		{
			name:     "user with no name information",
			user:     &internal.User{},
			expected: "Unknown",
		},
		{
			name:     "user with empty name field",
			user:     &internal.User{Name: "", FirstName: "Test", LastName: "User"},
			expected: "Test User",
		},
		{
			name:     "user with names containing spaces",
			user:     &internal.User{FirstName: "  Mary  ", LastName: "  Jane  "},
			expected: "Mary     Jane", // TrimSpace only trims outer spaces
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.buildUserName(tt.user)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
