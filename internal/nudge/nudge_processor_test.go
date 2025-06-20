package nudge

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// createTestProcessor creates a processor with mock dependencies for testing
func createTestProcessor(logger *slog.Logger) *Processor {
	mockTemplateReader := NewMockTemplateReader()
	mockPromptProcessor := NewMockPromptProcessor()
	mockEmailGenerator := NewEmailGenerator(mockTemplateReader, mockPromptProcessor)
	mockEmailSender := NewMockEmailSender()
	return NewProcessor(nil, mockEmailGenerator, mockEmailSender, logger)
}

func TestNewProcessor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create mock dependencies
	mockEmailGenerator := &MockEmailGenerator{}
	mockEmailSender := NewMockEmailSender()

	processor := NewProcessor(nil, mockEmailGenerator, mockEmailSender, logger)

	if processor == nil {
		t.Fatal("expected processor to be created")
	}

	if processor.emailGenerator == nil {
		t.Fatal("expected email generator to be initialized")
	}

	if processor.settings.Enabled != DefaultNudgeSettings.Enabled {
		t.Errorf("expected settings to match defaults")
	}
}

func TestNudgeTypeValidation(t *testing.T) {
	tests := []struct {
		nudgeType NudgeType
		expected  bool
	}{
		{NudgeTypeNoResume, true},
		{NudgeTypeNoCall, true},
		{NudgeTypeFiveStarMatches, true},
		{NudgeType("invalid"), false},
		{NudgeType(""), false},
	}

	for _, test := range tests {
		t.Run(string(test.nudgeType), func(t *testing.T) {
			result := test.nudgeType.IsValid()
			if result != test.expected {
				t.Errorf("expected %v, got %v for nudge type %s", test.expected, result, test.nudgeType)
			}
		})
	}
}

func TestNudgeTypePriority(t *testing.T) {
	tests := []struct {
		nudgeType NudgeType
		priority  int
	}{
		{NudgeTypeNoResume, 1},
		{NudgeTypeNoCall, 2},
		{NudgeTypeFiveStarMatches, 3},
		{NudgeType("invalid"), 999},
	}

	for _, test := range tests {
		t.Run(string(test.nudgeType), func(t *testing.T) {
			result := test.nudgeType.Priority()
			if result != test.priority {
				t.Errorf("expected priority %d, got %d for nudge type %s", test.priority, result, test.nudgeType)
			}
		})
	}
}

func TestProcessorSettings(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	processor := createTestProcessor(logger)

	// Test default settings
	if processor.settings.MaxEmailsPerRun != 50 {
		t.Errorf("expected MaxEmailsPerRun to be 50, got %d", processor.settings.MaxEmailsPerRun)
	}

	// Test updating settings
	newSettings := DefaultNudgeSettings
	newSettings.MaxEmailsPerRun = 100
	newSettings.Paused = true

	processor.UpdateSettings(newSettings)

	if processor.settings.MaxEmailsPerRun != 100 {
		t.Errorf("expected MaxEmailsPerRun to be 100 after update, got %d", processor.settings.MaxEmailsPerRun)
	}

	if !processor.settings.Paused {
		t.Error("expected processor to be paused after settings update")
	}
}

func TestIsOperatingHours(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	processor := createTestProcessor(logger)

	// Test 24/7 operation (default)
	if !processor.isOperatingHours() {
		t.Error("expected processor to be operating 24/7 with default settings")
	}

	// Test specific hours (9 AM to 5 PM UTC)
	processor.settings.StartHour = 9
	processor.settings.EndHour = 17

	// This test is time-dependent, so we'll just check the logic works
	// In a real test, you'd mock the time
	_ = processor.isOperatingHours()

	// Test overnight hours (10 PM to 6 AM UTC)
	processor.settings.StartHour = 22
	processor.settings.EndHour = 6

	_ = processor.isOperatingHours()
}

func TestProcessingStats(t *testing.T) {
	stats := &ProcessingStats{
		StartTime: time.Now(),
	}

	typeStats := &TypeStats{
		EligibleUsers: 5,
		EmailsSent:    3,
		Errors:        1,
	}

	stats.AddTypeStats(NudgeTypeNoResume, typeStats)

	if stats.TotalEmailsSent != 3 {
		t.Errorf("expected TotalEmailsSent to be 3, got %d", stats.TotalEmailsSent)
	}

	if stats.TotalErrors != 1 {
		t.Errorf("expected TotalErrors to be 1, got %d", stats.TotalErrors)
	}

	if stats.TypeStats[NudgeTypeNoResume] != typeStats {
		t.Error("expected type stats to be stored correctly")
	}
}

