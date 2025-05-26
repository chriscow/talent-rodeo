package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"talent-rodeo/internal"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/require"
)

// IntegrationTest provides a framework for running integration tests
// with consistent setup and teardown of resources.
type IntegrationTest struct {
	T          *testing.T
	Ctx        context.Context
	Fs         *internal.FirestoreClient
	User       *internal.User
	Browser    playwright.Browser
	Page       playwright.Page
	Playwright *playwright.Playwright
}

// NewIntegrationTest creates and sets up a new integration test environment.
// It automatically handles:
// - Setting up Firestore
// - Loading or creating a test user
// - Resetting user state
// - Setting up Playwright browser
// The returned IntegrationTest should be closed using the Close method when done.
func NewIntegrationTest(t *testing.T) *IntegrationTest {
	t.Helper()

	// We need the server to be running in TESTING_MODE so we can authenticate the test user
	if os.Getenv("TESTING_MODE") != "true" {
		t.Fatal("TESTING_MODE must be set to 'true' for integration tests")
	}

	if os.Getenv("FIRESTORE_EMULATOR_HOST") != "" {
		println(internal.Red("FIRESTORE_EMULATOR_HOST is set"))
		println("Do not use the Firestore emulator for integration tests")
		println("We end up with different databases and the server doesn't seem to be able to find the test user")
	}

	ctx := context.Background()
	it := &IntegrationTest{
		T:   t,
		Ctx: ctx,
	}

	// Setup each component
	it.setupFirestore()
	it.setupTestUser()
	it.setupBrowser()

	return it
}

// setupFirestore initializes the Firestore client for testing
func (it *IntegrationTest) setupFirestore() {
	it.T.Helper()

	fs, err := internal.NewFirestoreClient(it.Ctx)
	it.requireNoError(err, "failed to create Firestore client")
	it.Fs = fs
}

// setupTestUser loads or creates a test user and resets their state
func (it *IntegrationTest) setupTestUser() {
	it.T.Helper()

	// Create test user data
	testUser := &internal.User{
		ID:          "test-user-id",
		Email:       "test@example.com",
		Name:        "Test User",
		IsSuperUser: false,
	}

	// Save or update the test user
	err := internal.SaveUser(it.Ctx, it.Fs, testUser)
	it.requireNoError(err, "failed to save test user")

	// Reset user state to ensure clean test environment
	err = internal.ResetUserState(it.Ctx, it.Fs, testUser)
	it.requireNoError(err, "failed to reset user state")

	it.User = testUser
}

// setupBrowser initializes the Playwright browser for testing
func (it *IntegrationTest) setupBrowser() {
	it.T.Helper()

	// Initialize Playwright
	pw, err := playwright.Run()
	it.requireNoError(err, "failed to start playwright")
	it.Playwright = pw

	// Launch browser
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	it.requireNoError(err, "failed to launch browser")
	it.Browser = browser

	// Create a new context with persistent storage
	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		AcceptDownloads: playwright.Bool(true),
	})
	it.requireNoError(err, "failed to create browser context")

	// Create a new page
	page, err := context.NewPage()
	it.requireNoError(err, "failed to create new page")
	it.Page = page
}

// Close cleans up all resources used by the integration test.
// This should be called using defer immediately after creating a new IntegrationTest.
func (it *IntegrationTest) Close() {
	it.T.Helper()

	// Delete test user and their data
	if it.User != nil && it.Fs != nil {
		err := internal.DeleteUser(it.Ctx, it.Fs, it.User.ID)
		if err != nil {
			it.T.Logf("Warning: Failed to delete test user: %v", err)
		} else {
			it.T.Logf("Successfully deleted test user: %s", it.User.ID)
		}
	}

	if it.Page != nil {
		err := it.Page.Close()
		if err != nil {
			it.requireNoError(err, "failed to close page")
		}
	}

	if it.Browser != nil {
		err := it.Browser.Close()
		if err != nil {
			it.requireNoError(err, "failed to close browser")
		}
	}

	if it.Playwright != nil {
		err := it.Playwright.Stop()
		if err != nil {
			it.requireNoError(err, "failed to stop playwright")
		}
	}

	// Note: We don't close Firestore client as it's managed by the application
}

