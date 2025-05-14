package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/internal"
	"talent-rodeo/internal/transcript"
	"time"

	"github.com/chriscow/vapi-go"

	"github.com/getsentry/sentry-go"
)

// Then use it with different severity levels
// internal.reportToSentry(r.Context(), err, sentry.LevelError, tags)   // Most errors
// internal.reportToSentry(r.Context(), err, sentry.LevelWarning, tags) // Less critical errors
// internal.reportToSentry(r.Context(), err, sentry.LevelFatal, tags)   // Application-crashing errors

func (app *application) processCompletedAppointment(r *http.Request, userID, appointmentID string, env vapi.EndOfCallReportEnvelope) error {
	app.logger.Debug("processCompletedAppointment: received end of call report", "user_id", userID, "appointmentID", appointmentID)

	ipaddr := r.Header.Get("X-Forwarded-For")
	internal.SentrySetUserContext(r.Context(), userID, ipaddr, map[string]interface{}{
		"appointment_id": appointmentID,
	})

	// Get the appt
	appt, err := internal.GetAppointment(r.Context(), app.fs, userID, appointmentID)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to get appointment for user %s: %w", userID, err)
		app.logger.Error("failed to get appointment",
			"error", err,
			"customer_id", userID,
			"appointment_id", appointmentID)

		internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
			"error_type":  "database_retrieval",
			"entity_type": "appointment",
		})

		return wrappedErr
	}

	report := env.EndOfCallReport

	// Update the appointment with the call result and report
	appt.CallID = report.Call.ID
	appt.Summary = report.Summary
	appt.UpdatedAt = time.Now()
	endReason := report.EndedReason

	// Update appointment status based on call result
	status, shouldLog := internal.MapVapiEndReasonToAppointmentStatus(endReason)
	appt.EndReason = &status

	if err := internal.UpdateAppointment(r.Context(), app.fs, appt); err != nil {
		wrappedErr := fmt.Errorf("failed to update appointment: %w", err)
		app.logger.Error("failed to update appointment", "error", err)

		internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
			"error_type":  "database_update",
			"entity_type": "appointment",
		})

		return wrappedErr
	}

	user, found, err := internal.GetUser(r.Context(), app.fs, userID)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to get user: %w", err)
		app.logger.Error("failed to get user", "error", err)

		internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
			"error_type":  "database_retrieval",
			"entity_type": "user",
		})

		return wrappedErr
	}
	if !found {
		app.logger.Error("user not found", "customer_id", userID)
		return fmt.Errorf("user not found: %s", userID)
	}

	// if status == internal.AppointmentStatusCompleted {
	// 	if err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusCallAnalysisStarted); err != nil {
	// 		wrappedErr := fmt.Errorf("failed to update user status: %w", err)
	// 		app.logger.Error("failed to update user status", "error", err)

	// 		internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
	// 			"error_type":  "database_update",
	// 			"entity_type": "user_status",
	// 		})

	// 		return wrappedErr
	// 	}
	// } else {
	// 	user.State.Status = internal.UserStatusError
	// 	err := fmt.Sprintf("Call ended with reason: %s", endReason)
	// 	if err := internal.UpdateUserStateWithError(r.Context(), app.fs, user, err); err != nil {
	// 		wrappedErr := fmt.Errorf("failed to update user state with error: %w", err)
	// 		app.logger.Error("failed to update user state with error", "error", err)

	// 		internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
	// 			"error_type":  "database_update",
	// 			"entity_type": "user_state",
	// 		})

	// 		return wrappedErr
	// 	}
	// }

	if shouldLog {
		app.logger.Info("appointment status updated",
			"appointment_id", appointmentID,
			"status", status,
			"end_reason", endReason)
	}

	hub := sentry.GetHubFromContext(r.Context())

	// This actually just starts a goroutine to process the appointment and
	// doesn't return an error but we check anyway in case that changes
	// in the future.
	go transcript.ProcessCompletedAppointment(hub, app.fs, app.logger, appt)

	app.SendUserStateUpdate(user)
	return nil
}
