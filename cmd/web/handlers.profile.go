package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/internal"
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
	user := contextGetAuthenticatedUser(r)

	// Show appropriate view based on onboarding state
	switch user.State.Status {
	case internal.UserStatusPendingResumeUpload:
		app.showUploadPage(w, r)
	case internal.UserStatusProcessingResume:
		data := app.newTemplateData(r)
		if err := response.Page(w, http.StatusOK, data, "pages/profile-resume-processing.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render resume processing page: %w", err))
		}
	case internal.UserStatusResumeUploaded:
		app.showSchedulePage(w, r)
	case internal.UserStatusIntroCallScheduled:
		app.showWaitingCallPage(w, r)
	case internal.UserStatusError:
		// Get error message before resetting state
		data := app.newTemplateData(r)
		errorMsg := ""
		if user.State.Error != nil {
			errorMsg = *user.State.Error
		}

		// Reset state to pending resume upload
		if err := internal.UpdateUserState(r.Context(), app.fs, user, internal.UserStatusPendingResumeUpload); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to reset user state: %w", err))
			return
		}

		// Add error message to template data for toast notification
		data["Error"] = errorMsg
		if err := response.Page(w, http.StatusOK, data, "pages/profile-resume-upload.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render resume upload page: %w", err))
		}
	default:
		app.logger.Warn("unknown onboarding step", "step", user.State.Status)
		// Reset to initial state for unknown states
		if err := internal.UpdateUserState(r.Context(), app.fs, user, internal.UserStatusPendingResumeUpload); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to reset user state: %w", err))
			return
		}
		app.showUploadPage(w, r)
	}
}

func (app *application) showWaitingCallPage(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	if err := response.Page(w, http.StatusOK, data, "pages/profile-waiting-call.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render waiting call page: %w", err))
	}
}
