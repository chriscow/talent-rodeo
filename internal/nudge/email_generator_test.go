package nudge

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal"
)

func TestNewEmailGenerator(t *testing.T) {
	mockTemplateReader := NewMockTemplateReader()
	mockPromptProcessor := NewMockPromptProcessor()
	gen := NewEmailGenerator(mockTemplateReader, mockPromptProcessor)
	if gen == nil {
		t.Fatal("expected email generator to be created")
	}
}

func TestBuildEmailData(t *testing.T) {
	mockTemplateReader := NewMockTemplateReader()
	mockPromptProcessor := NewMockPromptProcessor()
	gen := NewEmailGenerator(mockTemplateReader, mockPromptProcessor)

	user := &internal.User{
		ID:             "test-user",
		Name:           "John Doe",
		FirstName:      "John",
		Email:          "john@example.com",
		CreatedAt:      time.Now().Add(-3 * 24 * time.Hour), // 3 days ago
		LastAccessedAt: time.Now().Add(-2 * 24 * time.Hour), // 2 days ago
	}

	tests := []struct {
		name           string
		nudgeType      NudgeType
		additionalData map[string]interface{}
		expectedType   string
	}{
		{
			name:           "no resume nudge",
			nudgeType:      NudgeTypeNoResume,
			additionalData: nil,
			expectedType:   "no_resume",
		},
		{
			name:           "no call nudge",
			nudgeType:      NudgeTypeNoCall,
			additionalData: nil,
			expectedType:   "no_call",
		},
		{
			name:      "five star matches nudge",
			nudgeType: NudgeTypeFiveStarMatches,
			additionalData: map[string]interface{}{
				"matches": []FiveStarMatchData{
					{Title: "Job 1", Summary: "Summary 1", Analysis: "Analysis 1"},
					{Title: "Job 2", Summary: "Summary 2", Analysis: "Analysis 2"},
				},
			},
			expectedType: "five_star_matches",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			emailData := gen.BuildEmailData(user, test.nudgeType, test.additionalData)

			if emailData == nil {
				t.Fatal("expected email data to be created")
			}

			if emailData.User != user {
				t.Error("expected user to be set correctly")
			}

			if emailData.NudgeType != test.expectedType {
				t.Errorf("expected nudge type %s, got %s", test.expectedType, emailData.NudgeType)
			}

			if emailData.FirstName != user.FirstName {
				t.Errorf("expected first name %s, got %s", user.FirstName, emailData.FirstName)
			}

			if emailData.FullName != user.Name {
				t.Errorf("expected full name %s, got %s", user.Name, emailData.FullName)
			}

			// Check days since event calculation
			if test.nudgeType == NudgeTypeFiveStarMatches && emailData.LastAccessed != user.LastAccessedAt {
				t.Errorf("expected last accessed to be set for five star matches, got %s", emailData.LastAccessed)
			}

			// Check additional data for five star matches
			if test.nudgeType == NudgeTypeFiveStarMatches {
				matches, ok := test.additionalData["matches"].([]FiveStarMatchData)
				if !ok {
					t.Fatal("test setup error: matches not provided or wrong type")
				}
				if len(emailData.Matches) != len(matches) {
					t.Errorf("expected %d matches, got %d", len(matches), len(emailData.Matches))
				}
				for i, m := range matches {
					if emailData.Matches[i] != m {
						t.Errorf("expected match %d to be %+v, got %+v", i, m, emailData.Matches[i])
					}
				}
			}
		})
	}
}

