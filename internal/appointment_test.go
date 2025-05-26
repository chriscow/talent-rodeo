package internal

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestAppointment(t *testing.T) {
	ctx := context.Background()

	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("newTestFirestoreClient() error = %v", err)
	}

	// Clean up before and after tests
	DeleteTestData(t, client, "test-user-")
	t.Cleanup(func() {
		DeleteTestData(t, client, "test-user-")
		client.Close()
	})

	// Create a test user first
	user := CreateTestUser(t, client, "Test User", "test@example.com", "+14251234567", false)

	// Test CreateAppointment
	t.Run("CreateAppointment", func(t *testing.T) {
		appointment := CreateTestAppointment(t, client, user)

		// Verify the appointment was created
		got, err := GetAppointment(ctx, client, user.ID, appointment.ID)
		if err != nil {
			t.Fatalf("GetAppointment() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetAppointment() got nil, want appointment")
		}
		if got.UserID != user.ID {
			t.Errorf("GetAppointment() got UserID = %v, want %v", got.UserID, user.ID)
		}
		if got.Status != AppointmentStatusScheduled {
			t.Errorf("GetAppointment() got Status = %v, want %v", got.Status, AppointmentStatusScheduled)
		}
	})

	// Test CreateAppointment with non-existent user
	t.Run("CreateAppointmentNonExistentUser", func(t *testing.T) {
		appointment := &Appointment{
			Name:          "Test Appointment",
			ScheduledTime: time.Now().Add(24 * time.Hour),
			Timezone:      "UTC",
			Status:        AppointmentStatusScheduled,
		}

		err := CreateAppointment(ctx, client, "non-existent-user", appointment)
		if err == nil {
			t.Error("CreateAppointment() with non-existent user should return error")
		}
	})

	// Test CreateAppointment with custom ID
	t.Run("CreateAppointmentWithCustomID", func(t *testing.T) {
		customID := "custom-appointment-id"
		appointment := &Appointment{
			ID:            customID,
			Name:          "Test Appointment",
			ScheduledTime: time.Now().Add(24 * time.Hour),
			Timezone:      "UTC",
			Status:        AppointmentStatusScheduled,
		}

		err := CreateAppointment(ctx, client, user.ID, appointment)
		if err != nil {
			t.Fatalf("CreateAppointment() error = %v", err)
		}

		// Verify the custom ID was preserved
		got, err := GetAppointment(ctx, client, user.ID, customID)
		if err != nil {
			t.Fatalf("GetAppointment() error = %v", err)
		}
		if got.ID != customID {
			t.Errorf("GetAppointment() got ID = %v, want %v", got.ID, customID)
		}
	})

	// Test GetAppointments
	t.Run("GetAppointments", func(t *testing.T) {
		// Create a second appointment for the same user
		CreateTestAppointment(t, client, user)
		CreateTestAppointment(t, client, user)

		appointments, err := ListAppointments(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetAppointments() error = %v", err)
		}
		if len(appointments) < 2 {
			t.Errorf("GetAppointments() got %d appointments, want at least 2", len(appointments))
		}
	})

	// Test UpdateAppointment
	t.Run("UpdateAppointment", func(t *testing.T) {
		appointment := CreateTestAppointment(t, client, user)

		// Update the appointment
		// Use UTC to ensure consistent timezone handling
		newTime := time.Now().UTC().Add(48 * time.Hour).Round(time.Second)
		appointment.ScheduledTime = newTime
		appointment.Status = AppointmentStatusCompleted

		err := UpdateAppointment(ctx, client, appointment)
		if err != nil {
			t.Fatalf("UpdateAppointment() error = %v", err)
		}

		// Verify the update
		got, err := GetAppointment(ctx, client, user.ID, appointment.ID)
		if err != nil {
			t.Fatalf("GetAppointment() after update error = %v", err)
		}

		// Compare times in UTC and rounded to seconds
		gotTime := got.ScheduledTime.UTC().Round(time.Second)
		wantTime := newTime.UTC().Round(time.Second)
		if !gotTime.Equal(wantTime) {
			t.Errorf("UpdateAppointment() got ScheduledTime = %v, want %v",
				gotTime,
				wantTime)
		}
		if got.Status != AppointmentStatusCompleted {
			t.Errorf("UpdateAppointment() got Status = %v, want %v", got.Status, AppointmentStatusCompleted)
		}
	})

	// Test CancelAppointment
	t.Run("CancelAppointment", func(t *testing.T) {
		appointment := CreateTestAppointment(t, client, user)

		err := CancelAppointment(ctx, client, user.ID, appointment.ID)
		if err != nil {
			t.Fatalf("CancelAppointment() error = %v", err)
		}

		// Verify the cancellation
		got, err := GetAppointment(ctx, client, user.ID, appointment.ID)
		if err != nil {
			t.Fatalf("GetAppointment() after cancel error = %v", err)
		}
		if got.Status != AppointmentStatusCancelled {
			t.Errorf("CancelAppointment() got Status = %v, want %v", got.Status, AppointmentStatusCancelled)
		}
		if got.CancelledAt == nil {
			t.Error("CancelAppointment() didn't set CancelledAt timestamp")
		}
	})

	// Test CancelAppointment with non-existent appointment
	t.Run("CancelAppointmentNotFound", func(t *testing.T) {
		err := CancelAppointment(ctx, client, user.ID, "non-existent-id")
		if err == nil {
			t.Error("CancelAppointment() with non-existent ID should return error")
		}
	})

	// Test appointment not found
	t.Run("AppointmentNotFound", func(t *testing.T) {
		_, err := GetAppointment(ctx, client, user.ID, "non-existent-id")
		if err == nil {
			t.Error("GetAppointment() with non-existent ID should return error")
		}
	})

	// Test GetLatestAppointment
	t.Run("GetLatestAppointment", func(t *testing.T) {
		// Create multiple appointments with different completion times
		now := time.Now().UTC()

		// Create first appointment and mark as completed
		appt1 := CreateTestAppointment(t, client, user)
		appt1.Status = AppointmentStatusCompleted
		completedAt1 := now.Add(-2 * time.Hour)
		appt1.CompletedAt = &completedAt1
		err := UpdateAppointment(ctx, client, appt1)
		if err != nil {
			t.Fatalf("UpdateAppointment() error = %v", err)
		}

		// Create second appointment and mark as completed later
		appt2 := CreateTestAppointment(t, client, user)
		appt2.Status = AppointmentStatusCompleted
		completedAt2 := now.Add(-1 * time.Hour)
		appt2.CompletedAt = &completedAt2
		err = UpdateAppointment(ctx, client, appt2)
		if err != nil {
			t.Fatalf("UpdateAppointment() error = %v", err)
		}

		// Get latest appointment
		latest, found, err := GetLatestAppointment(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetLatestAppointment() error = %v", err)
		}
		if !found {
			t.Fatal("GetLatestAppointment() got found = false, want true")
		}
		if latest.ID != appt2.ID {
			t.Errorf("GetLatestAppointment() got ID = %v, want %v", latest.ID, appt2.ID)
		}
	})

	// Test GetLatestAppointment with no appointments
	t.Run("GetLatestAppointmentNoAppointments", func(t *testing.T) {
		// Create a new user with no appointments
		newUser := CreateTestUser(t, client, "New User", "new@example.com", "+14251234568", false)

		latest, found, err := GetLatestAppointment(ctx, client, newUser.ID)
		if err != nil {
			t.Fatalf("GetLatestAppointment() error = %v", err)
		}
		if found {
			t.Error("GetLatestAppointment() got found = true, want false")
		}
		if latest != nil {
			t.Error("GetLatestAppointment() got non-nil appointment, want nil")
		}
	})

	// Test GetLatestCompletedAppointment
	t.Run("GetLatestCompletedAppointment", func(t *testing.T) {
		// Create a scheduled appointment (not completed)
		CreateTestAppointment(t, client, user)

		// Create a completed appointment
		completedAppt := CreateTestAppointment(t, client, user)
		completedAppt.Status = AppointmentStatusCompleted
		now := time.Now().UTC()
		completedAppt.CompletedAt = &now
		err := UpdateAppointment(ctx, client, completedAppt)
		if err != nil {
			t.Fatalf("UpdateAppointment() error = %v", err)
		}

		// Get latest completed appointment
		latest, found, err := GetLatestCompletedAppointment(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetLatestCompletedAppointment() error = %v", err)
		}
		if !found {
			t.Fatal("GetLatestCompletedAppointment() got found = false, want true")
		}
		if latest.ID != completedAppt.ID {
			t.Errorf("GetLatestCompletedAppointment() got ID = %v, want %v", latest.ID, completedAppt.ID)
		}
		if latest.Status != AppointmentStatusCompleted {
			t.Errorf("GetLatestCompletedAppointment() got Status = %v, want %v", latest.Status, AppointmentStatusCompleted)
		}
	})

	// Test GetLatestCompletedAppointment with no completed appointments
	t.Run("GetLatestCompletedAppointmentNone", func(t *testing.T) {
		// Create a new user with only scheduled appointments
		newUser := CreateTestUser(t, client, "User No Completed", "nocomp@example.com", "+14251234569", false)
		CreateTestAppointment(t, client, newUser) // This will be scheduled, not completed

		latest, found, err := GetLatestCompletedAppointment(ctx, client, newUser.ID)
		if err != nil {
			t.Fatalf("GetLatestCompletedAppointment() error = %v", err)
		}
		if found {
			t.Error("GetLatestCompletedAppointment() got found = true, want false")
		}
		if latest != nil {
			t.Error("GetLatestCompletedAppointment() got non-nil appointment, want nil")
		}
	})

	// Test ListCompletedAppointments
	t.Run("ListCompletedAppointments", func(t *testing.T) {
		// Create a new user for this test to avoid interference
		testUser := CreateTestUser(t, client, "Completed Test User", "completed@example.com", "+14251234570", false)

		// Create multiple appointments with different statuses
		now := time.Now().UTC()

		// Create scheduled appointment (should not appear in results)
		CreateTestAppointment(t, client, testUser)

		// Create first completed appointment
		appt1 := CreateTestAppointment(t, client, testUser)
		appt1.Status = AppointmentStatusCompleted
		completedAt1 := now.Add(-2 * time.Hour)
		appt1.CompletedAt = &completedAt1
		err := UpdateAppointment(ctx, client, appt1)
		if err != nil {
			t.Fatalf("UpdateAppointment() error = %v", err)
		}

		// Create second completed appointment (more recent)
		appt2 := CreateTestAppointment(t, client, testUser)
		appt2.Status = AppointmentStatusCompleted
		completedAt2 := now.Add(-1 * time.Hour)
		appt2.CompletedAt = &completedAt2
		err = UpdateAppointment(ctx, client, appt2)
		if err != nil {
			t.Fatalf("UpdateAppointment() error = %v", err)
		}

		// Create cancelled appointment (should not appear in results)
		cancelledAppt := CreateTestAppointment(t, client, testUser)
		cancelledAppt.Status = AppointmentStatusCancelled
		err = UpdateAppointment(ctx, client, cancelledAppt)
		if err != nil {
			t.Fatalf("UpdateAppointment() error = %v", err)
		}

		// Get completed appointments
		completed, err := ListCompletedAppointments(ctx, client, testUser.ID)
		if err != nil {
			t.Fatalf("ListCompletedAppointments() error = %v", err)
		}

		// Should have exactly 2 completed appointments
		if len(completed) != 2 {
			t.Errorf("ListCompletedAppointments() got %d appointments, want 2", len(completed))
		}

		// Should be sorted by CompletedAt in descending order (newest first)
		if len(completed) >= 2 {
			if completed[0].CompletedAt.Before(*completed[1].CompletedAt) {
				t.Error("ListCompletedAppointments() not sorted by CompletedAt descending")
			}
			// First should be appt2 (more recent)
			if completed[0].ID != appt2.ID {
				t.Errorf("ListCompletedAppointments() first appointment ID = %v, want %v", completed[0].ID, appt2.ID)
			}
		}

		// All returned appointments should have status completed
		for i, appt := range completed {
			if appt.Status != AppointmentStatusCompleted {
				t.Errorf("ListCompletedAppointments() appointment %d status = %v, want %v", i, appt.Status, AppointmentStatusCompleted)
			}
			if appt.CompletedAt == nil {
				t.Errorf("ListCompletedAppointments() appointment %d CompletedAt is nil", i)
			}
		}
	})

	// Test ListCompletedAppointments with no completed appointments
	t.Run("ListCompletedAppointmentsEmpty", func(t *testing.T) {
		// Create a new user with no completed appointments
		testUser := CreateTestUser(t, client, "No Completed User", "nocompleted@example.com", "+14251234571", false)
		CreateTestAppointment(t, client, testUser) // This will be scheduled, not completed

		completed, err := ListCompletedAppointments(ctx, client, testUser.ID)
		if err != nil {
			t.Fatalf("ListCompletedAppointments() error = %v", err)
		}
		if len(completed) != 0 {
			t.Errorf("ListCompletedAppointments() got %d appointments, want 0", len(completed))
		}
	})

	// Test DeleteAppointment
	t.Run("DeleteAppointment", func(t *testing.T) {
		appointment := CreateTestAppointment(t, client, user)

		// Delete the appointment
		err := DeleteAppointment(ctx, client, user.ID, appointment.ID)
		if err != nil {
			t.Fatalf("DeleteAppointment() error = %v", err)
		}

		// Verify the appointment was deleted
		_, err = GetAppointment(ctx, client, user.ID, appointment.ID)
		if err == nil {
			t.Error("GetAppointment() after delete should return error")
		}
	})

	// Test DeleteAppointment with non-existent appointment
	t.Run("DeleteAppointmentNotFound", func(t *testing.T) {
		// This should not error even if appointment doesn't exist
		err := DeleteAppointment(ctx, client, user.ID, "non-existent-id")
		if err != nil {
			t.Errorf("DeleteAppointment() with non-existent ID returned error: %v", err)
		}
	})

	// Test processing appointment tracking
	t.Run("ProcessingAppointmentTracking", func(t *testing.T) {
		ctx := context.Background()

		// Create an appointment
		appointment := CreateTestAppointment(t, client, user)

		// Mark as processing
		err := ProcessAppointmentStarted(ctx, client, user, appointment)
		if err != nil {
			t.Fatalf("ProcessAppointmentStarted() error = %v", err)
		}

		// Verify the appointment status was updated
		updatedAppointment, err := GetAppointment(ctx, client, user.ID, appointment.ID)
		if err != nil {
			t.Fatalf("GetAppointment() error = %v", err)
		}
		if updatedAppointment.Status != AppointmentStatusProcessing {
			t.Errorf("MarkAppointmentAsProcessing() didn't update status, got %v, want %v",
				updatedAppointment.Status, AppointmentStatusProcessing)
		}

		// Verify it appears in the processing list
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		processingAppointments, err := ListAppointmentsNeedingProcessing(ctx, client, logger)
		if err != nil {
			t.Fatalf("ListAppointmentsNeedingProcessing() error = %v", err)
		}

		found := false
		for _, a := range processingAppointments {
			if a.ID == appointment.ID {
				found = true
				break
			}
		}

		if !found {
			t.Error("Appointment not found in processing list after marking as processing")
		}

		// Test completing the processing
		err = ProcessAppointmentComplete(ctx, client, user, appointment)
		if err != nil {
			t.Fatalf("CompleteProcessing() error = %v", err)
		}

		// Verify status was updated to completed
		completedAppointment, err := GetAppointment(ctx, client, user.ID, appointment.ID)
		if err != nil {
			t.Fatalf("GetAppointment() error = %v", err)
		}
		if completedAppointment.Status != AppointmentStatusCompleted {
			t.Errorf("CompleteProcessing() didn't update status, got %v, want %v",
				completedAppointment.Status, AppointmentStatusCompleted)
		}
		if completedAppointment.CompletedAt == nil {
			t.Error("CompleteProcessing() didn't set CompletedAt timestamp")
		}

		// Verify it was removed from the processing list
		processingAppointments, err = ListAppointmentsNeedingProcessing(ctx, client, logger)
		if err != nil {
			t.Fatalf("ListAppointmentsNeedingProcessing() error = %v", err)
		}

		for _, a := range processingAppointments {
			if a.ID == appointment.ID {
				t.Error("Appointment still found in processing list after completion")
			}
		}
	})

	// Test ProcessAppointmentFailed
	t.Run("ProcessAppointmentFailed", func(t *testing.T) {
		appointment := CreateTestAppointment(t, client, user)

		// Mark as processing first
		err := ProcessAppointmentStarted(ctx, client, user, appointment)
		if err != nil {
			t.Fatalf("ProcessAppointmentStarted() error = %v", err)
		}

		// Mark as failed
		testError := errors.New("test processing error")
		err = ProcessAppointmentFailed(ctx, client, user, appointment, testError)
		if err != nil {
			t.Fatalf("ProcessAppointmentFailed() error = %v", err)
		}

		// Verify the appointment status was updated
		failedAppointment, err := GetAppointment(ctx, client, user.ID, appointment.ID)
		if err != nil {
			t.Fatalf("GetAppointment() error = %v", err)
		}
		if failedAppointment.Status != AppointmentStatusFailed {
			t.Errorf("ProcessAppointmentFailed() didn't update status, got %v, want %v",
				failedAppointment.Status, AppointmentStatusFailed)
		}
		if failedAppointment.FailedAt == nil {
			t.Error("ProcessAppointmentFailed() didn't set FailedAt timestamp")
		}

		// Verify it was removed from the processing list
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		processingAppointments, err := ListAppointmentsNeedingProcessing(ctx, client, logger)
		if err != nil {
			t.Fatalf("ListAppointmentsNeedingProcessing() error = %v", err)
		}

		for _, a := range processingAppointments {
			if a.ID == appointment.ID {
				t.Error("Appointment still found in processing list after failure")
			}
		}

		// Verify user state was updated with error
		updatedUser, found, err := GetUser(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetUser() error = %v", err)
		}
		if !found {
			t.Fatal("GetUser() got found = false, want true")
		}
		if updatedUser.State.Error == nil {
			t.Error("ProcessAppointmentFailed() didn't set user error state")
		} else if *updatedUser.State.Error != testError.Error() {
			t.Errorf("ProcessAppointmentFailed() user error = %v, want %v", *updatedUser.State.Error, testError.Error())
		}
	})

	// Test GetAppointmentTranscript with nil CallID
	t.Run("GetAppointmentTranscriptNilCallID", func(t *testing.T) {
		appointment := CreateTestAppointment(t, client, user)
		// appointment.CallID is nil by default

		_, err := GetAppointmentTranscript(ctx, client, *appointment)
		if err == nil {
			t.Error("GetAppointmentTranscript() with nil CallID should return error")
		}
		if err.Error() != "appointment does not have a call ID" {
			t.Errorf("GetAppointmentTranscript() error = %v, want 'appointment does not have a call ID'", err)
		}
	})

	// Test ListAppointmentsNeedingProcessing with empty list
	t.Run("ListAppointmentsNeedingProcessingEmpty", func(t *testing.T) {
		// Create a new user with no processing appointments
		testUser := CreateTestUser(t, client, "No Processing User", "noprocessing@example.com", "+14251234572", false)

		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		processingAppointments, err := ListAppointmentsNeedingProcessing(ctx, client, logger)
		if err != nil {
			t.Fatalf("ListAppointmentsNeedingProcessing() error = %v", err)
		}

		// Should not contain any appointments for this user
		for _, a := range processingAppointments {
			if a.UserID == testUser.ID {
				t.Error("Found appointment for user that shouldn't have any processing appointments")
			}
		}
	})
}

