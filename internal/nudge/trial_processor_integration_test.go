package nudge

import (
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/utils"
)

func TestTrialEmailProcessorIntegration(t *testing.T) {
	// Load templates for testing
	err := LoadAndCacheTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Create test user in trial period
	timezone := "America/Los_Angeles"
	trialStartTime := time.Now().Add(-2 * 24 * time.Hour) // Started 2 days ago
	trialEndTime, err := utils.CalculateTrialEndDate(trialStartTime, timezone)
	if err != nil {
		t.Fatalf("Failed to calculate trial end date: %v", err)
	}

	user := &internal.User{
		ID:              "test-trial-processor-user",
		FirstName:       "Alice",
		LastName:        "Johnson", 
		Name:            "Alice Johnson",
		Email:           "alice@example.com",
		Status:          internal.UserStatusCallCompleted,
		Timezone:        &timezone,
		SubscriptionEnd: &trialEndTime,
		LastAccessedAt:  time.Now().Add(-48 * time.Hour), // Last accessed 2 days ago
		StatusUpdatedAt: time.Now().Add(-72 * time.Hour), // Status updated 3 days ago
	}

	// Test eligibility for different trial email types
	settings := DefaultNudgeSettings

	testCases := []struct {
		name      string
		nudgeType NudgeType
		shouldBeEligible bool
		reason    string
	}{
		{
			name:      "Trial Welcome (Day 1)",
			nudgeType: NudgeTypeTrialWelcome,
			shouldBeEligible: false, // Too late (trial started 2 days ago)
			reason:    "Should be too late for day 1 email",
		},
		{
			name:      "Trial Expectations (Day 2)",
			nudgeType: NudgeTypeTrialExpectations,
			shouldBeEligible: true, // Should be eligible for day 2
			reason:    "Should be eligible for day 2 email",
		},
		{
			name:      "Trial Engagement (Day 3)",
			nudgeType: NudgeTypeTrialEngagement,
			shouldBeEligible: false, // Too early
			reason:    "Should be too early for day 3 email",
		},
		{
			name:      "Trial Conversion (Day 6)",
			nudgeType: NudgeTypeTrialConversion,
			shouldBeEligible: false, // Too early
			reason:    "Should be too early for day 6 email",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test eligibility
			eligible, reason := isUserEligibleForNudge(user, tc.nudgeType, time.Now(), settings)
			
			if eligible != tc.shouldBeEligible {
				t.Errorf("Expected eligibility %v, got %v. Reason: %s", tc.shouldBeEligible, eligible, reason)
			}

			t.Logf("✓ %s: eligible=%v, reason=%s", tc.name, eligible, reason)
		})
	}
}

func TestTrialEmailDataGeneration(t *testing.T) {
	// Load templates for testing
	err := LoadAndCacheTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Create test user
	timezone := "America/New_York"
	trialStartTime := time.Now().Add(-1 * 24 * time.Hour) // Started 1 day ago
	trialEndTime, err := utils.CalculateTrialEndDate(trialStartTime, timezone)
	if err != nil {
		t.Fatalf("Failed to calculate trial end date: %v", err)
	}

	user := &internal.User{
		ID:              "test-data-gen-user",
		FirstName:       "Bob",
		LastName:        "Smith",
		Name:            "Bob Smith",
		Email:           "bob@example.com",
		Status:          internal.UserStatusCallCompleted,
		Timezone:        &timezone,
		SubscriptionEnd: &trialEndTime,
		LastAccessedAt:  time.Now().Add(-2 * time.Hour),
		StatusUpdatedAt: time.Now().Add(-24 * time.Hour),
	}

	// Create email generator
	templateReader := NewAssetTemplateReader()
	promptProcessor := NewLLMPromptProcessor()
	emailGenerator := NewEmailGenerator(templateReader, promptProcessor)

	// Test data generation for different trial emails
	trialNudgeTypes := []NudgeType{
		NudgeTypeTrialWelcome,
		NudgeTypeTrialExpectations,
		NudgeTypeTrialEngagement,
		NudgeTypeTrialConversion,
	}

	for _, nudgeType := range trialNudgeTypes {
		t.Run(string(nudgeType), func(t *testing.T) {
			// Build email data with mock additional data
			additionalData := map[string]any{
				"qualityMatches": 8, // Typical volume
			}
			
			emailData := emailGenerator.BuildEmailData(user, nudgeType, additionalData)

			// Verify trial-specific data is populated
			if emailData.FirstName != user.FirstName {
				t.Error("FirstName should be populated")
			}
			if emailData.TrialStartDateUser == "" {
				t.Error("TrialStartDateUser should be populated")
			}
			if emailData.TrialEndDateUser == "" {
				t.Error("TrialEndDateUser should be populated")
			}
			if emailData.DaysRemaining < 0 || emailData.DaysRemaining > 7 {
				t.Errorf("DaysRemaining should be between 0-7, got %d", emailData.DaysRemaining)
			}
			if emailData.HighQualityMatches != 8 {
				t.Errorf("HighQualityMatches should be 8, got %d", emailData.HighQualityMatches)
			}
			if emailData.MatchVolumeCategory != "typical" {
				t.Errorf("MatchVolumeCategory should be 'typical', got %s", emailData.MatchVolumeCategory)
			}

			t.Logf("✓ %s data: start=%s, end=%s, days=%d, matches=%d (%s)",
				nudgeType,
				emailData.TrialStartDateUser,
				emailData.TrialEndDateUser,
				emailData.DaysRemaining,
				emailData.HighQualityMatches,
				emailData.MatchVolumeCategory)
		})
	}
}

