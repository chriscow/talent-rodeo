package internal

import (
	"context"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Overload("../.env"); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
}

func TestTranscriptProcessor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))

	client, err := NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("NewFirestoreClient() error = %v", err)
	}

	// Clean up test data at the start
	DeleteTestData(t, client, "test-user-")

	// Defer cleanup and client close
	defer func() {
		// Create a new context for cleanup since the original might be cancelled
		cleanupCtx := context.Background()
		cleanupClient, err := NewFirestoreClient(cleanupCtx)
		if err != nil {
			t.Logf("Failed to create cleanup client: %v", err)
			return
		}
		defer cleanupClient.Close()

		DeleteTestData(t, cleanupClient, "test-user-")
	}()
	defer client.Close()

	// Create test user
	user := CreateTestUser(t, client, "Test User", "test@example.com", "+14251234567", false)

	// Create test processor
	processor, err := NewTranscriptProcessor(client, logger)
	if err != nil {
		t.Fatalf("NewTranscriptProcessor() error = %v", err)
	}

	t.Run("CollectTranscripts", func(t *testing.T) {
		// Create appointments with transcripts
		appointment1 := CreateTestAppointment(t, client, user)
		callID1 := "test-call-1"
		appointment1.CallID = &callID1
		summary1 := "Test transcript summary 1"
		appointment1.TranscriptSummary = &summary1
		if err := UpdateAppointment(ctx, client, appointment1); err != nil {
			t.Fatalf("updateAppointment() error = %v", err)
		}

		appointment2 := CreateTestAppointment(t, client, user)
		callID2 := "test-call-2"
		appointment2.CallID = &callID2
		summary2 := "Test transcript summary 2"
		appointment2.TranscriptSummary = &summary2
		if err := UpdateAppointment(ctx, client, appointment2); err != nil {
			t.Fatalf("updateAppointment() error = %v", err)
		}

		// Test collecting transcripts
		got, err := processor.collectTranscripts(ctx, user.ID)
		if err != nil {
			t.Fatalf("collectTranscripts() error = %v", err)
		}

		want := map[string]string{
			callID1: summary1,
			callID2: summary2,
		}

		if len(got) != len(want) {
			t.Errorf("collectTranscripts() got %d transcripts, want %d", len(got), len(want))
		}

		for callID, wantTranscript := range want {
			if gotTranscript, ok := got[callID]; !ok {
				t.Errorf("collectTranscripts() missing transcript for call %s", callID)
			} else if gotTranscript != wantTranscript {
				t.Errorf("collectTranscripts() got transcript = %v, want %v for call %s", gotTranscript, wantTranscript, callID)
			}
		}
	})

	t.Run("AnalyzeTranscripts", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test in short mode")
		}

		transcripts := map[string]string{
			"test-call-1": "Candidate has 5 years of Go experience and strong backend skills",
			"test-call-2": "Looking for senior engineering role with focus on distributed systems",
		}

		analysis, err := processor.analyzeTranscripts(ctx, user.ID, transcripts)
		if err != nil {
			t.Fatalf("analyzeTranscripts() error = %v", err)
		}

		if analysis == "" {
			t.Error("analyzeTranscripts() got empty analysis")
		}
	})

	t.Run("GenerateSearchCriteria", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test in short mode")
		}

		analysis := `The candidate has demonstrated strong backend development experience with 5 years of Go programming. 
		They are interested in senior engineering positions focusing on distributed systems architecture.`

		search, err := processor.generateSearchCriteria(ctx, user.ID, analysis)
		if err != nil {
			t.Fatalf("generateSearchCriteria() error = %v", err)
		}

		if search == nil {
			t.Fatal("generateSearchCriteria() got nil search")
		}
	})

	t.Run("ProcessCompletedAppointment", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test in short mode")
		}

		// Create an appointment with a transcript
		appointment := &Appointment{
			ID:            "test-completed-appointment",
			UserID:        user.ID,
			Name:          "Test Completed Appointment",
			ScheduledTime: time.Now(),
			Status:        AppointmentStatusCompleted,
		}
		callID := "test-call-3"
		appointment.CallID = &callID
		summary := "Candidate has 5 years of Go experience and is looking for senior engineering roles"
		appointment.TranscriptSummary = &summary

		if err := processor.ProcessCompletedAppointment(ctx, user, appointment); err != nil {
			t.Fatalf("ProcessCompletedAppointment() error = %v", err)
		}

		// Verify user search was updated
		updatedUser, found, err := GetUser(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("GetUser() error = %v", err)
		}
		if !found {
			t.Fatal("GetUser() user not found")
		}
		if updatedUser.Search == nil {
			t.Error("ProcessCompletedAppointment() did not update user search")
		}
	})
}
