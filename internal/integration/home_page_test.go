package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestProfilePage verifies that authenticated users are redirected to the profile page
func TestProfilePage(t *testing.T) {
	// Create new test environment
	it := NewIntegrationTest(t)
	defer it.Close()

	// Authenticate the test user
	it.Authenticate()

	// Navigate to profile page
	it.NavigateTo("/profile")

	// Verify we're on the profile page
	currentURL := it.Page.URL()
	require.Contains(t, currentURL, "/profile", "Should be on profile page")

	// Verify page title
	title := it.GetTitle()
	require.Contains(t, title, "Upload Resume", "Page title should contain 'Upload Resume'")

	// Verify welcome message
	require.True(t, it.IsVisible("Welcome to Talent Rodeo"), "Welcome message should be visible on profile page")

	// Print page content for debugging
	content := it.GetContent()
	t.Logf("Page content: %s", content)

	// Verify profile page elements
	elements := []string{
		"Resume (PDF, DOCX, or TXT)",
		"Phone Number",
		"Continue",
	}

	for _, text := range elements {
		require.True(t, it.IsVisible(text), "%s should be visible on profile page", text)
	}
}
