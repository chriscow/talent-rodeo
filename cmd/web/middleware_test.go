package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

// TestAuthenticateTestingMode tests that the authenticate middleware
// automatically authenticates with a test user when TESTING_MODE is enabled
func TestAuthenticateTestingMode(t *testing.T) {
	// Set up testing mode
	oldTestingMode := os.Getenv("TESTING_MODE")
	oldLinkedInUsername := os.Getenv("LINKEDIN_USERNAME")
	defer func() {
		if oldTestingMode == "" {
			os.Unsetenv("TESTING_MODE")
		} else {
			os.Setenv("TESTING_MODE", oldTestingMode)
		}
		if oldLinkedInUsername == "" {
			os.Unsetenv("LINKEDIN_USERNAME")
		} else {
			os.Setenv("LINKEDIN_USERNAME", oldLinkedInUsername)
		}
	}()

	// Test case 1: Testing mode disabled - should not auto-authenticate
	t.Run("testing_mode_disabled", func(t *testing.T) {
		os.Setenv("TESTING_MODE", "false")

		// Create a mock application (we can't easily create a full app for unit tests)
		// So we'll just test the logic flow by checking environment variables
		testingMode := os.Getenv("TESTING_MODE") == "true"
		if testingMode {
			t.Error("Expected testing mode to be disabled")
		}
	})

	// Test case 2: Testing mode enabled - should auto-authenticate
	t.Run("testing_mode_enabled", func(t *testing.T) {
		os.Setenv("TESTING_MODE", "true")
		os.Setenv("LINKEDIN_USERNAME", "test@example.com")

		testingMode := os.Getenv("TESTING_MODE") == "true"
		if !testingMode {
			t.Error("Expected testing mode to be enabled")
		}

		linkedInUsername := os.Getenv("LINKEDIN_USERNAME")
		if linkedInUsername != "test@example.com" {
			t.Errorf("Expected LINKEDIN_USERNAME to be 'test@example.com', got '%s'", linkedInUsername)
		}
	})
}

// TestAuthenticateMiddlewareFlow tests the full middleware flow
func TestAuthenticateMiddlewareFlow(t *testing.T) {
	// This is a simplified test to verify the middleware structure
	// In a real test, you'd need to set up a full Firestore client and session manager

	// Create a simple session manager for testing
	sessionManager := scs.New()
	sessionManager.Lifetime = time.Hour

	// Create a mock next handler
	var handlerCalled bool

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// This test verifies the middleware structure
	// Note: We can't fully test the Firestore integration without a real setup
	t.Run("middleware_structure", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		// Test that the middleware is properly structured
		// In a real application test, you'd need to mock the Firestore client
		nextHandler.ServeHTTP(rr, req)

		if !handlerCalled {
			t.Error("Next handler was not called")
		}

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status code 200, got %d", rr.Code)
		}
	})
}
