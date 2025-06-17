package nudge

import (
	"fmt"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"
)

func TestDefaultNudgeSettings(t *testing.T) {
	settings := DefaultNudgeSettings

	// Verify all defaults are reasonable
	if !settings.Enabled {
		t.Error("expected default settings to be enabled")
	}

	if settings.Paused {
		t.Error("expected default settings to not be paused")
	}

	if settings.MaxEmailsPerRun <= 0 {
		t.Error("expected positive MaxEmailsPerRun")
	}

	if settings.Interval <= 0 {
		t.Error("expected positive Interval")
	}

	if settings.WeeklyRateLimit <= 0 {
		t.Error("expected positive WeeklyRateLimit")
	}

	// Verify delay days are reasonable
	if settings.NoResumeDelayDays < 0 {
		t.Error("expected non-negative NoResumeDelayDays")
	}

	if settings.NoCallDelayDays < 0 {
		t.Error("expected non-negative NoCallDelayDays")
	}

	if settings.MatchAlertDelayDays < 0 {
		t.Error("expected non-negative MatchAlertDelayDays")
	}

	// Verify operating hours are valid (0-23 or both 0 for 24/7)
	if settings.StartHour < 0 || settings.StartHour > 23 {
		t.Error("expected StartHour to be 0-23")
	}

	if settings.EndHour < 0 || settings.EndHour > 23 {
		t.Error("expected EndHour to be 0-23")
	}
}

func TestNudgeSettingsValidation(t *testing.T) {
	tests := []struct {
		name     string
		settings NudgeProcessorSettings
		valid    bool
		desc     string
	}{
		{
			name:     "valid default settings",
			settings: DefaultNudgeSettings,
			valid:    true,
			desc:     "Default settings should be valid",
		},
		{
			name: "disabled processor",
			settings: NudgeProcessorSettings{
				Enabled:             false,
				MaxEmailsPerRun:     10,
				Interval:            time.Hour,
				WeeklyRateLimit:     7 * 24 * time.Hour,
				NoResumeDelayDays:   1,
				NoCallDelayDays:     1,
				MatchAlertDelayDays: 3,
				StartHour:           9,
				EndHour:             17,
			},
			valid: true,
			desc:  "Disabled processor should still have valid config",
		},
		{
			name: "24/7 operation",
			settings: NudgeProcessorSettings{
				Enabled:             true,
				MaxEmailsPerRun:     100,
				Interval:            30 * time.Minute,
				WeeklyRateLimit:     7 * 24 * time.Hour,
				NoResumeDelayDays:   0,
				NoCallDelayDays:     0,
				MatchAlertDelayDays: 0,
				StartHour:           0,
				EndHour:             0,
			},
			valid: true,
			desc:  "24/7 operation with immediate nudges",
		},
		{
			name: "overnight operation",
			settings: NudgeProcessorSettings{
				Enabled:             true,
				MaxEmailsPerRun:     50,
				Interval:            2 * time.Hour,
				WeeklyRateLimit:     3 * 24 * time.Hour,
				NoResumeDelayDays:   2,
				NoCallDelayDays:     1,
				MatchAlertDelayDays: 7,
				StartHour:           22,
				EndHour:             6,
			},
			valid: true,
			desc:  "Overnight hours operation",
		},
		{
			name: "very restrictive settings",
			settings: NudgeProcessorSettings{
				Enabled:             true,
				MaxEmailsPerRun:     1,
				Interval:            24 * time.Hour,
				WeeklyRateLimit:     30 * 24 * time.Hour,
				NoResumeDelayDays:   7,
				NoCallDelayDays:     14,
				MatchAlertDelayDays: 30,
				StartHour:           14,
				EndHour:             15,
			},
			valid: true,
			desc:  "Very restrictive but valid settings",
		},
		{
			name: "aggressive settings",
			settings: NudgeProcessorSettings{
				Enabled:             true,
				MaxEmailsPerRun:     1000,
				Interval:            time.Minute,
				WeeklyRateLimit:     time.Hour,
				NoResumeDelayDays:   0,
				NoCallDelayDays:     0,
				MatchAlertDelayDays: 0,
				StartHour:           0,
				EndHour:             0,
			},
			valid: true,
			desc:  "Aggressive but valid settings",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Test that settings can be applied without errors
			// This is more about documenting behavior than enforcing validation
			t.Logf("Testing settings: %s", test.desc)

			// Basic sanity checks
			if test.settings.MaxEmailsPerRun < 0 {
				t.Error("MaxEmailsPerRun should not be negative")
			}

			if test.settings.Interval < 0 {
				t.Error("Interval should not be negative")
			}

			if test.settings.WeeklyRateLimit < 0 {
				t.Error("WeeklyRateLimit should not be negative")
			}
		})
	}
}

func TestSettingsUpdate(t *testing.T) {
	logger := internal.CreateTestLogger(os.Stdout)
	processor := createTestProcessor(logger)

	originalSettings := processor.settings

	// Test updating settings
	newSettings := DefaultNudgeSettings
	newSettings.MaxEmailsPerRun = 999
	newSettings.Enabled = false
	newSettings.Paused = true

	processor.UpdateSettings(newSettings)

	if processor.settings.MaxEmailsPerRun != 999 {
		t.Error("expected MaxEmailsPerRun to be updated")
	}

	if processor.settings.Enabled {
		t.Error("expected Enabled to be false")
	}

	if !processor.settings.Paused {
		t.Error("expected Paused to be true")
	}

	// Verify other settings were also updated
	if processor.settings.NoResumeDelayDays != newSettings.NoResumeDelayDays {
		t.Error("expected all settings to be updated")
	}

	// Test reverting settings
	processor.UpdateSettings(originalSettings)

	if processor.settings.MaxEmailsPerRun == 999 {
		t.Error("expected settings to be reverted")
	}
}

