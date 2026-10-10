package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"talent-rodeo/cmd/web/simulate"
	"talent-rodeo/internal"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/vapi-go"
	"github.com/getsentry/sentry-go"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

var (
	errOutboundNotScheduled = errors.New("appointment is not scheduled")
	errOutboundNotDue       = errors.New("appointment is not due")
	errOutboundExpired      = errors.New("appointment call window has expired")
)

const maxOutboundRequestBytes = 4096
const outboundCallWindow = 15 * time.Minute

// makeOutboundCall accepts only signed scheduled tasks and claims the saved
// appointment before attempting the external call.
func (app *application) makeOutboundCall(w http.ResponseWriter, r *http.Request) {
	if app.mode.Personal {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxOutboundRequestBytes+1))
	if err != nil || len(body) > maxOutboundRequestBytes {
		http.Error(w, "invalid call request", http.StatusBadRequest)
		return
	}
	if !verifyOutboundCallSignature(body, r.Header.Get(outboundSignatureHeader), app.config.session.secretKey) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var callRequest outboundCallRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&callRequest); err != nil || callRequest.AppointmentID == "" || callRequest.UserID == "" {
		http.Error(w, "invalid call request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		http.Error(w, "invalid call request", http.StatusBadRequest)
		return
	}

	user, found, err := internal.GetUser(r.Context(), app.fs, callRequest.UserID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("read outbound call user: %w", err))
		return
	}
	if !found || user.Phone == nil || *user.Phone == "" {
		http.Error(w, "call is not eligible", http.StatusConflict)
		return
	}
	if user.Status < internal.UserStatusCallScheduled {
		http.Error(w, "call setup is not complete", http.StatusServiceUnavailable)
		return
	}
	if user.Status != internal.UserStatusCallScheduled {
		http.Error(w, "call is not eligible", http.StatusConflict)
		return
	}
	assistantID := os.Getenv("VAPI_ASSISTANT_ID")
	if assistantID == "" || os.Getenv("VAPI_OUTBOUND_PHONE_ID") == "" {
		app.serverError(w, r, fmt.Errorf("outbound call configuration is incomplete"))
		return
	}

	overrides, err := internal.BuildAssistantOverrides(r.Context(), user, app.fs, app.logger)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("build outbound assistant overrides: %w", err))
		return
	}
	appointment, err := app.claimOutboundAppointment(r.Context(), callRequest)
	if err != nil {
		switch {
		case errors.Is(err, errOutboundNotScheduled):
			http.Error(w, "appointment is not scheduled", http.StatusConflict)
		case errors.Is(err, errOutboundNotDue):
			http.Error(w, "appointment is not due", http.StatusServiceUnavailable)
		case errors.Is(err, errOutboundExpired):
			http.Error(w, "appointment call window has expired", http.StatusConflict)
		default:
			app.serverError(w, r, fmt.Errorf("claim outbound appointment: %w", err))
		}
		return
	}

	var result *vapi.Call
	if os.Getenv("TESTING_MODE") == "true" {
		result, err = vapi.GetCall(r.Context(), simulate.TestingCallID)
	} else {
		phone := vapi.PhoneNumber{
			TwilioAccountSid:  os.Getenv("TWILIO_ACCOUNT_SID"),
			TwilioAuthToken:   os.Getenv("TWILIO_AUTH_TOKEN"),
			TwilioPhoneNumber: os.Getenv("TWILIO_FROM_NUMBER"),
		}
		callConfig := vapi.Call{
			Name:               &appointment.ID,
			AssistantID:        &assistantID,
			PhoneNumber:        &phone,
			AssistantOverrides: overrides,
			Customer:           &vapi.Customer{Number: *user.Phone, Name: user.ID},
		}
		result, err = vapi.CreateCall(r.Context(), callConfig)
	}
	if err != nil || result == nil || result.ID == nil {
		persistCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if persistErr := app.failClaimedAppointment(persistCtx, appointment); persistErr != nil {
			app.logger.Error("failed to record outbound call failure", "appointment_id", appointment.ID, "error", persistErr)
		}
		cancel()
		app.logger.Error("outbound call provider failed", "appointment_id", appointment.ID)
		http.Error(w, "failed to create call", http.StatusBadGateway)
		return
	}
	persistCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := app.recordOutboundCall(persistCtx, appointment, *result.ID); err != nil {
		cancel()
		// The provider may already have started the call. Leave the claim in
		// place so a task retry cannot start a second one.
		app.reportServerError(r, fmt.Errorf("record outbound call %s: %w", appointment.ID, err))
		http.Error(w, "call state needs reconciliation", http.StatusInternalServerError)
		return
	}
	cancel()

	app.logger.Info("outbound call created", "user_id", user.ID, "appointment_id", appointment.ID, "call_id", *result.ID)
	if err := internal.SendPusherRedirect(app.pusher, user.ID, "/dashboard"); err != nil {
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":  "failed_to_send_pusher_redirect",
			"user_id": user.ID,
		})
	}
	if os.Getenv("TESTING_MODE") == "true" {
		if err := simulate.EndOfCall(r.Context(), app.processor, appointment.UserID, appointment.ID, app.logger); err != nil {
			app.logger.Error("simulate end of call", "appointment_id", appointment.ID, "error", err)
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (app *application) claimOutboundAppointment(ctx context.Context, request outboundCallRequest) (*internal.Appointment, error) {
	// These paths match the persisted collections in internal/appointment.go.
	ref := app.fs.Collection("users").Doc(request.UserID).Collection("appointments").Doc(request.AppointmentID)
	var claimed internal.Appointment
	err := app.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(ref)
		if err != nil {
			if grpcstatus.Code(err) == codes.NotFound {
				return errOutboundNotScheduled
			}
			return err
		}
		var appointment internal.Appointment
		if err := doc.DataTo(&appointment); err != nil {
			return fmt.Errorf("decode appointment: %w", err)
		}
		if err := validateOutboundAppointment(appointment, request, time.Now()); err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Update(ref, []firestore.Update{
			{Path: "Status", Value: internal.AppointmentStatusRunning},
			{Path: "UpdatedAt", Value: now},
		}); err != nil {
			return err
		}
		claimed = appointment
		claimed.Status = internal.AppointmentStatusRunning
		claimed.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &claimed, nil
}

func validateOutboundAppointment(appointment internal.Appointment, request outboundCallRequest, now time.Time) error {
	if appointment.ID != request.AppointmentID || appointment.UserID != request.UserID ||
		appointment.Status != internal.AppointmentStatusScheduled || appointment.CallID != nil ||
		appointment.ScheduledTime.IsZero() {
		return errOutboundNotScheduled
	}
	if now.Before(appointment.ScheduledTime) {
		return errOutboundNotDue
	}
	if now.After(appointment.ScheduledTime.Add(outboundCallWindow)) {
		return errOutboundExpired
	}
	return nil
}

func (app *application) failClaimedAppointment(ctx context.Context, appointment *internal.Appointment) error {
	ref := app.fs.Collection("users").Doc(appointment.UserID).Collection("appointments").Doc(appointment.ID)
	userRef := app.fs.Collection("users").Doc(appointment.UserID)
	return app.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(ref)
		if err != nil {
			return err
		}
		var current internal.Appointment
		if err := doc.DataTo(&current); err != nil {
			return err
		}
		if current.Status != internal.AppointmentStatusRunning || current.CallID != nil {
			return nil
		}
		userDoc, err := tx.Get(userRef)
		if err != nil {
			return err
		}
		var currentUser internal.User
		if err := userDoc.DataTo(&currentUser); err != nil {
			return err
		}
		now := time.Now().UTC()
		failure := "unable to start call"
		if err := tx.Update(ref, []firestore.Update{
			{Path: "Status", Value: internal.AppointmentStatusFailed},
			{Path: "Error", Value: failure},
			{Path: "FailedAt", Value: now},
			{Path: "UpdatedAt", Value: now},
		}); err != nil {
			return err
		}
		if currentUser.Status != internal.UserStatusCallScheduled {
			return nil
		}
		return tx.Update(userRef, []firestore.Update{
			{Path: "Status", Value: internal.UserStatusError},
			{Path: "Error", Value: failure},
			{Path: "StatusUpdatedAt", Value: now},
		})
	})
}

func (app *application) recordOutboundCall(ctx context.Context, appointment *internal.Appointment, callID string) error {
	ref := app.fs.Collection("users").Doc(appointment.UserID).Collection("appointments").Doc(appointment.ID)
	userRef := app.fs.Collection("users").Doc(appointment.UserID)
	return app.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(ref)
		if err != nil {
			return err
		}
		var current internal.Appointment
		if err := doc.DataTo(&current); err != nil {
			return err
		}
		if current.Status != internal.AppointmentStatusRunning || current.CallID != nil {
			return errOutboundNotScheduled
		}
		userDoc, err := tx.Get(userRef)
		if err != nil {
			return err
		}
		var currentUser internal.User
		if err := userDoc.DataTo(&currentUser); err != nil {
			return err
		}
		if err := tx.Update(ref, []firestore.Update{{Path: "CallID", Value: callID}, {Path: "UpdatedAt", Value: time.Now().UTC()}}); err != nil {
			return err
		}
		if currentUser.Status != internal.UserStatusCallScheduled {
			return nil
		}
		return internal.UpdateUserStatusTx(ctx, app.fs, tx, &currentUser, internal.UserStatusCallStarted)
	})
}
