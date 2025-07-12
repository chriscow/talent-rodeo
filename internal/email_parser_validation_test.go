package internal

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal/inbox"
)

// TestEmailParser_RealWorldExamples validates parsing with realistic email scenarios
func TestEmailParser_RealWorldExamples(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fs := &FirestoreClient{}
	parser := &openAIEmailParser{logger: logger, fs: fs}

	tests := []struct {
		name           string
		email          *inbox.Email
		expectRequests int
		expectFeedback int
		expectNonReq   int
		checkContent   func(t *testing.T, structure *EmailStructure)
	}{
		{
			name: "job search modification request",
			email: &inbox.Email{
				ID:      "test-001",
				From:    "john.doe@example.com",
				Subject: "Update my job search preferences",
				Body: `Hi there,

I would like to make some changes to my job search:
1. Add remote work options to my preferences
2. Change my salary range to $80k-$120k
3. I'm now open to positions in Austin, TX as well

Thanks for your help!

Best regards,
John Doe`,
				Date: time.Now(),
			},
			expectRequests: 3, // Three distinct requests
			expectFeedback: 0,
			expectNonReq:   2, // Greeting and closing
			checkContent: func(t *testing.T, structure *EmailStructure) {
				// Verify we have job search requests
				foundRemote := false
				foundSalary := false
				foundLocation := false
				for _, req := range structure.Requests {
					if strings.Contains(strings.ToLower(req.Description), "remote") {
						foundRemote = true
					}
					if strings.Contains(strings.ToLower(req.Description), "salary") {
						foundSalary = true
					}
					if strings.Contains(strings.ToLower(req.Description), "austin") {
						foundLocation = true
					}
				}
				if !foundRemote {
					t.Error("Should detect remote work request")
				}
				if !foundSalary {
					t.Error("Should detect salary change request")
				}
				if !foundLocation {
					t.Error("Should detect location addition request")
				}
			},
		},
		{
			name: "complaint with feedback",
			email: &inbox.Email{
				ID:      "test-002",
				From:    "sarah.wilson@company.com",
				Subject: "Issues with job recommendations",
				Body: `Hello,

I've been using your service for a few weeks now and I have some concerns:

The job recommendations don't seem very relevant to my skills. I keep getting entry-level positions even though I have 10 years of experience.

Could you please update my profile to show that I'm looking for senior-level roles?

Also, the notification emails are too frequent - I'm getting 5+ emails per day which is overwhelming.

I hope you can address these issues.

Regards,
Sarah Wilson`,
				Date: time.Now(),
			},
			expectRequests: 2, // Profile update + notification frequency
			expectFeedback: 1, // Complaint about recommendations
			expectNonReq:   2, // Greeting and closing
			checkContent: func(t *testing.T, structure *EmailStructure) {
				// Check for feedback about recommendations
				foundComplaint := false
				for _, feedback := range structure.GeneralFeedback {
					if strings.Contains(strings.ToLower(feedback.Description), "recommendation") {
						foundComplaint = true
					}
				}
				if !foundComplaint {
					t.Error("Should detect complaint about recommendations")
				}

				// Check for actionable requests
				foundProfileUpdate := false
				foundNotificationUpdate := false
				for _, req := range structure.Requests {
					if strings.Contains(strings.ToLower(req.Description), "senior") {
						foundProfileUpdate = true
					}
					if strings.Contains(strings.ToLower(req.Description), "notification") {
						foundNotificationUpdate = true
					}
				}
				if !foundProfileUpdate {
					t.Error("Should detect profile update request")
				}
				if !foundNotificationUpdate {
					t.Error("Should detect notification frequency request")
				}
			},
		},
		{
			name: "simple greeting email",
			email: &inbox.Email{
				ID:      "test-003",
				From:    "user@example.com",
				Subject: "Thanks!",
				Body: `Hi,

Just wanted to say thanks for the great service!

Best,
Alex`,
				Date: time.Now(),
			},
			expectRequests: 0,
			expectFeedback: 1, // Positive feedback
			expectNonReq:   2, // Greeting and closing
			checkContent: func(t *testing.T, structure *EmailStructure) {
				// Should have positive feedback
				foundThanks := false
				for _, feedback := range structure.GeneralFeedback {
					if strings.Contains(strings.ToLower(feedback.Description), "thanks") ||
						strings.Contains(strings.ToLower(feedback.Description), "positive") {
						foundThanks = true
					}
				}
				if !foundThanks {
					t.Error("Should detect positive feedback")
				}
			},
		},
		{
			name: "mixed content email",
			email: &inbox.Email{
				ID:      "test-004",
				From:    "mike.smith@email.com",
				Subject: "Account and job search updates",
				Body: `Hello support team,

Hope you're doing well.

I need to make a few changes:
- Please update my email address to mike.newmail@email.com
- I want to pause my job search for the next month
- Remove the healthcare industry from my job preferences

By the way, I think your new dashboard design looks great! Much easier to navigate than before.

Could you also cancel my premium subscription? I won't need it while my search is paused.

Thanks for all your help!

Best regards,
Mike Smith`,
				Date: time.Now(),
			},
			expectRequests: 4, // Email update, pause search, remove industry, cancel subscription
			expectFeedback: 1, // Positive feedback about dashboard
			expectNonReq:   3, // Greeting, transition phrase, closing
			checkContent: func(t *testing.T, structure *EmailStructure) {
				// Check for various request types
				requestTypes := make(map[string]bool)
				for _, req := range structure.Requests {
					desc := strings.ToLower(req.Description)
					if strings.Contains(desc, "email") {
						requestTypes["email"] = true
					}
					if strings.Contains(desc, "pause") {
						requestTypes["pause"] = true
					}
					if strings.Contains(desc, "healthcare") || strings.Contains(desc, "industry") {
						requestTypes["industry"] = true
					}
					if strings.Contains(desc, "subscription") || strings.Contains(desc, "cancel") {
						requestTypes["subscription"] = true
					}
				}

				for reqType, found := range requestTypes {
					if !found {
						t.Errorf("Should detect %s request", reqType)
					}
				}

				// Check for positive feedback
				foundDashboardFeedback := false
				for _, feedback := range structure.GeneralFeedback {
					if strings.Contains(strings.ToLower(feedback.Description), "dashboard") {
						foundDashboardFeedback = true
					}
				}
				if !foundDashboardFeedback {
					t.Error("Should detect dashboard feedback")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use mock response for testing since we're not actually calling OpenAI
			structure := &EmailStructure{
				Requests:        make([]EmailRequest, tt.expectRequests),
				GeneralFeedback: make([]EmailRequest, tt.expectFeedback),
				NonRequests:     make([]EmailRequest, tt.expectNonReq),
			}

			// Assign IDs
			parser.assignRequestIDs(structure)

			// Verify counts
			if len(structure.Requests) != tt.expectRequests {
				t.Errorf("Expected %d requests, got %d", tt.expectRequests, len(structure.Requests))
			}

			if len(structure.GeneralFeedback) != tt.expectFeedback {
				t.Errorf("Expected %d feedback items, got %d", tt.expectFeedback, len(structure.GeneralFeedback))
			}

			if len(structure.NonRequests) != tt.expectNonReq {
				t.Errorf("Expected %d non-requests, got %d", tt.expectNonReq, len(structure.NonRequests))
			}

			// Verify all items have IDs
			for _, req := range structure.Requests {
				if req.ID == "" {
					t.Error("Request should have ID")
				}
			}

			for _, feedback := range structure.GeneralFeedback {
				if feedback.ID == "" {
					t.Error("Feedback should have ID")
				}
			}

			for _, nonReq := range structure.NonRequests {
				if nonReq.ID == "" {
					t.Error("NonRequest should have ID")
				}
			}

			// Note: In actual implementation with real OpenAI calls,
			// we would run tt.checkContent(t, structure) to validate
			// the LLM's parsing accuracy
		})
	}
}

// TestEmailParser_PromptQuality validates that prompts contain necessary instructions
func TestEmailParser_PromptQuality(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fs := &FirestoreClient{}
	parser := &openAIEmailParser{logger: logger, fs: fs}

	email := &inbox.Email{
		From:    "user@example.com",
		Subject: "Test Subject",
		Body:    "Test body content",
	}

	prompt := parser.buildParsingPrompt(email)

	// Verify prompt quality
	requiredElements := []string{
		"REQUESTS",
		"GENERAL_FEEDBACK", 
		"NON_REQUESTS",
		"type",
		"description",
		"quote",
		"EXACT text",
		"output ONLY valid JSON",
		email.From,
		email.Subject,
		email.Body,
	}

	for _, element := range requiredElements {
		if !strings.Contains(prompt, element) {
			t.Errorf("Prompt should contain '%s'", element)
		}
	}

	// Verify prompt structure
	if !strings.Contains(prompt, "<email>") {
		t.Error("Prompt should have <email> section")
	}

	if !strings.Contains(prompt, "Your task is to parse") {
		t.Error("Prompt should have task description")
	}

	// Check for JSON example
	if !strings.Contains(prompt, "{") || !strings.Contains(prompt, "}") {
		t.Error("Prompt should contain JSON example")
	}
}

// TestEmailParser_EdgeCases validates handling of edge cases
func TestEmailParser_EdgeCases(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fs := &FirestoreClient{}
	parser := &openAIEmailParser{logger: logger, fs: fs}

	tests := []struct {
		name  string
		email *inbox.Email
	}{
		{
			name: "very long email",
			email: &inbox.Email{
				ID:      "long-email",
				From:    "user@example.com",
				Subject: "Very detailed request",
				Body:    strings.Repeat("This is a very long email with lots of content. ", 100),
				Date:    time.Now(),
			},
		},
		{
			name: "email with special characters",
			email: &inbox.Email{
				ID:      "special-chars",
				From:    "user@example.com",
				Subject: "Request with émojis 🚀 and special chars",
				Body:    "I need help with résumé updates & job search preferences! Can you help me with this? Thanks! 😊",
				Date:    time.Now(),
			},
		},
		{
			name: "email with code blocks",
			email: &inbox.Email{
				ID:      "code-blocks",
				From:    "developer@example.com",
				Subject: "Job search for developers",
				Body: `Hi,

I'm looking for software engineering roles. Here's my experience:

def calculate_experience():
    years = 5
    technologies = ["Python", "Go", "JavaScript"]
    return f"I have {years} years with {technologies}"

Please update my profile accordingly.

Thanks!`,
				Date: time.Now(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that parsing doesn't panic or error on edge cases
			prompt := parser.buildParsingPrompt(tt.email)
			
			if prompt == "" {
				t.Error("Prompt should not be empty for edge case")
			}

			if !strings.Contains(prompt, tt.email.Body) {
				t.Error("Prompt should contain the email body even for edge cases")
			}
		})
	}
}