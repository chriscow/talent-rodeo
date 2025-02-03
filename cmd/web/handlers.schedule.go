package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/onboarding"
	"talent-rodeo/internal/request"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/validator"
	"time"

	"github.com/alexedwards/flow"
)

type scheduleFormData struct {
	Timezone           string   `form:"timezone"`
	PreferredDays      []string `form:"preferred_days"`
	PreferredTimeStart string   `form:"preferred_time_start"`
	PreferredTimeEnd   string   `form:"preferred_time_end"`
	validator.Validator
}

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

func (app *application) scheduleHandler(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	prefs, err := app.db.GetSchedulingPreferences(user.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data["Preferences"] = prefs

	// Get upcoming appointments
	appointments, err := app.db.GetUserAppointments(user.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data["Appointments"] = appointments

	err = response.Page(w, http.StatusOK, data, "pages/schedule.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) schedulePreferencesSubmitHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	form := scheduleFormData{}
	err = request.DecodePostForm(r, &form)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Validate form data
	form.CheckField(form.Timezone != "", "timezone", "Timezone is required")
	form.CheckField(len(form.PreferredDays) > 0, "preferred_days", "Please select at least one preferred day")
	form.CheckField(form.PreferredTimeStart != "", "preferred_time_start", "Start time is required")
	form.CheckField(form.PreferredTimeEnd != "", "preferred_time_end", "End time is required")

	if form.HasErrors() {
		data := app.newTemplateData(r)
		data["Form"] = form
		err = response.Page(w, http.StatusUnprocessableEntity, data, "pages/schedule.html")
		if err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	prefs := &database.SchedulingPreferences{
		UserID:             user.ID,
		Timezone:           form.Timezone,
		PreferredDays:      form.PreferredDays,
		PreferredTimeStart: form.PreferredTimeStart,
		PreferredTimeEnd:   form.PreferredTimeEnd,
	}

	err = app.db.UpsertSchedulingPreferences(prefs)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	session, err := app.sessionStore.Get(r, "session")
	if err == nil {
		session.AddFlash("Scheduling preferences updated successfully")
		session.Save(r, w)
	}

	http.Redirect(w, r, "/schedule", http.StatusSeeOther)
}

func (app *application) scheduleAppointmentHandler(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		DateTime string `json:"datetime"`
		Timezone string `json:"timezone"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	scheduledTime, err := time.Parse(time.RFC3339, req.DateTime)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	appointment := &database.Appointment{
		UserID:        user.ID,
		ScheduledTime: scheduledTime,
		Timezone:      req.Timezone,
		Status:        database.AppointmentStatusScheduled,
	}

	err = app.db.CreateAppointment(appointment)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Update onboarding state to indicate call is scheduled
	if err := app.db.UpdateOnboardingStep(user.ID, onboarding.StepPhoneCallScheduled); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to update onboarding state: %w", err))
		return
	}

	// If it's an HTMX request, return the waiting partial
	if r.Header.Get("HX-Request") == "true" {
		data := app.newTemplateData(r)
		err = response.Page(w, http.StatusOK, data, "partials/waiting-for-call.html")
		if err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	// For non-HTMX requests, return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":            appointment.ID,
		"scheduledTime": appointment.ScheduledTime,
		"timezone":      appointment.Timezone,
		"status":        appointment.Status,
	})
}

func (app *application) cancelAppointmentHandler(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idParam := flow.Param(r.Context(), "id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id < 1 {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	appointment, err := app.db.GetAppointment(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if appointment == nil || appointment.UserID != user.ID {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	err = app.db.UpdateAppointmentStatus(id, database.AppointmentStatusCancelled)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
