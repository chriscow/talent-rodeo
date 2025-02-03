package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/response"
)

func (app *application) InterviewPage(w http.ResponseWriter, r *http.Request) {
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

func (app *application) InterviewStart(w http.ResponseWriter, r *http.Request) {
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

func (app *application) InterviewEnd(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Mark the interview as completed
	w.WriteHeader(http.StatusOK)
}

func (app *application) HandleAudio(w http.ResponseWriter, r *http.Request) {
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
