package utils

import (
	"testing"
	"time"
)

func TestCalculateTrialEndDate(t *testing.T) {
	tests := []struct {
		name           string
		callTime       time.Time
		timezone       string
		expectedHour   int
		expectedMinute int
		expectedSecond int
	}{
		{
			name:           "UTC timezone",
			callTime:       time.Date(2025, 1, 1, 15, 30, 0, 0, time.UTC),
			timezone:       "UTC",
			expectedHour:   23,
			expectedMinute: 59,
			expectedSecond: 59,
		},
		{
			name:           "Pacific timezone",
			callTime:       time.Date(2025, 1, 1, 15, 30, 0, 0, time.UTC),
			timezone:       "America/Los_Angeles",
			expectedHour:   23,
			expectedMinute: 59,
			expectedSecond: 59,
		},
		{
			name:           "Eastern timezone",
			callTime:       time.Date(2025, 1, 1, 15, 30, 0, 0, time.UTC),
			timezone:       "America/New_York",
			expectedHour:   23,
			expectedMinute: 59,
			expectedSecond: 59,
		},
		{
			name:           "Hawaii timezone",
			callTime:       time.Date(2025, 1, 1, 15, 30, 0, 0, time.UTC),
			timezone:       "Pacific/Honolulu",
			expectedHour:   23,
			expectedMinute: 59,
			expectedSecond: 59,
		},
		{
			name:           "Auckland timezone",
			callTime:       time.Date(2025, 1, 1, 15, 30, 0, 0, time.UTC),
			timezone:       "Pacific/Auckland",
			expectedHour:   23,
			expectedMinute: 59,
			expectedSecond: 59,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trialEnd, err := CalculateTrialEndDate(tt.callTime, tt.timezone)
			if err != nil {
				t.Fatalf("CalculateTrialEndDate() error = %v", err)
			}

			// Convert back to user's timezone to verify the time
			loc, err := time.LoadLocation(tt.timezone)
			if err != nil {
				t.Fatalf("time.LoadLocation() error = %v", err)
			}

			userTrialEnd := trialEnd.In(loc)

			// Verify it's exactly 7 days later
			callTimeInUserTZ := tt.callTime.In(loc)
			expectedDate := callTimeInUserTZ.AddDate(0, 0, 7)

			if userTrialEnd.Year() != expectedDate.Year() ||
				userTrialEnd.Month() != expectedDate.Month() ||
				userTrialEnd.Day() != expectedDate.Day() {
				t.Errorf("Expected date %v, got %v", expectedDate.Format("2006-01-02"), userTrialEnd.Format("2006-01-02"))
			}

			// Verify it's set to 11:59:59 PM
			if userTrialEnd.Hour() != tt.expectedHour {
				t.Errorf("Expected hour %d, got %d", tt.expectedHour, userTrialEnd.Hour())
			}
			if userTrialEnd.Minute() != tt.expectedMinute {
				t.Errorf("Expected minute %d, got %d", tt.expectedMinute, userTrialEnd.Minute())
			}
			if userTrialEnd.Second() != tt.expectedSecond {
				t.Errorf("Expected second %d, got %d", tt.expectedSecond, userTrialEnd.Second())
			}
		})
	}
}

func TestCalculateTrialEndDateInvalidTimezone(t *testing.T) {
	callTime := time.Now()
	_, err := CalculateTrialEndDate(callTime, "Invalid/Timezone")
	if err == nil {
		t.Error("Expected error for invalid timezone, got nil")
	}
}

func TestFormatTimeInUserTimezone(t *testing.T) {
	utcTime := time.Date(2025, 1, 1, 15, 30, 0, 0, time.UTC)
	
	tests := []struct {
		name     string
		timezone string
		format   string
		expected string
	}{
		{
			name:     "Pacific timezone",
			timezone: "America/Los_Angeles",
			format:   "January 2, 2006 at 3:04 PM MST",
			expected: "January 1, 2025 at 7:30 AM PST",
		},
		{
			name:     "UTC timezone",
			timezone: "UTC",
			format:   "January 2, 2006 at 3:04 PM MST",
			expected: "January 1, 2025 at 3:30 PM UTC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatTimeInUserTimezone(utcTime, tt.timezone, tt.format)
			if err != nil {
				t.Fatalf("FormatTimeInUserTimezone() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCalculateDaysRemaining(t *testing.T) {
	// Test with a future date 3 days from now
	futureDate := time.Now().Add(3 * 24 * time.Hour)
	
	days := CalculateDaysRemaining(futureDate, "UTC")
	if days != 3 {
		t.Errorf("Expected 3 days remaining, got %d", days)
	}
	
	// Test with a past date
	pastDate := time.Now().Add(-24 * time.Hour)
	days = CalculateDaysRemaining(pastDate, "UTC")
	if days != 0 {
		t.Errorf("Expected 0 days remaining for past date, got %d", days)
	}
}