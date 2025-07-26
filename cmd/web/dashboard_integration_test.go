package main

import (
	"context"
	"strings"
	"talent-rodeo/internal"
	"testing"
	"time"
)

func TestDashboardSubscriptionExpiredAlert(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tests := []struct {
		name                        string
		subscriptionEnd             *time.Time
		lastEvaluationDate          *time.Time
		expectAlert                 bool
		expectDaysSinceEvaluation   bool
		expectedDaysValue           int
		expectUpgradeButton         bool
	}{
		{
			name:                "active subscription",
			subscriptionEnd:     timePtr(time.Now().Add(7 * 24 * time.Hour)), // Future date
			lastEvaluationDate:  timePtr(time.Now().Add(-1 * time.Hour)),     // Recent
			expectAlert:         false,
			expectUpgradeButton: false,
		},
		{
			name:                        "expired subscription with recent evaluation",
			subscriptionEnd:             timePtr(time.Now().Add(-1 * time.Hour)), // Past date
			lastEvaluationDate:          timePtr(time.Now().Add(-3 * 24 * time.Hour)), // 3 days ago
			expectAlert:                 true,
			expectDaysSinceEvaluation:   true,
			expectedDaysValue:           3,
			expectUpgradeButton:         true,
		},
		{
			name:                        "expired subscription with old evaluation",
			subscriptionEnd:             timePtr(time.Now().Add(-2 * 24 * time.Hour)), // 2 days ago
			lastEvaluationDate:          timePtr(time.Now().Add(-7 * 24 * time.Hour)), // 7 days ago
			expectAlert:                 true,
			expectDaysSinceEvaluation:   true,
			expectedDaysValue:           7,
			expectUpgradeButton:         true,
		},
		{
			name:                "expired subscription no evaluation date",
			subscriptionEnd:     timePtr(time.Now().Add(-1 * time.Hour)), // Past date
			lastEvaluationDate:  nil,                                     // No evaluation yet
			expectAlert:         true,
			expectUpgradeButton: true,
		},
		{
			name:                "no subscription end date (free trial user)",
			subscriptionEnd:     nil,
			lastEvaluationDate:  timePtr(time.Now().Add(-1 * time.Hour)),
			expectAlert:         false,
			expectUpgradeButton: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			ts := newTestServer(t, "dashboard-subscription")
			defer ts.Cleanup()

			// Create test user with specific subscription state
			ctx := context.Background()
			now := time.Now()
			
			user := &internal.User{
				ID:                   generateTestID("user"),
				CreatedAt:            now,
				Email:                "test@example.com",
				Name:                 "Test User",
				Status:               internal.UserStatusCallCompleted, // Ensure user can access dashboard
				StatusUpdatedAt:      now,
				Tier:                 internal.UserTierFree,
				SubscriptionEnd:      tt.subscriptionEnd,
				LastEvaluationDate:   tt.lastEvaluationDate,
				LastJobEvaluationRun: timePtr(now.Add(-1 * time.Hour)), // Some recent run
			}

			// Create user in database
			err := internal.CreateUser(ctx, ts.app.fs, user)
			if err != nil {
				t.Fatalf("Failed to create test user: %v", err)
			}

			// Clean up after test
			defer func() {
				_ = internal.DeleteUser(ctx, ts.app.fs, user.ID)
			}()

			// Make authenticated request to dashboard
			code, _, body := ts.authenticatedGetAsUser(t, "/dashboard", user)

			if code != 200 {
				t.Fatalf("Expected status 200, got %d", code)
			}

			// Check for subscription expired alert
			if tt.expectAlert {
				if !strings.Contains(body, "subscription-expired-alert") {
					t.Error("Expected subscription expired alert section to be present")
				}
				if !strings.Contains(body, "Your job search has been paused") {
					t.Error("Expected paused job search message")
				}
				if !strings.Contains(body, "How can we find your dream job if we're not looking?") {
					t.Error("Expected rhetorical question about job searching")
				}
			} else {
				if strings.Contains(body, "subscription-expired-alert") {
					t.Error("Did not expect subscription expired alert section")
				}
			}

			// Check for days since evaluation
			if tt.expectDaysSinceEvaluation && tt.expectedDaysValue > 0 {
				expectedDaysText := "since I last searched for jobs for you"
				if !strings.Contains(body, expectedDaysText) {
					t.Error("Expected days since evaluation message")
				}
			}

			// Check for upgrade button
			if tt.expectUpgradeButton {
				if !strings.Contains(body, "Resume Job Search - Upgrade Now") {
					t.Error("Expected upgrade button text")
				}
				if !strings.Contains(body, `href="/upgrade"`) {
					t.Error("Expected upgrade link")
				}
			} else {
				if strings.Contains(body, "Resume Job Search - Upgrade Now") {
					t.Error("Did not expect upgrade button")
				}
			}

			// Additional checks for specific scenarios
			if tt.subscriptionEnd != nil && tt.subscriptionEnd.Before(time.Now()) {
				// For expired subscriptions, ensure conversion message is present
				if !strings.Contains(body, "That perfect role could have been posted yesterday") {
					t.Error("Expected urgency message for expired subscription")
				}
			}
		})
	}
}

func TestDashboardSubscriptionExpiredStyling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test server
	ts := newTestServer(t, "dashboard-styling")
	defer ts.Cleanup()

	// Create test user with expired subscription
	ctx := context.Background()
	now := time.Now()
	
	user := &internal.User{
		ID:                   generateTestID("user"),
		CreatedAt:            now,
		Email:                "test@example.com",
		Name:                 "Test User",
		Status:               internal.UserStatusCallCompleted,
		StatusUpdatedAt:      now,
		Tier:                 internal.UserTierFree,
		SubscriptionEnd:      timePtr(now.Add(-24 * time.Hour)), // Expired yesterday
		LastEvaluationDate:   timePtr(now.Add(-3 * 24 * time.Hour)), // 3 days ago
		LastJobEvaluationRun: timePtr(now.Add(-1 * time.Hour)),
	}

	// Create user in database
	err := internal.CreateUser(ctx, ts.app.fs, user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Clean up after test
	defer func() {
		_ = internal.DeleteUser(ctx, ts.app.fs, user.ID)
	}()

	// Make authenticated request to dashboard
	code, _, body := ts.authenticatedGetAsUser(t, "/dashboard", user)

	if code != 200 {
		t.Fatalf("Expected status 200, got %d", code)
	}

	// Check for CSS classes that indicate proper styling
	expectedClasses := []string{
		"subscription-conversion",
		"conversion-content",
		"missed-opportunities",
		"urgency-message",
		"upgrade-button",
	}

	for _, class := range expectedClasses {
		if !strings.Contains(body, class) {
			t.Errorf("Expected CSS class '%s' to be present", class)
		}
	}

	// Check that the styles are actually included
	if !strings.Contains(body, "background: linear-gradient(135deg, #ff6b6b 0%, #ee5a24 100%)") {
		t.Error("Expected gradient background CSS to be present")
	}
}

// Helper function to create time pointers
func timePtr(t time.Time) *time.Time {
	return &t
}

// Helper function to generate unique test IDs
func generateTestID(prefix string) string {
	return prefix + "-" + time.Now().Format("20060102-150405") + "-" + randomString(6)
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}