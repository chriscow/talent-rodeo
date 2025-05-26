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
	"github.com/pusher/pusher-http-go/v5"
	"github.com/samber/lo"
)

type userNotifier struct {
	userID    string
	publisher *pusher.Client
	dispatch  *internal.JobEventDispatch
}

func (u *userNotifier) OnSearchStarted(task internal.SearchTask) {
	fmt.Println("userNotifier:OnSearchStarted", task)
}

func (u *userNotifier) OnJobListing(task internal.SearchTask, listing internal.JobListing) {
	// fmt.Println("userNotifier:OnJobListing", task, listing)
}

func (u *userNotifier) OnJobDetail(task internal.SearchTask, details internal.JobDetail) {
	fmt.Println("userNotifier:OnJobDetail", task.Criteria.ID, details.Title)
	internal.SendPusherMessage(u.publisher, u.userID, "Searching: "+details.Title, nil)
}

func (u *userNotifier) OnSearchCompleted(task internal.SearchTask, totalJobs int, errors []error) {
	fmt.Println("userNotifier:OnSearchCompleted", task, totalJobs, errors)
	u.dispatch.Deregister(u.userID)
	internal.SendPusherRedirect(u.publisher, u.userID, "/dashboard")
}

// ProcessCompletedCall updates the appointment after a successful call with the transcript summary.
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
func ProcessCompletedCall(hub *sentry.Hub, fs *internal.FirestoreClient, psh *pusher.Client, queue internal.SearchQueue, logger *slog.Logger, dispatch *internal.JobEventDispatch, user *internal.User, appointment *internal.Appointment) {
	ctx := context.Background()
	sentry.SetHubOnContext(ctx, hub)

	if appointment.CallID == nil {
		logger.Error("call ID is nil", "user-id", user.ID, "appointment-id", appointment.ID)
		hub.CaptureException(fmt.Errorf("ProcessCompletedAppointment: call ID is nil"))
		return
	}

	//
	// Mark the appointment as processing and update user status
	//
	if err := internal.ProcessAppointmentStarted(ctx, fs, user, appointment); err != nil {
		hub.CaptureException(err)
		logger.Error("failed to mark appointment as processing", "error", err)
		return
	}

	internal.SendPusherRedirect(psh, user.ID, "/dashboard")
	time.Sleep(3 * time.Second)
	internal.SendPusherMessage(psh, user.ID, "Analyzing your call", nil)

	start := time.Now()

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
	logger.Debug("getting transcript from VAPI call", "call-id", *appointment.CallID)
	transcript, err := internal.GetAppointmentTranscript(ctx, fs, *appointment)
	if err != nil {
		hub.CaptureException(err)
		logger.Error("failed to get transcript from VAPI call", "error", err)
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	//
	// Get work preferences
	//
	internal.SendPusherMessage(psh, user.ID, "Determining your work preferences", nil)
	logger.Debug("getting work preferences", "user-id", user.ID, "appointment-id", appointment.ID)
	if err := getWorkPreferences(ctx, fs, transcript, locations, user); err != nil {
		hub.CaptureException(err)
		logger.Error("failed to get work preferences", "error", err)
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	//
	// Generate steering query
	//
	internal.SendPusherMessage(psh, user.ID, "Generating your search criteria", nil)
	logger.Debug("generating steering query", "user-id", user.ID, "appointment-id", appointment.ID)
	if err := generateSteeringQuery(ctx, fs, user, transcript); err != nil {
		hub.CaptureException(err)
		logger.Error("failed to generate steering query", "error", err)
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	//
	// Subscribe the user to the generated SearchCriteria
	//
	logger.Debug("subscribing user to search criteria", "user-id", user.ID, "appointment-id", appointment.ID)
	if user.SteeringQuery == nil || user.WorkPreferences == nil {
		logger.Error("user steering query or work preferences not found", "user-id", user.ID)
		hub.CaptureException(fmt.Errorf("user steering query or work preferences not found"))
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, fmt.Errorf("user steering query or work preferences not found"))
		return
	}

	//
	// Get or create search criteria
	//
	logger.Debug("getting or creating search criteria", "user-id", user.ID, "appointment-id", appointment.ID)
	criteria, isNewCriteria, err := getOrCreateSearchCriteria(ctx, fs, user)
	if err != nil {
		hub.CaptureException(err)
		logger.Error("failed to get or create search criteria", "error", err)
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	//
	// Subscribe the user to the search criteria
	//
	logger.Debug("subscribing user to search criteria", "user-id", user.ID, "criteria-id", criteria.ID)
	if err := internal.SubscribeToCriteria(ctx, fs, user.ID, criteria.ID); err != nil {
		hub.CaptureException(err)
		logger.Error("failed to subscribe user to search criteria", "error", err)
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	if isNewCriteria {
		if queue == nil {
			logger.Error("queue is nil")
			hub.CaptureException(fmt.Errorf("queue is nil"))
			internal.ProcessAppointmentFailed(ctx, fs, user, appointment, fmt.Errorf("queue is nil"))
			return
		}

		if dispatch == nil {
			logger.Error("dispatch is nil")
			hub.CaptureException(fmt.Errorf("dispatch is nil"))
			internal.ProcessAppointmentFailed(ctx, fs, user, appointment, fmt.Errorf("dispatch is nil"))
			return
		}

		//
		// If a new criteria was created, we need to queue an immediate search
		//
		logger.Debug("search criteria created", "user-id", user.ID, "criteria-id", criteria.ID)
		jobSearch, err := internal.CreateSearchTask(ctx, fs, *criteria, "scrapingdog")
		if err != nil {
			logger.Error("failed to create search task", "error", err)
			hub.CaptureException(err)
			internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
			return
		}

		notifier := &userNotifier{userID: user.ID, dispatch: dispatch, publisher: psh}
		dispatch.Register(user.ID, notifier)

		if err := queue.QueueSearch(ctx, *jobSearch); err != nil {
			logger.Error("failed to queue search", "error", err)
			hub.CaptureException(err)
			internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
			return
		}
	}

	logger.Debug(internal.Blue("⏱️ appointment processing took"), "duration", time.Since(start).String())

	//
	// Set the apppointment status to completed
	//
	internal.SendPusherMessage(psh, user.ID, "Starting your first search (this takes a few minutes)", nil)
	logger.Debug("marking appointment as completed", "user-id", user.ID, "appointment-id", appointment.ID)
	if err := internal.ProcessAppointmentComplete(ctx, fs, user, appointment); err != nil {
		logger.Error("failed to mark appointment as completed", "error", err)
		hub.CaptureException(err)
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	logger.Debug(internal.Green("✅ appointment processing completed"), "user-id", user.ID, "appointment-id", appointment.ID)

	//
	// Refresh user jobs - queries Pinecone for relevant jobs using an existing criteria
	//
	if !isNewCriteria {
		logger.Debug("refreshing user jobs", "user-id", user.ID, "criteria-id", criteria.ID)
		internal.SendPusherMessage(psh, user.ID, "Refreshing your results", nil)
		if err := internal.RefreshUserJobs(ctx, fs, user, *criteria); err != nil {
			logger.Error("failed to refresh user jobs", "error", err)
			hub.CaptureException(err)
			return
		}

		internal.SendPusherRedirect(psh, user.ID, "/dashboard")
	}
}

// RecoverProcessingAppointments finds and reprocesses any appointments that were interrupted during processing
func RecoverProcessingAppointments(hub *sentry.Hub, fs *internal.FirestoreClient, psh *pusher.Client, logger *slog.Logger, dispatch *internal.JobEventDispatch, queue internal.SearchQueue) error {
	ctx := context.Background()

	logger = logger.With("pkg", "transcript.RecoverProcessingAppointments")

	logger.Info("recovering unprocessed appointments")

	appointments, err := internal.ListAppointmentsNeedingProcessing(ctx, fs, logger)
	if err != nil {
		return fmt.Errorf("failed to list appointments needing processing: %w", err)
	}

	if len(appointments) == 0 {
		logger.Info("no appointments needing processing found")
		return nil
	}

	logger.Info("found appointments needing processing", "count", len(appointments))

	// Collect unique user IDs from appointments
	userMap := make(map[string]*internal.User)
	for _, appointment := range appointments {
		userMap[appointment.UserID] = nil
	}

	userIDs := lo.Keys(userMap)

	// Get all users in one call
	users, err := internal.GetUsersByIDs(ctx, fs, userIDs)
	if err != nil {
		return fmt.Errorf("failed to get users by ids: %w", err)
	}

	// Populate userMap with actual user data
	for _, user := range users {
		if _, exists := userMap[user.ID]; exists {
			userMap[user.ID] = user
		} else {
			// If the user does not exist, skip the appointment
			logger.Error("user not found for appointment",
				"user-id", user.ID,
			)
			hub.CaptureException(fmt.Errorf("user not found for appointment: %s", user.ID))
			// remove the appointment user from the list
			delete(userMap, user.ID)
			continue
		}
	}

	// Process each appointment with its corresponding user
	for _, appointment := range appointments {
		user, ok := userMap[appointment.UserID]
		if !ok {
			logger.Error("user not found for appointment",
				"user-id", appointment.UserID,
				"appointment-id", appointment.ID)
			hub.CaptureException(fmt.Errorf("user not found for appointment: %s", appointment.UserID))
			continue
		}

		logger.Info("reprocessing appointment",
			"appointment-id", appointment.ID,
			"user-id", appointment.UserID)

		go ProcessCompletedCall(hub, fs, psh, queue, logger, dispatch, user, appointment)
	}

	return nil
}

func getOrCreateSearchCriteria(ctx context.Context, fs *internal.FirestoreClient, user *internal.User) (*internal.SearchCriteria, bool, error) {
	if user.SteeringQuery == nil || user.WorkPreferences == nil {
		return nil, false, fmt.Errorf("user steering query or work preferences not found")
	}

	steeringQuery := user.SteeringQuery.Query
	workPreferences := *user.WorkPreferences

	geoID, err := internal.ExtractGeoID(ctx, fs, workPreferences)
	if err != nil {
		return nil, false, fmt.Errorf("failed to extract geo id: %w", err)
	}

	id := internal.MakeSearchCriteriaID(steeringQuery, geoID)
	criteria, found, err := internal.GetSearchCriteria(ctx, fs, id)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get search criteria: %w", err)
	}

	if found {
		return criteria, false, nil
	}

	criteria, err = internal.CreateSearchCriteria(ctx, fs, steeringQuery, geoID, internal.DefaultMaxPages)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create search criteria: %w", err)
	}

	return criteria, true, nil
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
// 	p.logger.Debug("generating search criteria", "user-id", appointment.UserID, "appointment-id", appointment.ID)

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
// 	p.logger.Debug("generating user statement", "user-id", user.ID, "appointment-id", appointment.ID)
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
// 	p.logger.Debug("getting job description", "user-id", appointment.UserID, "appointment-id", appointment.ID)

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