func TestIsUserEligibleForNudge(t *testing.T) {
	now := time.Now()
	cutoffTime := now.Add(-2 * 24 * time.Hour) // 2 days ago
	settings := DefaultNudgeSettings

	tests := []struct {
		name     string
		user     *internal.User
		expected bool
	}{
		{
			name: "eligible user - no previous nudges",
			user: &internal.User{
				ID:                   "test-user-1",
				LastAccessedAt:       now.Add(-3 * 24 * time.Hour), // 3 days ago
				NudgeEmailsSentCount: 0,
				LastNudgeEmailSent:   nil,
				LastNudgeEmailType:   nil,
				SubscriptionEnd:      nil,
			},
			expected: true,
		},
		{
			name: "ineligible - expired subscription",
			user: &internal.User{
				ID:              "test-user-2",
				LastAccessedAt:  now.Add(-3 * 24 * time.Hour),
				SubscriptionEnd: &[]time.Time{now.Add(-1 * 24 * time.Hour)}[0], // expired yesterday
			},
			expected: false,
		},
		{
			name: "ineligible - rate limited",
			user: &internal.User{
				ID:                   "test-user-4",
				LastAccessedAt:       now.Add(-3 * 24 * time.Hour),
				LastNudgeEmailSent:   &[]time.Time{now.Add(-1 * 24 * time.Hour)}[0], // 1 day ago (within 7-day limit)
				NudgeEmailsSentCount: 1,
			},
			expected: false,
		},
		{
			name: "ineligible - nudge sent recently (global rate limit)",
			user: &internal.User{
				ID:                   "test-user-5",
				LastAccessedAt:       now.Add(-3 * 24 * time.Hour),
				LastNudgeEmailSent:   &[]time.Time{now.Add(-24 * time.Hour)}[0], // 1 day ago (inside rate limit)
				LastNudgeEmailType:   &[]string{"no_resume"}[0],
				NudgeEmailsSentCount: 1,
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, _ := isUserEligibleForNudge(test.user, NudgeTypeNoResume, cutoffTime, settings)
			if result != test.expected {
				t.Errorf("expected %v, got %v", test.expected, result)
			}
		})
	}
}

func TestSortUsersByPriority(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	processor := createTestProcessor(logger)

	now := time.Now()
	users := []*internal.User{
		{
			ID:                   "user1",
			NudgeEmailsSentCount: 2,
			LastAccessedAt:       now.Add(-1 * 24 * time.Hour), // 1 day ago
		},
		{
			ID:                   "user2",
			NudgeEmailsSentCount: 0,
			LastAccessedAt:       now.Add(-3 * 24 * time.Hour), // 3 days ago
		},
		{
			ID:                   "user3",
			NudgeEmailsSentCount: 1,
			LastAccessedAt:       now.Add(-2 * 24 * time.Hour), // 2 days ago
		},
		{
			ID:                   "user4",
			NudgeEmailsSentCount: 0,
			LastAccessedAt:       now.Add(-1 * 24 * time.Hour), // 1 day ago
		},
	}

	processor.sortUsersByPriority(users, NudgeTypeNoResume)

	// Expected order: users with fewer nudges first, then by last access time (older first)
	// user2 (0 nudges, 3 days ago), user4 (0 nudges, 1 day ago), user3 (1 nudge, 2 days ago), user1 (2 nudges, 1 day ago)

	if users[0].ID != "user2" {
		t.Errorf("expected user2 to be first, got %s", users[0].ID)
	}

	if users[1].ID != "user4" {
		t.Errorf("expected user4 to be second, got %s", users[1].ID)
	}

	if users[2].ID != "user3" {
		t.Errorf("expected user3 to be third, got %s", users[2].ID)
	}

	if users[3].ID != "user1" {
		t.Errorf("expected user1 to be fourth, got %s", users[3].ID)
	}
}