func TestGetEmailTemplatePath(t *testing.T) {
	tests := []struct {
		nudgeType    NudgeType
		expectedPath string
	}{
		{NudgeTypeNoResume, "shirely-no-resume.html"},
		{NudgeTypeNoCall, "shirely-no-call.html"},
		{NudgeTypeFiveStarMatches, "shirely-five-star-matches.html"},
		{NudgeType("invalid"), ""},
	}

	for _, test := range tests {
		t.Run(string(test.nudgeType), func(t *testing.T) {
			path := GetEmailTemplatePath(test.nudgeType)
			if path != test.expectedPath {
				t.Errorf("expected path %s, got %s", test.expectedPath, path)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	mockTemplateReader := NewMockTemplateReader()
	mockPromptProcessor := NewMockPromptProcessor()
	gen := NewEmailGenerator(mockTemplateReader, mockPromptProcessor)

	tests := []struct {
		name    string
		email   *GeneratedEmail
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid email",
			email: &GeneratedEmail{
				Subject:   "Test Subject",
				PlainBody: "Test body with Shirely and Talent Rodeo mentioned",
			},
			wantErr: false,
		},
		{
			name:    "nil email",
			email:   nil,
			wantErr: true,
			errMsg:  "email is nil",
		},
		{
			name: "empty subject",
			email: &GeneratedEmail{
				Subject:   "",
				PlainBody: "Test body with Shirley and Talent Rodeo mentioned",
			},
			wantErr: true,
			errMsg:  "email subject is empty",
		},
		{
			name: "empty body",
			email: &GeneratedEmail{
				Subject:   "Test Subject",
				PlainBody: "",
			},
			wantErr: true,
			errMsg:  "email body (both plain and HTML) is empty",
		},
		{
			name: "subject too long",
			email: &GeneratedEmail{
				Subject:   string(make([]byte, 201)), // 201 characters
				PlainBody: "Test body with Shirley and Talent Rodeo mentioned",
			},
			wantErr: true,
			errMsg:  "email subject too long",
		},
		{
			name: "body too long",
			email: &GeneratedEmail{
				Subject:   "Test Subject",
				PlainBody: string(make([]byte, 5001)), // 5001 characters
			},
			wantErr: true,
			errMsg:  "email body too long",
		},
		{
			name: "missing shirley signature",
			email: &GeneratedEmail{
				Subject:   "Test Subject",
				PlainBody: "Test body with Shirley and Talent Rodeo mentioned",
			},
			wantErr: true,
			errMsg:  "email body missing Shirley signature",
		},
		{
			name: "missing talent rodeo branding",
			email: &GeneratedEmail{
				Subject:   "Test Subject",
				PlainBody: "Test body with Shirely mentioned",
			},
			wantErr: true,
			errMsg:  "email body missing Talent Rodeo branding",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := gen.ValidateEmail(test.email)

			if test.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", test.errMsg)
				} else if test.errMsg != "" && err.Error() != test.errMsg {
					// For partial matching on longer error messages
					if test.errMsg == "email subject too long" && !contains(err.Error(), "email subject too long") {
						t.Errorf("expected error containing %q, got %q", test.errMsg, err.Error())
					} else if test.errMsg == "email body too long" && !contains(err.Error(), "email body too long") {
						t.Errorf("expected error containing %q, got %q", test.errMsg, err.Error())
					} else if test.errMsg != "email subject too long" && test.errMsg != "email body too long" && err.Error() != test.errMsg {
						t.Errorf("expected error %q, got %q", test.errMsg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestParseEmailResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  bool
		expected *GeneratedEmail
	}{
		{
			name: "valid response",
			response: `<email_content>
<subject>Test Subject</subject>
<body>Test body content</body>
</email_content>`,
			wantErr: false,
			expected: &GeneratedEmail{
				Subject:   "Test Subject",
				PlainBody: "Test body content",
			},
		},
		{
			name:     "missing email_content tags",
			response: `<subject>Test Subject</subject><body>Test body</body>`,
			wantErr:  true,
		},
		{
			name: "missing subject tags",
			response: `<email_content>
<body>Test body content</body>
</email_content>`,
			wantErr: true,
		},
		{
			name: "missing body tags",
			response: `<email_content>
<subject>Test Subject</subject>
</email_content>`,
			wantErr: true,
		},
		{
			name: "response with extra content",
			response: `Some preamble text
<email_content>
<subject>Test Subject</subject>
<body>Test body content with multiple lines
and formatting</body>
</email_content>
Some trailing text`,
			wantErr: false,
			expected: &GeneratedEmail{
				Subject:   "Test Subject",
				PlainBody: "Test body content with multiple lines\nand formatting",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			email, err := parseEmailResponse(test.response)

			if test.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}

				if email == nil {
					t.Fatal("expected email to be parsed")
				}

				if test.expected != nil {
					if email.Subject != test.expected.Subject {
						t.Errorf("expected subject %q, got %q", test.expected.Subject, email.Subject)
					}

					if email.PlainBody != test.expected.PlainBody {
						t.Errorf("expected body %q, got %q", test.expected.PlainBody, email.PlainBody)
					}
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || someContains(s, substr)))
}

func someContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// parseEmailResponse extracts subject and body from LLM response
func parseEmailResponse(response string) (*GeneratedEmail, error) {
	// Use regex to extract email content from XML-like tags
	emailContentPattern := regexp.MustCompile(`(?s)<email_content>(.*?)</email_content>`)
	emailMatch := emailContentPattern.FindStringSubmatch(response)
	if len(emailMatch) < 2 {
		return nil, fmt.Errorf("no email_content found in LLM response")
	}

	emailContent := emailMatch[1]

	// Extract subject
	subjectPattern := regexp.MustCompile(`(?s)<subject>(.*?)</subject>`)
	subjectMatch := subjectPattern.FindStringSubmatch(emailContent)
	if len(subjectMatch) < 2 {
		return nil, fmt.Errorf("no subject found in email content")
	}

	// Extract body
	bodyPattern := regexp.MustCompile(`(?s)<body>(.*?)</body>`)
	bodyMatch := bodyPattern.FindStringSubmatch(emailContent)
	if len(bodyMatch) < 2 {
		return nil, fmt.Errorf("no body found in email content")
	}

	return &GeneratedEmail{
		Subject:   strings.TrimSpace(subjectMatch[1]),
		PlainBody: strings.TrimSpace(bodyMatch[1]),
	}, nil
}
