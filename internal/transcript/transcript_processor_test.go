package transcript

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"talent-rodeo/internal"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/chriscow/minds"
)

func TestNewResponseSchema(t *testing.T) {
	t.Run("valid schema", func(t *testing.T) {
		_, err := minds.NewResponseSchema("search-criteria", "Structured search criteria from the transcript analysis", internal.SearchCriteria{})
		if err != nil {
			t.Errorf("NewResponseSchema() error = %v, want nil", err)
		}
	})

	t.Run("invalid schema", func(t *testing.T) {
		// Pass a channel which cannot be converted to JSON schema
		invalidType := make(chan int)
		_, err := minds.NewResponseSchema("invalid", "This should fail", invalidType)
		if err == nil {
			t.Error("NewResponseSchema() error = nil, want error for invalid schema type")
		}
	})
}

func TestRecoverProcessingAppointments(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	hub := sentry.CurrentHub()

	client, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("internal.NewTestFirestoreClient() error = %v", err)
	}

	// Clean up test data
	internal.DeleteTestData(t, client, "test-recovery-")
	defer func() {
		// Create a new context for cleanup since the original might be cancelled
		cleanupCtx := context.Background()
		cleanupClient, err := internal.NewTestFirestoreClient(cleanupCtx)
		if err != nil {
			t.Logf("Failed to create cleanup client: %v", err)
			return
		}
		defer cleanupClient.Close()
		internal.DeleteTestData(t, cleanupClient, "test-recovery-")
	}()
	defer client.Close()

	// Create test user
	user := internal.CreateTestUser(t, client, "Test Recovery", "test-recovery@example.com", "+14251234599", false)

	// Create an appointment and mark it as processing
	appointment := &internal.Appointment{
		ID:            "test-recovery-appointment",
		UserID:        user.ID,
		Name:          "Test Recovery Appointment",
		ScheduledTime: time.Now(),
		Status:        internal.AppointmentStatusScheduled,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err = internal.CreateAppointment(ctx, client, user.ID, appointment)
	if err != nil {
		t.Fatalf("internal.CreateAppointment() error = %v", err)
	}

	// Mark as processing - this adds to the processing collection
	err = internal.ProcessAppointmentStarted(ctx, client, user, appointment)
	if err != nil {
		t.Fatalf("internal.ProcessAppointmentStarted() error = %v", err)
	}

	// Simulate server restart by checking for appointments needing processing
	processingAppointments, err := internal.ListAppointmentsNeedingProcessing(ctx, client, logger)
	if err != nil {
		t.Fatalf("internal.ListAppointmentsNeedingProcessing() error = %v", err)
	}

	// Verify our appointment is in the processing list
	found := false
	for _, a := range processingAppointments {
		if a.ID == appointment.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Appointment not found in processing list")
	}

	// Set a mock call ID and summary so the processor has data to work with
	callID := "test-call-recovery"
	appointment.CallID = &callID
	summary := "This is a test summary for recovery"
	appointment.Summary = &summary

	// Update the appointment in Firestore with these changes
	if err := internal.UpdateAppointment(ctx, client, appointment); err != nil {
		t.Fatalf("internal.UpdateAppointment() error = %v", err)
	}

	// Run the recovery process - note that this would normally complete asynchronously
	// so we're just testing that it starts the background job without errors
	err = RecoverProcessingAppointments(hub, client, nil, logger, nil, nil, nil)
	if err != nil {
		t.Fatalf("RecoverProcessingAppointments() error = %v", err)
	}

	// In a real test, we might want to verify that the appointment is eventually completed
	// This would require a way to wait for the async processing to finish
	// For now, we'll just verify the recovery function didn't error
	t.Log("Recovery process started successfully")
}