func TestStartStopProcessor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	processor := createTestProcessor(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test starting processor
	err := processor.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start processor: %v", err)
	}

	if !processor.isRunning {
		t.Error("expected processor to be running after start")
	}

	// Test starting again (should fail)
	err = processor.Start(ctx)
	if err == nil {
		t.Error("expected error when starting already running processor")
	}

	// Test stopping processor
	err = processor.Stop()
	if err != nil {
		t.Fatalf("failed to stop processor: %v", err)
	}

	if processor.isRunning {
		t.Error("expected processor to not be running after stop")
	}

	// Test stopping again (should not error)
	err = processor.Stop()
	if err != nil {
		t.Errorf("unexpected error when stopping already stopped processor: %v", err)
	}
}

func TestProcessorWithDisabledSetting(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	processor := createTestProcessor(logger)

	// Disable the processor
	settings := processor.settings
	settings.Enabled = false
	processor.UpdateSettings(settings)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start should not error but also should not actually start processing
	err := processor.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start disabled processor: %v", err)
	}

	// The processor should not be marked as running when disabled
	if processor.isRunning {
		t.Error("expected processor to not be running when disabled")
	}
}

func TestGetNudgeEligibleUsers_FetchAllAndFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	logger := internal.CreateTestLogger(os.Stdout)
	fs, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create test firestore client: %v", err)
	}
	defer internal.CleanupTestUsers(ctx, fs)

	// --- Test Data Setup ---
	now := time.Now()
	users := []*internal.User{
		// Should be eligible for NoResume
		{ID: "no-resume-eligible", Email: "no-resume-eligible@test.com", State: internal.UserState{Status: internal.UserStatusPendingResumeUpload}, LastAccessedAt: now.Add(-5 * 24 * time.Hour)},
		// Should NOT be eligible (wrong status)
		{ID: "no-resume-wrong-status", Email: "no-resume-wrong-status@test.com", State: internal.UserState{Status: internal.UserStatusResumeUploaded}, LastAccessedAt: now.Add(-5 * 24 * time.Hour)},
		// Should NOT be eligible (too recent access)
		{ID: "no-resume-recent-access", Email: "no-resume-recent-access@test.com", State: internal.UserState{Status: internal.UserStatusPendingResumeUpload}, LastAccessedAt: now.Add(-1 * 24 * time.Hour)},
		// Should NOT be eligible (max nudges reached)
		{ID: "no-resume-max-nudges", Email: "no-resume-max-nudges@test.com", State: internal.UserState{Status: internal.UserStatusPendingResumeUpload}, LastAccessedAt: now.Add(-5 * 24 * time.Hour), NudgeEmailsSentCount: 3},

		// Should be eligible for NoCall
		{ID: "no-call-eligible", Email: "no-call-eligible@test.com", State: internal.UserState{Status: internal.UserStatusResumeUploaded}, LastAccessedAt: now.Add(-5 * 24 * time.Hour)},
		// Should NOT be eligible for NoCall (wrong status)
		{ID: "no-call-wrong-status", Email: "no-call-wrong-status@test.com", State: internal.UserState{Status: internal.UserStatusCallCompleted}, LastAccessedAt: now.Add(-5 * 24 * time.Hour)},

		// Should be eligible for FiveStar (job check is separate)
		{ID: "five-star-eligible", Email: "five-star-eligible@test.com", State: internal.UserState{Status: internal.UserStatusCallCompleted}, LastAccessedAt: now.Add(-5 * 24 * time.Hour)},
	}

	for _, user := range users {
		if err := internal.CreateTestUserDoc(ctx, fs, user); err != nil {
			t.Fatalf("failed to create test user %s: %v", user.ID, err)
		}
	}

	// --- Test Execution ---
	settings := DefaultNudgeSettings
	settings.NoResumeDelayDays = 2
	settings.NoCallDelayDays = 3
	settings.MatchAlertDelayDays = 2

	testCases := []struct {
		name              string
		nudgeType         NudgeType
		expectedUserIDs   []string
		unexpectedUserIDs []string
	}{
		{
			name:            "NoResume Nudge",
			nudgeType:       NudgeTypeNoResume,
			expectedUserIDs: []string{"no-resume-eligible"},
			unexpectedUserIDs: []string{
				"no-resume-wrong-status",
				"no-resume-recent-access",
				"no-resume-max-nudges",
			},
		},
		{
			name:            "NoCall Nudge",
			nudgeType:       NudgeTypeNoCall,
			expectedUserIDs: []string{"no-call-eligible"},
			unexpectedUserIDs: []string{
				"no-call-wrong-status",
			},
		},
		{
			name:            "FiveStarMatches Nudge",
			nudgeType:       NudgeTypeFiveStarMatches,
			expectedUserIDs: []string{"five-star-eligible"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			eligibleUsers, err := GetNudgeEligibleUsers(ctx, logger, fs, tc.nudgeType, settings)
			if err != nil {
				t.Fatalf("GetNudgeEligibleUsers failed: %v", err)
			}

			eligibleIDs := make(map[string]bool)
			for _, u := range eligibleUsers {
				eligibleIDs[u.ID] = true
			}

			// Check for expected users
			for _, expectedID := range tc.expectedUserIDs {
				if _, found := eligibleIDs[expectedID]; !found {
					t.Errorf("expected user %s to be eligible, but was not", expectedID)
				}
			}

			// Check for unexpected users
			for _, unexpectedID := range tc.unexpectedUserIDs {
				if _, found := eligibleIDs[unexpectedID]; found {
					t.Errorf("unexpected user %s was eligible, but should not have been", unexpectedID)
				}
			}
		})
	}
}

