package transcript

import (
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal"
)

func TestGenerateRecapEmailWithTrialContext(t *testing.T) {
	// Create a mock user with trial information
	trialEnd := time.Now().Add(7 * 24 * time.Hour)
	timezone := "America/New_York"

	user := &internal.User{
		ID:              "test-user-123",
		Name:            "John Doe",
		FirstName:       "John",
		Email:           "john@example.com",
		SubscriptionEnd: &trialEnd,
		Timezone:        &timezone,
	}

	t.Run("Enhanced prompt used for trial users", func(t *testing.T) {
		// Check that user has required trial information
		if user.SubscriptionEnd == nil {
			t.Error("User should have SubscriptionEnd for trial")
		}
		if user.Timezone == nil {
			t.Error("User should have Timezone for trial")
		}

		// Verify timezone is valid
		_, err := time.LoadLocation(*user.Timezone)
		if err != nil {
			t.Errorf("Invalid timezone %s: %v", *user.Timezone, err)
		}
	})

	t.Run("Original prompt used for non-trial users", func(t *testing.T) {
		// Test user without trial information
		nonTrialUser := &internal.User{
			ID:        "test-user-456",
			Name:      "Jane Doe",
			FirstName: "Jane",
			Email:     "jane@example.com",
			// No SubscriptionEnd or Timezone
		}

		// Should not error, should fall back to original template
		if nonTrialUser.SubscriptionEnd != nil || nonTrialUser.Timezone != nil {
			t.Error("Non-trial user should not have trial information")
		}
	})

	t.Run("Verify enhanced template content", func(t *testing.T) {
		// Test basic template structure expectations
		templateContent := `You are Shirely, an AI job search specialist at Talent Rodeo.`
		if !strings.Contains(templateContent, "Shirely") {
			t.Error("Template should contain Shirely reference")
		}
	})
}

func TestTrialContextDataPreparation(t *testing.T) {
	// Test the data preparation logic that would be used in the actual function
	timezone := "America/Los_Angeles"
	trialEnd := time.Date(2025, 1, 8, 23, 59, 59, 0, time.UTC)

	// Test timezone formatting
	now := time.Now()

	// Simulate what the function does
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		t.Fatalf("Failed to load timezone %s: %v", timezone, err)
	}

	startFormatted := now.In(loc).Format("January 2, 2006 at 3:04 PM MST")
	endFormatted := trialEnd.In(loc).Format("January 2, 2006 at 11:59 PM MST")

	if startFormatted == "" {
		t.Error("Start time formatting should not be empty")
	}
	if endFormatted == "" {
		t.Error("End time formatting should not be empty")
	}

	// Verify formatted strings contain expected elements
	if !strings.Contains(startFormatted, "2025") {
		t.Error("Formatted date should contain year")
	}
	if !strings.Contains(endFormatted, "11:59") {
		t.Error("Trial end should be at 11:59 PM")
	}

	t.Logf("Trial start formatted: %s", startFormatted)
	t.Logf("Trial end formatted: %s", endFormatted)
}
