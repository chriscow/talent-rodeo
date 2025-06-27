package main

import (
	"fmt"
	"net/http"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/smtp"
	"talent-rodeo/internal/transcript"
	"time"

	"github.com/chriscow/vapi-go"

	"github.com/getsentry/sentry-go"
)

// Then use it with different severity levels
// internal.reportToSentry(r.Context(), err, sentry.LevelError, tags)   // Most errors
// internal.reportToSentry(r.Context(), err, sentry.LevelWarning, tags) // Less critical errors
// internal.reportToSentry(r.Context(), err, sentry.LevelFatal, tags)   // Application-crashing errors

func (app *application) processCompletedAppointment(r *http.Request, userID, appointmentID string, report vapi.EndOfCallReport) error {
	app.logger.Debug("processCompletedAppointment: received end of call report", "userID", userID, "appointmentID", appointmentID)

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

	// Update the appointment with the call result and report
	appt.CallID = report.Call.ID
	appt.Summary = report.Summary
	appt.UpdatedAt = time.Now()
	endReason := report.EndedReason

	// Update appointment status based on call result
	shouldLog := true
	appt.EndReason = &endReason

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

	user.Status = internal.UserStatusCallCompleted

	if shouldLog {
		app.logger.Info("appointment status updated",
			"appointment_id", appointmentID,
			"status", endReason,
			"end_reason", endReason)
	}

	hub := sentry.GetHubFromContext(r.Context())

	// Always evaluate call success based on endReason and analysis (if present)
	shouldProcessFailure, err := app.evaluateCallSuccess(&report, endReason)
	if err != nil {
		return fmt.Errorf("failed to evaluate call success: %w", err)
	}

	if shouldProcessFailure {
		var successEval any
		if report.Analysis != nil {
			successEval = report.Analysis.SuccessEvaluation
		}
		app.logger.Info("call failed", "name", user.Name, "userID", user.ID, "endReason", endReason, "successEvaluation", successEval)

		if err := internal.ProcessAppointmentFailed(r.Context(), app.fs, user, appt, fmt.Errorf("call failed based on endReason or success evaluation")); err != nil {
			return fmt.Errorf("failed to process appointment failed: %w", err)
		}

		// Send failure email for failed calls
		cfg := smtp.Config{
			Host:     app.config.smtp.host,
			Port:     app.config.smtp.port,
			Username: app.config.smtp.username,
			Password: app.config.smtp.password,
			From:     app.config.smtp.from,
		}

		failureReason := app.getFailureReasonDescription(endReason)
		go transcript.SendFailureEmail(r.Context(), app.fs, user, appt, failureReason, &cfg, app.logger)
	} else {
		var successEval any
		if report.Analysis != nil {
			successEval = report.Analysis.SuccessEvaluation
		}
		app.logger.Info("call was successful", "name", user.Name, "userID", user.ID, "endReason", endReason, "successEvaluation", successEval)

		// Only process completion for successful calls
		// This actually just starts a goroutine to process the appointment and
		// doesn't return an error but we check anyway in case that changes
		// in the future.
		cfg := smtp.Config{
			Host:     app.config.smtp.host,
			Port:     app.config.smtp.port,
			Username: app.config.smtp.username,
			Password: app.config.smtp.password,
			From:     app.config.smtp.from,
		}

		go transcript.ProcessCompletedCall(hub, app.fs, app.pusher, app.searchProvider, app.logger, app.jobDispatch, user, appt, &cfg)
	}
	return nil
}

// evaluateCallSuccess processes both the Analysis object and endReason from VAPI webhook
// to determine whether the call should be treated as a failure requiring ProcessAppointmentFailed.
//
// This function always checks endReason first as the primary indicator of call success.
// Only specific endReason values are considered successful, everything else is a failure.
//
// If endReason indicates success, then we check the Analysis.SuccessEvaluation if present:
// - bool: true = success, false = failure
// - string: "true", "false", or must contain "was successful" to be considered success
//
// This function handles nil Analysis objects and missing SuccessEvaluation fields gracefully.
// It avoids webhook retries by treating any invalid/unparseable values as failures and logs them for debugging.
//
// Returns:
// - shouldProcessFailure: true if the call failed and ProcessAppointmentFailed should be called
// - error: only returned for unexpected system errors, not for invalid values
func (app *application) evaluateCallSuccess(report *vapi.EndOfCallReport, endReason string) (shouldProcessFailure bool, err error) {
	if report == nil {
		return false, fmt.Errorf("end of call report is nil")
	}

	var successEvaluation any
	if report.Analysis != nil {
		successEvaluation = report.Analysis.SuccessEvaluation
	}

	app.logger.Info("evaluating call success",
		"success_evaluation", successEvaluation,
		"end_reason", endReason,
		"has_analysis", report.Analysis != nil)

	// First check endReason - only specific values indicate success
	switch endReason {
	case "assistant-ended-call", "customer-ended-call", "exceeded-max-duration":
		// These are considered successful call completions
		app.logger.Info("call completed successfully based on end reason", "end_reason", endReason)

		// internal.AskBool(r.Context(), prompt, fs, userID)
	default:
		// All other endReason values (voicemail, errors, failures, etc.) are failures
		app.logger.Info("call failed based on end reason", "end_reason", endReason)
		return true, nil // failure, process failure
	}

	// If endReason indicates success, check successEvaluation if present
	if report.Analysis == nil || report.Analysis.SuccessEvaluation == nil {
		app.logger.Info("no analysis or success evaluation provided, using end reason result")
		return false, nil // success based on endReason
	}

	// Check if it's a bool (original API format)
	if boolVal, ok := successEvaluation.(bool); ok {
		// bool false = failure, bool true = success
		return !boolVal, nil
	}

	// Check if it's a string (new API format)
	if strVal, ok := successEvaluation.(string); ok {
		// Handle explicit "true"/"false" strings
		if strVal == "true" {
			return false, nil // success, don't process failure
		}
		if strVal == "false" {
			app.logger.Info("success evaluation is string 'false'", "success_evaluation", strVal)
			return true, nil // failure, process failure
		}

		// Handle descriptive strings - must contain "was successful" to be considered success
		if strings.Contains(strVal, "was successful") {
			return false, nil // success, don't process failure
		}

		// String doesn't contain success indicator - treat as failure
		app.logger.Info("success evaluation string doesn't indicate success",
			"success_evaluation", strVal)
		return true, nil // failure, process failure
	}

	// Neither bool nor string - treat as failure to avoid webhook retries
	app.logger.Error("success evaluation is neither bool nor string",
		"success_evaluation", successEvaluation)
	return true, nil // failure, process failure
}

// getFailureReasonDescription converts technical end reasons to user-friendly descriptions
func (app *application) getFailureReasonDescription(endReason string) string {
	switch endReason {
	case "voicemail":
		return "the call went to voicemail"
	case "busy":
		return "your phone was busy"
	case "no-answer":
		return "there was no answer"
	case "failed":
		return "the call failed to connect"
	case "cancelled":
		return "the call was cancelled"
	case "dial-busy", "dial-failed":
		return "we couldn't connect the call"
	case "pipeline-error-openai", "pipeline-error-deepgram":
		return "there was a technical issue with our AI system"
	default:
		if endReason == "" {
			return "there was an issue with the call"
		}
		// For any other technical error codes, provide a generic message
		if strings.Contains(endReason, "error") || strings.Contains(endReason, "failed") {
			return "there was a technical issue"
		}
		return "the call did not complete successfully"
	}
}
