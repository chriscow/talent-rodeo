package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/internal/onboarding"
	"talent-rodeo/internal/response"
)

func (app *application) scheduleCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.badRequest(w, r, fmt.Errorf("user not found"))
		return
	}

	preferredTime := r.FormValue("preferred_time")
	if preferredTime == "" {
		app.badRequest(w, r, fmt.Errorf("preferred time is required"))
		return
	}

	// Update onboarding state to indicate call is scheduled
	if err := app.db.UpdateOnboardingStep(user.ID, onboarding.StepPhoneCallScheduled); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to update onboarding state: %w", err))
		return
	}

	// TODO: Actually schedule the call with the AI system
	// For now, we'll just show the waiting screen

	data := app.newTemplateData(r)
	if err := response.Page(w, http.StatusOK, data, "pages/profile-waiting-call.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render waiting page: %w", err))
	}
}

func (app *application) rescheduleCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.badRequest(w, r, fmt.Errorf("user not found"))
		return
	}

	// Update onboarding state back to resume uploaded
	if err := app.db.UpdateOnboardingStep(user.ID, onboarding.StepResumeUploaded); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to update onboarding state: %w", err))
		return
	}

	data := app.newTemplateData(r)
	if err := response.Page(w, http.StatusOK, data, "pages/profile-phone-call.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render schedule page: %w", err))
	}
}