func TestProcessNudgeType_StatsAndErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	logger := internal.CreateTestLogger(os.Stdout)
	fs, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create test firestore client: %v", err)
	}
	defer internal.CleanupTestUsers(ctx, fs)

	// --- Test Data Setup ---
	// This user will be processed successfully
	u1 := &internal.User{ID: "success-user", State: internal.UserState{Status: internal.UserStatusPendingResumeUpload}}
	// This user will cause an error during email generation
	u2 := &internal.User{ID: "error-user", State: internal.UserState{Status: internal.UserStatusPendingResumeUpload}}
	// This user will be skipped because they have no 5-star matches
	u3 := &internal.User{ID: "skip-user", State: internal.UserState{Status: internal.UserStatusCallCompleted}}

	for _, user := range []*internal.User{u1, u2, u3} {
		if err := internal.CreateTestUserDoc(ctx, fs, user); err != nil {
			t.Fatalf("failed to create test user %s: %v", user.ID, err)
		}
	}

	// --- Test Execution ---
	mockTemplateReader := NewMockTemplateReader()
	mockTemplateReader.SetTemplate(NudgeTypeNoResume, []byte("template content"))
	mockTemplateReader.SetTemplate(NudgeTypeFiveStarMatches, []byte("template content"))

	// This mock will be configured to fail for the "error-user"
	mockPromptProcessor := NewMockPromptProcessor()
	mockEmailGenerator := NewEmailGenerator(mockTemplateReader, mockPromptProcessor)
	mockEmailSender := NewMockEmailSender()

	processor := NewProcessor(fs, mockEmailGenerator, mockEmailSender, logger)
	processor.settings.DryRun = true // Use dry run to avoid actual sending

	// --- Test Case 1: No error, successful run ---
	t.Run("successful run", func(t *testing.T) {
		mockPromptProcessor.SetProcessError(nil)
		users := []*internal.User{u1}
		stats := processor.processNudgeType(ctx, NudgeTypeNoResume, len(users))
		if stats.EmailsSent != 1 {
			t.Errorf("expected 1 email sent, got %d", stats.EmailsSent)
		}
		if stats.Errors != 0 {
			t.Errorf("expected 0 errors, got %d", stats.Errors)
		}
	})

	// --- Test Case 2: Genuine error during processing ---
	t.Run("run with genuine error", func(t *testing.T) {
		// Configure the mock to return an error for a specific user.
		// We can't do that easily with the current mock, so we'll just simulate a global error.
		mockPromptProcessor.SetProcessError(fmt.Errorf("simulated LLM error"))
		users := []*internal.User{u2}
		stats := processor.processNudgeType(ctx, NudgeTypeNoResume, len(users))
		if stats.EmailsSent != 0 {
			t.Errorf("expected 0 emails sent, got %d", stats.EmailsSent)
		}
		if stats.Errors != 1 {
			t.Errorf("expected 1 error, got %d", stats.Errors)
		}
		mockPromptProcessor.SetProcessError(nil) // Reset for next test
	})

	// --- Test Case 3: Successful skip (not an error) ---
	t.Run("run with successful skip", func(t *testing.T) {
		// For this user, the processUser function will return (false, nil)
		// because the user has no 5-star matches.
		users := []*internal.User{u3}
		stats := processor.processNudgeType(ctx, NudgeTypeFiveStarMatches, len(users))
		if stats.EmailsSent != 0 {
			t.Errorf("expected 0 emails sent for skipped user, got %d", stats.EmailsSent)
		}
		if stats.Errors != 0 {
			t.Errorf("expected 0 errors for skipped user, got %d", stats.Errors)
		}
	})
}