// requireNoError is a helper that captures a screenshot and page content
// before failing the test if there is an error
func (it *IntegrationTest) requireNoError(err error, msg string) {
	it.T.Helper()
	if err != nil {
		// Create testdata directory if it doesn't exist
		if err := os.MkdirAll("../../testdata", 0755); err != nil {
			it.T.Logf("Failed to create testdata directory: %v", err)
		}

		// Generate screenshot filename based on test name
		screenshotPath := filepath.Join("../../testdata", fmt.Sprintf("%s.png", it.T.Name()))

		// Take error screenshot
		if _, screenshotErr := it.Page.Screenshot(playwright.PageScreenshotOptions{
			Path:     playwright.String(screenshotPath),
			FullPage: playwright.Bool(true),
		}); screenshotErr != nil {
			it.T.Logf("Failed to take error screenshot: %v", screenshotErr)
		} else {
			it.T.Logf("Screenshot saved to: %s", screenshotPath)
		}

		// Capture page content
		if content, contentErr := it.Page.Content(); contentErr == nil {
			it.T.Logf("Page content at time of failure:\n%s", content)
		}

		// Now fail the test
		require.NoError(it.T, err, msg)
	}
}

// NavigateTo navigates to the given path and ensures the page loads successfully
func (it *IntegrationTest) NavigateTo(path string) {
	it.T.Helper()

	baseURL := "http://localhost:2304" // Match the port in home_page_test.go
	url := fmt.Sprintf("%s%s", baseURL, path)

	_, err := it.Page.Goto(url)
	it.requireNoError(err, fmt.Sprintf("Failed to navigate to %s", url))
}

// WaitForText waits for the given text to appear on the page
func (it *IntegrationTest) WaitForText(text string) {
	it.T.Helper()

	_, err := it.Page.WaitForSelector(fmt.Sprintf("text=%s", text))
	it.requireNoError(err, fmt.Sprintf("failed to find text: %s", text))
}

// ResetUserState resets the test user's state back to initial conditions
func (it *IntegrationTest) ResetUserState() {
	it.T.Helper()

	err := internal.ResetUserState(it.Ctx, it.Fs, it.User)
	it.requireNoError(err, "failed to reset user state")
}

// IsVisible checks if text is visible on the page using locator-based approach
func (it *IntegrationTest) IsVisible(text string) bool {
	locator := it.Page.Locator(fmt.Sprintf("text=%s", text))
	visible, err := locator.IsVisible()
	it.requireNoError(err, fmt.Sprintf("Failed to check if text is visible: %s", text))
	return visible
}

// GetTitle returns the page title with error handling and screenshots
func (it *IntegrationTest) GetTitle() string {
	title, err := it.Page.Title()
	it.requireNoError(err, "Failed to get page title")
	return title
}

// GetContent returns the page content with error handling and screenshots
func (it *IntegrationTest) GetContent() string {
	content, err := it.Page.TextContent("body")
	it.requireNoError(err, "Failed to get page content")
	return content
}

// Authenticate calls the test authentication endpoint to set up the session
func (it *IntegrationTest) Authenticate() {
	it.T.Helper()

	// First, add any required cookies to the context
	context := it.Page.Context() // Get the browser context from the page
	err := context.AddCookies([]playwright.OptionalCookie{
		{
			Name:  "test_mode",
			Value: "true",
			URL:   playwright.String("http://localhost:2304"),
		},
	})
	it.requireNoError(err, "failed to set test mode cookie")

	// Make the authentication request using Playwright's API
	response, err := it.Page.Request().Post("http://localhost:2304/testing/authenticate", playwright.APIRequestContextPostOptions{
		Headers: map[string]string{
			"X-Test-User-ID": it.User.ID,
			"Content-Type":   "application/json",
		},
	})
	it.requireNoError(err, "failed to make authentication request")

	// Verify the response
	if response.Status() != 200 {
		it.T.Fatalf("Authentication failed with status %d", response.Status())
	}
}
