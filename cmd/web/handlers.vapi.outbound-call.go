package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"talent-rodeo/cmd/web/simulate"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/vapi-go"

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

	var appointment *internal.Appointment
	var user *internal.User
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
			errStr := apptErr.Error()
			appointment.Error = &errStr
			appointment.Status = internal.AppointmentStatusFailed
			user.State.Status = internal.UserStatusError
			user.State.Error = &errStr
			app.fs.RunTransaction(r.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
				if err := internal.UpdateUserStatusTx(ctx, app.fs, tx, user, internal.UserStatusError); err != nil {
					return fmt.Errorf("failed to update user: %w", err)
				}

				if err := internal.UpdateAppointmentTx(ctx, app.fs, tx, appointment); err != nil {
					return fmt.Errorf("failed to update appointment: %w", err)
				}
				return nil
			})
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
		app.logger.Error("outbound call: missing required fields", "user_id_missing", appointment.UserID == "")
		app.reportServerError(r, apptErr)

		// This is likely our problem, so we don't want to retry
		w.WriteHeader(http.StatusOK)
		return
	}

	var found bool
	user, found, apptErr = internal.GetUser(r.Context(), app.fs, appointment.UserID)
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

	if user.State.Status < internal.UserStatusCallScheduled {
		apptErr = fmt.Errorf("user is not ready for a call: %s", appointment.UserID)
		app.logger.Error("User is not ready for a call", "user_id", appointment.UserID)
		app.reportServerError(r, apptErr)

		// this is likely our problem, so we don't want to retry
		w.WriteHeader(http.StatusOK)
		return
	}

	resume, found, err := internal.GetLatestResume(r.Context(), app.fs, user.ID)
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

	if !found {
		app.logger.Error("handleVapiCall: user has no resume", "user_id", appointment.UserID)
		w.WriteHeader(http.StatusOK)
		return
	}

	assistantOverrides, err := internal.BuildAssistantOverrides(r.Context(), user, resume, app.fs, app.logger)
	if err != nil {
		apptErr = fmt.Errorf("failed to build assistant overrides: %w", err)
		app.logger.Error("Failed to build assistant overrides", "error", apptErr)
		app.reportServerError(r, apptErr)
		w.WriteHeader(http.StatusOK)
		return
	}

	phone := vapi.PhoneNumber{
		TwilioAccountSid:  os.Getenv("TWILIO_ACCOUNT_SID"),
		TwilioAuthToken:   os.Getenv("TWILIO_AUTH_TOKEN"),
		TwilioPhoneNumber: os.Getenv("TWILIO_FROM_NUMBER"),
	}

	var result *vapi.Call
	if os.Getenv("TESTING_MODE") == "true" {
		result, err = vapi.GetCall(r.Context(), simulate.TestingCallID)
		if err != nil {
			apptErr = fmt.Errorf("failed to get call: %w", err)
			app.logger.Error("Failed to get call", "error", err)
			app.reportServerError(r, apptErr)
			return
		}

		appointment.CallID = result.ID
	} else {
		// Create call config
		callConfig := vapi.Call{
			Name:               &appointment.ID,
			AssistantID:        &assistantID,
			PhoneNumber:        &phone,
			AssistantOverrides: assistantOverrides,
			Customer: &vapi.Customer{
				Number: *user.Phone,
				Name:   user.ID,
			},
		}

		result, err = vapi.CreateCall(r.Context(), callConfig)
		if err != nil {
			apptErr = fmt.Errorf("handleVapiCall CreateCall: %w", err)
			app.logger.Error("failed to create call", "error", err)

			b, err := json.MarshalIndent(callConfig, "", "  ")
			if err != nil {
				app.logger.Error("failed to marshal call config", "error", err)
			}
			app.logger.Error("call config", "call_config", string(b))

			app.reportServerError(r, apptErr)

			// This is likely a transient error, but it could be we are not configuring the call correctly, so we want to retry
			response.JSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to create call: " + err.Error(),
			})
			return
		}
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

	app.fs.RunTransaction(r.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		appointment.Status = internal.AppointmentStatusRunning
		appointment.CallID = result.ID
		if err := internal.UpdateAppointmentTx(ctx, app.fs, tx, appointment); err != nil {
			return fmt.Errorf("failed to update appointment: %w", err)
		}

		user.State.Status = internal.UserStatusCallStarted
		if err := internal.UpdateUserStatusTx(ctx, app.fs, tx, user, internal.UserStatusCallStarted); err != nil {
			return fmt.Errorf("failed to update user: %w", err)
		}

		return nil
	})

	if err := internal.SendPusherRedirect(app.pusher, user.ID, "/dashboard"); err != nil {
		app.logger.Error("Failed to send pusher redirect", "error", err)
		// app.reportServerError(r, fmt.Errorf("failed to send pusher redirect: %w", err))
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