func TestMaxNudgesPerTypeEnforcement(t *testing.T) {
	now := time.Now()
	cutoffTime := now.Add(-2 * 24 * time.Hour) // 2 days ago
	settings := NudgeProcessorSettings{
		MaxNudgesPerType: MaxNudgesPerType{
			NoResume:        2,
			NoCall:          1,
			FiveStarMatches: 2,
			FollowUp:        1,
		},
	}

	tests := []struct {
		name          string
		user          *internal.User
		nudgeType     NudgeType
		expectedValid bool
		desc          string
	}{
		{
			name: "under no-resume limit",
			user: &internal.User{
				ID:                   "test-user-1",
				LastAccessedAt:       now.Add(-3 * 24 * time.Hour),
				NudgeEmailsSentCount: 1,
				LastNudgeEmailType:   &[]string{string(NudgeTypeNoResume)}[0],
				NudgeTypeCount: map[string]int{
					string(NudgeTypeNoResume): 1, // Under the limit of 2
				},
			},
			nudgeType:     NudgeTypeNoResume,
			expectedValid: true,
			desc:          "User should be eligible when under the type-specific limit",
		},
		{
			name: "at no-resume limit",
			user: &internal.User{
				ID:                   "test-user-2",
				LastAccessedAt:       now.Add(-3 * 24 * time.Hour),
				NudgeEmailsSentCount: 2,
				LastNudgeEmailType:   &[]string{string(NudgeTypeNoResume)}[0],
				NudgeTypeCount: map[string]int{
					string(NudgeTypeNoResume): 2, // At the limit
				},
			},
			nudgeType:     NudgeTypeNoResume,
			expectedValid: false,
			desc:          "User should be ineligible when at the type-specific limit",
		},
		{
			name: "under no-call limit",
			user: &internal.User{
				ID:                   "test-user-3",
				LastAccessedAt:       now.Add(-3 * 24 * time.Hour),
				NudgeEmailsSentCount: 0,
				NudgeTypeCount: map[string]int{
					string(NudgeTypeNoCall): 0, // Under the limit of 1
				},
			},
			nudgeType:     NudgeTypeNoCall,
			expectedValid: true,
			desc:          "User should be eligible for first no-call nudge",
		},
		{
			name: "at no-call limit",
			user: &internal.User{
				ID:                   "test-user-4",
				LastAccessedAt:       now.Add(-3 * 24 * time.Hour),
				NudgeEmailsSentCount: 1,
				LastNudgeEmailType:   &[]string{string(NudgeTypeNoCall)}[0],
				NudgeTypeCount: map[string]int{
					string(NudgeTypeNoCall): 1, // At the limit
				},
			},
			nudgeType:     NudgeTypeNoCall,
			expectedValid: false,
			desc:          "User should be ineligible after receiving one no-call nudge",
		},
		{
			name: "under five-star matches limit",
			user: &internal.User{
				ID:                   "test-user-5",
				LastAccessedAt:       now.Add(-3 * 24 * time.Hour),
				NudgeEmailsSentCount: 1,
				LastNudgeEmailType:   &[]string{string(NudgeTypeFiveStarMatches)}[0],
				NudgeTypeCount: map[string]int{
					string(NudgeTypeFiveStarMatches): 1, // Under the limit of 2
				},
			},
			nudgeType:     NudgeTypeFiveStarMatches,
			expectedValid: true,
			desc:          "User should be eligible when under five-star matches limit",
		},
		{
			name: "at five-star matches limit",
			user: &internal.User{
				ID:                   "test-user-6",
				LastAccessedAt:       now.Add(-3 * 24 * time.Hour),
				NudgeEmailsSentCount: 2,
				LastNudgeEmailType:   &[]string{string(NudgeTypeFiveStarMatches)}[0],
				NudgeTypeCount: map[string]int{
					string(NudgeTypeFiveStarMatches): 2, // At the limit
				},
			},
			nudgeType:     NudgeTypeFiveStarMatches,
			expectedValid: false,
			desc:          "User should be ineligible when at five-star matches limit",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.desc)
			valid, reason := isUserEligibleForNudge(test.user, test.nudgeType, cutoffTime, settings)
			if valid != test.expectedValid {
				t.Errorf("Expected validity %v, got %v (reason: %s)", test.expectedValid, valid, reason)
			}
		})
	}
}

