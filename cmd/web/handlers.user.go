package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/internal/onboarding"
	"talent-rodeo/internal/response"
)

func (app *application) profile(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
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
			template = "pages/profile-phone-call.html"
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
		return
	}

	if r.Method == http.MethodPost {
		app.handleResume(w, r)
		return
	}
}
