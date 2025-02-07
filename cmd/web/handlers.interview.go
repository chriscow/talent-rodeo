package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/response"
)

// showInterviewPage displays the main interview page where users can start their interview.
// It performs several prerequisite checks:
// 1. Verifies user authentication, redirecting to login if not authenticated
// 2. Checks for resume upload, redirecting to upload page if missing
// 3. Validates scheduled appointment, redirecting to scheduling if none exists
//
// Routes:
//   - GET /interview
//
// Templates:
//   - pages/interview.html
func (app *application) showInterviewPage(w http.ResponseWriter, r *http.Request) {
	app.logger.Info("Handling interview page request", "method", r.Method, "url", r.URL.String())

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.logger.Info("User not authenticated, redirecting to login")
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}

	// Check if user has uploaded a resume
	if user.ResumeFilename == nil || *user.ResumeFilename == "" {
		app.logger.Info("User has no resume, redirecting to upload", "user_id", user.ID)
		http.Redirect(w, r, "/profile/resume/upload", http.StatusFound)
		return
	}

	// Check if user has a scheduled appointment
	appointments, err := app.db.GetUserAppointments(user.ID)
	if err != nil {
		app.logger.Error("Failed to get user appointments", "error", err, "user_id", user.ID)
		app.serverError(w, r, err)
		return
	}

	hasScheduledAppointment := false
	for _, appointment := range appointments {
		if appointment.Status == database.AppointmentStatusScheduled {
			hasScheduledAppointment = true
			break
		}
	}

	if !hasScheduledAppointment {
		app.logger.Info("User has no scheduled appointment, redirecting to schedule", "user_id", user.ID)
		http.Redirect(w, r, "/profile/schedule", http.StatusFound)
		return
	}

	data := app.newTemplateData(r)
	if err := response.Page(w, http.StatusOK, data, "pages/interview.html"); err != nil {
		app.logger.Error("Failed to render interview page", "error", err)
		app.serverError(w, r, fmt.Errorf("failed to render interview page: %w", err))
	}
}

// startInterview initiates the interview process by displaying the interview start page.
// It performs the same prerequisite checks as showInterviewPage but returns errors instead
// of redirecting, as this endpoint is typically called via AJAX.
//
// Routes:
//   - GET /interview/start
//
// Templates:
//   - pages/interview-start.html
func (app *application) startInterview(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if user has uploaded a resume
	if user.ResumeFilename == nil || *user.ResumeFilename == "" {
		http.Error(w, "Resume required", http.StatusBadRequest)
		return
	}

	// Check if user has a scheduled appointment
	appointments, err := app.db.GetUserAppointments(user.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	hasScheduledAppointment := false
	for _, appointment := range appointments {
		if appointment.Status == database.AppointmentStatusScheduled {
			hasScheduledAppointment = true
			break
		}
	}

	if !hasScheduledAppointment {
		http.Error(w, "Scheduled appointment required", http.StatusBadRequest)
		return
	}

	data := app.newTemplateData(r)
	if err := response.Page(w, http.StatusOK, data, "pages/interview-start.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render interview start page: %w", err))
	}
}

// completeInterview marks the interview as completed. Currently a placeholder
// that will be expanded when interview completion logic is implemented.
//
// Routes:
//   - POST /interview/end
func (app *application) completeInterview(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Mark the interview as completed
	w.WriteHeader(http.StatusOK)
}

// handleAudioUpload processes audio recordings from the interview.
// Currently a placeholder that will be implemented when the audio
// interview feature is added.
//
// Routes:
//   - POST /interview/audio
//
// Expected request:
//   - Audio data in the request body (format TBD)
func (app *application) handleAudioUpload(w http.ResponseWriter, r *http.Request) {
	app.logger.Info("Handling audio request", "method", r.Method, "url", r.URL.String())

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.logger.Info("User not authenticated")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodPost {
		app.logger.Info("Invalid method", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if user has uploaded a resume
	if user.ResumeFilename == nil || *user.ResumeFilename == "" {
		app.logger.Info("User has no resume", "user_id", user.ID)
		http.Error(w, "Resume required", http.StatusBadRequest)
		return
	}

	// Check if user has a scheduled appointment
	appointments, err := app.db.GetUserAppointments(user.ID)
	if err != nil {
		app.logger.Error("Failed to get user appointments", "error", err, "user_id", user.ID)
		app.serverError(w, r, err)
		return
	}

	hasScheduledAppointment := false
	for _, appointment := range appointments {
		if appointment.Status == database.AppointmentStatusScheduled {
			hasScheduledAppointment = true
			break
		}
	}

	if !hasScheduledAppointment {
		app.logger.Info("User has no scheduled appointment", "user_id", user.ID)
		http.Error(w, "Scheduled appointment required", http.StatusBadRequest)
		return
	}

	// TODO: Implement audio handling
	// This will be implemented when we add the audio interview feature
	app.logger.Info("Audio handling not implemented yet")
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}
