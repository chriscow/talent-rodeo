package main

import (
	"fmt"
	"net/http"
	"os"
	"talent-rodeo/internal"
)

// handleTestingResetUserState resets the user's state to UserStatusPendingResumeUpload
// Only available when TESTING_MODE is enabled
func (app *application) handleTestingResetUserState(w http.ResponseWriter, r *http.Request) {
	// Only allow this in testing mode
	if os.Getenv("TESTING_MODE") != "true" {
		app.clientError(w, http.StatusForbidden)
		return
	}

	// Get the current user
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.clientError(w, http.StatusUnauthorized)
		return
	}

	// Reset the user's state using the proper UpdateUserStatus function
	if err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusPendingResumeUpload); err != nil {
		app.logger.Error("failed to reset user state", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to reset user state: %w", err))
		return
	}

	// Delete all appointments for the user
	appointments, err := internal.ListAppointments(r.Context(), app.fs, user.ID)
	if err != nil {
		app.logger.Error("failed to list appointments", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to list appointments: %w", err))
		return
	}

	for _, appointment := range appointments {
		if err := internal.DeleteAppointment(r.Context(), app.fs, user.ID, appointment.ID); err != nil {
			app.logger.Error("failed to delete appointment", "error", err)
			app.reportServerError(r, fmt.Errorf("failed to delete appointment: %w", err))
			return
		}
	}

	resumes, err := internal.ListResumes(r.Context(), app.fs, user.ID)
	if err != nil {
		app.logger.Error("failed to list resumes", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to list resumes: %w", err))
		return
	}

	for _, resume := range resumes {
		if err := internal.DeleteResume(r.Context(), app.fs, user.ID, resume.ID); err != nil {
			app.logger.Error("failed to delete resume", "error", err)
			app.reportServerError(r, fmt.Errorf("failed to delete resume: %w", err))
			return
		}
	}

	app.logger.Info("reset user state for testing",
		"user_id", user.ID,
		"new_status", internal.UserStatusPendingResumeUpload)

	// Redirect back to profile
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}