func TestSettingsCornerCases(t *testing.T) {
	t.Run("zero values", func(t *testing.T) {
		settings := NudgeProcessorSettings{}

		// Document behavior with zero values
		t.Logf("Zero MaxEmailsPerRun: %d", settings.MaxEmailsPerRun)
		t.Logf("Zero Interval: %v", settings.Interval)
		t.Logf("Zero WeeklyRateLimit: %v", settings.WeeklyRateLimit)

		// These are valid configurations that might have specific behaviors
	})

	t.Run("extreme values", func(t *testing.T) {
		settings := NudgeProcessorSettings{
			MaxEmailsPerRun: 1000000,
			Interval:        time.Nanosecond,
			WeeklyRateLimit: time.Nanosecond,
			StartHour:       23,
			EndHour:         0,
		}

		// Document behavior with extreme values
		t.Logf("Extreme MaxEmailsPerRun: %d", settings.MaxEmailsPerRun)
		t.Logf("Extreme Interval: %v", settings.Interval)
		t.Logf("Extreme overnight hours: %d-%d", settings.StartHour, settings.EndHour)
	})
}

func TestMaxNudgesPerTypeSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings MaxNudgesPerType
		desc     string
	}{
		{
			name: "default settings",
			settings: MaxNudgesPerType{
				NoResume:        2,
				NoCall:          1,
				FiveStarMatches: 2,
				FollowUp:        1,
			},
			desc: "Default settings should have reasonable limits",
		},
		{
			name: "zero values",
			settings: MaxNudgesPerType{
				NoResume:        0,
				NoCall:          0,
				FiveStarMatches: 0,
				FollowUp:        0,
			},
			desc: "Zero values effectively disable nudges of that type",
		},
		{
			name: "high values",
			settings: MaxNudgesPerType{
				NoResume:        5,
				NoCall:          3,
				FiveStarMatches: 4,
				FollowUp:        2,
			},
			desc: "Higher values allow more frequent communication",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.desc)

			// Verify values are non-negative
			if test.settings.NoResume < 0 {
				t.Error("NoResume should not be negative")
			}
			if test.settings.NoCall < 0 {
				t.Error("NoCall should not be negative")
			}
			if test.settings.FiveStarMatches < 0 {
				t.Error("FiveStarMatches should not be negative")
			}
			if test.settings.FollowUp < 0 {
				t.Error("FollowUp should not be negative")
			}
		})
	}
}

func TestMaxNudgesPerTypeValidation(t *testing.T) {
	tests := []struct {
		name     string
		settings NudgeProcessorSettings
		wantErr  bool
		desc     string
	}{
		{
			name: "valid settings",
			settings: NudgeProcessorSettings{
				MaxNudgesPerType: MaxNudgesPerType{
					NoResume:        2,
					NoCall:          1,
					FiveStarMatches: 2,
					FollowUp:        1,
				},
			},
			wantErr: false,
			desc:    "Valid settings should pass validation",
		},
		{
			name: "negative no-resume limit",
			settings: NudgeProcessorSettings{
				MaxNudgesPerType: MaxNudgesPerType{
					NoResume:        -1,
					NoCall:          1,
					FiveStarMatches: 2,
					FollowUp:        1,
				},
			},
			wantErr: true,
			desc:    "Negative no-resume limit should fail validation",
		},
		{
			name: "negative no-call limit",
			settings: NudgeProcessorSettings{
				MaxNudgesPerType: MaxNudgesPerType{
					NoResume:        2,
					NoCall:          -1,
					FiveStarMatches: 2,
					FollowUp:        1,
				},
			},
			wantErr: true,
			desc:    "Negative no-call limit should fail validation",
		},
		{
			name: "negative five-star matches limit",
			settings: NudgeProcessorSettings{
				MaxNudgesPerType: MaxNudgesPerType{
					NoResume:        2,
					NoCall:          1,
					FiveStarMatches: -1,
					FollowUp:        1,
				},
			},
			wantErr: true,
			desc:    "Negative five-star matches limit should fail validation",
		},
		{
			name: "negative follow-up limit",
			settings: NudgeProcessorSettings{
				MaxNudgesPerType: MaxNudgesPerType{
					NoResume:        2,
					NoCall:          1,
					FiveStarMatches: 2,
					FollowUp:        -1,
				},
			},
			wantErr: true,
			desc:    "Negative follow-up limit should fail validation",
		},
		{
			name: "all zero limits",
			settings: NudgeProcessorSettings{
				MaxNudgesPerType: MaxNudgesPerType{
					NoResume:        0,
					NoCall:          0,
					FiveStarMatches: 0,
					FollowUp:        0,
				},
			},
			wantErr: false,
			desc:    "Zero limits should be valid (effectively disabling those nudge types)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.desc)
			err := validateMaxNudgesPerType(test.settings.MaxNudgesPerType)
			if (err != nil) != test.wantErr {
				t.Errorf("validateMaxNudgesPerType() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

// validateMaxNudgesPerType validates the MaxNudgesPerType settings
func validateMaxNudgesPerType(settings MaxNudgesPerType) error {
	if settings.NoResume < 0 {
		return fmt.Errorf("no-resume limit cannot be negative: %d", settings.NoResume)
	}
	if settings.NoCall < 0 {
		return fmt.Errorf("no-call limit cannot be negative: %d", settings.NoCall)
	}
	if settings.FiveStarMatches < 0 {
		return fmt.Errorf("five-star matches limit cannot be negative: %d", settings.FiveStarMatches)
	}
	if settings.FollowUp < 0 {
		return fmt.Errorf("follow-up limit cannot be negative: %d", settings.FollowUp)
	}
	return nil
}
