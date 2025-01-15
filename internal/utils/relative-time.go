package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func TimeFromRelative(relativeTime string, from time.Time) (time.Time, error) {
	parts := strings.Fields(relativeTime)
	if len(parts) != 3 || parts[2] != "ago" {
		return time.Time{}, fmt.Errorf("invalid format: expected 'X unit ago'")
	}

	amount, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid number: %w", err)
	}

	var duration time.Duration
	switch parts[1] {
	case "week", "weeks":
		duration = time.Duration(amount) * 24 * 7 * time.Hour
	case "hour", "hours":
		duration = time.Duration(amount) * time.Hour
	default:
		return time.Time{}, fmt.Errorf("unsupported unit: %s", parts[1])
	}

	return from.Add(-duration), nil
}

func RelativeTime(t time.Time, now time.Time) string {
	diff := now.Sub(t)
	weeks := int64(diff.Hours() / (24 * 7))
	days := int64(diff.Hours() / 24)
	hours := int64(diff.Hours())

	if weeks > 0 {
		if weeks == 1 {
			return "1 week ago"
		}

		if weeks >= 4 {
			months := weeks / 4
			if months == 1 {
				return "1 month ago"
			}
			return fmt.Sprintf("%d months ago", months)
		}

		return fmt.Sprintf("%d weeks ago", weeks)
	}

	if days > 0 {
		if days == 1 {
			return "1 day ago"
		}

		return fmt.Sprintf("%d days ago", days)
	}

	if hours == 1 {
		return "1 hour ago"
	}

	return fmt.Sprintf("%d hours ago", hours)
}
