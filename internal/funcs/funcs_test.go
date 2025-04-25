package funcs

import (
	"testing"
	"time"
)

func TestTimeUntilDuration(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{
			name:     "past_time_1_hour",
			time:     now.Add(-1 * time.Hour),
			expected: "1 hour",
		},
		{
			name:     "past_time_2_hours",
			time:     now.Add(-2 * time.Hour),
			expected: "2 hours",
		},
		{
			name:     "past_time_1_day",
			time:     now.Add(-24 * time.Hour),
			expected: "1 day",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the exact formula used in assistant.go
			duration := -time.Until(tt.time)
			got := ApproxDuration(duration)
			if got != tt.expected {
				t.Errorf("TimeUntilDuration() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTimeCalculations(t *testing.T) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)

	// Let's see what time.Until gives us for a past time
	untilDuration := time.Until(oneHourAgo)
	t.Logf("time.Until(oneHourAgo) = %v (negative for past)", untilDuration)

	// Now let's see what happens when we negate it
	negatedDuration := -untilDuration
	t.Logf("-time.Until(oneHourAgo) = %v (positive for past)", negatedDuration)

	// Finally, let's see what ApproxDuration formats it as
	result := ApproxDuration(negatedDuration)
	t.Logf("ApproxDuration(-time.Until(oneHourAgo)) = %v", result)

	if result != "1 hour" {
		t.Errorf("Expected '1 hour', got '%v'", result)
	}

	// Let's also test with a future time to be thorough
	oneHourFuture := now.Add(1 * time.Hour)
	futureDuration := -time.Until(oneHourFuture)
	t.Logf("\nFor future time:")
	t.Logf("-time.Until(oneHourFuture) = %v (should be negative)", futureDuration)
	t.Logf("ApproxDuration of that = %v", ApproxDuration(futureDuration))
}
