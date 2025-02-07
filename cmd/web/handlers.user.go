package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/internal/onboarding"
	"talent-rodeo/internal/response"
)

// showProfilePage handles GET requests to /profile.
// It displays different templates based on the user's onboarding state:
//   - StepInitial: profile-resume-upload.html
//   - StepResumeUploaded: profile-schedule-phone-call.html
//   - StepPhoneCallScheduled: profile-waiting-call.html
//   - StepPhoneCallCompleted: profile-preferences.html
//   - StepPreferencesExtracted: profile-job-search.html
//   - StepJobSearchStarted/Complete: profile-jobs.html
//   - Default: profile.html
//
// Requires: Authenticated user
func (app *application) showProfilePage(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	user := contextGetAuthenticatedUser(r)

	// Get current onboarding state
	state, err := app.db.GetOnboardingState(user.ID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get onboarding state: %w", err))
		return
	}

	data["OnboardingState"] = state

	// Safe logging of timestamps
	startedAt := "nil"
	if state.StartedAt != nil {
		startedAt = state.StartedAt.String()
	}
	completedAt := "nil"
	if state.CompletedAt != nil {
		completedAt = state.CompletedAt.String()
	}

	app.logger.Debug("profile handler",
		"user_id", user.ID,
		"onboarding_step", state.CurrentStep,
		"is_complete", state.IsComplete(),
		"started_at", startedAt,
		"completed_at", completedAt)

	// Show appropriate view based on onboarding state
	var template string
	switch state.CurrentStep {
	case onboarding.StepInitial:
		template = "pages/profile-resume-upload.html"
		app.logger.Debug("showing resume upload template")
	case onboarding.StepResumeUploaded:
		template = "pages/profile-schedule-phone-call.html"
	case onboarding.StepPhoneCallScheduled:
		template = "pages/profile-waiting-call.html"
	case onboarding.StepPhoneCallCompleted:
		template = "pages/profile-preferences.html"
	case onboarding.StepPreferencesExtracted:
		template = "pages/profile-job-search.html"
	case onboarding.StepJobSearchStarted, onboarding.StepComplete:
		template = "pages/profile-jobs.html"
	default:
		template = "pages/profile.html"
		app.logger.Warn("unknown onboarding step", "step", state.CurrentStep)
	}

	app.logger.Debug("selected template", "template", template)

	if err := response.Page(w, http.StatusOK, data, template); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render profile page: %w", err))
	}
}
