package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/tasks/handlers"
	"time"

	"github.com/alexedwards/flow"
)

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
		http.Redirect(w, r, "/auth/linkedin", http.StatusSeeOther)
		return
	}

	// Check if user is in error state - redirect to resume upload
	if user.State.Status == internal.UserStatusError {
		app.logger.Info("User in error state, redirecting to resume upload", "user_id", user.ID)

		// Get error message if available
		errorMsg := "There was a problem with your resume. Please upload a new one."
		if user.State.Error != nil {
			errorMsg = *user.State.Error
		}

		// Add flash message
		app.sessionManager.Put(r.Context(), "flash", errorMsg)

		http.Redirect(w, r, "/profile/resume/upload", http.StatusSeeOther)
		return
	}

	data := app.newTemplateData(r)

	// Only set the minimum date time
	now := time.Now().UTC()
	data["MinDateTime"] = now.Format("2006-01-02T15:04")

	if err := response.Page(w, http.StatusOK, data, "pages/profile-schedule-phone-call.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render schedule page: %w", err))
	}
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
		http.Redirect(w, r, "/auth/linkedin", http.StatusSeeOther)
		return
	}

	if os.Getenv("VAPI_START_CALL_PATH") == "" {
		app.serverError(w, r, fmt.Errorf("VAPI_START_CALL_PATH is not set"))
		return
	}

	app.logger.Debug("Starting appointment scheduling",
		"user_id", user.ID,
		"method", r.Method,
		"content_type", r.Header.Get("Content-Type"))

	// Parse form data
	err := r.ParseForm()
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	datetime := r.FormValue("datetime")
	timezone := r.FormValue("timezone")

	// Check required fields
	if datetime == "" {
		app.badRequest(w, r, fmt.Errorf("datetime is required"))
		return
	}
	if timezone == "" {
		app.badRequest(w, r, fmt.Errorf("timezone is required"))
		return
	}

	// Parse the datetime string
	scheduledTime, err := time.Parse("2006-01-02T15:04", datetime)
	if err != nil {
		app.badRequest(w, r, fmt.Errorf("invalid date format"))
		return
	}

	// Check if time is in the past
	if scheduledTime.Before(time.Now()) {
		app.badRequest(w, r, fmt.Errorf("cannot schedule appointments in the past"))
		return
	}

	// Check phone number
	if user.Phone == nil {
		app.logger.Error("User has no phone number", "user_id", user.ID)
		app.badRequest(w, r, fmt.Errorf("phone number is required"))
		return
	}

	// Create appointment
	appointment := &internal.Appointment{
		UserID:        user.ID,
		Name:          "initial-call",
		ScheduledTime: scheduledTime.UTC(),
		Timezone:      timezone,
		Status:        internal.AppointmentStatusScheduled,
	}

	err = internal.CreateAppointment(r.Context(), app.fs, user.ID, appointment)
	if err != nil {
		app.logger.Error("Failed to create appointment", "error", err)
		http.Error(w, fmt.Sprintf("Failed to create appointment: %v", err), http.StatusInternalServerError)
		return
	}

	// Create simple call request for webhook
	callRequest := struct {
		CustomerID string `json:"customer_id"`
	}{
		CustomerID: user.ID,
	}

	body, err := json.Marshal(callRequest)
	if err != nil {
		app.logger.Error("Failed to marshal call request", "error", err)
		http.Error(w, fmt.Sprintf("Failed to marshal call request: %v", err), http.StatusInternalServerError)
		return
	}

	app.logger.Debug("scheduling voice call",
		"phone_number", *user.Phone,
		"scheduled_time", scheduledTime.UTC(),
		"timezone", timezone,
	)

	// Create webhook payload
	payload := handlers.WebhookPayload{
		URL:    buildWebhookURL(os.Getenv("VAPI_START_CALL_PATH")),
		Method: "POST",
		Body:   body,
	}
	payloadBytes, _ := json.Marshal(payload)

	_, err = app.processor.CreateAndEnqueueTask(r.Context(), handlers.TaskTypeWebhookCall, "initial-call", payloadBytes, scheduledTime.UTC())
	if err != nil {
		app.logger.Error("Failed to create and enqueue task", "error", err)
		http.Error(w, fmt.Sprintf("Failed to create and enqueue task: %v", err), http.StatusInternalServerError)
		return
	}

	// Update onboarding state
	err = internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusCallScheduled)
	if err != nil {
		app.logger.Error("Failed to update onboarding state", "error", err)
		http.Error(w, fmt.Sprintf("Failed to update onboarding state: %v", err), http.StatusInternalServerError)
		return
	}

	// Always redirect to /profile on success
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

