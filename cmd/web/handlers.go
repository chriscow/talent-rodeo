package main

import (
	"fmt"
	"net/http"

	"talent-rodeo/internal"
	"talent-rodeo/internal/response"

	"github.com/getsentry/sentry-go"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	app.logger.Debug("Handling home request",
		"method", r.Method,
		"url", r.URL.String(),
		"headers", fmt.Sprintf("%+v", r.Header))

	data := app.newTemplateData(r)

	// Get flash message from session
	data["Flash"] = app.sessionManager.PopString(r.Context(), "flash")

	// Log authentication state
	if user := contextGetAuthenticatedUser(r); user != nil {
		app.logger.Debug("Home: authenticated user",
			"user_id", user.ID,
			"name", user.Name,
			"state", user.State.Status)

		state := user.State

		// Redirect to profile if onboarding is not complete
		if state.Status < internal.UserStatusCallAnalysisStarted {
			app.logger.Debug("Home: redirecting incomplete onboarding to profile")
			http.Redirect(w, r, "/profile", http.StatusSeeOther)
			return
		}
	} else {
		app.logger.Debug("Home: no authenticated user")
	}

	if err := response.Page(w, http.StatusOK, data, "pages/home.html"); err != nil {
		app.serverError(w, r, err)
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"error_type":  "template_rendering",
			"entity_type": "home_page",
		})
	}

	app.logger.Debug("Home: rendered successfully")
}

func (app *application) showPrivacyPage(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	if err := response.Page(w, http.StatusOK, data, "pages/privacy.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) showTermsPage(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	if err := response.Page(w, http.StatusOK, data, "pages/terms.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) showAboutPage(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	if err := response.Page(w, http.StatusOK, data, "pages/about.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteAccount(w http.ResponseWriter, r *http.Request) {
	// Get the authenticated user
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Verify the form was submitted with confirmation
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
		return
	}

	if r.PostForm.Get("confirm_delete") != "on" {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	// Delete the user from Firestore
	if err := internal.DeleteUser(r.Context(), app.fs, user.ID); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to delete user: %w", err))
		return
	}

	// Clear the session
	app.sessionManager.Remove(r.Context(), "authenticatedUserID")

	// Redirect to home page with a flash message
	app.sessionManager.Put(r.Context(), "flash", "Your account has been successfully deleted.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
