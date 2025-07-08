package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"

	"github.com/getsentry/sentry-go"
)

// showOnboardingPage serves the unified onboarding container with HTMX
func (app *application) showOnboardingPage(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	app.logger.Debug("showOnboardingPage", "userID", user.ID, "userStatus", user.Status)

	// Get or create onboarding session
	session, err := app.onboardingSessionMgr.GetOrCreateSession(r, user.ID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get onboarding session: %w", err))
		return
	}

	// Determine current step based on user status and session
	currentStep := app.determineOnboardingStep(user, session)
	
	// Update session step if it's different
	if session.Step != currentStep {
		session.Step = currentStep
		if err := app.onboardingSessionMgr.SetSession(r, session); err != nil {
			app.logger.Error("failed to update session step", "error", err)
		}
	}

	// Build template data
	data := app.newTemplateData(r)
	data["OnboardingStep"] = string(currentStep)
	data["OnboardingStepCompleted"] = app.buildStepCompletionStatus(user, session)
	data["OnboardingSession"] = session
	
	// Check if this is an HTMX request for specific content
	if r.Header.Get("HX-Request") == "true" {
		// Return the appropriate partial template for HTMX
		templateName := ""
		switch currentStep {
		case internal.OnboardingStepResumeUpload:
			templateName = "partial:onboarding-resume-upload"
		case internal.OnboardingStepResumeProcessing:
			templateName = "partial:onboarding-resume-processing"
		case internal.OnboardingStepCommunicationChoice:
			templateName = "partial:onboarding-communication-choice"
		case internal.OnboardingStepActiveConversation:
			templateName = "partial:onboarding-active-conversation"
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Unknown step"))
			return
		}

		if err := response.NamedTemplate(w, http.StatusOK, data, templateName, "partials/*.html", "shared/*.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render partial %s: %w", templateName, err))
			return
		}
		return
	}

	// For non-HTMX requests, don't set OnboardingStepContent - let the template handle it directly based on step

	if err := response.Page(w, http.StatusOK, data, "pages/onboarding-main.html"); err != nil {
		app.serverError(w, r, err)
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":   "render_template",
			"template": "pages/profile-onboarding.html",
			"userID":   user.ID,
		})
	}
}

// determineOnboardingStep determines the current step based on user status and session
func (app *application) determineOnboardingStep(user *internal.User, session *internal.OnboardingSession) internal.OnboardingStep {
	switch user.Status {
	case internal.UserStatusPendingResumeUpload:
		return internal.OnboardingStepResumeUpload
	case internal.UserStatusProcessingResume:
		return internal.OnboardingStepResumeProcessing
	case internal.UserStatusResumeUploaded:
		return internal.OnboardingStepCommunicationChoice
	case internal.UserStatusCallScheduled, internal.UserStatusCallStarted:
		return internal.OnboardingStepActiveConversation
	default:
		// Default to resume upload if we can't determine
		return internal.OnboardingStepResumeUpload
	}
}

// buildStepCompletionStatus builds the completion status for the progress indicator
func (app *application) buildStepCompletionStatus(user *internal.User, session *internal.OnboardingSession) map[string]bool {
	status := map[string]bool{
		"ResumeUpload":        user.Status >= internal.UserStatusProcessingResume,
		"ResumeProcessing":    user.Status >= internal.UserStatusResumeUploaded,
		"CommunicationChoice": user.Status >= internal.UserStatusCallScheduled,
		"Conversation":        user.Status >= internal.UserStatusCallCompleted,
	}
	
	return status
}


