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

	"github.com/chriscow/minds/tools"
	"github.com/getsentry/sentry-go"
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
func ProcessCompletedAppointment(hub *sentry.Hub, fs *internal.FirestoreClient, logger *slog.Logger, appointment *internal.Appointment) {
	ctx := context.Background()
	sentry.SetHubOnContext(ctx, hub)

	user, found, err := internal.GetUser(ctx, fs, appointment.UserID)
	if err != nil {
		logger.Error("failed to get user", "error", err)
		hub.CaptureException(err)
		return
	}
	if !found {
		logger.Error("user not found", "user_id", appointment.UserID)
		hub.CaptureException(err)
		return
	}

	// Send initial processing state update
	logger.Debug("updating user state to processing", "user_id", user.ID)
	if err := internal.UpdateUserStatus(ctx, fs, user, internal.UserStatusCallProcessing); err != nil {
		logger.Error("failed to update user state", "error", err)
		hub.CaptureException(err)
		return
	}

	start := time.Now()

	if appointment.CallID == nil {
		logger.Error("call ID is nil", "user_id", user.ID, "appointment_id", appointment.ID)
		hub.CaptureException(fmt.Errorf("ProcessCompletedAppointment: call ID is nil"))
		return
	}

	//
	// Mark the appointment as processing
	//
	logger.Debug("processing appointment", "user_id", user.ID, "appointment_id", appointment.ID)
	if err := internal.MarkAppointmentAsProcessing(ctx, fs, appointment); err != nil {
		hub.CaptureException(err)
		logger.Error("failed to mark appointment as processing", "error", err)
		return
	}

	//
	// Get the supported locations (Geo ID keys)
	//
	logger.Debug("getting supported locations")
	locations, err := internal.GetGeoIDKeys(ctx, fs)
	if err != nil {
		hub.CaptureException(err)
		logger.Error("failed to get supported locations", "error", err)
		return
	}

	//
	// Get the transcript from the VAPI call
	//
	logger.Debug("getting transcript from VAPI call", "call_id", *appointment.CallID)
	transcript, err := internal.GetAppointmentTranscript(ctx, fs, *appointment)
	if err != nil {
	}

	//
	// Get work preferences
	//
	logger.Debug("getting work preferences", "user_id", user.ID, "appointment_id", appointment.ID)
	if err := getWorkPreferences(ctx, fs, transcript, locations, user); err != nil {
		hub.CaptureException(err)
		logger.Error("failed to get work preferences", "error", err)
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	//
	// Generate steering query
	//
	logger.Debug("generating steering query", "user_id", user.ID, "appointment_id", appointment.ID)
	if err := generateSteeringQuery(ctx, fs, user, transcript); err != nil {
		hub.CaptureException(err)
		logger.Error("failed to generate steering query", "error", err)
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	//
	// Subscribe the user to the generated SearchCriteria
	//
	logger.Debug("subscribing user to search criteria", "user_id", user.ID, "appointment_id", appointment.ID)
	if user.SteeringQuery == nil || user.WorkPreferences == nil {
		logger.Error("user steering query or work preferences not found", "user_id", user.ID)
		hub.CaptureException(fmt.Errorf("user steering query or work preferences not found"))
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, fmt.Errorf("user steering query or work preferences not found"))
		return
	}

	//
	// Get or create search criteria
	//
	logger.Debug("getting or creating search criteria", "user_id", user.ID, "appointment_id", appointment.ID)
	criteria, err := getOrCreateSearchCriteria(ctx, fs, user)
	if err != nil {
		hub.CaptureException(err)
		logger.Error("failed to get or create search criteria", "error", err)
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	//
	// Subscribe the user to the search criteria
	//
	logger.Debug("subscribing user to search criteria", "user_id", user.ID, "criteria-id", criteria.ID)
	if err := internal.SubscribeToCriteria(ctx, fs, user.ID, criteria.ID); err != nil {
		hub.CaptureException(err)
		logger.Error("failed to subscribe user to search criteria", "error", err)
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	logger.Debug(internal.Blue("⏱️ appointment processing took"), "duration", time.Since(start).String())

	//
	// Set the apppointment status to completed
	//
	logger.Debug("marking appointment as completed", "user_id", user.ID, "appointment_id", appointment.ID)
	if err := internal.ProcessAppointmentComplete(ctx, fs, appointment); err != nil {
		logger.Error("failed to mark appointment as completed", "error", err)
	}

	logger.Debug(internal.Green("✅ appointment processing completed"), "user_id", user.ID, "appointment_id", appointment.ID)

	//
	// Send completion state update
	//
	if err := internal.UpdateUserStatus(ctx, fs, user, internal.UserStatusCallCompleted); err != nil {
		logger.Error("failed to update user state", "error", err)
	}

	//
	// Refresh user jobs - queries Pinecone for relevant jobs
	//
	logger.Debug("refreshing user jobs", "user_id", user.ID, "criteria-id", criteria.ID)
	if err := internal.RefreshUserJobs(ctx, fs, user, *criteria); err != nil {
		logger.Error("failed to refresh user jobs", "error", err)
		hub.CaptureException(err)
		return
	}
}

// RecoverProcessingAppointments finds and reprocesses any appointments that were interrupted during processing
func RecoverProcessingAppointments(hub *sentry.Hub, fs *internal.FirestoreClient, logger *slog.Logger) error {
	ctx := context.Background()

	logger = logger.With("pkg", "transcript.RecoverProcessingAppointments")

	logger.Info("recovering processing appointments")

	appointments, err := internal.ListAppointmentsNeedingProcessing(ctx, fs, logger)
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

		go ProcessCompletedAppointment(hub, fs, logger, appointment)
	}

	return nil
}

// processor handles the processing of appointment transcripts
// type processor struct {
// 	fs     *internal.FirestoreClient
// 	logger *slog.Logger
// 	llm    *openai.Provider
// }

// // newProcessor creates a new TranscriptProcessor
// func newProcessor(fs *internal.FirestoreClient, logger *slog.Logger) (*processor, error) {
// 	llm, err := openai.NewProvider(openai.WithModel(transcriptModel))
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create openai provider: %w", err)
// 	}

// 	logger = logger.With("pkg", "transcript.Processor")

// 	return &processor{
// 		fs:     fs,
// 		logger: logger,
// 		llm:    llm,
// 	}, nil
// }

// // process updates the appointment after a successful call with the transcript summary
// func (p *processor) process(ctx context.Context, user *internal.User, appointment *internal.Appointment) error {
// 	p.logger.Info(internal.BGGreen("⏱️ processing completed appointment"), "user_id", user.ID, "appointment_id", appointment.ID)
// 	defer p.logger.Info(internal.BGGreen("✅ processed completed appointment"), "user_id", user.ID, "appointment_id", appointment.ID)

// 	internal.SentryBreadcrumb(ctx, sentry.Breadcrumb{
// 		Type:     "appointment",
// 		Category: "processing",
// 		Message:  fmt.Sprintf("Processing completed appointment for user %s", user.ID),
// 		Data: map[string]any{
// 			"appointment_id": appointment.ID,
// 			"user_id":        user.ID,
// 		},
// 	})

// 	//
// 	// Generate search queries
// 	//
// 	// if err := p.getSearchQueries(ctx, resume, appointment); err != nil {
// 	// 	internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 	// 	return fmt.Errorf("failed to generate search queries: %w", err)
// 	// }

// 	//
// 	// Generate user statement
// 	//
// 	// if err := p.generateUserStatement(ctx, user, appointment); err != nil {
// 	// 	internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 	// 	return fmt.Errorf("failed to generate user statement: %w", err)
// 	// }

// 	//
// 	// Generate job description
// 	//
// 	// if err := p.getJobDescription(ctx, resume, appointment); err != nil {
// 	// 	internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 	// 	return fmt.Errorf("failed to generate job description: %w", err)
// 	// }

// 	return nil
// }

func getOrCreateSearchCriteria(ctx context.Context, fs *internal.FirestoreClient, user *internal.User) (*internal.SearchCriteria, error) {
	if user.SteeringQuery == nil || user.WorkPreferences == nil {
		return nil, fmt.Errorf("user steering query or work preferences not found")
	}

	steeringQuery := user.SteeringQuery.Query
	workPreferences := *user.WorkPreferences

	geoID, err := internal.ExtractGeoID(ctx, fs, workPreferences)
	if err != nil {
		return nil, fmt.Errorf("failed to extract geo id: %w", err)
	}

	id := internal.MakeSearchCriteriaID(steeringQuery, geoID)
	criteria, found, err := internal.GetSearchCriteria(ctx, fs, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get search criteria: %w", err)
	}

	if found {
		return criteria, nil
	}

	criteria, err = internal.CreateSearchCriteria(ctx, fs, steeringQuery, geoID, internal.DefaultMaxPages)
	if err != nil {
		return nil, fmt.Errorf("failed to create search criteria: %w", err)
	}

	return criteria, nil
}

// generateSteeringQuery generates a steering query based on the transcript
func generateSteeringQuery(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, transcript *internal.Transcript) error {
	query, err := internal.GenerateSteeringQuery(ctx, transcript)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to ask LLM for steering query: %w", err)
	}

	user.SteeringQuery = query

	if err := internal.SetUserSteeringQuery(ctx, fs, user.ID, *query); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to update user with steering query: %w", err)
	}
	return nil
}

