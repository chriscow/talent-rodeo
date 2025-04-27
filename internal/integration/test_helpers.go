package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"talent-rodeo/internal"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/require"
)

type contextKey string

// requireNoErrorWithScreenshot is a helper that captures a screenshot and page content
// before failing the test if there is an error
func requireNoErrorWithScreenshot(t *testing.T, page playwright.Page, err error, msg string) {
	t.Helper()
	if err != nil {
		// Create testdata directory if it doesn't exist
		if err := os.MkdirAll("../../testdata", 0755); err != nil {
			t.Logf("Failed to create testdata directory: %v", err)
		}

		// Generate screenshot filename based on test name
		screenshotPath := filepath.Join("../../testdata", fmt.Sprintf("%s.png", t.Name()))

		// Take error screenshot
		if _, screenshotErr := page.Screenshot(playwright.PageScreenshotOptions{
			Path:     playwright.String(screenshotPath),
			FullPage: playwright.Bool(true),
		}); screenshotErr != nil {
			t.Logf("Failed to take error screenshot: %v", screenshotErr)
		} else {
			t.Logf("Screenshot saved to: %s", screenshotPath)
		}

		// Capture page content
		if content, contentErr := page.Content(); contentErr == nil {
			t.Logf("Page content at time of failure:\n%s", content)
		}

		// Now fail the test
		require.NoError(t, err, msg)
	}
}

// CreateTestRequest creates an HTTP request with an authenticated user and CSRF token if needed
func CreateTestRequest(t *testing.T, method, urlPath string, user *internal.User, body io.Reader) *http.Request {
	t.Helper()

	r := httptest.NewRequest(method, urlPath, body)

	if user != nil {
		// Add user to request context using the correct context key
		r = r.WithContext(context.WithValue(r.Context(), contextKey("authenticatedUser"), user))
	}

	// Add CSRF token for POST requests
	if method == http.MethodPost {
		r.Header.Set("X-CSRF-Token", "test-csrf-token")
		cookie := &http.Cookie{
			Name:     "csrf_token",
			Value:    "test-csrf-token",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		}
		r.AddCookie(cookie)
	}

	return r
}

// CreateTestUser creates a test user with the given details
func CreateTestUser(t *testing.T, fs *internal.FirestoreClient, name string, isAdmin bool) *internal.User {
	t.Helper()

	now := time.Now()
	phone := "+14251234567"
	user := &internal.User{
		ID:          fmt.Sprintf("test-user-%s-%d", name, now.UnixNano()),
		Email:       fmt.Sprintf("%s@example.com", name),
		Name:        name,
		IsSuperUser: isAdmin,
		Phone:       &phone,
		State: internal.UserState{
			Status:  internal.UserStatusPendingResumeUpload,
			Started: now,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := internal.CreateUser(context.Background(), fs, user); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return user
}

// CleanupTestUser deletes a test user and all associated data
func CleanupTestUser(t *testing.T, fs *internal.FirestoreClient, userID string) {
	t.Helper()

	ctx := context.Background()

	// Delete user's appointments
	appointments, err := internal.ListAppointments(ctx, fs, userID)
	if err != nil {
		t.Errorf("Failed to list appointments for cleanup: %v", err)
	}
	for _, appointment := range appointments {
		if err := internal.DeleteAppointment(ctx, fs, userID, appointment.ID); err != nil {
			t.Errorf("Failed to delete appointment %s: %v", appointment.ID, err)
		}
	}

	// Delete user's resumes
	resumes, err := internal.ListResumes(ctx, fs, userID)
	if err != nil {
		t.Errorf("Failed to list resumes for cleanup: %v", err)
	}
	for _, resume := range resumes {
		if err := internal.DeleteResume(ctx, fs, userID, resume.ID); err != nil {
			t.Errorf("Failed to delete resume %s: %v", resume.ID, err)
		}
	}

	// Delete the user document
	if err := internal.DeleteUser(ctx, fs, userID); err != nil {
		t.Errorf("Failed to delete user %s: %v", userID, err)
	}
}
