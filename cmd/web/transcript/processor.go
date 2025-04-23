package transcript

import (
	"context"
	"fmt"
	"log/slog"

	// Removing self-import of internal package
	"talent-rodeo/internal"
	"talent-rodeo/internal/search"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/openai"
	"github.com/chriscow/vapi-go"
	"github.com/getsentry/sentry-go"
)

const (
	searchCriteriaModel = "gpt-4.1-nano"
	transcriptModel     = "gpt-4.1-nano"
)

// ProcessCompletedAppointment updates the appointment after a successful call with the transcript summary.
// It is called as a background task after the transcript is fully processed.
//
// # It is called after an an end of call webhook is received in handlers.vapi.webhooks.go
//
// The workflow is:
//
//   - Get all appointments
//   - For each appointment, get the transcript summary
//   - Generate a prompt for the transcript analysis. This is generated from all the call transcripts.
//   - Ask OpenAI to analyze the transcripts
//   - Generate a search criteria based on the analysis
//   - Save the search criteria to the user
//
// Saving a new search criteria triggers a new search.
func ProcessCompletedAppointment(hub *sentry.Hub, fs *firestore.Client, logger *slog.Logger, appointment *internal.Appointment) error {

	go func() {
		ctx := context.Background()
		sentry.SetHubOnContext(ctx, hub)

		user, found, err := internal.GetUser(ctx, fs, appointment.UserID)
		if err != nil {
			logger.Error("failed to get user", "error", err)
			return
		}
		if !found {
			logger.Error("user not found", "user_id", appointment.UserID)
			return
		}

		// Initialize user.Search if it is nil
		if user.Search == nil {
			user.Search = &internal.UserSearch{}
		}

		start := time.Now()

		processor, err := newProcessor(fs, logger)
		if err != nil {
			sentry.CaptureException(err)
			logger.Error("failed to create transcript processor", "error", err)
			return
		}

		err = processor.process(ctx, user, appointment)
		if err != nil {
			hub.CaptureException(err)
			logger.Error("failed to process completed appointment", "error", err)
			user.Search.Error = err.Error()
			if err := internal.UpdateUserSearch(ctx, fs, user.ID, user.Search); err != nil {
				hub.CaptureException(err)
				logger.Error("failed to update user search", "error", err)
			}
			// On error, mark the appointment as failed
			appointment.Status = internal.AppointmentStatusFailed
			appointment.FailedAt = &time.Time{}
			*appointment.FailedAt = time.Now()
			if err := internal.UpdateAppointment(ctx, fs, appointment); err != nil {
				logger.Error("failed to update appointment status to failed", "error", err)
			}
		}

		logger.Debug(internal.Blue("⏱️ appointment processing took"), "duration", time.Since(start).String())

		// Only mark as completed if the entire process succeeds
		appointment.Status = internal.AppointmentStatusCompleted
		appointment.CompletedAt = &time.Time{}
		*appointment.CompletedAt = time.Now()
		if err := internal.UpdateAppointment(ctx, fs, appointment); err != nil {
			logger.Error("failed to update appointment status to completed", "error", err)
		}
	}()

	return nil
}

// RecoverProcessingAppointments finds and reprocesses any appointments that were interrupted during processing
func RecoverProcessingAppointments(hub *sentry.Hub, fs *firestore.Client, logger *slog.Logger) error {
	ctx := context.Background()
	logger.Info("recovering processing appointments")

	appointments, err := internal.ListAppointmentsNeedingProcessing(ctx, fs)
	if err != nil {
		return fmt.Errorf("failed to list appointments needing processing: %w", err)
	}

	if len(appointments) > 0 {
		logger.Info("found appointments needing processing", "count", len(appointments))
	}

	for _, appointment := range appointments {
		logger.Info("reprocessing appointment",
			"appointment_id", appointment.ID,
			"user_id", appointment.UserID)

		if err := ProcessCompletedAppointment(hub, fs, logger, appointment); err != nil {
			logger.Error("failed to reprocess appointment",
				"error", err,
				"appointment_id", appointment.ID,
				"user_id", appointment.UserID)
			continue
		}
	}

	return nil
}

// processor handles the processing of appointment transcripts
type processor struct {
	fs     *firestore.Client
	logger *slog.Logger
	llm    *openai.Provider
}

