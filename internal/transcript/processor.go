package transcript

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	// Removing self-import of internal package
	"talent-rodeo/assets"
	"talent-rodeo/internal"
	"time"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/openai"
	"github.com/chriscow/minds/tools"
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
func ProcessCompletedAppointment(hub *sentry.Hub, fs *internal.FirestoreClient, logger *slog.Logger, appointment *internal.Appointment) error {
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

		resume, found, err := internal.GetLatestResume(ctx, fs, user.ID)
		if err != nil {
			logger.Error("failed to get resume", "error", err)
			return
		}

		if !found {
			logger.Error("resume not found", "user_id", user.ID)
			return
		}

		// Initialize user.Search if it is nil
		// if user.Search == nil {
		// 	user.Search = &internal.UserSearch{}
		// }

		// Send initial processing state update
		if err := internal.UpdateUserStatus(ctx, fs, user, internal.UserStatusCallProcessing); err != nil {
			logger.Error("failed to update user state", "error", err)
		}

		start := time.Now()

		processor, err := newProcessor(fs, logger)
		if err != nil {
			sentry.CaptureException(err)
			logger.Error("failed to create transcript processor", "error", err)
			return
		}

		err = processor.process(ctx, user, resume, appointment)
		if err != nil {
			hub.CaptureException(err)
			logger.Error("failed to process completed appointment", "error", err)
			// user.Search.Error = err.Error()
			// if err := internal.UpdateUserSearch(ctx, fs, user.ID, user.Search); err != nil {
			// 	hub.CaptureException(err)
			// 	logger.Error("failed to update user search", "error", err)
			// }
			// On error, mark the appointment as failed
			appointment.Status = internal.AppointmentStatusFailed
			appointment.FailedAt = &time.Time{}
			*appointment.FailedAt = time.Now()
			if err := internal.UpdateAppointment(ctx, fs, appointment); err != nil {
				logger.Error("failed to update appointment status to failed", "error", err)
			}

			// Update user state to error
			if err := internal.UpdateUserStateWithError(ctx, fs, user, err.Error()); err != nil {
				logger.Error("failed to update user state", "error", err)
			}
			return
		}

		logger.Debug(internal.Blue("⏱️ appointment processing took"), "duration", time.Since(start).String())

		// Only mark as completed if the entire process succeeds
		// appointment.Status = internal.AppointmentStatusCompleted
		// appointment.CompletedAt = &time.Time{}
		// *appointment.CompletedAt = time.Now()
		// if err := internal.UpdateAppointment(ctx, fs, appointment); err != nil {
		// 	logger.Error("failed to update appointment status to completed", "error", err)
		// }
		if err := internal.CompleteProcessing(ctx, fs, appointment); err != nil {
			logger.Error("failed to mark appointment as completed", "error", err)
		}
		logger.Debug(internal.Green("✅ appointment processing completed"), "user_id", user.ID, "appointment_id", appointment.ID)

		// Send completion state update
		if err := internal.UpdateUserStatus(ctx, fs, user, internal.UserStatusCallCompleted); err != nil {
			logger.Error("failed to update user state", "error", err)
		}
	}()

	return nil
}

// RecoverProcessingAppointments finds and reprocesses any appointments that were interrupted during processing
func RecoverProcessingAppointments(hub *sentry.Hub, fs *internal.FirestoreClient, logger *slog.Logger) error {
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
	fs     *internal.FirestoreClient
	logger *slog.Logger
	llm    *openai.Provider
}

// newProcessor creates a new TranscriptProcessor
func newProcessor(fs *internal.FirestoreClient, logger *slog.Logger) (*processor, error) {
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
func (p *processor) process(ctx context.Context, user *internal.User, resume *internal.Resume, appointment *internal.Appointment) error {
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
	if err := internal.MarkAppointmentAsProcessing(ctx, p.fs, appointment); err != nil {
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
	// Get work preferences
	//
	if err := p.getWorkPreferences(ctx, locations, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to get work preferences: %w", err)
	}

	//
	// Generate search queries
	//
	if err := p.getSearchQueries(ctx, resume, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate search queries: %w", err)
	}

	//
	// Generate user statement
	//
	if err := p.generateUserStatement(ctx, user, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate user statement: %w", err)
	}

	//
	// Generate job description
	//
	if err := p.getJobDescription(ctx, resume, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate job description: %w", err)
	}

	//
	// Finally, subscribe to the search criteria
	//
	return p.subscribeSearchCriteria(ctx, user, appointment)
}

func (p *processor) extract(ctx context.Context, transcript string, appointment *internal.Appointment) error {
	p.logger.Debug("extracting data from transcript", "user_id", appointment.UserID, "appointment_id", appointment.ID)

	start := time.Now()
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-010-extract.txt")
	if err != nil {
		return fmt.Errorf("failed to read extract prompt: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"Transcript": transcript,
	})
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type":  "prompt_execution",
			"entity_type": "appointment",
		})
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	data, err := tools.Ask(ctx, prompt)
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

// func (p *processor) resolveDiscrepancies(ctx context.Context, appointment *internal.Appointment) error {
// 	p.logger.Debug("resolving preferences", "user_id", appointment.UserID, "appointment_id", appointment.ID)
// 	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-020-resolved-prefs.txt")
// 	if err != nil {
// 		return fmt.Errorf("failed to read resolved preferences prompt: %w", err)
// 	}

// 	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
// 		"ExtractedData": appointment.ExtractedData,
// 	})
// 	if err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to generate prompt: %w", err)
// 	}

// 	// Ask LLM to resolve preferences
// 	data, err := tools.Ask(ctx, prompt)
// 	if err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to ask LLM: %w", err)
// 	}

// 	// Save resolved preferences to appointment
// 	appointment.ExtractedData.ResolvedPreferences = &data
// 	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to update appointment with resolved preferences: %w", err)
// 	}

// 	return nil
// }

func (p *processor) getWorkPreferences(ctx context.Context, locations []string, appointment *internal.Appointment) error {
	p.logger.Debug("resolving work preferences", "user_id", appointment.UserID, "appointment_id", appointment.ID)
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-030-work-location.txt")
	if err != nil {
		return fmt.Errorf("failed to read work location prompt: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"TranscriptExtract": appointment.ExtractedData.TranscriptExtract,
		"Locations":         locations,
	})
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	data, err := tools.Ask(ctx, prompt, tools.WithModel(tools.GPT41Mini))
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

func (p *processor) getSearchQueries(ctx context.Context, resume *internal.Resume, appointment *internal.Appointment) error {
	p.logger.Debug("generating search criteria", "user_id", appointment.UserID, "appointment_id", appointment.ID)

	queries, err := internal.GenerateSearchQueries(ctx, *resume, *appointment)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate search queries: %w", err)
	}

	appointment.ExtractedData.SearchQueries = queries
	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to update appointment with search queries: %w", err)
	}

	return nil
}

