package main

import (
	"fmt"
	"net/http"

	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	app.logger.Debug("Handling home request",
		"method", r.Method,
		"url", r.URL.String(),
		"headers", fmt.Sprintf("%+v", r.Header))

	data := app.newTemplateData(r)

	// Log authentication state
	if user := contextGetAuthenticatedUser(r); user != nil {
		app.logger.Debug("Home: authenticated user",
			"user_id", user.ID,
			"state", user.State.Status)

		state := user.State

		// Redirect to profile if onboarding is not complete
		if state.Status < internal.UserStatusIntroCallCompleted {
			app.logger.Debug("Home: redirecting incomplete onboarding to profile")
			http.Redirect(w, r, "/profile", http.StatusSeeOther)
			return
		}
	} else {
		app.logger.Debug("Home: no authenticated user")
	}

	if err := response.Page(w, http.StatusOK, data, "pages/home.html"); err != nil {
		app.serverError(w, r, err)
	}

	app.logger.Debug("Home: rendered successfully")
}