// newProcessor creates a new TranscriptProcessor
func newProcessor(fs *firestore.Client, logger *slog.Logger) (*processor, error) {
	llm, err := openai.NewProvider(openai.WithModel(transcriptModel))
	if err != nil {
		return nil, fmt.Errorf("failed to create openai provider: %w", err)
	}

	return &processor{
		fs:     fs,
		logger: logger,
		llm:    llm,
	}, nil
}

// process updates the appointment after a successful call with the transcript summary
func (p *processor) process(ctx context.Context, user *internal.User, appointment *internal.Appointment) error {
	p.logger.Info(internal.BGGreen("⏱️ processing completed appointment"), "user_id", user.ID, "appointment_id", appointment.ID)
	defer p.logger.Info(internal.BGGreen("✅ processed completed appointment"), "user_id", user.ID, "appointment_id", appointment.ID)

	internal.SentryBreadcrumb(ctx, sentry.Breadcrumb{
		Type:     "appointment",
		Category: "processing",
		Message:  fmt.Sprintf("Processing completed appointment for user %s", user.ID),
		Data: map[string]any{
			"appointment_id": appointment.ID,
			"user_id":        user.ID,
		},
	})

	// Initialize the ExtractedData struct if it doesn't exist
	if appointment.ExtractedData == nil {
		appointment.ExtractedData = &internal.ExtractedData{}
	}

	p.logger.Debug("setting appointment state to `processing`", "user_id", user.ID, "appointment_id", appointment.ID)
	if err := p.setApptProcessing(ctx, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to update appointment with transcript summary: %w", err)
	}

	//
	// Get the supported locations (Geo ID keys)
	//
	p.logger.Debug("getting geo ids")
	locations, err := internal.GetGeoIDKeys(ctx, p.fs)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to get geo ids: %w", err)
	}

	//
	// Get the transcript from the VAPI call
	//
	p.logger.Debug("getting transcript from VAPI call", "call_id", *appointment.CallID)
	transcript, err := p.getTranscript(ctx, *appointment.CallID)
	if err != nil {
		return fmt.Errorf("processor: failed to get transcript: %w", err)
	}

	//
	// Extract data from the transcript
	//
	if err := p.extract(ctx, transcript.String(), appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return err
	}

	//
	// Resolve discrepancies in the extracted data
	//
	if err := p.resolveDiscrepancies(ctx, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to resolve appointment: %w", err)
	}

	//
	// Get work preferences
	//
	if err := p.getWorkPreferences(ctx, locations, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to get work preferences: %w", err)
	}

	//
	// Generate keyword analysis
	//
	if err := p.generateKeywordAnalysis(ctx, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate keyword analysis: %w", err)
	}

	//
	// Generate user statement
	//
	if err := p.generateUserStatement(ctx, user, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate user statement: %w", err)
	}

	//
	// Finally, generate search criteria
	//
	return p.generateSearchCriteria(ctx, user, appointment)
}

func (p *processor) extract(ctx context.Context, transcript string, appointment *internal.Appointment) error {
	p.logger.Debug("extracting data from transcript", "user_id", appointment.UserID, "appointment_id", appointment.ID)

	start := time.Now()
	prompt, err := internal.GeneratePrompt("prompts/transcript-010-extract.txt", map[string]any{
		"Transcript": transcript,
	})
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type":  "prompt_execution",
			"entity_type": "appointment",
		})
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	data, err := internal.Ask(ctx, prompt)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to ask LLM: %w", err)
	}

	// Save extracted data to appointment
	appointment.ExtractedData.TranscriptExtract = &data
	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to update appointment with extracted data: %w", err)
	}

	p.logger.Debug("extracted data from transcript", "user_id", appointment.UserID, "appointment_id", appointment.ID, "elapsed", time.Since(start).String())
	return nil
}

func (p *processor) resolveDiscrepancies(ctx context.Context, appointment *internal.Appointment) error {
	p.logger.Debug("resolving preferences", "user_id", appointment.UserID, "appointment_id", appointment.ID)
	prompt, err := internal.GeneratePrompt("prompts/transcript-020-resolved-prefs.txt", map[string]any{
		"ExtractedData": appointment.ExtractedData,
	})
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	// Ask LLM to resolve preferences
	data, err := internal.Ask(ctx, prompt)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to ask LLM: %w", err)
	}

	// Save resolved preferences to appointment
	appointment.ExtractedData.ResolvedPreferences = &data
	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to update appointment with resolved preferences: %w", err)
	}

	return nil
}

