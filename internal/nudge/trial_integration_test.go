package nudge

import (
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/utils"
)

func TestTrialEmailSequenceIntegration(t *testing.T) {
	// Load templates for testing
	err := LoadAndCacheTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Create test user with trial data
	trialStartTime := time.Now().Add(-2 * 24 * time.Hour) // Started 2 days ago
	timezone := "America/Los_Angeles"
	trialEndTime, err := utils.CalculateTrialEndDate(trialStartTime, timezone)
	if err != nil {
		t.Fatalf("Failed to calculate trial end date: %v", err)
	}

	user := &internal.User{
		ID:              "test-trial-user",
		FirstName:       "Sarah",
		LastName:        "Johnson",
		Name:            "Sarah Johnson",
		Email:           "sarah@example.com",
		Timezone:        timezone,
		SubscriptionEnd: &trialEndTime,
	}

	// Test scenarios for different match volumes
	testScenarios := []struct {
		name               string
		highQualityMatches int
		expectedCategory   string
	}{
		{"High Volume User", 15, "high"},
		{"Typical Volume User", 6, "typical"},
		{"Low Volume User", 2, "low"},
	}

	for _, scenario := range testScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Test all 4 trial email types
			trialEmails := []struct {
				nudgeType    NudgeType
				day          int
				expectedText string
			}{
				{NudgeTypeTrialWelcome, 1, "Welcome to Talent Rodeo"},
				{NudgeTypeTrialExpectations, 2, "here's the thing about quality job searches"},
				{NudgeTypeTrialEngagement, 3, "3 ways to make me even smarter"},
				{NudgeTypeTrialConversion, 6, "your trial ends tomorrow"},
			}

			for _, email := range trialEmails {
				t.Run(string(email.nudgeType), func(t *testing.T) {
					// Calculate days remaining based on email day
					daysRemaining := 7 - email.day

					// Build email data
					emailData := &NudgeEmailData{
						User:      user,
						NudgeType: email.nudgeType.String(),
						FirstName: user.FirstName,
						LastName:  user.LastName,
						FullName:  user.Name,

						// Trial-specific data
						TrialStartDateUser:  formatTimeForUser(trialStartTime, timezone),
						TrialEndDateUser:    formatTimeForUser(trialEndTime, timezone),
						DaysRemaining:       daysRemaining,
						HighQualityMatches:  scenario.highQualityMatches,
						MatchVolumeCategory: DetermineMatchVolumeCategory(scenario.highQualityMatches),

						// Standard fields
						LastAccessed:    time.Now().Add(-24 * time.Hour),
						LastStateChange: time.Now().Add(-48 * time.Hour),
					}

					// Test email generation
					template, err := GetTemplate(email.nudgeType)
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

					// Verify email content
					subjectText := subject.String()
					plainText := plainBody.String()
					htmlText := htmlBody.String()

					// Basic validation
					if subjectText == "" {
						t.Error("Subject should not be empty")
					}
					if plainText == "" {
						t.Error("Plain body should not be empty")
					}
					if htmlText == "" {
						t.Error("HTML body should not be empty")
					}

					// Verify personalization
					if !strings.Contains(subjectText, user.FirstName) {
						t.Error("Subject should contain user's first name")
					}
					if !strings.Contains(plainText, user.FirstName) {
						t.Error("Plain body should contain user's first name")
					}
					if !strings.Contains(htmlText, user.FirstName) {
						t.Error("HTML body should contain user's first name")
					}

					// Verify Shirely voice elements
					if !strings.Contains(plainText, "Shirely") {
						t.Error("Email should contain Shirely's name")
					}
					if !strings.Contains(plainText, "HIRE") {
						t.Error("Email should contain HIRE reference")
					}
					if !strings.Contains(plainText, "Talent Rodeo") {
						t.Error("Email should contain Talent Rodeo branding")
					}

					// Verify trial-specific content
					if !strings.Contains(plainText, emailData.TrialEndDateUser) {
						t.Error("Email should contain trial end date")
					}

					// Verify expected content for each email type
					if !strings.Contains(strings.ToLower(subjectText), strings.ToLower(email.expectedText)) {
						t.Errorf("Subject should contain expected text: %s", email.expectedText)
					}

					// Special validation for expectations email (match volume switching)
					if email.nudgeType == NudgeTypeTrialExpectations {
						switch scenario.expectedCategory {
						case "high":
							if !strings.Contains(plainText, "Great news") && !strings.Contains(plainText, "GREAT NEWS") {
								t.Error("High volume expectations email should contain 'Great news'")
							}
						case "typical":
							if !strings.Contains(plainText, "right on track") {
								t.Error("Typical volume expectations email should contain 'right on track'")
							}
						case "low":
							if !strings.Contains(plainText, "actually great") {
								t.Error("Low volume expectations email should contain 'actually great'")
							}
						}
					}

					// Log success
					t.Logf("✓ %s email generated successfully for %s scenario", email.nudgeType, scenario.name)
					t.Logf("  Subject: %s", subjectText)
					t.Logf("  Days remaining: %d", daysRemaining)
					t.Logf("  Quality matches: %d (%s volume)", scenario.highQualityMatches, scenario.expectedCategory)
				})
			}
		})
	}
}

