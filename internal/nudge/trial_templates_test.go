package nudge

import (
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal"
)

func TestTrialEmailTemplatesRender(t *testing.T) {
	// Load templates before testing
	err := LoadAndCacheTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Create mock user with trial data
	user := &internal.User{
		ID:        "test-user-123",
		FirstName: "John",
		LastName:  "Doe",
		Name:      "John Doe",
		Email:     "john@example.com",
	}

	trialEmailTypes := []NudgeType{
		NudgeTypeTrialWelcome,
		NudgeTypeTrialExpectations,
		NudgeTypeTrialEngagement,
		NudgeTypeTrialConversion,
	}

	for _, nudgeType := range trialEmailTypes {
		t.Run(string(nudgeType), func(t *testing.T) {
			template, err := GetTemplate(nudgeType)
			if err != nil {
				t.Fatalf("Failed to get template from cache: %v", err)
			}

			// Test data for different match volume scenarios
			testScenarios := []struct {
				name                string
				matchVolumeCategory string
				highQualityMatches  int
			}{
				{"High Volume", "high", 15},
				{"Typical Volume", "typical", 6},
				{"Low Volume", "low", 2},
			}

			for _, scenario := range testScenarios {
				t.Run(scenario.name, func(t *testing.T) {
					emailData := &NudgeEmailData{
						User:                user,
						NudgeType:           nudgeType.String(),
						FirstName:           user.FirstName,
						LastName:            user.LastName,
						FullName:            user.Name,
						LastAccessed:        time.Now().Add(-24 * time.Hour),
						LastStateChange:     time.Now().Add(-48 * time.Hour),
						TrialStartDateUser:  "July 3, 2025 at 9:00 AM PDT",
						TrialEndDateUser:    "July 10, 2025 at 11:59 PM PDT",
						DaysRemaining:       3,
						HighQualityMatches:  scenario.highQualityMatches,
						MatchVolumeCategory: scenario.matchVolumeCategory,
					}

					// Test subject template
					var subject strings.Builder
					if template.Lookup("subject") == nil {
						t.Errorf("Template %s is missing {{define \"subject\"}}", nudgeType.EmailTemplatePath())
						return
					}
					err = template.ExecuteTemplate(&subject, "subject", emailData)
					if err != nil {
						t.Errorf("Failed to execute subject template: %v", err)
						return
					}

					subjectText := subject.String()
					if subjectText == "" {
						t.Error("Subject should not be empty")
					}
					if !strings.Contains(subjectText, user.FirstName) {
						t.Error("Subject should contain user's first name")
					}

					// Test plain body template
					var plainBody strings.Builder
					if template.Lookup("plainBody") == nil {
						t.Errorf("Template %s is missing {{define \"plainBody\"}}", nudgeType.EmailTemplatePath())
						return
					}
					err = template.ExecuteTemplate(&plainBody, "plainBody", emailData)
					if err != nil {
						t.Errorf("Failed to execute plainBody template: %v", err)
						return
					}

					plainText := plainBody.String()
					if plainText == "" {
						t.Error("Plain body should not be empty")
					}

					// Verify Shirely voice elements
					if !strings.Contains(plainText, "Shirely") {
						t.Error("Plain body should contain 'Shirely'")
					}
					if !strings.Contains(plainText, "Talent Rodeo") {
						t.Error("Plain body should contain 'Talent Rodeo'")
					}
					if !strings.Contains(plainText, "HIRE") {
						t.Error("Plain body should contain 'HIRE' (Shirely's name reference)")
					}

					// Verify trial-specific content
					if !strings.Contains(plainText, emailData.TrialEndDateUser) {
						t.Error("Plain body should contain trial end date")
					}
					if !strings.Contains(plainText, user.FirstName) {
						t.Error("Plain body should contain user's first name")
					}

					// Test HTML body template
					var htmlBody strings.Builder
					if template.Lookup("htmlBody") == nil {
						t.Errorf("Template %s is missing {{define \"htmlBody\"}}", nudgeType.EmailTemplatePath())
						return
					}
					err = template.ExecuteTemplate(&htmlBody, "htmlBody", emailData)
					if err != nil {
						t.Errorf("Failed to execute htmlBody template: %v", err)
						return
					}

					htmlText := htmlBody.String()
					if htmlText == "" {
						t.Error("HTML body should not be empty")
					}

					// Verify HTML structure
					if !strings.Contains(htmlText, "<div") {
						t.Error("HTML body should contain div elements")
					}
					if !strings.Contains(htmlText, "style=") {
						t.Error("HTML body should contain inline styles")
					}

					// Test match volume switching (only for expectations email)
					if nudgeType == NudgeTypeTrialExpectations {
						if scenario.matchVolumeCategory == "high" && !strings.Contains(htmlText, "Great news") {
							t.Error("High volume template should contain 'Great news'")
						}
						if scenario.matchVolumeCategory == "typical" && !strings.Contains(htmlText, "right on track") {
							t.Error("Typical volume template should contain 'right on track'")
						}
						if scenario.matchVolumeCategory == "low" && !strings.Contains(htmlText, "actually great") {
							t.Error("Low volume template should contain 'actually great'")
						}
					}

					t.Logf("✓ %s template rendered successfully for %s scenario", nudgeType, scenario.name)
				})
			}
		})
	}
}

func TestMatchVolumeDetection(t *testing.T) {
	tests := []struct {
		matches  int
		expected string
	}{
		{15, "high"},
		{10, "high"},
		{9, "typical"},
		{4, "typical"},
		{3, "low"},
		{1, "low"},
		{0, "low"},
	}

	for _, test := range tests {
		result := DetermineMatchVolumeCategory(test.matches)
		if result != test.expected {
			t.Errorf("DetermineMatchVolumeCategory(%d) = %s, want %s", test.matches, result, test.expected)
		}
	}
}

func TestTrialNudgeTypesValidation(t *testing.T) {
	trialNudgeTypes := []NudgeType{
		NudgeTypeTrialWelcome,
		NudgeTypeTrialExpectations,
		NudgeTypeTrialEngagement,
		NudgeTypeTrialConversion,
	}

	for _, nudgeType := range trialNudgeTypes {
		t.Run(string(nudgeType), func(t *testing.T) {
			// Verify nudge type is valid
			if !nudgeType.IsValid() {
				t.Errorf("Trial nudge type %s should be valid", nudgeType)
			}

			// Verify email template path exists
			templatePath := nudgeType.EmailTemplatePath()
			if templatePath == "" {
				t.Errorf("Trial nudge type %s should have an email template path", nudgeType)
			}

			// Verify template path follows expected pattern
			expectedPrefix := "emails/trial-"
			if !strings.HasPrefix(templatePath, expectedPrefix) {
				t.Errorf("Template path %s should start with %s", templatePath, expectedPrefix)
			}
		})
	}
}
