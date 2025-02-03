package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/alexedwards/flow"
	"github.com/markbates/goth/gothic"
)

func (app *application) socialAuth(w http.ResponseWriter, r *http.Request) {

	// goth looks for the provider in the query string, so we need to add it there
	provider := flow.Param(r.Context(), "provider")
	q := r.URL.Query()
	q.Set("provider", provider)
	r.URL.RawQuery = q.Encode()

	gothic.BeginAuthHandler(w, r)
}

func (app *application) socialCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" {
		app.logger.Error("error", "error", r.URL.Query().Get("error"))
		app.logger.Error("error", "error-description", r.URL.Query().Get("error_description"))
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
		dbUser, err = app.db.NewUserFromGothUser(user)
		if err != nil {
			app.serverError(w, r, err)
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
		app.logger.Info("redirecting to profile for onboarding", "user_id", dbUser.ID, "step", state.CurrentStep)
	} else if path, ok := session.Values["redirectPathAfterLogin"].(string); ok {
		redirectPath = path
		delete(session.Values, "redirectPathAfterLogin")
	}

	err = session.Save(r, w)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	http.Redirect(w, r, redirectPath, http.StatusSeeOther)
}

func (app *application) handleAuthError(w http.ResponseWriter, r *http.Request, err error) {
	// Log the original error
	app.logger.Error("Auth error encountered", "error", err)

	// Check if it's a known error that might be cookie-related
	if strings.Contains(err.Error(), "session") ||
		strings.Contains(err.Error(), "cookie") {

		// Clear cookies
		clearAuthCookies(w, r)

		// Redirect back to the login page
		http.Redirect(w, r, "/auth/linkedin", http.StatusTemporaryRedirect)
		return
	}

	// Handle other types of errors
	http.Error(w, "Authentication error", http.StatusInternalServerError)
}

func clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	// Clear the gothic session cookie
	cookie := &http.Cookie{
		Name:     gothic.SessionName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // This makes the browser delete the cookie immediately
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)

	// Clear any provider-specific cookies that might exist
	// The exact names might vary depending on your providers
	// providerCookie := &http.Cookie{
	// 	Name:     "gothic_provider_session",
	// 	Value:    "",
	// 	Path:     "/",
	// 	MaxAge:   -1,
	// 	HttpOnly: true,
	// }
	// http.SetCookie(w, providerCookie)

	// You might also want to clear the actual session
	gothic.Logout(w, r)
}

func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	q.Set("provider", "linkedin")
	r.URL.RawQuery = q.Encode()

	gothic.Logout(w, r)

	session, err := app.sessionStore.Get(r, "session")
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	delete(session.Values, "UserID")
	delete(session.Values, "redirectPathAfterLogin")
	session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}