func (p *processor) getWorkPreferences(ctx context.Context, locations []string, appointment *internal.Appointment) error {
	p.logger.Debug("resolving work preferences", "user_id", appointment.UserID, "appointment_id", appointment.ID)

	prompt, err := internal.GeneratePrompt("prompts/transcript-030-work-location.txt", map[string]any{
		"ExtractedData":       appointment.ExtractedData.TranscriptExtract,
		"ResolvedPreferences": appointment.ExtractedData.ResolvedPreferences,
		"Locations":           locations,
	})
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	data, err := internal.Ask(ctx, prompt)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to ask LLM: %w", err)
	}

	appointment.ExtractedData.WorkPreferences = &data
	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to update appointment with work preferences: %w", err)
	}

	return nil
}

func (p *processor) generateKeywordAnalysis(ctx context.Context, appointment *internal.Appointment) error {
	p.logger.Debug("generating search criteria", "user_id", appointment.UserID, "appointment_id", appointment.ID)

	prompt, err := internal.GeneratePrompt("prompts/transcript-040-search.txt", map[string]any{
		"ExtractedData":       appointment.ExtractedData,
		"ResolvedPreferences": appointment.ExtractedData.ResolvedPreferences,
	})
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	data, err := internal.Ask(ctx, prompt)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to ask LLM: %w", err)
	}

	appointment.ExtractedData.KeywordAnalysis = &data
	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to update appointment with keyword analysis: %w", err)
	}

	return nil
}

func (p *processor) generateUserStatement(ctx context.Context, user *internal.User, appointment *internal.Appointment) error {
	p.logger.Debug("generating user statement", "user_id", user.ID, "appointment_id", appointment.ID)
	prompt, err := internal.GeneratePrompt("prompts/transcript-050-user-statement.txt", map[string]any{
		"ExtractedData":       appointment.ExtractedData,
		"ResolvedPreferences": appointment.ExtractedData.ResolvedPreferences,
		"WorkAnalysis":        appointment.ExtractedData.WorkPreferences,
		"SearchPhrases":       appointment.ExtractedData.KeywordAnalysis,
	})
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	// Ask LLM to generate user statement
	data, err := internal.Ask(ctx, prompt)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to ask LLM: %w", err)
	}

	// Save user statement to appointment
	appointment.ExtractedData.UserStatement = &data
	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to update appointment with user statement: %w", err)
	}

	if user.Search == nil {
		user.Search = &internal.UserSearch{}
	}

	user.Search.Statement = data
	if err := internal.UpdateUserSearch(ctx, p.fs, user.ID, user.Search); err != nil {
		p.logger.Error("failed to update user search with user statement", "error", err)
	}
	return nil
}

// setApptProcessing updates the appointment with completion status and transcript summary
func (p *processor) setApptProcessing(ctx context.Context, appointment *internal.Appointment) error {
	appointment.UpdatedAt = time.Now()
	appointment.CompletedAt = &appointment.UpdatedAt
	appointment.Status = internal.AppointmentStatusProcessing

	// I don't think we need to summarize the transcript anymore.
	// summary, err := transcript.Summarize(ctx)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to summarize transcript: %w", err)
	// }
	// appointment.TranscriptSummary = &summary

	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type":  "database_update",
			"entity_type": "appointment",
		})
		p.logger.Error("failed to update appointment", "error", err)
		return fmt.Errorf("failed to update appointment: %w", err)
	}

	return nil
}

func (p *processor) getTranscript(ctx context.Context, callID string) (*internal.Transcript, error) {

	p.logger.Debug("getting VAPI call result", "call_id", callID)
	call, err := vapi.GetCall(ctx, callID)
	if err != nil {
		return nil, fmt.Errorf("failed to get call: %w", err)
	}

	p.logger.Debug("converting call result to transcript", "call_id", *call.ID)
	// An internal/Transcript is an array of messages from the call
	transcript := convertCallResultToTranscript(*call)

	return transcript, nil
}