func TestTrialEmailEndToEndGeneration(t *testing.T) {
	// Load templates for testing
	err := LoadAndCacheTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Create test user
	timezone := "UTC"
	trialStartTime := time.Now().Add(-3 * 24 * time.Hour) // Started 3 days ago
	trialEndTime, err := utils.CalculateTrialEndDate(trialStartTime, timezone)
	if err != nil {
		t.Fatalf("Failed to calculate trial end date: %v", err)
	}

	user := &internal.User{
		ID:              "test-e2e-user",
		FirstName:       "Carol",
		LastName:        "Davis",
		Name:            "Carol Davis",
		Email:           "carol@example.com",
		Status:          internal.UserStatusCallCompleted,
		Timezone:        &timezone,
		SubscriptionEnd: &trialEndTime,
		LastAccessedAt:  time.Now().Add(-1 * time.Hour),
		StatusUpdatedAt: time.Now().Add(-72 * time.Hour),
	}

	// Create email generator
	templateReader := NewAssetTemplateReader()
	promptProcessor := NewLLMPromptProcessor()
	emailGenerator := NewEmailGenerator(templateReader, promptProcessor)

	// Test end-to-end email generation for trial engagement (Day 3)
	nudgeType := NudgeTypeTrialEngagement
	additionalData := map[string]any{
		"qualityMatches": 15, // High volume
	}

	// Build email data
	emailData := emailGenerator.BuildEmailData(user, nudgeType, additionalData)

	// Generate email using template (not LLM since we have templates)
	template, err := GetTemplate(nudgeType)
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	// Generate subject
	var subject strings.Builder
	err = template.ExecuteTemplate(&subject, "subject", emailData)
	if err != nil {
		t.Fatalf("Failed to generate subject: %v", err)
	}

	// Generate plain body
	var plainBody strings.Builder
	err = template.ExecuteTemplate(&plainBody, "plainBody", emailData)
	if err != nil {
		t.Fatalf("Failed to generate plain body: %v", err)
	}

	// Generate HTML body
	var htmlBody strings.Builder
	err = template.ExecuteTemplate(&htmlBody, "htmlBody", emailData)
	if err != nil {
		t.Fatalf("Failed to generate HTML body: %v", err)
	}

	// Create generated email
	generatedEmail := &GeneratedEmail{
		Subject:   subject.String(),
		PlainBody: plainBody.String(),
		HTMLBody:  htmlBody.String(),
	}

	// Validate the email
	err = emailGenerator.ValidateEmail(generatedEmail)
	if err != nil {
		t.Fatalf("Generated email failed validation: %v", err)
	}

	// Verify content
	if !strings.Contains(generatedEmail.Subject, user.FirstName) {
		t.Error("Subject should contain user's first name")
	}
	if !strings.Contains(generatedEmail.PlainBody, "Shirely") {
		t.Error("Email should contain Shirely's name")
	}
	if !strings.Contains(generatedEmail.PlainBody, "15") {
		t.Error("Email should contain the quality matches count")
	}
	if !strings.Contains(generatedEmail.PlainBody, "4 days") {
		t.Error("Email should contain the days remaining")
	}

	t.Logf("✓ Generated trial email successfully")
	t.Logf("  Subject: %s", generatedEmail.Subject)
	t.Logf("  Plain body length: %d characters", len(generatedEmail.PlainBody))
	t.Logf("  HTML body length: %d characters", len(generatedEmail.HTMLBody))
}