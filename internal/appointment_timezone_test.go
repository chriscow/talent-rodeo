package internal

import (
	"testing"
	"time"

	"talent-rodeo/internal/utils"
)

func TestTrialEndDateCalculation(t *testing.T) {
	tests := []struct {
		name              string
		timezone          string
		callTime          time.Time
		expectedDayOffset int
	}{
		{
			name:              "Pacific timezone",
			timezone:          "America/Los_Angeles",
			callTime:          time.Date(2025, 1, 1, 15, 30, 0, 0, time.UTC),
			expectedDayOffset: 7,
		},
		{
			name:              "Eastern timezone",
			timezone:          "America/New_York",
			callTime:          time.Date(2025, 1, 1, 15, 30, 0, 0, time.UTC),
			expectedDayOffset: 7,
		},
		{
			name:              "Hawaii timezone (worst case)",
			timezone:          "Pacific/Honolulu",
			callTime:          time.Date(2025, 1, 30, 23, 0, 0, 0, time.UTC), // 1 PM HST
			expectedDayOffset: 7,
		},
		{
			name:              "Auckland timezone (best case)",
			timezone:          "Pacific/Auckland",
			callTime:          time.Date(2025, 1, 30, 1, 0, 0, 0, time.UTC), // 2 PM NZDT
			expectedDayOffset: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trialEnd, err := utils.CalculateTrialEndDate(tt.callTime, tt.timezone)
			if err != nil {
				t.Fatalf("CalculateTrialEndDate() error = %v", err)
			}

			// Load timezone for comparison
			loc, err := time.LoadLocation(tt.timezone)
			if err != nil {
				t.Fatalf("time.LoadLocation() error = %v", err)
			}

			// Convert times to user's timezone for comparison
			callTimeInUserTZ := tt.callTime.In(loc)
			trialEndInUserTZ := trialEnd.In(loc)

			// Verify it's exactly 7 days later
			expectedDate := callTimeInUserTZ.AddDate(0, 0, tt.expectedDayOffset)

			if trialEndInUserTZ.Year() != expectedDate.Year() ||
				trialEndInUserTZ.Month() != expectedDate.Month() ||
				trialEndInUserTZ.Day() != expectedDate.Day() {
				t.Errorf("Expected trial to end on %v, got %v",
					expectedDate.Format("2006-01-02"),
					trialEndInUserTZ.Format("2006-01-02"))
			}

			// Verify it ends at 11:59:59 PM in user's timezone
			if trialEndInUserTZ.Hour() != 23 ||
				trialEndInUserTZ.Minute() != 59 ||
				trialEndInUserTZ.Second() != 59 {
				t.Errorf("Expected trial to end at 23:59:59, got %02d:%02d:%02d",
					trialEndInUserTZ.Hour(),
					trialEndInUserTZ.Minute(),
					trialEndInUserTZ.Second())
			}

			t.Logf("Call completed: %s", callTimeInUserTZ.Format("2006-01-02 15:04:05 MST"))
			t.Logf("Trial ends: %s", trialEndInUserTZ.Format("2006-01-02 15:04:05 MST"))
		})
	}
}

func TestTrialEndDateEdgeCases(t *testing.T) {
	t.Run("Invalid timezone falls back to UTC", func(t *testing.T) {
		callTime := time.Now()

		// This should fail for invalid timezone
		_, err := utils.CalculateTrialEndDate(callTime, "Invalid/Timezone")
		if err == nil {
			t.Error("Expected error for invalid timezone")
		}

		// Verify error is returned but contains helpful message
		if err != nil && err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	})

	t.Run("Daylight saving time transition", func(t *testing.T) {
		// Test during a DST transition period
		// Using a time just before DST starts in US Eastern
		callTime := time.Date(2025, 3, 8, 12, 0, 0, 0, time.UTC) // Day before DST

		trialEnd, err := utils.CalculateTrialEndDate(callTime, "America/New_York")
		if err != nil {
			t.Fatalf("CalculateTrialEndDate() error = %v", err)
		}

		loc, _ := time.LoadLocation("America/New_York")
		trialEndInET := trialEnd.In(loc)

		// Should still end at 11:59 PM regardless of DST
		if trialEndInET.Hour() != 23 || trialEndInET.Minute() != 59 {
			t.Errorf("Expected 23:59, got %02d:%02d", trialEndInET.Hour(), trialEndInET.Minute())
		}
	})
}
