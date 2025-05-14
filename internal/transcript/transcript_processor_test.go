package transcript

import (
	"context"
	"log/slog"
	"os"
	"talent-rodeo/internal"
	"talent-rodeo/internal/utils"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/tools"
)

func TestTranscriptProcessor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))

	client, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("internal.NewTestFirestoreClient() error = %v", err)
	}

	// Clean up test data at the start
	internal.DeleteTestData(t, client, "test-user-")

	// Defer cleanup and client close
	defer func() {
		// Create a new context for cleanup since the original might be cancelled
		cleanupCtx := context.Background()
		cleanupClient, err := internal.NewTestFirestoreClient(cleanupCtx)
		if err != nil {
			t.Logf("Failed to create cleanup client: %v", err)
			return
		}
		defer cleanupClient.Close()

		internal.DeleteTestData(t, cleanupClient, "test-user-")
	}()
	defer client.Close()

	// Create test user
	user := internal.CreateTestUser(t, client, "Test User", "test@example.com", "+14251234567", false)

	// Create test processor
	processor, err := newProcessor(client, logger)
	if err != nil {
		t.Fatalf("NewTranscriptProcessor() error = %v", err)
	}

	t.Run("ProcessCompletedAppointment", func(t *testing.T) {
		// Set up mock responses for each prompt
		mockResponses := []string{
			// Mock response for transcript extract
			`{
				"skills": ["Go", "Python", "AWS"],
				"experience": "5 years of software development",
				"jobPreferences": "Senior engineering roles"
			}`,
			// Mock response for resolved preferences
			`{
				"roleTypes": ["Software Engineer", "Tech Lead"],
				"experienceLevel": "Senior",
				"locations": ["US-WA-SEA", "US-CA-SF"]
			}`,
			// Mock response for work preferences
			`{
				"remotePreference": "Hybrid",
				"salaryRange": "$150,000 - $200,000",
				"industryPreferences": ["Technology", "Finance"]
			}`,
			// Mock response for keyword analysis
			`{
				"keywords": ["golang", "distributed systems", "cloud architecture"],
				"industries": ["tech", "finance"],
				"roles": ["senior engineer", "tech lead"]
			}`,
			// Mock response for user statement
			`{"statement": "Professional software engineer with 5 years of experience in Go and distributed systems"}`,
		}

		// Create a channel to feed mock responses in sequence
		responseIndex := 0
		tools.MockLLMResponse = mockResponses[responseIndex]
		tools.MockLLMError = nil

		// Create a goroutine to update the mock response after each LLM call
		done := make(chan bool)
		go func() {
			for responseIndex < len(mockResponses)-1 {
				time.Sleep(100 * time.Millisecond) // Give time for the LLM call to complete
				responseIndex++
				tools.MockLLMResponse = mockResponses[responseIndex]
			}
			done <- true
		}()

		// Create an appointment with a transcript
		appointment := &internal.Appointment{
			ID:            "test-completed-appointment",
			UserID:        user.ID,
			Name:          "Test Completed Appointment",
			ScheduledTime: time.Now(),
			Status:        internal.AppointmentStatusCompleted,
		}
		callID := "2709318a-1b74-44da-9ba9-e1973804b6b9"
		appointment.CallID = &callID
		summary := "Candidate has 5 years of Go experience and is looking for senior engineering roles"
		appointment.Summary = &summary

		if err := processor.process(ctx, user, appointment); err != nil {
			t.Fatalf("ProcessCompletedAppointment() error = %v", err)
		}

		// Wait for all mock responses to be used
		<-done

		// Verify user search was updated
		updatedUser, found, err := internal.GetUser(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("internal.GetUser() error = %v", err)
		}
		if !found {
			t.Fatal("internal.GetUser() user not found")
		}
		if updatedUser.SteeringQuery == nil {
			t.Error("ProcessCompletedAppointment() did not update user search")
		}

		if updatedUser.WorkPreferences == nil {
			t.Error("ProcessCompletedAppointment() did not update user work preferences")
		}

		subs, err := internal.GetSubscriptionsForUser(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("internal.GetSubscriptionsForUser() error = %v", err)
		}
		if len(subs) == 0 {
			t.Error("ProcessCompletedAppointment() did not create subscriptions")
		}
		if len(subs) != 1 {
			t.Errorf("ProcessCompletedAppointment() created %d subscriptions, want 1", len(subs))
		}

		// Get the updated appointment to verify all prompt results were saved
		// updatedAppointment, err := internal.GetAppointment(ctx, client, user.ID, appointment.ID)
		// if err != nil {
		// 	t.Fatalf("internal.GetAppointment() error = %v", err)
		// }

		// Verify each prompt result field was saved in the ExtractedData struct
		// if updatedAppointment.ExtractedData == nil {
		// 	t.Error("ProcessCompletedAppointment() did not save ExtractedData")
		// } else {
		// 	if updatedAppointment.ExtractedData.TranscriptExtract == nil {
		// 		t.Error("ProcessCompletedAppointment() did not save TranscriptExtract")
		// 	}

		// 	// if updatedAppointment.ExtractedData.ResolvedPreferences == nil {
		// 	// 	t.Error("ProcessCompletedAppointment() did not save ResolvedPreferences")
		// 	// }

		// 	if updatedAppointment.ExtractedData.WorkPreferences == nil {
		// 		t.Error("ProcessCompletedAppointment() did not save WorkPreferences")
		// 	}

		// 	if updatedAppointment.ExtractedData.SearchQueries == nil {
		// 		t.Error("ProcessCompletedAppointment() did not save SearchQueries")
		// 	}

		// 	if updatedAppointment.ExtractedData.UserStatement == nil {
		// 		t.Error("ProcessCompletedAppointment() did not save UserStatement")
		// 	}
		// }
	})
}