// generateSearchCriteria converts the analysis into structured search criteria
func (p *processor) generateSearchCriteria(ctx context.Context, user *internal.User, appt *internal.Appointment) error {
	p.logger.Debug("generating search criteria", "user_id", appt.UserID)

	prompt, err := internal.GeneratePrompt("prompts/transcript-060-final.txt", map[string]any{
		"ExtractedData":       appt.ExtractedData,
		"ResolvedPreferences": appt.ExtractedData.ResolvedPreferences,
		"WorkAnalysis":        appt.ExtractedData.WorkPreferences,
		"SearchPhrases":       appt.ExtractedData.KeywordAnalysis,
		"PreferenceSummary":   appt.ExtractedData.UserStatement,
	})
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("generateSearchCriteria: failed to generate prompt: %w", err)
	}

	response, err := internal.Ask(ctx, prompt)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("generateSearchCriteria: failed to ask LLM: %w", err)
	}

	criteria, err := internal.StructuredAsk[search.Criteria](ctx, "search_criteria", response)
	if err != nil {
		return fmt.Errorf("failed to generate search criteria: %w", err)
	}

	user.Search.Criteria = criteria
	if err := internal.UpdateUserSearch(ctx, p.fs, user.ID, user.Search); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to update user search: %w", err)
	}

	return nil
}

// convertCallResultToTranscript converts a CallResult's messages to a Transcript
func convertCallResultToTranscript(callResult vapi.Call) *internal.Transcript {
	messages := make([]minds.Message, 0, len(callResult.Messages))

	for _, msg := range callResult.Messages {
		// Map VAPI roles to OpenAI roles
		role := ""
		switch msg.Role {
		case "bot":
			role = string(minds.RoleAssistant)
		case "system":
			role = string(minds.RoleSystem)
		case "user":
			role = string(minds.RoleUser)
		case "tool_calls":
			role = string(minds.RoleTool)
		case "tool_call_result":
			role = string(minds.RoleFunction)
		case "developer":
			role = "developer"
		default:
			continue
		}

		// Skip empty messages except for tool_calls and tool_call_result
		if msg.Message == "" &&
			role != string(minds.RoleTool) &&
			role != string(minds.RoleFunction) {
			continue
		}

		messages = append(messages, minds.Message{
			Role:    minds.Role(role),
			Content: msg.Message,
		})
	}

	return &internal.Transcript{
		Messages: messages,
	}
}

// GetPromptResults retrieves all the intermediate prompt results for an appointment
func GetPromptResults(ctx context.Context, fs *firestore.Client, userID string, appointmentID string) (map[string]string, error) {
	appointment, err := internal.GetAppointment(ctx, fs, userID, appointmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get appointment: %w", err)
	}

	results := make(map[string]string)

	// Add each prompt result to the map if it exists
	if appointment.ExtractedData != nil {
		if appointment.ExtractedData.TranscriptExtract != nil {
			results["ExtractedData"] = *appointment.ExtractedData.TranscriptExtract
		}

		if appointment.ExtractedData.ResolvedPreferences != nil {
			results["ResolvedPreferences"] = *appointment.ExtractedData.ResolvedPreferences
		}

		if appointment.ExtractedData.WorkPreferences != nil {
			results["WorkPreferences"] = *appointment.ExtractedData.WorkPreferences
		}

		if appointment.ExtractedData.KeywordAnalysis != nil {
			results["KeywordAnalysis"] = *appointment.ExtractedData.KeywordAnalysis
		}

		if appointment.ExtractedData.UserStatement != nil {
			results["UserStatement"] = *appointment.ExtractedData.UserStatement
		}
	}

	if appointment.TranscriptSummary != nil {
		results["TranscriptSummary"] = *appointment.TranscriptSummary
	}

	return results, nil
}

// func (p *processor) promptLLM(ctx context.Context, promptFile string, data any) (string, error) {
// 	tmpl, err := utils.CreateTemplate(promptFile)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to create template %q: %w", promptFile, err)
// 	}

// 	prompt, err := tmpl.Execute(data)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to execute template %q with data type %T: %w", promptFile, data, err)
// 	}

// 	messages := []minds.Message{
// 		{
// 			Role:    minds.RoleUser,
// 			Content: prompt,
// 		},
// 	}

// 	response, err := p.llm.GenerateContent(ctx, minds.NewRequest(messages))
// 	if err != nil {
// 		return "", fmt.Errorf("failed to generate content for template %q: %w", promptFile, err)
// 	}

// 	return response.String(), nil
// }
