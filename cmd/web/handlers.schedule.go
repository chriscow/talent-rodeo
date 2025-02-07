package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"talent-rodeo/internal"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/onboarding"
	"talent-rodeo/internal/request"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/tasks/handlers"
	"talent-rodeo/internal/validator"
	"talent-rodeo/internal/voice"
	"time"

	"github.com/alexedwards/flow"
	"github.com/gorilla/websocket"
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

func (app *application) handleVoiceWebSocket(w http.ResponseWriter, r *http.Request) {
	app.logger.Debug("Handling voice websocket connection")

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.logger.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	// Create a new voice session
	session, err := voice.NewSession(conn, app.logger)
	if err != nil {
		app.logger.Error("Failed to create voice session", "error", err)
		return
	}
	defer session.Close()

	// Start the session
	if err := session.Start(r.Context()); err != nil {
		app.logger.Error("Voice session failed", "error", err)
	}
}

func (app *application) scheduleHandler(w http.ResponseWriter, r *http.Request) {
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

func (app *application) schedulePreferencesSubmitHandler(w http.ResponseWriter, r *http.Request) {
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

func (app *application) scheduleAppointmentHandler(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}

	app.logger.Debug("Starting appointment scheduling",
		"user_id", user.ID,
		"method", r.Method,
		"content_type", r.Header.Get("Content-Type"))

	err := r.ParseForm()
	if err != nil {
		app.logger.Error("Failed to parse form", "error", err)
		http.Error(w, fmt.Sprintf("Failed to parse form: %v", err), http.StatusBadRequest)
		return
	}

	// Log raw request details
	app.logger.Debug("Raw request details",
		"form", r.Form,
		"post_form", r.PostForm,
		"headers", r.Header)

	// Handle both form data and JSON requests
	var preferredTime string
	var timezone string

	if r.Header.Get("Content-Type") == "application/json" {
		var req struct {
			DateTime string `json:"datetime"`
			Timezone string `json:"timezone"`
		}

		// Read the raw body for logging
		body, err := io.ReadAll(r.Body)
		if err != nil {
			app.logger.Error("Failed to read request body", "error", err)
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		app.logger.Debug("Raw JSON body", "body", string(body))

		// Parse the JSON
		err = json.Unmarshal(body, &req)
		if err != nil {
			app.logger.Error("Failed to decode JSON",
				"error", err,
				"body", string(body))
			http.Error(w, fmt.Sprintf("Failed to decode JSON: %v", err), http.StatusBadRequest)
			return
		}
		preferredTime = req.DateTime
		timezone = req.Timezone
	} else {
		preferredTime = r.FormValue("datetime")
		timezone = r.FormValue("timezone")
	}

	app.logger.Debug("Parsed form values",
		"preferred_time", preferredTime,
		"timezone", timezone)

	if preferredTime == "" {
		msg := "preferred time is required"
		app.logger.Error(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	if timezone == "" {
		msg := "timezone is required"
		app.logger.Error(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Validate datetime
	scheduledTime, err := time.Parse(time.RFC3339, preferredTime)
	if err != nil {
		msg := fmt.Sprintf("invalid datetime format (expected RFC3339): %v", err)
		app.logger.Error(msg,
			"received_value", preferredTime,
			"expected_format", "2006-01-02T15:04:05Z07:00")
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Load the user's timezone
	userLoc, err := time.LoadLocation(timezone)
	if err != nil {
		msg := fmt.Sprintf("invalid timezone: %v", err)
		app.logger.Error(msg,
			"timezone", timezone)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Convert the UTC time to user's timezone
	userTime := scheduledTime.In(userLoc)
	app.logger.Debug("Time conversion",
		"original_time", scheduledTime,
		"original_location", scheduledTime.Location(),
		"user_timezone", timezone,
		"user_time", userTime,
		"user_location", userTime.Location())

	// Check if time is in the past (truncate to minutes for comparison)
	now := time.Now().In(userLoc).Truncate(time.Minute)
	userTimeMinute := userTime.Truncate(time.Minute)

	app.logger.Debug("Time comparison",
		"now_truncated", now,
		"scheduled_truncated", userTimeMinute)

	if userTimeMinute.Before(now) {
		msg := "cannot schedule appointments in the past"
		app.logger.Error(msg,
			"scheduled_time", userTimeMinute,
			"current_time", now)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Add a small buffer to ensure we're safely in the future
	userTimeMinute = userTimeMinute.Add(time.Minute)
	app.logger.Debug("Adding buffer to scheduled time",
		"original_time", userTimeMinute,
		"buffered_time", userTimeMinute)

	// Store appointment in user's timezone
	appointment := &database.Appointment{
		UserID:        user.ID,
		ScheduledTime: userTimeMinute,
		Timezone:      timezone,
		Status:        database.AppointmentStatusScheduled,
	}

	err = app.db.CreateAppointment(appointment)
	if err != nil {
		app.logger.Error("Failed to create appointment", "error", err)
		http.Error(w, fmt.Sprintf("Failed to create appointment: %v", err), http.StatusInternalServerError)
		return
	}

	// Create a phone call task
	if user.Phone == nil {
		app.logger.Error("User has no phone number", "user_id", user.ID)
		http.Error(w, "Phone number is required", http.StatusBadRequest)
		return
	}

	payload := handlers.PhoneCallPayload{
		PhoneNumber: *user.Phone,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		app.logger.Error("Failed to marshal phone call payload", "error", err)
		http.Error(w, fmt.Sprintf("Failed to create task: %v", err), http.StatusInternalServerError)
		return
	}

	task := &internal.Task{
		ID:          time.Now().UnixNano(),
		Type:        "phone_call",
		Payload:     payloadBytes,
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       userTime,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := app.processor.EnqueueTask(r.Context(), task); err != nil {
		app.logger.Error("Failed to enqueue phone call task", "error", err)
		http.Error(w, fmt.Sprintf("Failed to schedule call: %v", err), http.StatusInternalServerError)
		return
	}

	app.logger.Debug("Phone call task created",
		"task_id", task.ID,
		"run_at", task.RunAt,
		"phone_number", *user.Phone,
	)

	// Update onboarding state to indicate call is scheduled
	if err := app.db.UpdateOnboardingStep(user.ID, onboarding.StepPhoneCallScheduled); err != nil {
		app.logger.Error("Failed to update onboarding state", "error", err)
		http.Error(w, fmt.Sprintf("Failed to update onboarding state: %v", err), http.StatusInternalServerError)
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

	// For regular form submissions, redirect to profile
	http.Redirect(w, r, "/profile", http.StatusFound)
}

func (app *application) cancelAppointmentHandler(w http.ResponseWriter, r *http.Request) {
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
