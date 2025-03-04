package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/alexedwards/flow"
	"github.com/markbates/goth/gothic"
)

// startSocialAuth handles GET requests to /auth/:provider.
// It initiates the OAuth flow with the specified provider (e.g., LinkedIn).
// The provider parameter is extracted from the URL and passed to the gothic library.
//
// Flow:
//  1. Extracts provider from URL path
//  2. Adds provider to query string (required by gothic)
//  3. Redirects user to provider's OAuth page
//
// Routes:
//   - GET /auth/:provider
//
// Requires: Anonymous user (handled by middleware)
func (app *application) startSocialAuth(w http.ResponseWriter, r *http.Request) {
	provider := flow.Param(r.Context(), "provider")
	q := r.URL.Query()
	q.Set("provider", provider)
	r.URL.RawQuery = q.Encode()

	gothic.BeginAuthHandler(w, r)
}

// completeSocialAuth handles GET requests to /auth/:provider/callback.
// It completes the OAuth flow, creates or retrieves the user, and starts their session.
//
// Flow:
//  1. Validates OAuth response
//  2. Completes authentication with provider
//  3. Creates or retrieves user from database
//  4. Creates session
//  5. Redirects based on onboarding state
//
// Routes:
//   - GET /auth/:provider/callback
//
// Requires: Anonymous user (handled by middleware)
func (app *application) completeSocialAuth(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" {
		app.logger.Error("OAuth error",
			"error", r.URL.Query().Get("error"),
			"description", r.URL.Query().Get("error_description"))
		http.Error(w, "You must grant access to continue.", http.StatusUnauthorized)
		return
	}

	provider := flow.Param(r.Context(), "provider")
	q := r.URL.Query()
	q.Set("provider", provider)
	r.URL.RawQuery = q.Encode()

	user, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		app.handleAuthError(w, r, err)
		return
	}

	dbUser, found, err := app.db.GetUserByEmail(user.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if !found {
		dbUser, err = app.db.NewUserFromGothUser(user, "candidate")
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		// Create candidate record for new user
		_, err = app.db.CreateCandidate(r.Context(), dbUser.ID)
		if err != nil {
			app.serverError(w, r, fmt.Errorf("failed to create candidate record: %w", err))
			return
		}
	}

	session, err := app.sessionStore.Get(r, "session")
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	session.Values["UserID"] = dbUser.ID
	r = contextSetAuthenticatedUser(r, dbUser)

	// Check onboarding state
	state, err := app.db.GetOnboardingState(dbUser.ID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get onboarding state: %w", err))
		return
	}

	// If onboarding is not complete, redirect to profile
	redirectPath := "/"
	if !state.IsComplete() {
		redirectPath = "/profile"
	}

	err = session.Save(r, w)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	http.Redirect(w, r, redirectPath, http.StatusFound)
}

// handleAuthError is a helper function that processes authentication errors.
// It provides specific error messages for access denied cases.
func (app *application) handleAuthError(w http.ResponseWriter, r *http.Request, err error) {
	if strings.Contains(err.Error(), "denied") {
		http.Error(w, "You must grant access to continue.", http.StatusUnauthorized)
		return
	}

	app.serverError(w, r, err)
}

// logoutUser handles POST requests to /logout.
// It clears the user's session and redirects them to the login page.
//
// Flow:
//  1. Retrieves current session
//  2. Clears all session values
//  3. Sets session cookie to expire
//  4. Redirects to login page
//
// Routes:
//   - POST /logout
//
// Requires: Authenticated user (handled by middleware)
func (app *application) logoutUser(w http.ResponseWriter, r *http.Request) {
	app.logger.Info("Processing logout request", "method", r.Method, "url", r.URL.String())

	session, err := app.sessionStore.Get(r, "session")
	if err != nil {
		app.logger.Error("Failed to get session", "error", err)
		app.serverError(w, r, err)
		return
	}

	app.logger.Info("Current session values", "values", session.Values)

	// Clear all session values
	for key := range session.Values {
		delete(session.Values, key)
	}

	if app.config.testing {
		// In testing mode, explicitly reset session values
		session.Values = make(map[any]any)
	}

	// Set MaxAge to -1 to expire the cookie immediately
	session.Options.MaxAge = -1

	// Save the session before redirecting
	err = session.Save(r, w)
	if err != nil {
		app.logger.Error("Failed to save session", "error", err)
		app.serverError(w, r, err)
		return
	}

	app.logger.Info("Session cleared successfully, redirecting to login")

	// Redirect to login page
	http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
}