func TestTrialEmailSequenceFlow(t *testing.T) {
	// Load templates for testing
	err := LoadAndCacheTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Test the complete 7-day flow
	trialStartTime := time.Now()
	timezone := "America/New_York"
	trialEndTime, err := utils.CalculateTrialEndDate(trialStartTime, timezone)
	if err != nil {
		t.Fatalf("Failed to calculate trial end date: %v", err)
	}

	user := &internal.User{
		ID:              "test-flow-user",
		FirstName:       "John",
		LastName:        "Doe",
		Name:            "John Doe",
		Email:           "john@example.com",
		Timezone:        timezone,
		SubscriptionEnd: &trialEndTime,
	}

	// Simulate the email sequence timeline
	emailSchedule := []struct {
		day       int
		nudgeType NudgeType
		name      string
	}{
		{1, NudgeTypeTrialWelcome, "Welcome Email"},
		{2, NudgeTypeTrialExpectations, "Expectations Email"},
		{3, NudgeTypeTrialEngagement, "Engagement Email"},
		{6, NudgeTypeTrialConversion, "Conversion Email"},
	}

	for _, schedule := range emailSchedule {
		t.Run(schedule.name, func(t *testing.T) {
			// Calculate current day metrics
			currentTime := trialStartTime.Add(time.Duration(schedule.day-1) * 24 * time.Hour)
			daysRemaining := 7 - schedule.day
			qualityMatches := 5 + schedule.day // Simulate growing matches

			emailData := &NudgeEmailData{
				User:      user,
				NudgeType: schedule.nudgeType.String(),
				FirstName: user.FirstName,
				LastName:  user.LastName,
				FullName:  user.Name,

				TrialStartDateUser:  formatTimeForUser(trialStartTime, timezone),
				TrialEndDateUser:    formatTimeForUser(trialEndTime, timezone),
				DaysRemaining:       daysRemaining,
				HighQualityMatches:  qualityMatches,
				MatchVolumeCategory: DetermineMatchVolumeCategory(qualityMatches),

				LastAccessed:    currentTime.Add(-1 * time.Hour),
				LastStateChange: currentTime.Add(-2 * time.Hour),
			}

			// Verify email can be generated
			template, err := GetTemplate(schedule.nudgeType)
			if err != nil {
				t.Fatalf("Failed to get template for %s: %v", schedule.name, err)
			}

			var subject strings.Builder
			err = template.ExecuteTemplate(&subject, "subject", emailData)
			if err != nil {
				t.Fatalf("Failed to generate subject for %s: %v", schedule.name, err)
			}

			// Verify content makes sense for the timeline
			subjectText := subject.String()
			if subjectText == "" {
				t.Error("Subject should not be empty")
			}

			// Check day-specific logic
			if schedule.day == 6 { // Conversion email
				if !strings.Contains(subjectText, "tomorrow") && !strings.Contains(subjectText, "ends") {
					t.Error("Conversion email should mention urgency")
				}
			}

			t.Logf("✓ Day %d - %s: %s", schedule.day, schedule.name, subjectText)
		})
	}
}

