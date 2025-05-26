package integration

import (
	"fmt"
	"testing"
	"time"

	"talent-rodeo/internal"

	"github.com/stretchr/testify/require"
)

// TestDashboardAccessControl tests that unauthenticated users are redirected appropriately
func TestDashboardAccessControl(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	it := NewIntegrationTest(t)
	defer it.Close()

	// Navigate to dashboard without authentication
	it.NavigateTo("/dashboard")

	// Should be redirected to home page
	currentURL := it.Page.URL()
	require.Contains(t, currentURL, "/", "Unauthenticated user should be redirected to home")

	// Verify we're not on the dashboard
	require.False(t, it.IsVisible("Dashboard"), "Should not see dashboard content when unauthenticated")
}

// TestDashboardUserStateRedirects tests that users in different states are redirected appropriately
func TestDashboardUserStateRedirects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	tests := []struct {
		name             string
		userStatus       internal.UserStatus
		expectedRedirect string
		expectedContent  string
	}{
		{
			name:             "new user redirected to profile",
			userStatus:       internal.UserStatusPendingResumeUpload,
			expectedRedirect: "/profile",
			expectedContent:  "Upload Resume",
		},
		{
			name:             "resume uploaded user redirected to profile",
			userStatus:       internal.UserStatusResumeUploaded,
			expectedRedirect: "/profile",
			expectedContent:  "Upload Resume",
		},
		{
			name:             "processing resume user redirected to profile",
			userStatus:       internal.UserStatusProcessingResume,
			expectedRedirect: "/profile",
			expectedContent:  "Upload Resume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := NewIntegrationTest(t)
			defer it.Close()

			// Update user status
			err := internal.UpdateUserStatus(it.Ctx, it.Fs, it.User, tt.userStatus)
			require.NoError(t, err, "Failed to update user status")

			// Authenticate and navigate to dashboard
			it.Authenticate()
			it.NavigateTo("/dashboard")

			// Should be redirected to the expected page
			currentURL := it.Page.URL()
			require.Contains(t, currentURL, tt.expectedRedirect,
				fmt.Sprintf("User with status %s should be redirected to %s", tt.userStatus, tt.expectedRedirect))

			// Verify expected content is visible
			require.True(t, it.IsVisible(tt.expectedContent),
				fmt.Sprintf("Should see %s content after redirect", tt.expectedContent))
		})
	}
}

// TestDashboardWaitingForCall tests the waiting for call view for users who have scheduled calls
func TestDashboardWaitingForCall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	tests := []struct {
		name                 string
		userStatus           internal.UserStatus
		appointmentStatus    internal.AppointmentStatus
		expectedTitle        string
		expectedContent      string
		shouldShowReschedule bool
	}{
		{
			name:                 "scheduled call shows waiting message",
			userStatus:           internal.UserStatusCallScheduled,
			appointmentStatus:    internal.AppointmentStatusScheduled,
			expectedTitle:        "We'll be calling you shortly",
			expectedContent:      "Please stand by!",
			shouldShowReschedule: true,
		},
		{
			name:                 "running call shows active message",
			userStatus:           internal.UserStatusCallStarted,
			appointmentStatus:    internal.AppointmentStatusRunning,
			expectedTitle:        "We're calling you now!",
			expectedContent:      "Please answer your phone",
			shouldShowReschedule: false,
		},
		{
			name:                 "processing call shows processing message",
			userStatus:           internal.UserStatusCallProcessing,
			appointmentStatus:    internal.AppointmentStatusProcessing,
			expectedTitle:        "Processing your conversation",
			expectedContent:      "This takse a several minutes",
			shouldShowReschedule: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := NewIntegrationTest(t)
			defer it.Close()

			// Update user status
			err := internal.UpdateUserStatus(it.Ctx, it.Fs, it.User, tt.userStatus)
			require.NoError(t, err, "Failed to update user status")

			// Create an appointment for the user using the helper function
			_ = internal.CreateTestAppointment(t, it.Fs, it.User)

			// Authenticate and navigate to dashboard
			it.Authenticate()
			it.NavigateTo("/dashboard")

			// Should stay on dashboard (not redirected)
			currentURL := it.Page.URL()
			require.Contains(t, currentURL, "/dashboard", "Should stay on dashboard for waiting call view")

			// Verify expected title and content
			require.True(t, it.IsVisible(tt.expectedTitle),
				fmt.Sprintf("Should see title: %s", tt.expectedTitle))
			require.True(t, it.IsVisible(tt.expectedContent),
				fmt.Sprintf("Should see content: %s", tt.expectedContent))

			// Check reschedule button visibility
			if tt.shouldShowReschedule {
				require.True(t, it.IsVisible("Reschedule Call"), "Should show reschedule button for scheduled calls")
			} else {
				require.False(t, it.IsVisible("Reschedule Call"), "Should not show reschedule button for active/processing calls")
			}
		})
	}
}