// func (p *processor) extract(ctx context.Context, transcript string, appointment *internal.Appointment) error {
// 	p.logger.Debug("extracting data from transcript", "user_id", appointment.UserID, "appointment_id", appointment.ID)

// 	start := time.Now()
// 	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-010-extract.txt")
// 	if err != nil {
// 		return fmt.Errorf("failed to read extract prompt: %w", err)
// 	}

// 	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
// 		"Transcript": transcript,
// 	})
// 	if err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
// 			"error_type":  "prompt_execution",
// 			"entity_type": "appointment",
// 		})
// 		return fmt.Errorf("failed to generate prompt: %w", err)
// 	}

// 	data, err := tools.Ask(ctx, prompt)
// 	if err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to ask LLM: %w", err)
// 	}

// 	// Save extracted data to appointment
// 	appointment.ExtractedData.TranscriptExtract = &data
// 	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to update appointment with extracted data: %w", err)
// 	}

// 	p.logger.Debug("extracted data from transcript", "user_id", appointment.UserID, "appointment_id", appointment.ID, "elapsed", time.Since(start).String())
// 	return nil
// }

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

func extractWorkPreferences(_ context.Context, data string) (*string, error) {
	if strings.Contains(data, "<final_determinations>") {
		start := strings.Index(data, "<final_determinations>")
		if start == -1 {
			return nil, fmt.Errorf("failed to generate work preferences: <final_determinations> tag not found")
		}
		end := strings.Index(data, "</final_determinations>")
		if end == -1 {
			return nil, fmt.Errorf("failed to generate work preferences: </final_determinations> tag not found")
		}
		data = data[start+len("<final_determinations>") : end]
	} else {
		return nil, fmt.Errorf("failed to generate work preferences: <final_determinations> tag not found")
	}

	return &data, nil
}
func getWorkPreferences(ctx context.Context, fs *internal.FirestoreClient, transcript *internal.Transcript, locations []string, user *internal.User) error {
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-030-work-location.txt")
	if err != nil {
		return fmt.Errorf("failed to read work location prompt: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"Transcript": transcript.String(),
		"Locations":  locations,
	})
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	data, err := tools.Ask(ctx, prompt, tools.WithModel(tools.DeepSeekChat))
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to ask LLM: %w", err)
	}

	prefs, err := extractWorkPreferences(ctx, data)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to extract work preferences: %w", err)
	}

	user.WorkPreferences = prefs
	if err := internal.SetUserWorkPreferences(ctx, fs, user.ID, *prefs); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to update appointment with work preferences: %w", err)
	}

	return nil
}