func TestTemplateExecution(t *testing.T) {
	templates := []struct {
		name     string
		file     string
		testData any
	}{
		{
			name: "Extract Template",
			file: "prompts/transcript-010-extract.txt",
			testData: map[string]any{
				"Transcript": "Sample transcript text with some conversation between the assistant and user " +
					"discussing job preferences and experience.",
			},
		},
		{
			name: "Resolved Preferences Template",
			file: "prompts/transcript-020-resolved-prefs.txt",
			testData: map[string]any{
				"ExtractedData": "Sample extracted data about job preferences and skills",
			},
		},
		{
			name: "Work Location Template",
			file: "prompts/transcript-030-work-location.txt",
			testData: map[string]any{
				"ExtractedData":       "Sample extracted data",
				"ResolvedPreferences": "Sample resolved preferences",
				"Locations":           []string{"US-CA-SF", "US-WA-SEA"},
			},
		},
		{
			name: "Search Template",
			file: "prompts/transcript-040-search.txt",
			testData: map[string]any{
				"ExtractedData":       "Sample extracted data",
				"ResolvedPreferences": "Sample resolved preferences",
			},
		},
		{
			name: "User Statement Template",
			file: "prompts/transcript-050-user-statement.txt",
			testData: map[string]any{
				"ExtractedData":       "Sample extracted data",
				"ResolvedPreferences": "Sample resolved preferences",
				"WorkAnalysis":        "Sample work analysis",
				"SearchPhrases":       "Sample search phrases",
			},
		},
	}

	for _, tt := range templates {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := utils.CreateTemplate(tt.file)
			if err != nil {
				t.Fatalf("Failed to create template %s: %v", tt.file, err)
			}

			_, err = tmpl.Execute(tt.testData)
			if err != nil {
				t.Errorf("Template execution failed for %s: %v", tt.file, err)
			}
		})
	}
}

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
	err = internal.MarkAppointmentAsProcessing(ctx, client, appointment)
	if err != nil {
		t.Fatalf("internal.MarkAppointmentAsProcessing() error = %v", err)
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
	err = RecoverProcessingAppointments(hub, client, logger)
	if err != nil {
		t.Fatalf("RecoverProcessingAppointments() error = %v", err)
	}

	// In a real test, we might want to verify that the appointment is eventually completed
	// This would require a way to wait for the async processing to finish
	// For now, we'll just verify the recovery function didn't error
	t.Log("Recovery process started successfully")
}
