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
	}

	err = session.Save(r, w)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	http.Redirect(w, r, redirectPath, http.StatusFound)
}

func (app *application) handleAuthError(w http.ResponseWriter, r *http.Request, err error) {
	if strings.Contains(err.Error(), "denied") {
		http.Error(w, "You must grant access to continue.", http.StatusUnauthorized)
		return
	}

	app.serverError(w, r, err)
}

func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	app.logger.Info("Handling logout request", "method", r.Method, "url", r.URL.String())

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
		session.Values = make(map[interface{}]interface{})
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
