package main

import (
	"context"
	"fmt"
	"net/http"
	"talent-rodeo/internal"
)

// resetOnboardingState handles POST requests to reset a user's onboarding progress.
// It sets the user back to the initial onboarding step and refreshes the profile
// resume section of the page.
//
// Flow:
//  1. Verifies user authentication
//  2. Resets onboarding step to initial
//  3. Re-renders the profile resume partial template
//
// Routes:
//   - POST /profile/onboarding/reset
//
// Templates:
//   - partial:profile-resume (partials/*.html)
//
// State Transitions:
//   - Any State → StepInitial
func (app *application) resetOnboardingState(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.badRequest(w, r, fmt.Errorf("resetOnboardingState: user not found"))
		return
	}

	user.State.Status = internal.UserStatusPendingResumeUpload

	if err := internal.UpdateUser(context.Background(), app.fs, user.ID, map[string]interface{}{
		"state": user.State,
	}); err != nil {
		app.serverError(w, r, fmt.Errorf("resetOnboardingState: failed to update onboarding: %w", err))
		return
	}

	http.Redirect(w, r, "/profile", http.StatusFound)
}
