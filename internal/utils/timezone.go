package utils

import (
	"fmt"
	"time"
)

// CalculateTrialEndDate calculates the trial end date in the user's timezone
// The trial always ends at 11:59 PM on the 7th day in the user's local time
func CalculateTrialEndDate(callCompletionTime time.Time, userTimezone string) (time.Time, error) {
	// Load user's timezone
	loc, err := time.LoadLocation(userTimezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone %s: %w", userTimezone, err)
	}

	// Convert call completion to user's timezone
	userTime := callCompletionTime.In(loc)

	// Add 7 days and set to end of day (23:59:59)
	trialEndDate := userTime.AddDate(0, 0, 7)
	trialEndDate = time.Date(
		trialEndDate.Year(),
		trialEndDate.Month(),
		trialEndDate.Day(),
		23, 59, 59, 0, // End of day
		loc,
	)

	// Convert back to UTC for storage
	return trialEndDate.UTC(), nil
}

// FormatTimeInUserTimezone formats a UTC time for display in user's timezone
func FormatTimeInUserTimezone(utcTime time.Time, timezone string, format string) (string, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return "", fmt.Errorf("invalid timezone %s: %w", timezone, err)
	}

	userTime := utcTime.In(loc)
	return userTime.Format(format), nil
}

// CalculateDaysRemaining calculates days remaining from user's current time perspective
func CalculateDaysRemaining(trialEndUTC time.Time, userTimezone string) int {
	loc, err := time.LoadLocation(userTimezone)
	if err != nil {
		// Fallback to UTC calculation
		return int(trialEndUTC.Sub(time.Now()).Hours() / 24)
	}

	now := time.Now().In(loc)
	trialEnd := trialEndUTC.In(loc)

	duration := trialEnd.Sub(now)
	days := int(duration.Hours() / 24)

	if days < 0 {
		return 0
	}
	return days
}