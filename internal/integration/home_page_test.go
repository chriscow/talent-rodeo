package integration

import (
	"talent-rodeo/internal"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOnboardingPage verifies that authenticated users are redirected to the onboarding page
func TestOnboardingPage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	// Create new test environment
	it := NewIntegrationTest(t)
	defer it.Close()

	_, found, err := internal.GetUser(it.Ctx, it.Fs, it.User.ID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if !found {
		t.Fatalf("User not found")
	}

	// Authenticate the test user
	it.Authenticate()

	// Navigate to onboarding page
	it.NavigateTo("/onboarding")

	// Verify we're on the onboarding page
	currentURL := it.Page.URL()
	require.Contains(t, currentURL, "/onboarding", "Should be on onboarding page")

	// Verify page title
	title := it.GetTitle()
	require.Contains(t, title, "Upload Resume", "Page title should contain 'Upload Resume'")

	// Verify welcome message
	require.True(t, it.IsVisible("Welcome to Talent Rodeo"), "Welcome message should be visible on onboarding page")

	// Print page content for debugging
	content := it.GetContent()
	t.Logf("Page content: %s", content)

	// Verify onboarding page elements
	elements := []string{
		"Resume (PDF, DOCX, or TXT)",
		"Phone Number",
		"Continue",
	}

	for _, text := range elements {
		require.True(t, it.IsVisible(text), "%s should be visible on onboarding page", text)
	}
}