func TestExtractWorkPreferences(t *testing.T) {
	ctx := context.Background()

	t.Run("valid final determinations", func(t *testing.T) {
		data := "Some text before <final_determinations>Remote work preferred, San Francisco Bay Area</final_determinations> some text after"
		result, err := extractWorkPreferences(ctx, data)
		if err != nil {
			t.Errorf("extractWorkPreferences() error = %v, want nil", err)
		}
		if result == nil {
			t.Error("extractWorkPreferences() result = nil, want non-nil")
		}
		expected := "Remote work preferred, San Francisco Bay Area"
		if *result != expected {
			t.Errorf("extractWorkPreferences() = %v, want %v", *result, expected)
		}
	})

	t.Run("missing opening tag", func(t *testing.T) {
		data := "Some text without tags Remote work preferred</final_determinations>"
		_, err := extractWorkPreferences(ctx, data)
		if err == nil {
			t.Error("extractWorkPreferences() error = nil, want error for missing opening tag")
		}
		if !strings.Contains(err.Error(), "<final_determinations> tag not found") {
			t.Errorf("extractWorkPreferences() error = %v, want error about missing opening tag", err)
		}
	})

	t.Run("missing closing tag", func(t *testing.T) {
		data := "Some text <final_determinations>Remote work preferred without closing"
		_, err := extractWorkPreferences(ctx, data)
		if err == nil {
			t.Error("extractWorkPreferences() error = nil, want error for missing closing tag")
		}
		if !strings.Contains(err.Error(), "</final_determinations> tag not found") {
			t.Errorf("extractWorkPreferences() error = %v, want error about missing closing tag", err)
		}
	})

	t.Run("no determinations tags", func(t *testing.T) {
		data := "Some text without any special tags"
		_, err := extractWorkPreferences(ctx, data)
		if err == nil {
			t.Error("extractWorkPreferences() error = nil, want error for no tags")
		}
	})

	t.Run("empty determinations", func(t *testing.T) {
		data := "Some text <final_determinations></final_determinations> after"
		result, err := extractWorkPreferences(ctx, data)
		if err != nil {
			t.Errorf("extractWorkPreferences() error = %v, want nil", err)
		}
		if result == nil {
			t.Error("extractWorkPreferences() result = nil, want non-nil")
		}
		if *result != "" {
			t.Errorf("extractWorkPreferences() = %v, want empty string", *result)
		}
	})
}

func TestGetOrCreateSearchCriteria(t *testing.T) {
	ctx := context.Background()
	client, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("internal.NewTestFirestoreClient() error = %v", err)
	}

	// Clean up test data
	internal.DeleteTestData(t, client, "test-criteria-")
	defer func() {
		cleanupCtx := context.Background()
		cleanupClient, err := internal.NewTestFirestoreClient(cleanupCtx)
		if err != nil {
			t.Logf("Failed to create cleanup client: %v", err)
			return
		}
		defer cleanupClient.Close()
		internal.DeleteTestData(t, cleanupClient, "test-criteria-")
	}()
	defer client.Close()

	t.Run("missing steering query", func(t *testing.T) {
		user := &internal.User{
			ID:                 "test-criteria-user-1",
			LocationPreference: internal.StringPtr("Remote work in San Francisco"),
			// SteeringQuery is nil
		}

		_, _, err := getOrCreateSearchCriteria(ctx, client, user)
		if err == nil {
			t.Error("getOrCreateSearchCriteria() error = nil, want error for missing steering query")
		}
		if !strings.Contains(err.Error(), "steering query or work preferences not found") {
			t.Errorf("getOrCreateSearchCriteria() error = %v, want error about missing steering query", err)
		}
	})

	t.Run("missing work preferences", func(t *testing.T) {
		user := &internal.User{
			ID: "test-criteria-user-2",
			SteeringQuery: &internal.SteeringQuery{
				Query: "software engineer python",
			},
			// WorkPreferences is nil
		}

		_, _, err := getOrCreateSearchCriteria(ctx, client, user)
		if err == nil {
			t.Error("getOrCreateSearchCriteria() error = nil, want error for missing work preferences")
		}
		if !strings.Contains(err.Error(), "steering query or work preferences not found") {
			t.Errorf("getOrCreateSearchCriteria() error = %v, want error about missing work preferences", err)
		}
	})

	t.Run("create new search criteria", func(t *testing.T) {
		testGeoID := internal.GeoID{
			ID:    "test-criteria-geoid",
			Value: "San Francisco",
		}
		if err := internal.CreateGeoID(ctx, client, testGeoID); err != nil {
			t.Fatalf("Failed to create test geo ID: %v", err)
		}

		user := &internal.User{
			ID: "test-criteria-user-3",
			SteeringQuery: &internal.SteeringQuery{
				Query: "software engineer python",
			},
			LocationPreference: internal.StringPtr("Remote work in San Francisco"),
		}

		criteria, _, err := getOrCreateSearchCriteria(ctx, client, user)
		if err != nil {
			t.Errorf("getOrCreateSearchCriteria() error = %v, want nil", err)
		}
		if criteria == nil {
			t.Error("getOrCreateSearchCriteria() returned nil criteria")
		}
		if criteria.Query != "software engineer python" {
			t.Errorf("getOrCreateSearchCriteria() query = %v, want %v", criteria.Query, "software engineer python")
		}
	})

	t.Run("reuse existing search criteria", func(t *testing.T) {
		user := &internal.User{
			ID: "test-criteria-user-4",
			SteeringQuery: &internal.SteeringQuery{
				Query: "software engineer python",
			},
			LocationPreference: internal.StringPtr("Remote work in San Francisco"),
		}

		// Create criteria first time
		criteria1, _, err := getOrCreateSearchCriteria(ctx, client, user)
		if err != nil {
			t.Fatalf("getOrCreateSearchCriteria() first call error = %v", err)
		}

		// Call again with same data
		criteria2, _, err := getOrCreateSearchCriteria(ctx, client, user)
		if err != nil {
			t.Errorf("getOrCreateSearchCriteria() second call error = %v, want nil", err)
		}

		if criteria1.ID != criteria2.ID {
			t.Errorf("getOrCreateSearchCriteria() should reuse existing criteria, got different IDs: %v vs %v", criteria1.ID, criteria2.ID)
		}
	})
}