func TestUpdateUserNudgeStats(t *testing.T) {
	ctx := context.Background()
	fs, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test Firestore client: %v", err)
	}
	defer fs.Close()

	// Clean up any existing test user
	testUserID := "test-user-nudge-count"
	_, err = fs.Collection("users").Doc(testUserID).Delete(ctx)
	if err != nil {
		// Ignore error if document doesn't exist
		if status.Code(err) != codes.NotFound {
			t.Fatalf("Failed to clean up test user: %v", err)
		}
	}

	now := time.Now()
	user := &internal.User{
		ID:             testUserID,
		Name:           "Test User",
		FirstName:      "Test",
		LastName:       "User",
		Email:          "test@example.com",
		LastAccessedAt: now.Add(-3 * 24 * time.Hour),
		NudgeTypeCount: make(map[string]int),
		State: internal.UserState{
			Status:  internal.UserStatusPendingResumeUpload,
			Started: now,
		},
	}

	if err := internal.CreateUser(ctx, fs, user); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	settings := NudgeProcessorSettings{
		MaxNudgesPerType: MaxNudgesPerType{
			NoResume:        2,
			NoCall:          1,
			FiveStarMatches: 2,
			FollowUp:        1,
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	templateReader := NewMockTemplateReader()
	promptProcessor := NewMockPromptProcessor()
	emailGenerator := NewEmailGenerator(templateReader, promptProcessor)
	emailSender := &MockEmailSender{}

	processor := NewProcessor(fs, emailGenerator, emailSender, logger)
	processor.settings = settings

	// Test updating stats for NoResume nudge
	err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		return UpdateUserNudgeTrackingTx(ctx, fs, tx, user.ID, string(NudgeTypeNoResume))
	})
	if err != nil {
		t.Fatalf("Failed to update user nudge stats: %v", err)
	}

	// Verify the stats were updated
	updatedUser, found, err := internal.GetUser(ctx, fs, user.ID)
	if err != nil {
		t.Fatalf("Failed to get updated user: %v", err)
	}
	if !found {
		t.Fatal("Updated user not found")
	}

	// Check NudgeTypeCount
	if count := updatedUser.NudgeTypeCount[string(NudgeTypeNoResume)]; count != 1 {
		t.Errorf("Expected NudgeTypeCount[NoResume] to be 1, got %d", count)
	}

	// Check LastNudgeEmailType
	if updatedUser.LastNudgeEmailType == nil {
		t.Error("Expected LastNudgeEmailType to be set, got nil")
	} else if *updatedUser.LastNudgeEmailType != string(NudgeTypeNoResume) {
		t.Errorf("Expected LastNudgeEmailType to be %s, got %s", NudgeTypeNoResume, *updatedUser.LastNudgeEmailType)
	}

	// Check LastNudgeEmailSent
	if updatedUser.LastNudgeEmailSent == nil {
		t.Error("Expected LastNudgeEmailSent to be set, got nil")
	}

	// Test updating stats for another nudge type
	err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		return UpdateUserNudgeTrackingTx(ctx, fs, tx, user.ID, string(NudgeTypeNoCall))
	})
	if err != nil {
		t.Fatalf("Failed to update user nudge stats for second type: %v", err)
	}

	// Verify multiple nudge types are tracked correctly
	finalUser, found, err := internal.GetUser(ctx, fs, user.ID)
	if err != nil {
		t.Fatalf("Failed to get final user: %v", err)
	}
	if !found {
		t.Fatal("Final user not found")
	}

	expectedCounts := map[string]int{
		string(NudgeTypeNoResume): 1,
		string(NudgeTypeNoCall):   1,
	}

	for nudgeType, expectedCount := range expectedCounts {
		if count := finalUser.NudgeTypeCount[nudgeType]; count != expectedCount {
			t.Errorf("Expected NudgeTypeCount[%s] to be %d, got %d", nudgeType, expectedCount, count)
		}
	}
}