// func (p *processor) getSearchQueries(ctx context.Context, resume *internal.Resume, appointment *internal.Appointment) error {
// 	p.logger.Debug("generating search criteria", "user_id", appointment.UserID, "appointment_id", appointment.ID)

// 	queries, err := internal.GenerateSearchQueries(ctx, *resume, *appointment)
// 	if err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to generate search queries: %w", err)
// 	}

// 	appointment.ExtractedData.SearchQueries = queries
// 	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to update appointment with search queries: %w", err)
// 	}

// 	return nil
// }

// func (p *processor) generateUserStatement(ctx context.Context, user *internal.User, appointment *internal.Appointment) error {
// 	p.logger.Debug("generating user statement", "user_id", user.ID, "appointment_id", appointment.ID)
// 	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-050-user-statement.txt")
// 	if err != nil {
// 		return fmt.Errorf("failed to read user statement prompt: %w", err)
// 	}

// 	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
// 		"TranscriptExtract": appointment.ExtractedData,
// 		"WorkPreferences":   appointment.ExtractedData.WorkPreferences,
// 		"SearchQueries":     appointment.ExtractedData.SearchQueries,
// 	})
// 	if err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to generate prompt: %w", err)
// 	}

// 	// Ask LLM to generate user statement
// 	data, err := tools.Ask(ctx, prompt, tools.WithModel(tools.GPT41Mini))
// 	if err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to ask LLM: %w", err)
// 	}

