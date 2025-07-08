package main

import (
	"net/http"
)

// showProfilePage handles GET requests to /profile - TODO: Remove this handler once all routes are migrated to /onboarding
func (app *application) showProfilePage(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)

	app.logger.Debug("showProfilePage [TODO] - redirecting to /onboarding", "name", user.Name, "userID", user.ID, "status", user.Status)

	// Redirect to the new onboarding flow
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}
