package main

import (
	"net/http"
	"net/http/httptest"
	"talent-rodeo/internal"
	"testing"
	"time"
)

func TestAuthFlow(t *testing.T) {
	ts := newTestServer(t, "auth flow")
	defer ts.Cleanup()

	t.Run("Unauthenticated user is redirected to login", func(t *testing.T) {
		code, headers, _ := ts.get(t, "/profile")

		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}

		if headers.Get("Location") != "/auth/linkedin" {
			t.Errorf("want %s; got %s", "/auth/linkedin", headers.Get("Location"))
		}
	})

	t.Run("Authenticated user can access profile", func(t *testing.T) {
		user := internal.CreateTestUser(t, ts.app.fs, "Test User", "test@example.com", "+14251234567", false)
		defer internal.DeleteTestData(t, ts.app.fs, "test-user-")

		req := ts.authenticatedRequest(t, "GET", "/profile", user, nil)
		w := httptest.NewRecorder()
		ts.app.routes().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Logout clears session", func(t *testing.T) {
		// Create an authenticated request
		now := time.Now()
		user := &internal.User{
			ID:        "1",
			Email:     "test@example.com",
			CreatedAt: now,
			Status:    internal.UserStatusPendingResumeUpload,
		}
		req := ts.authenticatedRequest(t, "POST", "/logout", user, nil)

		// Debug: Print request details
		t.Logf("Request Method: %s", req.Method)
		t.Logf("Request URL: %s", req.URL)
		t.Logf("Request Headers: %v", req.Header)
		t.Logf("Request Cookies: %v", req.Cookies())

		w := httptest.NewRecorder()

		// Send the request
		ts.app.routes().ServeHTTP(w, req)

		// Debug: Print response details
		t.Logf("Response Status: %d", w.Code)
		t.Logf("Response Headers: %v", w.Header())
		t.Logf("Response Body: %s", w.Body.String())

		if w.Code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, w.Code)
		}

		if w.Header().Get("Location") != "/" {
			t.Errorf("want %s; got %s", "/", w.Header().Get("Location"))
		}

		// Check that the session was cleared
		userID := ts.app.sessionManager.GetString(req.Context(), "UserID")
		if userID != "" {
			t.Error("session should be cleared")
		}
	})
}
