package main

import (
	"fmt"
	"net/http"

	"talent-rodeo/internal/response"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	// If user is authenticated, check onboarding state
	if user := contextGetAuthenticatedUser(r); user != nil {
		state, err := app.db.GetOnboardingState(user.ID)
		if err != nil {
			app.serverError(w, r, fmt.Errorf("failed to get onboarding state: %w", err))
			return
		}

		// Redirect to profile if onboarding is not complete
		if !state.IsComplete() {
			http.Redirect(w, r, "/profile", http.StatusSeeOther)
			return
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/dashboard.html"); err != nil {
		app.serverError(w, r, err)
	}
}
