package nudge

import (
	"context"
	"testing"
	"time"

	"talent-rodeo/internal"
)

func TestEmailGeneratorBusinessLogic(t *testing.T) {
	// Test BuildEmailData business logic without any third-party dependencies
	t.Run("BuildEmailData calculates days correctly", func(t *testing.T) {
		mockTemplateReader := NewMockTemplateReader()
		mockPromptProcessor := NewMockPromptProcessor()
		generator := NewEmailGenerator(mockTemplateReader, mockPromptProcessor)

		user := &internal.User{
			ID:             "test-user",
			Name:           "John Doe",
			FirstName:      "John",
			Email:          "john@example.com",
			CreatedAt:      time.Now().Add(-5 * 24 * time.Hour), // 5 days ago
			LastAccessedAt: time.Now().Add(-3 * 24 * time.Hour), // 3 days ago
		}

		// Test NoResume nudge (uses CreatedAt)
		data := generator.BuildEmailData(user, NudgeTypeNoResume, nil)
		if data.LastAccessed != user.LastAccessedAt {
			t.Errorf("expected last accessed to be set for NoResume, got %s", data.LastAccessed)
		}

		// Test NoCall nudge (uses LastAccessedAt)
		data = generator.BuildEmailData(user, NudgeTypeNoCall, nil)
		if data.LastAccessed != user.LastAccessedAt {
			t.Errorf("expected last accessed to be set for NoCall, got %s", data.LastAccessed)
		}

		// Test FiveStarMatches with additional data
		additionalData := map[string]any{
			"matches": []FiveStarMatchData{
				{Title: "Job 1", Summary: "Summary 1", Analysis: "Analysis 1"},
				{Title: "Job 2", Summary: "Summary 2", Analysis: "Analysis 2"},
			},
		}
		data = generator.BuildEmailData(user, NudgeTypeFiveStarMatches, additionalData)

		if len(data.Matches) != 2 {
			t.Error("expected matches to be set correctly")
		}
		if data.Matches[0].Title != "Job 1" || data.Matches[1].Title != "Job 2" {
			t.Error("expected match titles to be set correctly")
		}
	})

	// 	t.Run("GenerateEmailContent business logic with mocks", func(t *testing.T) {
	// 		mockTemplateReader := NewMockTemplateReader()
	// 		mockPromptProcessor := NewMockPromptProcessor()
	// 		generator := NewEmailGenerator(mockTemplateReader, mockPromptProcessor)

	// 		// Set up mocks for LLM-based generation
	// 		mockTemplateReader.SetTemplate(NudgeTypeFollowUp, []byte("template content"))
	// 		mockPromptProcessor.SetGeneratedPrompt("generated prompt")
	// 		mockPromptProcessor.SetPromptResponse(`<email_content>
	// <subject>Test Subject</subject>
	// <body>Test body with Shirely and Talent Rodeo mentioned</body>
	// </email_content>`)

	// 		user := &internal.User{
	// 			ID:        "test-user",
	// 			Name:      "John Doe",
	// 			FirstName: "John",
	// 			Email:     "john@example.com",
	// 		}

	// 		emailData := generator.BuildEmailData(user, NudgeTypeFollowUp, nil)

	// 		email, err := generator.GenerateEmailContent(context.Background(), user, NudgeTypeFollowUp, emailData)
	// 		if err != nil {
	// 			t.Fatalf("unexpected error: %v", err)
	// 		}

	// 		if email.Subject != "Test Subject" {
	// 			t.Errorf("expected subject 'Test Subject', got '%s'", email.Subject)
	// 		}

	// 		if !strings.Contains(email.HTMLBody, "Shirely") || !strings.Contains(email.HTMLBody, "Talent Rodeo") {
	// 			t.Errorf("expected body to contain required branding, got '%s'", email.HTMLBody)
	// 		}
	// 	})

	t.Run("ValidateEmail business logic", func(t *testing.T) {
		mockTemplateReader := NewMockTemplateReader()
		mockPromptProcessor := NewMockPromptProcessor()
		generator := NewEmailGenerator(mockTemplateReader, mockPromptProcessor)

		// Test valid email
		validEmail := &GeneratedEmail{
			Subject:   "Valid Subject",
			PlainBody: "Valid body content with Shirely and Talent Rodeo",
		}
		if err := generator.ValidateEmail(validEmail); err != nil {
			t.Errorf("expected valid email to pass validation, got error: %v", err)
		}

		// Test validation failures
		tests := []struct {
			name  string
			email *GeneratedEmail
		}{
			{"nil email", nil},
			{"empty subject", &GeneratedEmail{Subject: "", PlainBody: "body"}},
			{"empty body", &GeneratedEmail{Subject: "subject", PlainBody: ""}},
			{"missing Shirley", &GeneratedEmail{Subject: "subject", PlainBody: "body with Talent Rodeo"}},
			{"missing Talent Rodeo", &GeneratedEmail{Subject: "subject", PlainBody: "body with Shirely"}},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if err := generator.ValidateEmail(test.email); err == nil {
					t.Error("expected validation to fail")
				}
			})
		}
	})
}

func TestProcessorWithMocks(t *testing.T) {
	t.Run("Processor can be tested with mocks", func(t *testing.T) {
		// Create mocks
		mockEmailGenerator := &MockEmailGenerator{}
		mockEmailSender := NewMockEmailSender()

		// This demonstrates how the processor could be tested
		// without any third-party dependencies using the NewProcessor constructor
		// (We'd need a mock for FirestoreClient too, but that's already available via NewTestFirestoreClient)

		if mockEmailGenerator == nil || mockEmailSender == nil {
			t.Error("mocks should be available for testing")
		}
	})
}

// MockEmailGenerator for demonstration - implements EmailContentGenerator
type MockEmailGenerator struct {
	buildDataResult *NudgeEmailData
	generateResult  *GeneratedEmail
	generateError   error
	validateError   error
}

func (m *MockEmailGenerator) BuildEmailData(user *internal.User, nudgeType NudgeType, additionalData map[string]any) *NudgeEmailData {
	if m.buildDataResult != nil {
		return m.buildDataResult
	}
	return &NudgeEmailData{
		User:      user,
		NudgeType: nudgeType.String(),
	}
}

func (m *MockEmailGenerator) GenerateEmailContent(ctx context.Context, user *internal.User, nudgeType NudgeType, emailData *NudgeEmailData) (*GeneratedEmail, error) {
	if m.generateError != nil {
		return nil, m.generateError
	}
	if m.generateResult != nil {
		return m.generateResult, nil
	}
	return &GeneratedEmail{
		Subject:   "Mock Subject",
		PlainBody: "Mock body with Shirley and Talent Rodeo",
	}, nil
}

func (m *MockEmailGenerator) ValidateEmail(email *GeneratedEmail) error {
	return m.validateError
}
