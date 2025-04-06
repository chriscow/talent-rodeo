package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"talent-rodeo/cmd/web/simulate"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/utils"
	"talent-rodeo/internal/vapi"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// makeOutboundCall handles a scheduled webhook task to make an outbound call with VAPI.
// When an appointment is created, we also schedule a task to make a webhook
// call to this endpoint at the time of the appointment.  This endpoint calls
// the VAPI API to create a call and then updates the appointment with the call
// ID.
func (app *application) makeOutboundCall(w http.ResponseWriter, r *http.Request) {
	app.logger.Debug("handleMakeOutboundVapiCall (handle scheduled outbound call)", "method", r.Method, "url", r.URL.String())

	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var appointment internal.Appointment
	var apptErr error

	defer func() {
		// We need to update the appointment status with the result of this function
		/*
			AppointmentStatusCompleted AppointmentStatus = "completed"
			AppointmentStatusCancelled AppointmentStatus = "cancelled"
			AppointmentStatusFailed    AppointmentStatus = "failed"
			AppointmentStatusOther     AppointmentStatus = "other"
		*/
		if apptErr != nil {
			appointment.Status = internal.AppointmentStatusFailed
			internal.UpdateAppointment(r.Context(), app.fs, &appointment)
		} else {
			appointment.Status = internal.AppointmentStatusCompleted
			internal.UpdateAppointment(r.Context(), app.fs, &appointment)
		}
	}()

	// verify all env vars are set
	if os.Getenv("VAPI_ASSISTANT_ID") == "" || os.Getenv("VAPI_OUTBOUND_PHONE_ID") == "" {
		apptErr = fmt.Errorf("missing required environment variables")
		app.logger.Error("Missing required environment variables",
			"assistant_id", os.Getenv("VAPI_ASSISTANT_ID") == "",
			"phone_id", os.Getenv("VAPI_OUTBOUND_PHONE_ID") == "")

		app.reportServerError(r, apptErr)
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "missing required environment variables",
		})
		return
	}

	// Get assistant and phone IDs from environment
	assistantID := os.Getenv("VAPI_ASSISTANT_ID")
	outboundPhoneID := os.Getenv("VAPI_OUTBOUND_PHONE_ID")

	if assistantID == "" || outboundPhoneID == "" {
		apptErr = fmt.Errorf("missing required environment variables")

		app.logger.Error("Missing required environment variables",
			"assistant_id", assistantID == "",
			"phone_id", outboundPhoneID == "")

		app.reportServerError(r, apptErr)
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "missing required environment variables",
		})
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&appointment); err != nil {
		apptErr = fmt.Errorf("failed to decode request body: %w", err)
		app.logger.Error("Failed to decode request body", "error", apptErr)
		app.reportServerError(r, apptErr)

		// This is likely our problem, so we don't want to retry
		w.WriteHeader(http.StatusOK)
		return
	}

	// validate that none of the fields are empty
	if appointment.UserID == "" {
		apptErr = fmt.Errorf("missing required fields: UserID")
		app.logger.Error("Missing required fields", "user_id_missing", appointment.UserID == "")
		app.reportServerError(r, apptErr)

		// This is likely our problem, so we don't want to retry
		w.WriteHeader(http.StatusOK)
		return
	}

	user, found, apptErr := internal.GetUser(r.Context(), app.fs, appointment.UserID)
	if apptErr != nil {
		apptErr = fmt.Errorf("failed to get user: %w", apptErr)
		app.logger.Error("Failed to get user", "error", apptErr)
		app.reportServerError(r, apptErr)

		// this might just be a transient error, so we want to retry
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to get user",
		})
		return
	}

	if !found {
		apptErr = fmt.Errorf("user not found: %s", appointment.UserID)
		app.logger.Error("User not found", "user_id", appointment.UserID)
		app.reportServerError(r, apptErr)

		// this is likely our problem, so we don't want to retry
		w.WriteHeader(http.StatusOK)
		return
	}

	if user.Phone == nil || *user.Phone == "" {
		apptErr = fmt.Errorf("user has no phone number: %s", appointment.UserID)
		app.logger.Error("User has no phone number", "user_id", appointment.UserID)
		app.reportServerError(r, apptErr)

		// this is likely our problem, so we don't want to retry
		w.WriteHeader(http.StatusOK)
		return
	}

	if user.State.Status == internal.UserStatusError {
		apptErr = fmt.Errorf("user is in error state: %s", appointment.UserID)
		app.logger.Error("User is in error state", "user_id", appointment.UserID)
		app.reportServerError(r, apptErr)

		// this is likely our problem, so we don't want to retry
		w.WriteHeader(http.StatusOK)
		return
	}

	if user.State.Status < internal.UserStatusIntroCallScheduled {
		apptErr = fmt.Errorf("user is not ready for a call: %s", appointment.UserID)
		app.logger.Error("User is not ready for a call", "user_id", appointment.UserID)
		app.reportServerError(r, apptErr)

		// this is likely our problem, so we don't want to retry
		w.WriteHeader(http.StatusOK)
		return
	}

	resume, err := internal.GetLatestResume(r.Context(), app.fs, user.ID)
	if err != nil {
		apptErr = fmt.Errorf("failed to get resume: %w", err)
		if status.Code(err) == codes.NotFound {
			app.logger.Error("handleVapiCall: user has no resume", "user_id", appointment.UserID)
			w.WriteHeader(http.StatusOK)
			return
		}
		app.logger.Error("Failed to get resume", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to get resume: %w", err))

		// This might be a transient error, so we want to retry
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to get resume",
		})
		return
	}

	if !resume.IsValid {
		apptErr = fmt.Errorf("resume is not valid")
		app.logger.Error("handleVapiCall: resume is not valid", "user_id", appointment.UserID)
		app.logger.Debug("handleVapiCall: resume", "resume-analysis", resume.Analysis)
		app.reportServerError(r, apptErr)

		// return a 200 because we don't want them to retry
		w.WriteHeader(http.StatusOK)
		return
	}

	resumeText := resume.RawText
	if resume.Annotated != nil {
		resumeText = *resume.Annotated
	}

	// Create model config with system prompt
	tmpl, err := utils.CreateTemplate("prompts/shirely.001.txt")
	if err != nil {
		apptErr = fmt.Errorf("failed to create system prompt template: %w", err)
		app.logger.Error("Failed to create system prompt template", "error", apptErr)
		app.reportServerError(r, apptErr)

		// This is likely our problem, so we don't want to retry
		w.WriteHeader(http.StatusOK)
		return
	}

	systemPrompt, err := tmpl.Execute(map[string]any{
		"FullName": user.Name,
		"Resume":   resumeText,
	})
	if err != nil {
		apptErr = fmt.Errorf("failed to render system prompt template: %w", err)
		app.logger.Error("Failed to render system prompt template", "error", apptErr)
		app.reportServerError(r, apptErr)

		// This is likely our problem, so we don't want to retry
		w.WriteHeader(http.StatusOK)
		return
	}

	modelConfig := vapi.DefaultModelConfig
	modelConfig.Messages = []vapi.ModelMessage{{
		Role:    "system",
		Content: systemPrompt,
	}}

	// Create call config
	callConfig := vapi.Call{
		Name:          &appointment.ID,
		AssistantID:   &assistantID,
		PhoneNumberID: &outboundPhoneID,
		AssistantOverrides: &vapi.Assistant{
			Model: &modelConfig,
			Server: &vapi.ServerConfig{
				URL:            os.Getenv("VAPI_WEBHOOK_URL"),
				TimeoutSeconds: 20,
			},
			ServerMessages: []string{
				// "conversation-update",
				"end-of-call-report",
				"function-call",
				"hang",
				// "speech-update",
				"status-update",
				"tool-calls",
				"transfer-destination-request",
				"user-interrupted",
			},
		},
		CustomerID: &user.ID,
		Customer: &vapi.Customer{
			Number: *user.Phone,
			Name:   user.Name,
		},
	}

	// Create the call
	result, err := vapi.CreateCall(r.Context(), callConfig)
	if err != nil {
		apptErr = fmt.Errorf("handleVapiCall CreateCall: %w", err)
		app.logger.Error("Failed to create call", "error", err)
		app.reportServerError(r, apptErr)

		// This is likely a transient error, but it could be we are not configuring the call correctly, so we want to retry
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to create call: " + err.Error(),
		})
		return
	}

	if result.ID != nil && result.Status != nil {
		app.logger.Info(internal.Green("call created successfully"),
			"users_id", user.ID,
			"resume_id", resume.ID,
			"call_id", *result.ID,
			"status", *result.Status,
		)
	} else {
		app.logger.Warn(internal.BGYellow("unexpected nil pointer result from CreateCall"), "result", result)
		app.reportServerError(r, fmt.Errorf("unexpected nil pointer result from CreateCall"))
	}

	user.State.Status = internal.UserStatusIntroCallStarted
	if err := internal.UpdateUserStatus(r.Context(), app.fs, user, user.State.Status); err != nil {
		app.logger.Error("Failed to update user", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to update user: %w", err))
	}

	// In test mode, schedule a simulated end-of-call webhook
	if os.Getenv("TESTING_MODE") == "true" {
		if err := simulate.EndOfCall(r.Context(), app.processor, appointment.UserID, appointment.ID, app.logger); err != nil {
			// Log error but continue with normal flow since this is just test simulation
			app.logger.Error("Failed to simulate end-of-call", "error", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}
