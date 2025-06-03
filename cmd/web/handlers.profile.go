package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"

	"github.com/getsentry/sentry-go"
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

	app.logger.Debug("showProfilePage", "user_id", user.ID, "status", user.State.Status)

	if user.State.Status >= internal.UserStatusCallScheduled {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Show appropriate view based on onboarding state
	switch user.State.Status {
	case internal.UserStatusPendingResumeUpload:
		app.showUploadPage(w, r)
	case internal.UserStatusProcessingResume:
		data := app.newTemplateData(r)
		if err := response.Page(w, http.StatusOK, data, "pages/profile-resume-processing.html"); err != nil {
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"action":   "render_template",
				"template": "pages/profile-resume-processing.html",
				"userID":   user.ID,
			})
			app.serverError(w, r, fmt.Errorf("failed to render resume processing page: %w", err))
		}
	case internal.UserStatusResumeUploaded:
		app.showSchedulePage(w, r)

	case internal.UserStatusError:
		// Get error message before resetting state
		data := app.newTemplateData(r)
		errorMsg := ""
		if user.State.Error != nil {
			errorMsg = *user.State.Error
		}

		// Reset state to pending resume upload
		if err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusPendingResumeUpload); err != nil {
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"action":      "update_user_status",
				"userID":      user.ID,
				"from_status": user.State.Status.String(),
				"to_status":   internal.UserStatusPendingResumeUpload.String(),
			})
			app.serverError(w, r, fmt.Errorf("failed to reset user state: %w", err))
			return
		}

		// Add error message to template data for toast notification
		data["Error"] = errorMsg
		if err := response.Page(w, http.StatusOK, data, "pages/profile-resume-upload.html"); err != nil {
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"action":   "render_template",
				"template": "pages/profile-resume-upload.html",
				"userID":   user.ID,
			})
			app.serverError(w, r, fmt.Errorf("failed to render resume upload page: %w", err))
		}
	default:
		app.logger.Warn("unknown onboarding step", "step", user.State.Status)
		// Reset to initial state for unknown states
		if err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusPendingResumeUpload); err != nil {
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"action":      "reset_user_status",
				"userID":      user.ID,
				"from_status": user.State.Status.String(),
				"to_status":   internal.UserStatusPendingResumeUpload.String(),
			})
			app.serverError(w, r, fmt.Errorf("failed to reset user state: %w", err))
			return
		}
		app.showUploadPage(w, r)
	}
}

func (app *application) showWaitingCallPage(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	data := app.newTemplateData(r)

	// Get the user's appointments
	appointments, err := internal.ListAppointments(r.Context(), app.fs, user.ID)
	if err != nil {
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action": "list_appointments",
			"userID": user.ID,
		})
		app.serverError(w, r, fmt.Errorf("failed to get appointments for user %s: %w", user.ID, err))
		return
	}

	// Find the most recent active appointment
	var activeAppointment *internal.Appointment
	for _, appt := range appointments {
		if appt.Status == internal.AppointmentStatusScheduled || appt.Status == internal.AppointmentStatusRunning {
			if activeAppointment == nil || appt.ScheduledTime.After(activeAppointment.ScheduledTime) {
				activeAppointment = appt
			}
		}
	}

	if activeAppointment == nil {
		app.logger.Error("No active appointment found", "user_id", user.ID)
		internal.SentryReportError(r.Context(), fmt.Errorf("no active appointment found"), sentry.LevelError, map[string]string{
			"action": "find_active_appointment",
			"userID": user.ID,
		})
		app.serverError(w, r, fmt.Errorf("no active appointment found"))
		return
	}

	data["Appointment"] = activeAppointment

	if err := response.Page(w, http.StatusOK, data, "pages/profile-waiting-call.html"); err != nil {
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":   "render_template",
			"template": "pages/profile-waiting-call.html",
			"userID":   user.ID,
		})
		app.serverError(w, r, fmt.Errorf("failed to render waiting call page: %w", err))
	}
}