func (p *processor) generateUserStatement(ctx context.Context, user *internal.User, appointment *internal.Appointment) error {
	p.logger.Debug("generating user statement", "user_id", user.ID, "appointment_id", appointment.ID)
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-050-user-statement.txt")
	if err != nil {
		return fmt.Errorf("failed to read user statement prompt: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"TranscriptExtract": appointment.ExtractedData,
		"WorkPreferences":   appointment.ExtractedData.WorkPreferences,
		"SearchQueries":     appointment.ExtractedData.SearchQueries,
	})
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	// Ask LLM to generate user statement
	data, err := tools.Ask(ctx, prompt, tools.WithModel(tools.GPT41Mini))
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

	// TODO: Update user search with user statement
	// if user.Search == nil {
	// 	user.Search = &internal.UserSearch{}
	// }

	// user.Search.Statement = data
	// if err := internal.UpdateUserSearch(ctx, p.fs, user.ID, user.Search); err != nil {
	// 	p.logger.Error("failed to update user search with user statement", "error", err)
	// }
	return nil
}

func (p *processor) getJobDescription(ctx context.Context, resume *internal.Resume, appointment *internal.Appointment) error {
	p.logger.Debug("getting job description", "user_id", appointment.UserID, "appointment_id", appointment.ID)

	description, err := internal.GenerateJobDescription(ctx, *resume, *appointment)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate job description: %w", err)
	}

	appointment.ExtractedData.JobDescription = &description
	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to update appointment with job description: %w", err)
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

// subscribeSearchCriteria converts the analysis into structured search criteria
func (p *processor) subscribeSearchCriteria(ctx context.Context, user *internal.User, appt *internal.Appointment) error {
	p.logger.Debug("generating search criteria", "user_id", appt.UserID)

	/*
		tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-060-final.txt")
		if err != nil {
			return fmt.Errorf("failed to read final prompt: %w", err)
		}

		prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
			"ExtractedData":       appt.ExtractedData,
			"ResolvedPreferences": appt.ExtractedData.ResolvedPreferences,
			"WorkPreferences":        appt.ExtractedData.WorkPreferences,
			"SearchPhrases":       appt.ExtractedData.KeywordAnalysis,
			"PreferenceSummary":   appt.ExtractedData.UserStatement,
		})
		if err != nil {
			internal.SentryReportError(ctx, err, sentry.LevelError, nil)
			return fmt.Errorf("subscribeSearchCriteria: failed to generate prompt: %w", err)
		}

		response, err := tools.Ask(ctx, prompt)
		if err != nil {
			internal.SentryReportError(ctx, err, sentry.LevelError, nil)
			return fmt.Errorf("subscribeSearchCriteria: failed to ask LLM: %w", err)
		}

		criteria, err := tools.StructuredAsk[internal.SearchCriteria](ctx, "search_criteria", response)
		if err != nil {
			return fmt.Errorf("failed to generate search criteria: %w", err)
		}

		user.Search.Criteria = criteria
		if err := internal.UpdateUserSearch(ctx, p.fs, user.ID, user.Search); err != nil {
			internal.SentryReportError(ctx, err, sentry.LevelError, nil)
			return fmt.Errorf("failed to update user search: %w", err)
		}
	*/

	return fmt.Errorf("subscribeSearchCriteria: not implemented")
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

// getPromptResults retrieves the prompt results for a specific appointment
func getPromptResults(ctx context.Context, client *internal.FirestoreClient, userID, appointmentID string) (map[string]string, error) {
	appointment, err := internal.GetAppointment(ctx, client, userID, appointmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get appointment: %w", err)
	}

	results := make(map[string]string)

	if appointment.ExtractedData != nil {
		if appointment.ExtractedData.TranscriptExtract != nil {
			results["ExtractedData"] = *appointment.ExtractedData.TranscriptExtract
		}
		if appointment.ExtractedData.SearchQueries != nil {
			results["SearchQueries"] = strings.Join(appointment.ExtractedData.SearchQueries, "\n")
		}
		if appointment.ExtractedData.UserStatement != nil {
			results["UserStatement"] = *appointment.ExtractedData.UserStatement
		}
	}

	if appointment.Summary != nil {
		results["TranscriptSummary"] = *appointment.Summary
	}

	return results, nil
}