// Test MapVapiEndReasonToAppointmentStatus function
func TestMapVapiEndReasonToAppointmentStatus(t *testing.T) {
	tests := []struct {
		name          string
		endReason     string
		wantStatus    string
		wantShouldLog bool
	}{
		{
			name:          "unknown reason returns reason and should log",
			endReason:     "unknown-reason",
			wantStatus:    "unknown-reason",
			wantShouldLog: true,
		},
		{
			name:          "empty reason returns empty and should log",
			endReason:     "",
			wantStatus:    "",
			wantShouldLog: true,
		},
		{
			name:          "custom reason returns reason and should log",
			endReason:     "custom-end-reason",
			wantStatus:    "custom-end-reason",
			wantShouldLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotShouldLog := MapVapiEndReasonToAppointmentStatus(tt.endReason)
			if gotStatus != tt.wantStatus {
				t.Errorf("MapVapiEndReasonToAppointmentStatus() status = %v, want %v", gotStatus, tt.wantStatus)
			}
			if gotShouldLog != tt.wantShouldLog {
				t.Errorf("MapVapiEndReasonToAppointmentStatus() shouldLog = %v, want %v", gotShouldLog, tt.wantShouldLog)
			}
		})
	}
}

// Test edge cases and error conditions
func TestAppointmentEdgeCases(t *testing.T) {
	ctx := context.Background()

	client, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("newTestFirestoreClient() error = %v", err)
	}

	// Clean up before and after tests
	DeleteTestData(t, client, "test-user-")
	t.Cleanup(func() {
		DeleteTestData(t, client, "test-user-")
		client.Close()
	})

	user := CreateTestUser(t, client, "Edge Case User", "edge@example.com", "+14251234573", false)

	// Test CreateAppointment with empty userID
	t.Run("CreateAppointmentEmptyUserID", func(t *testing.T) {
		appointment := &Appointment{
			Name:          "Test Appointment",
			ScheduledTime: time.Now().Add(24 * time.Hour),
			Timezone:      "UTC",
			Status:        AppointmentStatusScheduled,
		}

		err := CreateAppointment(ctx, client, "", appointment)
		if err == nil {
			t.Error("CreateAppointment() with empty userID should return error")
		}
	})

	// Test GetAppointment with empty userID
	t.Run("GetAppointmentEmptyUserID", func(t *testing.T) {
		_, err := GetAppointment(ctx, client, "", "some-id")
		if err == nil {
			t.Error("GetAppointment() with empty userID should return error")
		}
	})

	// Test GetAppointment with empty appointmentID
	t.Run("GetAppointmentEmptyAppointmentID", func(t *testing.T) {
		_, err := GetAppointment(ctx, client, user.ID, "")
		if err == nil {
			t.Error("GetAppointment() with empty appointmentID should return error")
		}
	})

	// Test CancelAppointment with empty userID
	t.Run("CancelAppointmentEmptyUserID", func(t *testing.T) {
		err := CancelAppointment(ctx, client, "", "some-id")
		if err == nil {
			t.Error("CancelAppointment() with empty userID should return error")
		}
	})

	// Test CancelAppointment with empty appointmentID
	t.Run("CancelAppointmentEmptyAppointmentID", func(t *testing.T) {
		err := CancelAppointment(ctx, client, user.ID, "")
		if err == nil {
			t.Error("CancelAppointment() with empty appointmentID should return error")
		}
	})

	// Test UpdateAppointment with nil appointment
	t.Run("UpdateAppointmentNil", func(t *testing.T) {
		err := UpdateAppointment(ctx, client, nil)
		if err == nil {
			t.Error("UpdateAppointment() with nil appointment should return error")
		}
	})

	// Test appointment with all optional fields set
	t.Run("AppointmentWithAllFields", func(t *testing.T) {
		now := time.Now().UTC().Round(time.Second)
		callID := "test-call-id"
		summary := "Test summary"
		endReason := "test-end-reason"
		errorMsg := "test error"
		notes := "test notes"

		appointment := &Appointment{
			Name:          "Full Test Appointment",
			ScheduledTime: now.Add(24 * time.Hour),
			Timezone:      "America/New_York",
			Status:        AppointmentStatusCompleted,
			EndReason:     &endReason,
			Error:         &errorMsg,
			CallID:        &callID,
			Summary:       &summary,
			CompletedAt:   &now,
			Notes:         &notes,
		}

		err := CreateAppointment(ctx, client, user.ID, appointment)
		if err != nil {
			t.Fatalf("CreateAppointment() error = %v", err)
		}

		// Verify all fields were saved correctly
		got, err := GetAppointment(ctx, client, user.ID, appointment.ID)
		if err != nil {
			t.Fatalf("GetAppointment() error = %v", err)
		}

		if got.CallID == nil || *got.CallID != callID {
			t.Errorf("GetAppointment() CallID = %v, want %v", got.CallID, callID)
		}
		if got.Summary == nil || *got.Summary != summary {
			t.Errorf("GetAppointment() Summary = %v, want %v", got.Summary, summary)
		}
		if got.EndReason == nil || *got.EndReason != endReason {
			t.Errorf("GetAppointment() EndReason = %v, want %v", got.EndReason, endReason)
		}
		if got.Error == nil || *got.Error != errorMsg {
			t.Errorf("GetAppointment() Error = %v, want %v", got.Error, errorMsg)
		}
		if got.Notes == nil || *got.Notes != notes {
			t.Errorf("GetAppointment() Notes = %v, want %v", got.Notes, notes)
		}
		if got.Timezone != "America/New_York" {
			t.Errorf("GetAppointment() Timezone = %v, want %v", got.Timezone, "America/New_York")
		}
	})
}