func TestProcessCompletedAppointmentErrorHandling(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	hub := sentry.CurrentHub()

	client, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("internal.NewTestFirestoreClient() error = %v", err)
	}

	// Clean up test data
	internal.DeleteTestData(t, client, "test-process-error-")
	defer func() {
		cleanupCtx := context.Background()
		cleanupClient, err := internal.NewTestFirestoreClient(cleanupCtx)
		if err != nil {
			t.Logf("Failed to create cleanup client: %v", err)
			return
		}
		defer cleanupClient.Close()
		internal.DeleteTestData(t, cleanupClient, "test-process-error-")
	}()
	defer client.Close()

	t.Run("appointment with nil call ID", func(t *testing.T) {
		user := internal.CreateTestUser(t, client, "Test Error User", "test-error@example.com", "+14251234567", false)
		appointment := &internal.Appointment{
			ID:            "test-process-error-appointment-1",
			UserID:        user.ID,
			Name:          "Test Error Appointment",
			ScheduledTime: time.Now(),
			Status:        internal.AppointmentStatusScheduled,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			CallID:        nil, // This should cause an error
		}

		err = internal.CreateAppointment(ctx, client, user.ID, appointment)
		if err != nil {
			t.Fatalf("internal.CreateAppointment() error = %v", err)
		}

		// This should not crash but should handle the error gracefully
		ProcessCompletedCall(hub, client, nil, nil, logger, nil, user, appointment, nil)

		// Verify appointment status was not changed to processing since CallID was nil
		updatedAppointment, err := internal.GetAppointment(ctx, client, user.ID, appointment.ID)
		if err != nil {
			t.Fatalf("internal.GetAppointment() error = %v", err)
		}
		if updatedAppointment.Status != internal.AppointmentStatusScheduled {
			t.Errorf("ProcessCompletedAppointment() with nil CallID changed status to %v, want %v",
				updatedAppointment.Status, internal.AppointmentStatusScheduled)
		}
	})

	t.Run("non-existent user", func(t *testing.T) {
		user := &internal.User{
			ID: "test-process-error-user-1",
		}

		appointment := &internal.Appointment{
			ID:            "test-process-error-appointment-2",
			UserID:        "non-existent-user-id",
			Name:          "Test Error Appointment",
			ScheduledTime: time.Now(),
			Status:        internal.AppointmentStatusScheduled,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			CallID:        internal.StringPtr("test-call-id"),
		}

		// This should not crash but should handle the error gracefully
		ProcessCompletedCall(hub, client, nil, nil, logger, nil, user, appointment, nil)

		// Since the user doesn't exist, the function should return early
		// and not create any processing entries
		processingAppointments, err := internal.ListAppointmentsNeedingProcessing(ctx, client, logger)
		if err != nil {
			t.Fatalf("internal.ListAppointmentsNeedingProcessing() error = %v", err)
		}

		for _, a := range processingAppointments {
			if a.ID == appointment.ID {
				t.Error("ProcessCompletedAppointment() with non-existent user should not create processing entry")
			}
		}
	})
}

