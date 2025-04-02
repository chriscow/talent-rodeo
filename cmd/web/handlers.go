package main

import (
	"net/http"

	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	// If user is authenticated, check onboarding state
	if user := contextGetAuthenticatedUser(r); user != nil {
		state := user.State

		// Redirect to profile if onboarding is not complete
		if state.Status < internal.UserStatusIntroCallCompleted {
			http.Redirect(w, r, "/profile", http.StatusSeeOther)
			return
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/home.html"); err != nil {
		app.serverError(w, r, err)
	}
}
