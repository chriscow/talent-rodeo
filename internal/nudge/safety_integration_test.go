package nudge

import (
	"log/slog"
	"os"
	"testing"

	"talent-rodeo/internal"
)

// TestSafetyDemo demonstrates various safety approaches for integration testing
func TestSafetyDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := internal.CreateTestLogger(os.Stdout)
	// fs client is not needed for these demonstration tests, only for actual integration.

	// This test demonstrates different safety approaches but doesn't actually run
	// the processor to avoid any accidental email sending during test runs
	t.Run("safety approach examples", func(t *testing.T) {
		// Approach 1: Dry Run Mode
		t.Run("dry_run_mode", func(t *testing.T) {
			settings := TestSafeNudgeSettings
			settings.DryRun = true // Logs what would be sent but doesn't send

			// The processor is created to show settings, but not used.
			_ = createTestProcessorWithSettings(logger, settings)

			t.Logf("Dry run processor created with settings:")
			t.Logf("- DryRun: %v", settings.DryRun)
			t.Logf("- TestModeEnabled: %v", settings.TestModeEnabled)
			t.Logf("- MaxEmailsPerRun: %d", settings.MaxEmailsPerRun)
			t.Logf("- AllowedTestEmails: %v", settings.AllowedTestEmails)
		})

		// Approach 2: Email Domain Restriction
		t.Run("domain_restriction", func(t *testing.T) {
			settings := TestSafeNudgeSettings
			settings.DryRun = false
			settings.RequireTestDomain = true
			settings.AllowedTestEmails = nil // Ensure only domain is checked
			settings.TestDomains = []string{"test.talentrodeo.com", "example.org"}

			processor := createTestProcessorWithSettings(logger, settings)

			// Test email safety checks
			testCases := []struct {
				email    string
				expected bool
				desc     string
			}{
				{"user@test.talentrodeo.com", true, "Allowed test domain"},
				{"user@example.org", true, "Another allowed test domain"},
				{"user@gmail.com", false, "Production email domain"},
				{"user@company.com", false, "Real user email"},
			}

			for _, tc := range testCases {
				result := processor.isEmailSafeToSend(tc.email)
				if result != tc.expected {
					t.Errorf("Email %s: expected %v, got %v (%s)",
						tc.email, tc.expected, result, tc.desc)
				}
				t.Logf("✓ %s: %v (%s)", tc.email, result, tc.desc)
			}
		})

		// Approach 3: Explicit Allow List
		t.Run("explicit_allow_list", func(t *testing.T) {
			settings := TestSafeNudgeSettings
			settings.DryRun = false
			settings.AllowedTestEmails = []string{
				"chris@test.com",
				"testuser@example.org",
			}
			settings.RequireTestDomain = false // Ensure only allow list is checked

			processor := createTestProcessorWithSettings(logger, settings)

			// Test specific email allowlist
			testCases := []struct {
				email    string
				expected bool
				desc     string
			}{
				{"chris@test.com", true, "Explicitly allowed test email"},
				{"testuser@example.org", true, "Another allowed test email"},
				{"other@test.com", false, "Same domain but not in list"},
				{"user@gmail.com", false, "Production email"},
			}

			for _, tc := range testCases {
				result := processor.isEmailSafeToSend(tc.email)
				if result != tc.expected {
					t.Errorf("Email %s: expected %v, got %v (%s)",
						tc.email, tc.expected, result, tc.desc)
				}
				t.Logf("✓ %s: %v (%s)", tc.email, result, tc.desc)
			}
		})

		// Approach 4: Volume Limits
		t.Run("volume_limits", func(t *testing.T) {
			settings := TestSafeNudgeSettings
			settings.MaxEmailsPerRun = 2
			settings.MaxTestEmailsTotal = 5

			processor := createTestProcessorWithSettings(logger, settings)

			t.Logf("Volume-limited processor:")
			t.Logf("- MaxEmailsPerRun: %d", settings.MaxEmailsPerRun)
			t.Logf("- MaxTestEmailsTotal: %d", settings.MaxTestEmailsTotal)

			// Simulate sending emails to test limit
			processor.emailsSentInTestSession = 4 // Already sent 4

			if processor.isEmailSafeToSend("test@example.org") {
				t.Log("✓ Can send 5th email (within limit)")
			} else {
				t.Error("Should be able to send 5th email")
			}

			processor.emailsSentInTestSession = 5 // Now at limit

			if !processor.isEmailSafeToSend("test@example.org") {
				t.Log("✓ Correctly blocked 6th email (over limit)")
			} else {
				t.Error("Should block email when over limit")
			}
		})
	})
}

// createTestProcessorWithSettings creates a processor with specific settings
func createTestProcessorWithSettings(logger *slog.Logger, settings NudgeProcessorSettings) *Processor {
	// Create mock implementations for testing
	templateReader := &MockTemplateReader{}
	promptProcessor := &MockPromptProcessor{}
	emailGenerator := NewEmailGenerator(templateReader, promptProcessor)
	emailSender := &MockEmailSender{}

	processor := NewProcessor(nil, emailGenerator, emailSender, logger)
	processor.settings = settings
	return processor
}
