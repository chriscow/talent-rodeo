package main

import (
	"fmt"
	"net/http"
	"os"
	"talent-rodeo/internal"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/response"
	"time"
)

func (app *application) adminVoice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminUser := contextGetAuthenticatedUser(r)
	data := app.newTemplateData(r)
	data["VapiAPIKey"] = os.Getenv("VAPI_PUBLIC_API_KEY")
	data["AssistantID"] = os.Getenv("VAPI_ASSISTANT_ID")

	// Fetch all users for the dropdown
	users, err := internal.ListUsers(ctx, app.fs, 100, 0) // Limiting to 100 users for now
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data["Users"] = users

	// Handle POST request for user selection
	if r.Method == http.MethodPost {
		app.logger.Info("handling POST request", "url", r.URL.String())

		if err := r.ParseForm(); err != nil {
			app.logger.Error("failed to parse form", "error", err)
			app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
			return
		}

		selectedUserID := r.PostForm.Get("user_id")
		app.logger.Info("received form data", "user_id", selectedUserID)

		if selectedUserID == "" {
			app.logger.Error("no user selected")
			http.Error(w, "No user selected", http.StatusBadRequest)
			return
		}

		data["SelectedUserID"] = selectedUserID

		// Handle the "no user" case
		if selectedUserID == "_none" {
			app.logger.Info("handling _none user case")
			// Build assistant overrides with nil user and nil resume
			overrides, err := internal.BuildAssistantOverrides(ctx, nil, nil, app.fs, app.logger)
			if err != nil {
				app.logger.Error("failed to build assistant overrides", "error", err)
				app.serverError(w, r, fmt.Errorf("failed to build assistant overrides: %w", err))
				return
			}
			data["AssistantOverrides"] = overrides
		} else {
			app.logger.Info("fetching user details", "user_id", selectedUserID)
			// Fetch the selected user's details
			selectedUser, found, err := internal.GetUser(ctx, app.fs, selectedUserID)
			if err != nil {
				app.logger.Error("failed to get user", "error", err, "user_id", selectedUserID)
				app.serverError(w, r, fmt.Errorf("failed to get user: %w", err))
				return
			}
			if !found {
				app.logger.Error("user not found", "user_id", selectedUserID)
				http.Error(w, "User not found", http.StatusNotFound)
				return
			}

			// Fetch the user's latest resume
			resume, _, err := internal.GetLatestResume(ctx, app.fs, selectedUserID)
			if err != nil {
				app.logger.Warn("failed to get resume", "error", err, "user_id", selectedUserID)
				// Continue without resume - it's optional
			}

			// Build assistant overrides for the selected user
			overrides, err := internal.BuildAssistantOverrides(ctx, selectedUser, resume, app.fs, app.logger)
			if err != nil {
				app.logger.Error("failed to build assistant overrides", "error", err, "user_id", selectedUserID)
				app.serverError(w, r, fmt.Errorf("failed to build assistant overrides: %w", err))
				return
			}

			// call.name is the appointment ID
			// call.customer.name is the customer ID
			apptName := funcs.Slugify(fmt.Sprintf("admin-%s", adminUser.Name))
			appt := &internal.Appointment{
				UserID:        selectedUser.ID,
				ScheduledTime: time.Now(),
				Status:        internal.AppointmentStatusRunning,
			}
			if err := internal.CreateAppointment(ctx, app.fs, apptName, *selectedUser, appt); err != nil {
				app.logger.Error("failed to create appointment", "error", err, "user_id", selectedUserID)
				app.serverError(w, r, fmt.Errorf("failed to create appointment: %w", err))
				return
			}

			data["CallName"] = appt.ID
			data["CallCustomerName"] = selectedUser.ID
			data["AssistantOverrides"] = overrides
			data["Prompt"] = overrides.Model.Messages[0].Content
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/voice.html"); err != nil {
		app.logger.Error("failed to render page", "error", err)
		app.serverError(w, r, err)
	}
}
