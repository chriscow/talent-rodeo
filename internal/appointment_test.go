package internal

import (
	"context"
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

	// Test GetAppointments
	t.Run("GetAppointments", func(t *testing.T) {
		// Create a second appointment for the same user
		CreateTestAppointment(t, client, user)

		appointments, err := ListAppointments(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetAppointments() error = %v", err)
		}
		if len(appointments) != 2 {
			t.Errorf("GetAppointments() got %d appointments, want 2", len(appointments))
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
	})

	// Test appointment not found
	t.Run("AppointmentNotFound", func(t *testing.T) {
		_, err := GetAppointment(ctx, client, user.ID, "non-existent-id")
		if err == nil {
			t.Error("GetAppointment() with non-existent ID should return error")
		}
	})

	// Test processing appointment tracking
	t.Run("ProcessingAppointmentTracking", func(t *testing.T) {
		ctx := context.Background()

		// Create an appointment
		appointment := CreateTestAppointment(t, client, user)

		// Mark as processing
		err := MarkAppointmentAsProcessing(ctx, client, appointment)
		if err != nil {
			t.Fatalf("MarkAppointmentAsProcessing() error = %v", err)
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
		processingAppointments, err := ListAppointmentsNeedingProcessing(ctx, client)
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
		err = CompleteProcessing(ctx, client, appointment)
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
		processingAppointments, err = ListAppointmentsNeedingProcessing(ctx, client)
		if err != nil {
			t.Fatalf("ListAppointmentsNeedingProcessing() error = %v", err)
		}

		for _, a := range processingAppointments {
			if a.ID == appointment.ID {
				t.Error("Appointment still found in processing list after completion")
			}
		}
	})
}