func TestTrialEmailDataValidation(t *testing.T) {
	// Load templates for testing
	err := LoadAndCacheTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Test edge cases and validation
	timezone := "UTC"
	trialStartTime := time.Now()
	trialEndTime, _ := utils.CalculateTrialEndDate(trialStartTime, timezone)

	user := &internal.User{
		ID:              "test-validation-user",
		FirstName:       "Test",
		LastName:        "User",
		Name:            "Test User",
		Email:           "test@example.com",
		Timezone:        timezone,
		SubscriptionEnd: &trialEndTime,
	}

	testCases := []struct {
		name             string
		qualityMatches   int
		daysRemaining    int
		expectedValid    bool
		expectedCategory string
	}{
		{"Zero matches", 0, 5, true, "low"},
		{"Single match", 1, 5, true, "low"},
		{"Boundary low", 3, 5, true, "low"},
		{"Boundary typical low", 4, 5, true, "typical"},
		{"Boundary typical high", 9, 5, true, "typical"},
		{"Boundary high", 10, 5, true, "high"},
		{"High matches", 25, 5, true, "high"},
		{"Last day", 15, 1, true, "high"},
		{"Trial expired", 5, 0, true, "typical"},
		{"Negative days", 5, -1, true, "typical"}, // Should handle gracefully
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			emailData := &NudgeEmailData{
				User:      user,
				NudgeType: NudgeTypeTrialExpectations.String(),
				FirstName: user.FirstName,
				LastName:  user.LastName,
				FullName:  user.Name,

				TrialStartDateUser:  formatTimeForUser(trialStartTime, timezone),
				TrialEndDateUser:    formatTimeForUser(trialEndTime, timezone),
				DaysRemaining:       tc.daysRemaining,
				HighQualityMatches:  tc.qualityMatches,
				MatchVolumeCategory: DetermineMatchVolumeCategory(tc.qualityMatches),

				LastAccessed:    time.Now().Add(-1 * time.Hour),
				LastStateChange: time.Now().Add(-2 * time.Hour),
			}

			// Verify category calculation
			if emailData.MatchVolumeCategory != tc.expectedCategory {
				t.Errorf("Expected category %s, got %s", tc.expectedCategory, emailData.MatchVolumeCategory)
			}

			// Try to generate email
			template, err := GetTemplate(NudgeTypeTrialExpectations)
			if err != nil {
				if tc.expectedValid {
					t.Fatalf("Expected valid template, got error: %v", err)
				}
				return
			}

			var subject strings.Builder
			err = template.ExecuteTemplate(&subject, "subject", emailData)
			if err != nil {
				if tc.expectedValid {
					t.Fatalf("Expected valid email generation, got error: %v", err)
				}
				return
			}

			if !tc.expectedValid {
				t.Error("Expected email generation to fail, but it succeeded")
			}

			t.Logf("✓ %s: %d matches → %s category", tc.name, tc.qualityMatches, tc.expectedCategory)
		})
	}
}

// Helper function to format time for user display
func formatTimeForUser(t time.Time, timezone string) string {
	formatted, err := utils.FormatTimeInUserTimezone(t, timezone, "January 2, 2006 at 3:04 PM MST")
	if err != nil {
		return t.Format("January 2, 2006 at 3:04 PM MST")
	}
	return formatted
}
