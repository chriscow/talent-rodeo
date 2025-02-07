package main

import (
	"fmt"
	"net/http"
	"strconv"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/handlers"
	"talent-rodeo/internal/onboarding"
	"talent-rodeo/internal/request"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/validator"

	"github.com/alexedwards/flow"
)

type scheduleFormData struct {
	Timezone           string   `form:"timezone"`
	PreferredDays      []string `form:"preferred_days"`
	PreferredTimeStart string   `form:"preferred_time_start"`
	PreferredTimeEnd   string   `form:"preferred_time_end"`
	validator.Validator
}

// showSchedulePage displays the scheduling interface where users can set their
// availability preferences and view/manage their appointments.
//
// Flow:
//  1. Verifies user authentication
//  2. Loads existing scheduling preferences
//  3. Loads upcoming appointments
//  4. Handles websocket connection for voice if on voice route
//
// Routes:
//   - GET /profile/schedule
//   - GET /schedule/voice (websocket endpoint)
//
// Templates:
//   - pages/schedule.html
//
// State:
//   - Does not modify onboarding state
//   - Accessible after StepResumeUploaded
func (app *application) showSchedulePage(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
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

	// Add websocket endpoint for voice
	if r.URL.Path == "/schedule/voice" {
		app.handleVoiceWebSocket(w, r)
		return
	}

	err = response.Page(w, http.StatusOK, data, "pages/schedule.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}

// updateSchedulePreferences handles POST requests to update a user's scheduling preferences.
// These preferences are used to determine available time slots for appointments.
//
// Flow:
//  1. Validates form data (timezone, days, time range)
//  2. Updates preferences in database
//  3. Redirects back to schedule page
//
// Routes:
//   - POST /profile/schedule/preferences
//
// Templates:
//   - pages/schedule.html (on validation error)
//
// State:
//   - Does not modify onboarding state
//   - Accessible after StepResumeUploaded
func (app *application) updateSchedulePreferences(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}

	err := r.ParseForm()
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	form := scheduleFormData{}
	err = request.DecodePostForm(r, &form)
	if err != nil {
		app.badRequest(w, r, err)
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
		err = response.Page(w, http.StatusBadRequest, data, "pages/schedule.html")
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

	http.Redirect(w, r, "/profile/schedule", http.StatusFound)
}

// createAppointment handles POST requests to schedule a new appointment.
// It validates the requested time, creates the appointment, and schedules
// the voice call.
//
// Flow:
//  1. Validates user has phone number
//  2. Validates requested time
//  3. Creates appointment record
//  4. Schedules voice call
//  5. Updates onboarding state
//  6. Returns appropriate view based on request type
//
// Routes:
//   - POST /profile/schedule/appointment
//
// Templates:
//   - partials/waiting-for-call.html (HTMX request)
//
// State Transitions:
//   - StepResumeUploaded → StepPhoneCallScheduled
func (app *application) createAppointment(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}

	app.logger.Debug("Starting appointment scheduling",
		"user_id", user.ID,
		"method", r.Method,
		"content_type", r.Header.Get("Content-Type"))

	// Use the request decoder
	scheduleReq, err := request.DecodeScheduleRequest(w, r)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	// Check phone number
	if user.Phone == nil {
		app.logger.Error("User has no phone number", "user_id", user.ID)
		app.badRequest(w, r, fmt.Errorf("phone number is required"))
		return
	}

	// Validate time using helper
	timeResult, err := request.ValidateScheduledTime(scheduleReq.DateTime, scheduleReq.Timezone)
	if err != nil {
		app.logger.Error("Time validation failed", "error", err)
		app.badRequest(w, r, err)
		return
	}

	// Create appointment using validated time
	appointment := &database.Appointment{
		UserID:        user.ID,
		ScheduledTime: timeResult.UserTime,
		Timezone:      scheduleReq.Timezone,
		Status:        database.AppointmentStatusScheduled,
	}

	err = app.db.CreateAppointment(appointment)
	if err != nil {
		app.logger.Error("Failed to create appointment", "error", err)
		http.Error(w, fmt.Sprintf("Failed to create appointment: %v", err), http.StatusInternalServerError)
		return
	}

	// Create voice call request
	callRequest := handlers.CallRequest{
		PhoneNumber: *user.Phone,
	}

	app.logger.Debug("Scheduling voice call",
		"phone_number", *user.Phone,
		"scheduled_time", timeResult.UserTime,
		"timezone", scheduleReq.Timezone,
	)

	// Schedule the call
	err = app.scheduler.ScheduleMessage(
		r.Context(),
		handlers.TopicVoiceInput,
		callRequest,
		map[string]string{
			"appointment_id": fmt.Sprintf("%d", appointment.ID),
			"user_id":        fmt.Sprintf("%d", user.ID),
		},
		timeResult.UserTime,
		app.processor,
	)
	if err != nil {
		app.logger.Error("Failed to schedule voice call",
			"error", err,
			"phone_number", *user.Phone,
		)
		http.Error(w, fmt.Sprintf("Failed to schedule call: %v", err), http.StatusInternalServerError)
		return
	}

	// Update onboarding state
	if err := app.db.UpdateOnboardingStep(user.ID, onboarding.StepPhoneCallScheduled); err != nil {
		app.logger.Error("Failed to update onboarding state", "error", err)
		http.Error(w, fmt.Sprintf("Failed to update onboarding state: %v", err), http.StatusInternalServerError)
		return
	}

	// Handle response based on request type
	if request.IsHTMXRequest(r) {
		data := app.newTemplateData(r)
		err = response.Page(w, http.StatusOK, data, "partials/waiting-for-call.html")
		if err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	http.Redirect(w, r, "/profile", http.StatusFound)
}

// rescheduleAppointment handles POST requests to reschedule an appointment.
// It resets the onboarding state back to resume uploaded and shows the
// scheduling page.
//
// Flow:
//  1. Verifies user authentication
//  2. Resets onboarding state to resume uploaded
//  3. Shows scheduling page for new appointment
//
// Routes:
//   - POST /profile/schedule/reschedule
//
// Templates:
//   - pages/profile-schedule-phone-call.html
//
// State Transitions:
//   - StepPhoneCallScheduled → StepResumeUploaded
func (app *application) rescheduleAppointment(w http.ResponseWriter, r *http.Request) {
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
	if err := response.Page(w, http.StatusOK, data, "pages/profile-schedule-phone-call.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render schedule page: %w", err))
	}
}

// cancelAppointment handles DELETE requests to cancel an existing appointment.
// It marks the appointment as cancelled in the database.
//
// Flow:
//  1. Verifies user authentication and appointment ownership
//  2. Updates appointment status to cancelled
//
// Routes:
//   - DELETE /profile/schedule/appointment/:id
//
// State:
//   - Does not modify onboarding state
//   - May require manual state reset if no other appointments exist
func (app *application) cancelAppointment(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}

	idParam := flow.Param(r.Context(), "id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id < 1 {
		app.badRequest(w, r, fmt.Errorf("invalid appointment ID"))
		return
	}

	appointment, err := app.db.GetAppointment(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if appointment == nil || appointment.UserID != user.ID {
		http.NotFound(w, r)
		return
	}

	err = app.db.UpdateAppointmentStatus(id, database.AppointmentStatusCancelled)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
