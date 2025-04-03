package internal

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Load("../.env"); err != nil {
		log.Fatal("No .env file found, using system env")
	}

	if os.Getenv("FIREBASE_CREDENTIALS_FILE") == "" {
		log.Fatal("FIREBASE_CREDENTIALS_FILE is not set")
	}

	if os.Getenv("FIREBASE_PROJECT_ID") == "" {
		log.Fatal("FIREBASE_PROJECT_ID is not set")
	}
}

func TestAppointment(t *testing.T) {
	ctx := context.Background()

	client, err := NewFirestoreClient(ctx, os.Getenv("FIREBASE_PROJECT_ID"), os.Getenv("FIREBASE_CREDENTIALS_FILE"))
	if err != nil {
		t.Fatalf("NewFirestoreClient() error = %v", err)
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

		appointments, err := GetAppointments(ctx, client, user.ID)
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

	// Test invalid appointment creation
	t.Run("InvalidAppointmentCreation", func(t *testing.T) {
		invalidAppointment := &Appointment{
			ID:     "test-invalid-appointment",
			UserID: "non-existent-user", // This should fail because we're using wrong user
		}

		// Create a new user object that doesn't exist in the database
		nonExistentUser := &User{
			ID:   "non-existent-user",
			Name: "Non Existent",
		}

		err := CreateAppointment(ctx, client, "InvalidAppointmentCreation test", *nonExistentUser, invalidAppointment)
		if err == nil {
			t.Error("CreateAppointment() with non-existent user should return error")
		}
	})
}

// TestGetSpecificAppointment is a temporary test to verify getting a specific appointment
func TestGetSpecificAppointment(t *testing.T) {
	ctx := context.Background()

	client, err := NewFirestoreClient(ctx, os.Getenv("FIREBASE_PROJECT_ID"), os.Getenv("FIREBASE_CREDENTIALS_FILE"))
	if err != nil {
		t.Fatalf("NewFirestoreClient() error = %v", err)
	}
	defer client.Close()

	userID := "Zwhbs7YdFy"
	appointmentID := "initial-call-1743886397809642797"

	got, err := GetAppointment(ctx, client, userID, appointmentID)
	if err != nil {
		t.Fatalf("GetAppointment() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetAppointment() got nil, want appointment")
	}

	t.Logf("Found appointment: ID=%s, UserID=%s, Status=%s, ScheduledTime=%v",
		got.ID, got.UserID, got.Status, got.ScheduledTime)
}