func buildWebhookURL(callPath string) string {
	baseURL := os.Getenv("BASE_URL")

	// Ensure base URL doesn't end with slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Ensure call path starts with slash
	if !strings.HasPrefix(callPath, "/") {
		callPath = "/" + callPath
	}

	return baseURL + callPath
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
	err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusResumeUploaded)
	if err != nil {
		app.logger.Error("Failed to update onboarding state", "error", err)
		http.Error(w, fmt.Sprintf("Failed to update onboarding state: %v", err), http.StatusInternalServerError)
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
		http.Redirect(w, r, "/auth/linkedin", http.StatusSeeOther)
		return
	}

	id := flow.Param(r.Context(), "id")

	// Update to pass both userID and appointmentID
	err := internal.CancelAppointment(r.Context(), app.fs, user.ID, id)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to cancel appointment: %w", err))
		return
	}

	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

// callNow handles POST requests to initiate an immediate call.
// It creates an appointment for the current time and schedules
// the voice call to start immediately.
//
// Flow:
//  1. Validates user has phone number
//  2. Creates appointment record for now
//  3. Schedules immediate voice call
//  4. Updates onboarding state
//  5. Returns appropriate view based on request type
//
// Routes:
//   - POST /profile/schedule/call-now
//
// Templates:
//   - partials/waiting-for-call.html (HTMX request)
//
// State Transitions:
//   - StepResumeUploaded → StepPhoneCallScheduled
func (app *application) callNow(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusSeeOther)
		return
	}

	app.logger.Debug("Starting immediate call",
		"user_id", user.ID,
		"method", r.Method,
		"content_type", r.Header.Get("Content-Type"))

	// Check phone number
	if user.Phone == nil {
		app.logger.Error("User has no phone number", "user_id", user.ID)
		app.reportServerError(r, fmt.Errorf("user has no phone number"))
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}

	// Schedule the call to happen in 1 second to avoid "in the past" error
	scheduledTime := time.Now().UTC().Add(1 * time.Second)

	// Create appointment
	appointment := &internal.Appointment{
		UserID:        user.ID,
		Name:          "initial-call",
		ScheduledTime: scheduledTime,
		Timezone:      "UTC", // Immediate calls don't need specific timezone
		Status:        internal.AppointmentStatusScheduled,
	}

	err := internal.CreateAppointment(r.Context(), app.fs, user.ID, appointment)
	if err != nil {
		app.logger.Error("Failed to create appointment", "error", err)
		app.serverError(w, r, fmt.Errorf("failed to create appointment: %w", err))
		return
	}

	app.logger.Debug("handlers.schedule::callNow", "appointment", appointment)

	body, err := json.Marshal(appointment)
	if err != nil {
		app.logger.Error("Failed to marshal call request", "error", err)
		app.serverError(w, r, fmt.Errorf("failed to marshal call request: %w", err))
		return
	}

	app.logger.Debug("Scheduling voice call",
		"phone_number", *user.Phone,
		"scheduled_time", scheduledTime.UTC(),
		"timezone", "UTC",
	)

	// Create webhook payload
	payload := handlers.WebhookPayload{
		URL:    buildWebhookURL(os.Getenv("VAPI_START_CALL_PATH")),
		Method: "POST",
		Body:   body,
	}
	payloadBytes, _ := json.Marshal(payload)

	_, err = app.processor.CreateAndEnqueueTask(r.Context(), handlers.TaskTypeWebhookCall, "initial-call", payloadBytes, scheduledTime)
	if err != nil {
		app.logger.Error("Failed to create and enqueue task", "error", err)
		app.serverError(w, r, fmt.Errorf("failed to create and enqueue task: %w", err))
		return
	}

	app.logger.Debug("Voice call scheduled",
		"user_id", user.ID,
		"phone_number", *user.Phone)

	// Update onboarding state
	err = internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusCallScheduled)
	if err != nil {
		app.logger.Error("Failed to update onboarding state",
			"error", err,
			"user_id", user.ID,
		)
		app.reportServerError(r, fmt.Errorf("failed to update onboarding state: %w", err))
		// Don't return error here since the call is already scheduled
	}

	app.logger.Debug("Redirecting to profile",
		"user_id", user.ID)

	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}
