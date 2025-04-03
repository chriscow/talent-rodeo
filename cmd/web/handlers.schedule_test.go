package main

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Load("../../.env"); err != nil {
		log.Fatal("Error loading .env file")
	}

	// Set default VAPI webhook URL for tests if not already set
	if os.Getenv("VAPI_START_CALL_URL") == "" {
		os.Setenv("VAPI_START_CALL_URL", "https://api.vapi.ai/v1/call")
	}
}

func TestScheduling(t *testing.T) {
	ts := newTestServer(t, "test scheduling")
	defer ts.Cleanup()

	// Save original webhook URL and restore after tests
	originalURL := os.Getenv("VAPI_START_CALL_URL")
	defer os.Setenv("VAPI_START_CALL_URL", originalURL)

	t.Run("Missing VAPI webhook URL fails appointment creation", func(t *testing.T) {
		// Save and clear webhook URL
		originalURL := os.Getenv("VAPI_START_CALL_URL")
		os.Setenv("VAPI_START_CALL_URL", "")
		defer os.Setenv("VAPI_START_CALL_URL", originalURL)

		// Ensure user has required fields
		phone := "+1234567890"
		ts.user1.Phone = &phone

		// Add a test resume for the user since handler requires it
		annotated := "test resume content"
		ts.resume1.Annotated = &annotated

		tomorrow := time.Now().AddDate(0, 0, 1)
		tomorrow = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 10, 0, 0, 0, time.UTC)

		form := url.Values{}
		form.Add("datetime", tomorrow.Format("2006-01-02T15:04"))
		form.Add("timezone", "America/New_York")
		form.Add("csrf_token", "test-csrf-token")

		code, _, body := ts.authenticatedPostForm(t, ts.user1, "/profile/schedule/appointment", form)

		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}

		// Check that the response contains the error message
		if !strings.Contains(body, "Server error") {
			t.Error("expected error message not found in response")
		}
	})

	t.Run("Unauthenticated user cannot access scheduling", func(t *testing.T) {
		// Use get() instead of authenticatedGet() to test unauthenticated access
		code, headers, _ := ts.get(t, "/profile/schedule")

		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}

		if headers.Get("Location") != "/auth/linkedin" {
			t.Errorf("want %s; got %s", "/auth/linkedin", headers.Get("Location"))
		}
	})

	t.Run("Authenticated user can access scheduling page", func(t *testing.T) {
		code, _, body := ts.authenticatedGet(t, "/profile/schedule")

		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}

		// Check that the page contains the expected form elements with exact matches
		if !strings.Contains(body, `<form method="post" action="/profile/schedule/appointment">`) {
			t.Error("schedule appointment form not found in response")
		}
		if !strings.Contains(body, `<form method="post" action="/profile/schedule/call-now">`) {
			t.Error("call now form not found in response")
		}
	})

	t.Run("Authenticated user can create appointment", func(t *testing.T) {
		// Set appointment time to tomorrow at 10 AM
		tomorrow := time.Now().AddDate(0, 0, 1)
		tomorrow = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 10, 0, 0, 0, time.UTC)

		form := url.Values{}
		form.Add("datetime", tomorrow.Format("2006-01-02T15:04"))
		form.Add("timezone", "America/New_York")
		form.Add("csrf_token", "test-csrf-token") // Added because we're using CSRF protection

		code, headers, _ := ts.authenticatedPostForm(t, ts.user1, "/profile/schedule/appointment", form)

		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}

		if headers.Get("Location") != "/profile" {
			t.Errorf("want %s; got %s", "/profile", headers.Get("Location"))
		}
	})

	t.Run("Invalid datetime format is rejected", func(t *testing.T) {
		form := url.Values{}
		form.Add("datetime", "invalid-datetime")
		form.Add("timezone", "America/New_York")
		form.Add("csrf_token", "test-csrf-token")

		code, _, _ := ts.authenticatedPostForm(t, ts.user1, "/profile/schedule/appointment", form)

		if code != http.StatusBadRequest {
			t.Errorf("want %d; got %d", http.StatusBadRequest, code)
		}
	})

	t.Run("Past datetime is rejected", func(t *testing.T) {
		yesterday := time.Now().AddDate(0, 0, -1)
		form := url.Values{}
		form.Add("datetime", yesterday.Format("2006-01-02T15:04"))
		form.Add("timezone", "America/New_York")
		form.Add("csrf_token", "test-csrf-token")

		code, _, _ := ts.authenticatedPostForm(t, ts.user1, "/profile/schedule/appointment", form)

		if code != http.StatusBadRequest {
			t.Errorf("want %d; got %d", http.StatusBadRequest, code)
		}
	})

	t.Run("Missing timezone is rejected", func(t *testing.T) {
		tomorrow := time.Now().AddDate(0, 0, 1)
		form := url.Values{}
		form.Add("datetime", tomorrow.Format("2006-01-02T15:04"))
		form.Add("csrf_token", "test-csrf-token")

		code, _, _ := ts.authenticatedPostForm(t, ts.user1, "/profile/schedule/appointment", form)

		if code != http.StatusBadRequest {
			t.Errorf("want %d; got %d", http.StatusBadRequest, code)
		}
	})

	t.Run("Call Now creates immediate appointment", func(t *testing.T) {
		form := url.Values{}
		form.Add("timezone", "America/New_York")
		form.Add("csrf_token", "test-csrf-token")

		code, headers, _ := ts.authenticatedPostForm(t, ts.user1, "/profile/schedule/call-now", form)

		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}

		if headers.Get("Location") != "/profile" {
			t.Errorf("want %s; got %s", "/profile", headers.Get("Location"))
		}
	})

	t.Run("Call Now without timezone still works", func(t *testing.T) {
		form := url.Values{}
		form.Add("csrf_token", "test-csrf-token")

		code, headers, _ := ts.authenticatedPostForm(t, ts.user1, "/profile/schedule/call-now", form)

		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}

		if headers.Get("Location") != "/profile" {
			t.Errorf("want %s; got %s", "/profile", headers.Get("Location"))
		}
	})
}
