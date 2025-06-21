package nudge

import (
	"bytes"
	"talent-rodeo/internal"
	"testing"
	"time"
)

// TestAllTemplatesCanExecute ensures that all cached templates can be executed with mock data.
// This test prevents runtime panics caused by mismatched template variables.
func TestAllTemplatesCanExecute(t *testing.T) {
	// Must load templates first, this would be done in main.go on startup
	if err := LoadAndCacheTemplates(); err != nil {
		t.Fatalf("Failed to load and cache templates: %v", err)
	}

	mockUser := &internal.User{
		ID:              "test-user-id",
		FirstName:       "John",
		LastName:        "Doe",
		Name:            "John Doe",
		Email:           "john.doe@example.com",
		Status:          internal.UserStatusCallScheduled,
		StatusUpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		LastAccessedAt:  time.Now().Add(-2 * 24 * time.Hour),
	}

	mockMatches := []FiveStarMatchData{
		{
			Title:    "Senior Software Engineer",
			Summary:  "A role for a seasoned developer.",
			Analysis: "This matches your skillset in Go and distributed systems.",
		},
		{
			Title:    "Backend Developer",
			Summary:  "Work on scalable microservices.",
			Analysis: "Your experience with Kubernetes is a strong fit.",
		},
	}

	for _, nudgeType := range AllNudgeTypes {
		// Skip types that don't have an email template
		if nudgeType.EmailTemplatePath() == "" {
			continue
		}

		t.Run(string(nudgeType), func(t *testing.T) {
			template, err := GetTemplate(nudgeType)
			if err != nil {
				t.Fatalf("Failed to get template from cache: %v", err)
			}

			// Create mock data for the template
			emailData := &NudgeEmailData{
				User:            mockUser,
				NudgeType:       nudgeType.String(),
				FirstName:       mockUser.FirstName,
				LastName:        mockUser.LastName,
				FullName:        mockUser.Name,
				LastAccessed:    mockUser.LastAccessedAt,
				LastStateChange: mockUser.StatusUpdatedAt,
				Matches:         mockMatches,
			}

			// Attempt to execute all parts of the template
			var buf bytes.Buffer
			if subjectTmpl := template.Lookup("subject"); subjectTmpl != nil {
				if err := subjectTmpl.Execute(&buf, emailData); err != nil {
					t.Errorf("Failed to execute subject template: %v", err)
				}
			} else {
				t.Errorf("Template is missing a 'subject' block")
			}
			buf.Reset()

			if htmlBodyTmpl := template.Lookup("htmlBody"); htmlBodyTmpl != nil {
				if err := htmlBodyTmpl.Execute(&buf, emailData); err != nil {
					t.Errorf("Failed to execute htmlBody template: %v", err)
				}
			}
			buf.Reset()

			if plainBodyTmpl := template.Lookup("plainBody"); plainBodyTmpl != nil {
				if err := plainBodyTmpl.Execute(&buf, emailData); err != nil {
					t.Errorf("Failed to execute plainBody template: %v", err)
				}
			}
		})
	}
}
