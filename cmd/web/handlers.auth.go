package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"talent-rodeo/internal"

	"io"

	"github.com/alexedwards/flow"
	"github.com/google/uuid"
	"github.com/markbates/goth/gothic"
	"github.com/pusher/pusher-http-go/v5"
)

// HTTP Status Code Notes:
// We use http.StatusSeeOther (303) for all redirects in the authentication flow.
// This is crucial for security and usability because:
// 1. After login/logout operations (POST), we want to force a GET to the next page
// 2. Prevents session fixation by ensuring a clean GET request
// 3. Avoids browser caching of sensitive auth-related redirects
// 4. Maintains consistency with OAuth providers that also use 303
// 5. Follows security best practices for authentication flows
//
// The 303 status code is particularly important for auth handlers as it helps prevent
// authentication/session related attacks and ensures clean session state transitions.

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

	gothUser, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		app.handleAuthError(w, r, err)
		return
	}

	u, found, err := internal.GetUserByEmail(r.Context(), app.fs, gothUser.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if !found {
		app.logger.Info("User not found, creating new user", "email", gothUser.Email)

		u = &internal.User{
			ID:          uuid.New().String(),
			Email:       gothUser.Email,
			Provider:    gothUser.Provider,
			Name:        gothUser.Name,
			FirstName:   gothUser.FirstName,
			LastName:    gothUser.LastName,
			NickName:    gothUser.NickName,
			Description: gothUser.Description,
			UserID:      gothUser.UserID,
			AvatarURL:   gothUser.AvatarURL,
			Location:    gothUser.Location,
			State: internal.UserState{
				Status:  internal.UserStatusPendingResumeUpload,
				Started: time.Now(),
			},
		}

		if err := internal.CreateUser(r.Context(), app.fs, u); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to create firestore user: %w", err))
			return
		}
	}

	// Store user ID in session
	app.sessionManager.Put(r.Context(), "UserID", u.ID)
	r = contextSetAuthenticatedUser(r, u)

	// If onboarding is not complete, redirect to profile
	redirectPath := "/"
	if u.State.Status < internal.UserStatusCallScheduled {
		redirectPath = "/profile"
	}

	http.Redirect(w, r, redirectPath, http.StatusSeeOther)
}

// handleAuthError is a helper function that processes authentication errors.
// It provides specific error messages for access denied cases.
func (app *application) handleAuthError(w http.ResponseWriter, r *http.Request, err error) {
	// If the user denied the request on the provider side, it's not a server error.
	if strings.Contains(err.Error(), "denied") {
		http.Error(w, "You must grant access to continue.", http.StatusUnauthorized)
		return
	}

	// This error often means the user's session expired or got corrupted
	// during the OAuth flow. We'll clear their session and send them back to
	// the home page to try again.
	if strings.Contains(err.Error(), "could not find a matching session for this request") {
		app.logger.Warn("clearing stale oauth session and redirecting to home")

		// We can't use sessionManager.Destroy() because that relies on the session
		// token being present in the context, which it isn't in this error case.
		// Instead, we'll manually delete the cookie.
		cookie := &http.Cookie{
			Name:     app.sessionManager.Cookie.Name,
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   app.sessionManager.Cookie.Secure,
			SameSite: app.sessionManager.Cookie.SameSite,
		}
		http.SetCookie(w, cookie)

		// Redirect to home to restart the flow
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// For all other errors, return a generic 500.
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
	app.logger.Debug("Starting logout process",
		"method", r.Method,
		"url", r.URL.String(),
		"headers", fmt.Sprintf("%+v", r.Header))

	// Get user before destroying session
	user := contextGetAuthenticatedUser(r)
	if user != nil {
		app.logger.Debug("Logging out user", "userID", user.ID, "email", user.Email)
	}

	// Destroy the session
	err := app.sessionManager.Destroy(r.Context())
	if err != nil {
		app.logger.Error("Failed to destroy session", "error", err)
		app.serverError(w, r, err)
		return
	}

	app.logger.Debug("Session destroyed successfully, redirecting to home")

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusSeeOther)

	app.logger.Debug("Logout redirect sent",
		"location", "/",
		"status", http.StatusSeeOther)
}

func (app *application) pusherAuth(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	params, err := io.ReadAll(r.Body)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Presence data to send to other clients
	presenceData := pusher.MemberData{
		UserID: user.ID,
		UserInfo: map[string]string{
			"name":      user.Name,
			"avatarURL": user.AvatarURL,
		},
	}

	// AuthenticatePresenceChannel is deprecated, use AuthorizePresenceChannel instead
	response, err := app.pusher.AuthorizePresenceChannel(params, presenceData)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(response)
}