// handleOnboardingStepRequest handles HTMX requests for specific onboarding steps
func (app *application) handleOnboardingStepRequest(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Get current session
	session, err := app.onboardingSessionMgr.GetSession(r)
	if err != nil {
		app.logger.Error("failed to get onboarding session", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if session == nil {
		// No session, redirect to main onboarding page
		w.Header().Set("HX-Redirect", "/onboarding")
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	// Determine current step
	currentStep := app.determineOnboardingStep(user, session)
	
	// Build template data
	data := app.newTemplateData(r)
	data["User"] = user
	data["OnboardingSession"] = session
	data["OnboardingStep"] = string(currentStep)
	data["OnboardingStepCompleted"] = app.buildStepCompletionStatus(user, session)
	
	// Return the appropriate partial template for HTMX
	templateName := ""
	switch currentStep {
	case internal.OnboardingStepResumeUpload:
		templateName = "partial:onboarding-resume-upload"
	case internal.OnboardingStepResumeProcessing:
		templateName = "partial:onboarding-resume-processing"
	case internal.OnboardingStepCommunicationChoice:
		templateName = "partial:onboarding-communication-choice"
	case internal.OnboardingStepActiveConversation:
		templateName = "partial:onboarding-active-conversation"
	default:
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Unknown step"))
		return
	}

	if err := response.NamedTemplate(w, http.StatusOK, data, templateName, "partials/*.html", "shared/*.html"); err != nil {
		app.logger.Error("failed to render step content", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// startChatFromOnboarding handles starting a chat from the onboarding flow
func (app *application) startChatFromOnboarding(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Get current session
	session, err := app.onboardingSessionMgr.GetSession(r)
	if err != nil {
		app.logger.Error("failed to get onboarding session", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if session == nil {
		// No session, redirect to main onboarding page
		w.Header().Set("HX-Redirect", "/onboarding")
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	// Create a new chat appointment directly (don't call startChat since it returns JSON)
	appointment := &internal.Appointment{
		Name:          "initial-chat",
		Type:          internal.AppointmentTypeChat,
		ScheduledTime: time.Now(),
		Status:        internal.AppointmentStatusRunning,
		ChatMessages:  []internal.ChatMessage{},
	}

	if err := internal.CreateAppointment(r.Context(), app.fs, user.ID, appointment); err != nil {
		app.logger.Error("Failed to create chat appointment", "error", err, "user_id", user.ID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Update user status to indicate chat started
	if err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusCallStarted); err != nil {
		app.logger.Error("Failed to update user status for chat", "error", err, "user_id", user.ID)
		// Continue despite error - chat session is already created
	}

	// Update onboarding session
	if err := app.onboardingSessionMgr.SetConversationType(r, "chat"); err != nil {
		app.logger.Error("failed to set conversation type", "error", err)
	}
	if err := app.onboardingSessionMgr.UpdateStep(r, internal.OnboardingStepActiveConversation); err != nil {
		app.logger.Error("failed to update onboarding step", "error", err)
	}

	// Redirect to dedicated chat page
	w.Header().Set("HX-Redirect", fmt.Sprintf("/chat?appointment_id=%s", appointment.ID))
	w.WriteHeader(http.StatusSeeOther)
}

// resetOnboardingState resets the user's onboarding state and session
func (app *application) resetOnboardingState(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	app.logger.Debug("resetOnboardingState", "userID", user.ID, "currentStatus", user.Status)

	// Reset user status to pending resume upload
	if err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusPendingResumeUpload); err != nil {
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":      "reset_user_status",
			"userID":      user.ID,
			"from_status": user.Status.String(),
			"to_status":   internal.UserStatusPendingResumeUpload.String(),
		})
		app.serverError(w, r, fmt.Errorf("failed to reset user state: %w", err))
		return
	}

	// Clear onboarding session
	if err := app.onboardingSessionMgr.ClearSession(r); err != nil {
		app.logger.Error("failed to clear onboarding session", "error", err)
		// Don't fail the request if we can't clear the session
	}

	// Set success message
	app.sessionManager.Put(r.Context(), "flash", "Your onboarding has been reset. You can start over.")

	// Redirect to profile (which will show the onboarding flow)
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

// returnHTMXError returns an error response for HTMX requests
func (app *application) returnHTMXError(w http.ResponseWriter, r *http.Request, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	
	errorResponse := map[string]string{
		"message": message,
	}
	
	response, _ := json.Marshal(errorResponse)
	w.Write(response)
}

// getUserStatus returns the current user status as JSON
func (app *application) getUserStatus(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"status":     int(user.Status),
		"statusName": user.Status.String(),
		"userID":     user.ID,
	}
	
	json.NewEncoder(w).Encode(response)
}