// 	// Save user statement to appointment
// 	appointment.ExtractedData.UserStatement = &data
// 	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to update appointment with user statement: %w", err)
// 	}

// 	// TODO: Update user search with user statement
// 	// if user.Search == nil {
// 	// 	user.Search = &internal.UserSearch{}
// 	// }

// 	// user.Search.Statement = data
// 	// if err := internal.UpdateUserSearch(ctx, p.fs, user.ID, user.Search); err != nil {
// 	// 	p.logger.Error("failed to update user search with user statement", "error", err)
// 	// }
// 	return nil
// }

// func (p *processor) getJobDescription(ctx context.Context, resume *internal.Resume, appointment *internal.Appointment) error {
// 	p.logger.Debug("getting job description", "user_id", appointment.UserID, "appointment_id", appointment.ID)

// 	description, err := internal.GenerateJobDescription(ctx, *resume, *appointment)
// 	if err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to generate job description: %w", err)
// 	}

// 	appointment.ExtractedData.JobDescription = &description
// 	if err := internal.UpdateAppointment(ctx, p.fs, appointment); err != nil {
// 		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
// 		return fmt.Errorf("failed to update appointment with job description: %w", err)
// 	}

// 	return nil
// }

// getPromptResults retrieves the prompt results for a specific appointment
// func getPromptResults(ctx context.Context, client *internal.FirestoreClient, userID, appointmentID string) (map[string]string, error) {
// 	appointment, err := internal.GetAppointment(ctx, client, userID, appointmentID)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get appointment: %w", err)
// 	}

// 	results := make(map[string]string)

// 	if appointment.ExtractedData != nil {
// 		if appointment.ExtractedData.TranscriptExtract != nil {
// 			results["ExtractedData"] = *appointment.ExtractedData.TranscriptExtract
// 		}
// 		if appointment.ExtractedData.SearchQueries != nil {
// 			results["SearchQueries"] = strings.Join(appointment.ExtractedData.SearchQueries, "\n")
// 		}
// 		if appointment.ExtractedData.UserStatement != nil {
// 			results["UserStatement"] = *appointment.ExtractedData.UserStatement
// 		}
// 	}

// 	if appointment.Summary != nil {
// 		results["TranscriptSummary"] = *appointment.Summary
// 	}

// 	return results, nil
// }
