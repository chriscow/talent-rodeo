package main

import (
	"context"
	"fmt"
	"net/http"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"

	"cloud.google.com/go/firestore"
	"github.com/getsentry/sentry-go"
)

// OnboardingData holds data for onboarding templates
type OnboardingData struct {
	Step               int
	User               *internal.User
	ProgressPercentage int
	IsLastStep         bool
}

// showOnboardingFlow handles GET requests to display the onboarding modal sequence.
// It shows a single page with 4 modal dialogs that display in sequence.
//
// Routes:
//   - GET /onboarding
//
// Templates:
//   - pages/onboarding.html
func (app *application) showOnboardingFlow(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	templateData := app.newTemplateData(r)
	templateData["User"] = user

	if err := response.Page(w, http.StatusOK, templateData, "pages/onboarding.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render onboarding page: %w", err))
		return
	}
}

// completeOnboarding handles the completion of the onboarding flow.
// It marks onboarding as complete and redirects to the main profile.
//
// Routes:
//   - GET /onboarding/complete
func (app *application) completeOnboarding(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// In a full implementation, we would mark onboarding as complete
	// For now, just redirect to profile
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

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

	user.Status = internal.UserStatusPendingResumeUpload

	if err := internal.UpdateUser(context.Background(), app.fs, user.ID, map[string]any{
		"Status": user.Status,
		"Search": firestore.Delete,
	}); err != nil {
		app.serverError(w, r, fmt.Errorf("resetOnboardingState: failed to update onboarding: %w", err))
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action": "resetOnboardingState",
			"userID": user.ID,
		})
		return
	}

	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}