func TestRecoverProcessingAppointmentsEdgeCases(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	hub := sentry.CurrentHub()

	client, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("internal.NewTestFirestoreClient() error = %v", err)
	}

	// Clean up test data
	internal.DeleteTestData(t, client, "test-recover-edge-")
	defer func() {
		cleanupCtx := context.Background()
		cleanupClient, err := internal.NewTestFirestoreClient(cleanupCtx)
		if err != nil {
			t.Logf("Failed to create cleanup client: %v", err)
			return
		}
		defer cleanupClient.Close()
		internal.DeleteTestData(t, cleanupClient, "test-recover-edge-")
	}()
	defer client.Close()

	t.Run("empty processing list", func(t *testing.T) {
		// Ensure no processing appointments exist for this test
		processingAppointments, err := internal.ListAppointmentsNeedingProcessing(ctx, client, logger)
		if err != nil {
			t.Fatalf("internal.ListAppointmentsNeedingProcessing() error = %v", err)
		}

		// Clean any existing processing appointments for a clean test
		for _, appt := range processingAppointments {
			if strings.HasPrefix(appt.ID, "test-recover-edge-") {
				user, found, err := internal.GetUser(ctx, client, appt.UserID)
				if err == nil && found {
					internal.ProcessAppointmentComplete(ctx, client, user, appt)
				}
			}
		}

		err = RecoverProcessingAppointments(hub, client, nil, logger, nil, nil, nil)
		if err != nil {
			t.Errorf("RecoverProcessingAppointments() with empty list error = %v, want nil", err)
		}
	})

	t.Run("multiple appointments to recover", func(t *testing.T) {
		user := internal.CreateTestUser(t, client, "Test Multiple Recovery", "test-multiple@example.com", "+14251234568", false)

		// Create multiple appointments and mark them as processing
		var appointments []*internal.Appointment
		for i := 0; i < 3; i++ {
			appointment := &internal.Appointment{
				ID:            fmt.Sprintf("test-recover-edge-appointment-%d", i),
				UserID:        user.ID,
				Name:          fmt.Sprintf("Test Multiple Recovery Appointment %d", i),
				ScheduledTime: time.Now(),
				Status:        internal.AppointmentStatusScheduled,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				CallID:        internal.StringPtr(fmt.Sprintf("test-call-%d", i)),
				Summary:       internal.StringPtr(fmt.Sprintf("Test summary %d", i)),
			}

			err = internal.CreateAppointment(ctx, client, user.ID, appointment)
			if err != nil {
				t.Fatalf("internal.CreateAppointment() error = %v", err)
			}

			err = internal.ProcessAppointmentStarted(ctx, client, user, appointment)
			if err != nil {
				t.Fatalf("internal.ProcessAppointmentStarted() error = %v", err)
			}

			appointments = append(appointments, appointment)
		}

		// Run recovery
		err = RecoverProcessingAppointments(hub, client, nil, logger, nil, nil, nil)
		if err != nil {
			t.Errorf("RecoverProcessingAppointments() with multiple appointments error = %v, want nil", err)
		}

		// Verify all appointments are being processed
		// Note: The actual processing happens asynchronously, so we just verify
		// the recovery function started without error
		t.Logf("Recovery process started for %d appointments", len(appointments))
	})
}