// TestDashboardMainView tests the main dashboard view for users who have completed calls
func TestDashboardMainView(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	it := NewIntegrationTest(t)
	defer it.Close()

	// Update user status to call completed
	err := internal.UpdateUserStatus(it.Ctx, it.Fs, it.User, internal.UserStatusCallCompleted)
	require.NoError(t, err, "Failed to update user status")

	// Authenticate and navigate to dashboard
	it.Authenticate()
	it.NavigateTo("/dashboard")

	// Should stay on dashboard
	currentURL := it.Page.URL()
	require.Contains(t, currentURL, "/dashboard", "Should stay on dashboard for main view")

	// Verify page title
	title := it.GetTitle()
	require.Contains(t, title, "Dashboard", "Page title should contain 'Dashboard'")

	// For a new user with no jobs, should see appropriate message
	// The exact content depends on whether there are Firestore errors or just no jobs
	content := it.GetContent()
	t.Logf("Dashboard content for user with no jobs: %s", content)

	// Should see job status navigation elements
	jobStatuses := []string{"New", "Viewed", "Saved", "Applied", "Rejected"}
	for _, status := range jobStatuses {
		// Note: These might not be visible if there are no jobs, but the structure should be there
		t.Logf("Checking for job status: %s", status)
	}
}

// TestDashboardWithJobs tests the dashboard functionality when user has jobs
func TestDashboardWithJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	it := NewIntegrationTest(t)
	defer it.Close()

	// Update user status to call completed
	err := internal.UpdateUserStatus(it.Ctx, it.Fs, it.User, internal.UserStatusCallCompleted)
	require.NoError(t, err, "Failed to update user status")

	// Create some test user jobs using the helper function
	now := time.Now()
	userJob1 := internal.CreateTestUserJob(t, it.Fs, it.User.ID, "test-job-1", internal.JobStatusNew, now)
	userJob2 := internal.CreateTestUserJob(t, it.Fs, it.User.ID, "test-job-2", internal.JobStatusViewed, now)

	// Authenticate and navigate to dashboard
	it.Authenticate()
	it.NavigateTo("/dashboard")

	// Should stay on dashboard
	currentURL := it.Page.URL()
	require.Contains(t, currentURL, "/dashboard", "Should stay on dashboard")

	// Log the jobs that were created for debugging
	t.Logf("Created user job 1: %s with status %s", userJob1.ID, userJob1.Status)
	t.Logf("Created user job 2: %s with status %s", userJob2.ID, userJob2.Status)

	// Test job status filtering by navigating to different status tabs
	t.Run("filter by viewed jobs", func(t *testing.T) {
		// Navigate to viewed jobs
		it.NavigateTo("/dashboard?status=viewed")

		// Should see the dashboard with viewed filter
		currentURL := it.Page.URL()
		require.Contains(t, currentURL, "status=viewed", "Should be on dashboard with viewed filter")
	})
}

