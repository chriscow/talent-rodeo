package main

import (
	"fmt"
	"net/http"
	"os"
	"talent-rodeo/internal"
)

// handleTestingResetUserState resets the user's state to UserStatusPendingResumeUpload
// Only available when TESTING_MODE is enabled
func (app *application) handleTestingResetUserState(w http.ResponseWriter, r *http.Request) {
	// Only allow this in testing mode
	if os.Getenv("TESTING_MODE") != "true" {
		app.clientError(w, http.StatusForbidden)
		return
	}

	// Get the current user
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.clientError(w, http.StatusUnauthorized)
		return
	}

	// Reset the user's state using the helper function
	if err := internal.ResetUserState(r.Context(), app.fs, user); err != nil {
		app.logger.Error("failed to reset user state", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to reset user state: %w", err))
		return
	}

	app.logger.Info("reset user state for testing",
		"user_id", user.ID,
		"new_status", internal.UserStatusPendingResumeUpload)

	// Redirect back to profile
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

// handleTestingAuthenticate sets up authentication for a specific test user.
// Only available when TESTING_MODE is enabled.
func (app *application) handleTestingAuthenticate(w http.ResponseWriter, r *http.Request) {
	// Only allow this in testing mode
	if os.Getenv("TESTING_MODE") != "true" {
		app.clientError(w, http.StatusForbidden)
		return
	}

	// Require POST method
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		app.clientError(w, http.StatusMethodNotAllowed)
		return
	}

	userID := r.Header.Get("X-Test-User-ID")
	if userID == "" {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	// Get the user from Firestore to ensure it exists
	user, found, err := internal.GetUser(r.Context(), app.fs, userID)
	if err != nil {
		app.logger.Error("failed to get test user", "error", err)
		app.serverError(w, r, fmt.Errorf("failed to get test user: %w", err))
		return
	}
	if !found {
		app.clientError(w, http.StatusNotFound)
		return
	}

	// Set the user in the session
	app.sessionManager.Put(r.Context(), "UserID", userID)
	app.logger.Info("test mode: authenticated user",
		"user_id", user.ID,
		"email", user.Email)

	w.WriteHeader(http.StatusOK)
}