func TestGenerateSteeringQueryEdgeCases(t *testing.T) {
	ctx := context.Background()
	client, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("internal.NewTestFirestoreClient() error = %v", err)
	}

	// Clean up test data
	internal.DeleteTestData(t, client, "test-steering-")
	defer func() {
		cleanupCtx := context.Background()
		cleanupClient, err := internal.NewTestFirestoreClient(cleanupCtx)
		if err != nil {
			t.Logf("Failed to create cleanup client: %v", err)
			return
		}
		defer cleanupClient.Close()
		internal.DeleteTestData(t, cleanupClient, "test-steering-")
	}()
	defer client.Close()

	t.Run("empty transcript", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test in short mode")
		}
		user := &internal.User{
			ID: "test-steering-user-1",
		}

		// Create an empty transcript
		transcript := &internal.Transcript{
			Messages: []minds.Message{},
		}

		// This should handle empty transcript gracefully
		err := generateSteeringQuery(ctx, client, user, transcript)

		// The function might succeed with an empty query or fail gracefully
		// Either is acceptable behavior for an empty transcript
		if err != nil {
			t.Logf("generateSteeringQuery() with empty transcript error = %v (expected)", err)
		} else {
			t.Log("generateSteeringQuery() with empty transcript succeeded")
		}
	})

	t.Run("very short transcript", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test in short mode")
		}

		user := &internal.User{
			ID: "test-steering-user-2",
		}

		transcript := &internal.Transcript{
			Messages: []minds.Message{
				{
					Role:    minds.RoleUser,
					Content: "I prefer remote work.",
				},
			},
		}

		err := generateSteeringQuery(ctx, client, user, transcript)

		// Should handle short transcript gracefully
		if err != nil {
			t.Logf("generateSteeringQuery() with short transcript error = %v (may be expected)", err)
		} else {
			t.Log("generateSteeringQuery() with short transcript succeeded")
		}
	})
}

func TestGetWorkPreferencesEdgeCases(t *testing.T) {
	ctx := context.Background()
	client, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("internal.NewTestFirestoreClient() error = %v", err)
	}

	// Clean up test data
	internal.DeleteTestData(t, client, "test-work-prefs-")
	defer func() {
		cleanupCtx := context.Background()
		cleanupClient, err := internal.NewTestFirestoreClient(cleanupCtx)
		if err != nil {
			t.Logf("Failed to create cleanup client: %v", err)
			return
		}
		defer cleanupClient.Close()
		internal.DeleteTestData(t, cleanupClient, "test-work-prefs-")
	}()
	defer client.Close()

	t.Run("empty locations list", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test in short mode")
		}

		user := &internal.User{
			ID: "test-work-prefs-user-1",
		}

		transcript := &internal.Transcript{
			Messages: []minds.Message{
				{
					Role:    minds.RoleUser,
					Content: "I prefer remote work.",
				},
			},
		}

		var locations []string // empty slice

		err := getWorkPreferences(ctx, client, transcript, locations, user)

		// Should handle empty locations gracefully
		if err != nil {
			t.Logf("getWorkPreferences() with empty locations error = %v (may be expected)", err)
		} else {
			t.Log("getWorkPreferences() with empty locations succeeded")
		}
	})

	t.Run("nil locations list", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test in short mode")
		}

		user := &internal.User{
			ID: "test-work-prefs-user-2",
		}

		transcript := &internal.Transcript{
			Messages: []minds.Message{
				{
					Role:    minds.RoleUser,
					Content: "I want to work in San Francisco.",
				},
			},
		}

		err := getWorkPreferences(ctx, client, transcript, nil, user)

		// Should handle nil locations gracefully
		if err != nil {
			t.Logf("getWorkPreferences() with nil locations error = %v (may be expected)", err)
		} else {
			t.Log("getWorkPreferences() with nil locations succeeded")
		}
	})

	t.Run("transcript with no location mentions", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test in short mode")
		}

		user := &internal.User{
			ID: "test-work-prefs-user-3",
		}

		transcript := &internal.Transcript{
			Messages: []minds.Message{
				{
					Role:    minds.RoleUser,
					Content: "I like programming and solving problems.",
				},
			},
		}

		locations := []string{"san-francisco-ca", "new-york-ny", "austin-tx"}

		err := getWorkPreferences(ctx, client, transcript, locations, user)

		// Should handle transcript with no location mentions gracefully
		if err != nil {
			t.Logf("getWorkPreferences() with no location mentions error = %v (may be expected)", err)
		} else {
			t.Log("getWorkPreferences() with no location mentions succeeded")
		}
	})
}