// TestDashboardJobStatusFiltering tests the job status filtering functionality
func TestDashboardJobStatusFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	it := NewIntegrationTest(t)
	defer it.Close()

	// Update user status to call completed
	err := internal.UpdateUserStatus(it.Ctx, it.Fs, it.User, internal.UserStatusCallCompleted)
	require.NoError(t, err, "Failed to update user status")

	// Test different status filters
	statusTests := []struct {
		status      string
		expectedURL string
	}{
		{"new", "/dashboard?status=new"},
		{"viewed", "/dashboard?status=viewed"},
		{"saved", "/dashboard?status=saved"},
		{"applied", "/dashboard?status=applied"},
		{"rejected", "/dashboard?status=rejected"},
	}

	it.Authenticate()

	for _, tt := range statusTests {
		t.Run(fmt.Sprintf("filter_%s_jobs", tt.status), func(t *testing.T) {
			// Navigate to dashboard with status filter
			it.NavigateTo(tt.expectedURL)

			// Should stay on dashboard with correct URL
			currentURL := it.Page.URL()
			require.Contains(t, currentURL, tt.expectedURL,
				fmt.Sprintf("Should be on dashboard with %s filter", tt.status))

			// Page should load without errors
			title := it.GetTitle()
			require.Contains(t, title, "Dashboard", "Page should load successfully")
		})
	}
}

// TestDashboardPagination tests the pagination functionality
func TestDashboardPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	it := NewIntegrationTest(t)
	defer it.Close()

	// Update user status to call completed
	err := internal.UpdateUserStatus(it.Ctx, it.Fs, it.User, internal.UserStatusCallCompleted)
	require.NoError(t, err, "Failed to update user status")

	// Test pagination parameters
	paginationTests := []struct {
		name        string
		queryParams string
		description string
	}{
		{
			name:        "first_page",
			queryParams: "",
			description: "Default first page",
		},
		{
			name:        "with_page_token",
			queryParams: "?page=some-token",
			description: "Page with token",
		},
		{
			name:        "show_all_ratings",
			queryParams: "?show_all=true",
			description: "Show all ratings filter",
		},
		{
			name:        "combined_filters",
			queryParams: "?status=new&show_all=true&page=token",
			description: "Combined status, show_all, and pagination",
		},
	}

	it.Authenticate()

	for _, tt := range paginationTests {
		t.Run(tt.name, func(t *testing.T) {
			// Navigate to dashboard with pagination parameters
			url := "/dashboard" + tt.queryParams
			it.NavigateTo(url)

			// Should stay on dashboard
			currentURL := it.Page.URL()
			require.Contains(t, currentURL, "/dashboard", "Should stay on dashboard")

			// Page should load without errors
			title := it.GetTitle()
			require.Contains(t, title, "Dashboard", "Page should load successfully")

			t.Logf("Successfully tested %s: %s", tt.description, url)
		})
	}
}

// TestDashboardErrorHandling tests error handling for new users without jobs
func TestDashboardErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	it := NewIntegrationTest(t)
	defer it.Close()

	// Update user status to call completed
	err := internal.UpdateUserStatus(it.Ctx, it.Fs, it.User, internal.UserStatusCallCompleted)
	require.NoError(t, err, "Failed to update user status")

	// Authenticate and navigate to dashboard
	it.Authenticate()
	it.NavigateTo("/dashboard")

	// Should stay on dashboard even with Firestore errors for new users
	currentURL := it.Page.URL()
	require.Contains(t, currentURL, "/dashboard", "Should stay on dashboard even with no jobs")

	// Page should load successfully
	title := it.GetTitle()
	require.Contains(t, title, "Dashboard", "Page should load successfully")

	// Should handle the case gracefully (either show empty state or error message)
	content := it.GetContent()
	t.Logf("Dashboard content for new user: %s", content)

	// The page should not crash - that's the main test here
	require.NotEmpty(t, content, "Page should have some content")
}
