package transcript

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search"
	"talent-rodeo/internal/utils"
	"testing"
	"time"

	"github.com/chriscow/vapi-go"

	"github.com/chriscow/minds"
	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
)

func init() {
	if err := godotenv.Overload("../../../.env"); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
}

func TestConvertCallResultToTranscript(t *testing.T) {
	// Read the test data file
	data, err := os.ReadFile("../../../testdata/vapi/create-call-response.json")
	if err != nil {
		t.Fatalf("Failed to read test data: %v", err)
	}

	// Parse the JSON into a struct that has the messages field
	var callResult vapi.Call
	if err := json.Unmarshal(data, &callResult); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}

	// Convert the messages to a transcript
	transcript := convertCallResultToTranscript(callResult)

	// Verify the conversion
	if len(transcript.Messages) == 0 {
		t.Error("Expected non-empty transcript messages")
	}

	// Define valid OpenAI roles
	validRoles := map[minds.Role]bool{
		minds.RoleSystem:    true,
		minds.RoleUser:      true,
		minds.RoleAssistant: true,
		minds.RoleFunction:  true,
		minds.RoleTool:      true,
		"developer":         true,
	}

	// Check role mapping and content
	for i, msg := range transcript.Messages {
		// Verify roles are correctly mapped to valid OpenAI roles
		if !validRoles[msg.Role] {
			t.Errorf("Message %d has invalid role: %s", i, msg.Role)
		}

		// Verify content is not empty (except for tool/function calls which might be empty)
		if msg.Content == "" && msg.Role != openai.ChatMessageRoleTool && msg.Role != openai.ChatMessageRoleFunction {
			t.Errorf("Message %d has empty content with role %s", i, msg.Role)
		}
	}

	// Test specific role mappings from the call response
	expectedMappings := []struct {
		vapiRole   string
		openAIRole string
		hasContent bool
		sampleText string
	}{
		{
			vapiRole:   "system",
			openAIRole: openai.ChatMessageRoleSystem,
			hasContent: true,
			sampleText: "You are Shirley, an AI job search specialist at Talent Rodeo",
		},
		{
			vapiRole:   "bot",
			openAIRole: openai.ChatMessageRoleAssistant,
			hasContent: true,
			sampleText: "Hi there. I'm Shirley from Talent Rodeo",
		},
		{
			vapiRole:   "user",
			openAIRole: openai.ChatMessageRoleUser,
			hasContent: true,
			sampleText: "Sure",
		},
		{
			vapiRole:   "tool_calls",
			openAIRole: openai.ChatMessageRoleTool,
			hasContent: false,
		},
		{
			vapiRole:   "tool_call_result",
			openAIRole: openai.ChatMessageRoleFunction,
			hasContent: false,
		},
	}

	// Verify each expected mapping exists in the transcript
	for _, mapping := range expectedMappings {
		found := false
		for _, msg := range transcript.Messages {
			if msg.Role == minds.Role(mapping.openAIRole) {
				if mapping.hasContent && mapping.sampleText != "" {
					if mapping.sampleText != "" && msg.Content != "" && strings.Contains(msg.Content, mapping.sampleText) {
						found = true
						break
					}
				} else {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("Failed to find message with role %s mapped from VAPI role %s",
				mapping.openAIRole, mapping.vapiRole)
		}
	}
}

func TestTranscriptProcessor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))

	client, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("internal.NewFirestoreClient() error = %v", err)
	}

	// Clean up test data at the start
	internal.DeleteTestData(t, client, "test-user-")

	// Defer cleanup and client close
	defer func() {
		// Create a new context for cleanup since the original might be cancelled
		cleanupCtx := context.Background()
		cleanupClient, err := internal.NewFirestoreClient(cleanupCtx)
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
		if testing.Short() {
			t.Skip("skipping test in short mode")
		}

		// Create an appointment with a transcript
		appointment := &internal.Appointment{
			ID:            "test-completed-appointment",
			UserID:        user.ID,
			Name:          "Test Completed Appointment",
			ScheduledTime: time.Now(),
			Status:        internal.AppointmentStatusCompleted,
		}
		callID := "test-call-3"
		appointment.CallID = &callID
		summary := "Candidate has 5 years of Go experience and is looking for senior engineering roles"
		appointment.Summary = &summary

		if err := processor.process(ctx, user, appointment); err != nil {
			t.Fatalf("ProcessCompletedAppointment() error = %v", err)
		}

		// Verify user search was updated
		updatedUser, found, err := internal.GetUser(ctx, client, user.ID)
		if err != nil {
			t.Fatalf("internal.GetUser() error = %v", err)
		}
		if !found {
			t.Fatal("internal.GetUser() user not found")
		}
		if updatedUser.Search == nil {
			t.Error("ProcessCompletedAppointment() did not update user search")
		}

		// Get the updated appointment to verify all prompt results were saved
		updatedAppointment, err := internal.GetAppointment(ctx, client, user.ID, appointment.ID)
		if err != nil {
			t.Fatalf("internal.GetAppointment() error = %v", err)
		}

		// Verify each prompt result field was saved in the ExtractedData struct
		if updatedAppointment.ExtractedData == nil {
			t.Error("ProcessCompletedAppointment() did not save ExtractedData")
		} else {
			if updatedAppointment.ExtractedData.TranscriptExtract == nil {
				t.Error("ProcessCompletedAppointment() did not save TranscriptExtract")
			}

			if updatedAppointment.ExtractedData.ResolvedPreferences == nil {
				t.Error("ProcessCompletedAppointment() did not save ResolvedPreferences")
			}

			if updatedAppointment.ExtractedData.WorkPreferences == nil {
				t.Error("ProcessCompletedAppointment() did not save WorkPreferences")
			}

			if updatedAppointment.ExtractedData.KeywordAnalysis == nil {
				t.Error("ProcessCompletedAppointment() did not save KeywordAnalysis")
			}

			if updatedAppointment.ExtractedData.UserStatement == nil {
				t.Error("ProcessCompletedAppointment() did not save UserStatement")
			}
		}
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
		_, err := minds.NewResponseSchema("search-criteria", "Structured search criteria from the transcript analysis", search.Criteria{})
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

func TestGetPromptResults(t *testing.T) {
	ctx := context.Background()
	client, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("internal.NewFirestoreClient() error = %v", err)
	}
	defer client.Close()

	// Create test user
	user := internal.CreateTestUser(t, client, "Test User Results", "test-results@example.com", "+14251234568", false)

	// Clean up at the end
	defer internal.DeleteTestData(t, client, "test-user-")

	// Create an appointment with prompt results
	appointment := &internal.Appointment{
		ID:            "test-prompt-results",
		UserID:        user.ID,
		Name:          "Test Prompt Results",
		ScheduledTime: time.Now(),
		Status:        internal.AppointmentStatusCompleted,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		ExtractedData: &internal.ExtractedData{},
	}

	// Set sample prompt results
	extractedData := "Sample extracted data"
	resolvedPrefs := "Sample resolved preferences"
	workPrefs := "Sample work preferences"
	keywordAnalysis := "Sample keyword analysis"
	userStatement := "Sample user statement"
	transcriptSummary := "Sample transcript summary"

	appointment.ExtractedData.TranscriptExtract = &extractedData
	appointment.ExtractedData.ResolvedPreferences = &resolvedPrefs
	appointment.ExtractedData.WorkPreferences = &workPrefs
	appointment.ExtractedData.KeywordAnalysis = &keywordAnalysis
	appointment.ExtractedData.UserStatement = &userStatement
	appointment.Summary = &transcriptSummary

	// Save the appointment
	err = internal.CreateAppointment(ctx, client, "Test Prompt Results", *user, appointment)
	if err != nil {
		t.Fatalf("internal.CreateAppointment() error = %v", err)
	}

	// Get the prompt results
	results, err := GetPromptResults(ctx, client, user.ID, appointment.ID)
	if err != nil {
		t.Fatalf("GetPromptResults() error = %v", err)
	}

	// Verify each result
	expectedResults := map[string]string{
		"ExtractedData":       extractedData,
		"ResolvedPreferences": resolvedPrefs,
		"WorkPreferences":     workPrefs,
		"KeywordAnalysis":     keywordAnalysis,
		"UserStatement":       userStatement,
		"TranscriptSummary":   transcriptSummary,
	}

	if len(results) != len(expectedResults) {
		t.Errorf("GetPromptResults() returned %d results, want %d", len(results), len(expectedResults))
	}

	for key, expectedValue := range expectedResults {
		if results[key] != expectedValue {
			t.Errorf("GetPromptResults() for %s = %s, want %s", key, results[key], expectedValue)
		}
	}
}

// TestProcessorWithMockTranscript tests the processor using a mock transcript file instead of calling VAPI API
