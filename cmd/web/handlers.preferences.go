package main

import (
	"fmt"
	"net/http"

	"talent-rodeo/internal/response"
)

// showPreferencesPage displays the job preferences interface where users can set their
// preferences for job matching.
//
// Flow:
//  1. Verifies user authentication
//  2. Loads existing preferences
//  3. Shows preferences form
//
// Routes:
//   - GET /profile/preferences
//
// Templates:
//   - pages/user-preferences.html
//
// State:
//   - Does not modify onboarding state
//   - Accessible after StepPhoneCallCompleted
func (app *application) showPreferencesPage(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}

	// Get candidate ID for the user
	candidateID, err := app.db.GetCandidateIDForUser(r.Context(), user.ID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get candidate ID: %w", err))
		return
	}

	// Load skills data
	skills, err := app.db.GetCandidateSkills(r.Context(), candidateID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to fetch skills: %w", err))
		return
	}

	// For now, we'll just show the template without any data
	// In the future, we'll load existing preferences from the database
	data := app.newTemplateData(r)
	data["Skills"] = skills

	err = response.Page(w, http.StatusOK, data, "pages/profile-preferences.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}

// updatePreferences handles POST requests to update a user's job preferences.
// These preferences are used to determine job matches and search criteria.
//
// Flow:
//  1. Validates form data
//  2. Updates preferences in database
//  3. Redirects back to preferences page
//
// Routes:
//   - POST /profile/preferences
//
// Templates:
//   - pages/user-preferences.html (on validation error)
//
// State:
//   - Does not modify onboarding state
//   - Accessible after StepPhoneCallCompleted
func (app *application) updatePreferences(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}

	err := r.ParseForm()
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	// For now, we'll just redirect back to the preferences page
	// In the future, we'll process the form data and save to database
	http.Redirect(w, r, "/profile/preferences", http.StatusFound)
}
