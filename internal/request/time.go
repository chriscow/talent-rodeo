package request

import (
	"fmt"
	"time"
)

// TimeValidationResult holds the parsed and validated time information
type TimeValidationResult struct {
	ScheduledTime time.Time
	UserTime      time.Time
	Location      *time.Location
}

// ValidateScheduledTime validates a datetime string in RFC3339 format and timezone,
// ensuring the time is in the future and returning the parsed times
func ValidateScheduledTime(datetime, timezone string) (*TimeValidationResult, error) {
	if datetime == "" {
		return nil, fmt.Errorf("datetime is required")
	}
	if timezone == "" {
		return nil, fmt.Errorf("timezone is required")
	}

	// Parse datetime
	scheduledTime, err := time.Parse(time.RFC3339, datetime)
	if err != nil {
		return nil, fmt.Errorf("invalid datetime format (expected RFC3339): %w", err)
	}

	// Load timezone
	userLoc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone: %w", err)
	}

	// Convert to user's timezone
	userTime := scheduledTime.In(userLoc)

	// Check if time is in the past
	now := time.Now().In(userLoc).Truncate(time.Minute)
	userTimeMinute := userTime.Truncate(time.Minute)

	if userTimeMinute.Before(now) {
		return nil, fmt.Errorf("cannot schedule appointments in the past")
	}

	// Add buffer
	userTimeMinute = userTimeMinute.Add(time.Minute)

	return &TimeValidationResult{
		ScheduledTime: scheduledTime,
		UserTime:      userTimeMinute,
		Location:      userLoc,
	}, nil
}
