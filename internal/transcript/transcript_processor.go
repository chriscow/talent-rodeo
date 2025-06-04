package transcript

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"talent-rodeo/internal/smtp"

	// Removing self-import of internal package
	"talent-rodeo/assets"
	"talent-rodeo/internal"
	"time"

	"github.com/chriscow/minds/tools"
	"github.com/getsentry/sentry-go"
	"github.com/pusher/pusher-http-go/v5"
	"github.com/samber/lo"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

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

func (u *userNotifier) OnPageComplete(task internal.SearchTask, page int, jobsFound int, errors []error) {
	fmt.Println("userNotifier:OnPageComplete", task, page, jobsFound, errors)
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
func ProcessCompletedCall(hub *sentry.Hub, fs *internal.FirestoreClient, psh *pusher.Client, queue internal.SearchQueue, logger *slog.Logger, dispatch *internal.JobEventDispatch, user *internal.User, appointment *internal.Appointment, smtpCfg *SMTPConfig) {
	ctx := context.Background()
	sentry.SetHubOnContext(ctx, hub)

	// Log initial state for debugging
	logger.Info("starting call processing",
		"user-id", user.ID,
		"appointment-id", appointment.ID,
		"user-email", user.Email,
		"appointment-status", appointment.Status,
		"has-call-id", appointment.CallID != nil,
		"has-smtp-config", smtpCfg != nil,
		"has-queue", queue != nil,
		"has-dispatch", dispatch != nil)

	if appointment.CallID == nil {
		logger.Error("call ID is nil", "user-id", user.ID, "appointment-id", appointment.ID)
		internal.SentryReportError(ctx, fmt.Errorf("call ID is nil for appointment %s", appointment.ID), sentry.LevelError, map[string]string{
			"user_id":        user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"step":           "validation",
		})
		return
	}

	//
	// Mark the appointment as processing and update user status
	//
	logger.Debug("marking appointment as processing started", "user-id", user.ID, "appointment-id", appointment.ID)
	if err := internal.ProcessAppointmentStarted(ctx, fs, user, appointment); err != nil {
		logger.Error("failed to mark appointment as processing", "error", err, "user-id", user.ID, "appointment-id", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"user_id":        user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"step":           "ProcessAppointmentStarted",
		})
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
		logger.Error("failed to get supported locations", "error", err, "user-id", user.ID, "appointment-id", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"user_id":        user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"step":           "GetGeoIDKeys",
		})
		return
	}
	logger.Debug("retrieved supported locations", "count", len(locations), "user-id", user.ID, "appointment-id", appointment.ID)

	//
	// Get the transcript from the VAPI call
	//
	logger.Debug("getting transcript from VAPI call", "call-id", *appointment.CallID, "user-id", user.ID, "appointment-id", appointment.ID)
	transcript, err := internal.GetAppointmentTranscript(ctx, fs, *appointment)
	if err != nil {
		logger.Error("failed to get transcript from VAPI call", "error", err, "call-id", *appointment.CallID, "user-id", user.ID, "appointment-id", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"user_id":        user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "GetAppointmentTranscript",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}
	logger.Debug("retrieved transcript", "transcript-messages", len(transcript.Messages), "user-id", user.ID, "appointment-id", appointment.ID)

	//
	// Get work preferences
	//
	internal.SendPusherMessage(psh, user.ID, "Determining your work preferences", nil)
	logger.Debug("extracting GeoID from transcript", "user-id", user.ID, "appointment-id", appointment.ID, "locations-count", len(locations))
	if err := getWorkPreferences(ctx, fs, transcript, locations, user); err != nil {
		logger.Error("failed to get work preferences", "error", err, "user-id", user.ID, "appointment-id", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"user_id":        user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "getWorkPreferences",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}
	logger.Debug("work preferences extracted", "user-id", user.ID, "appointment-id", appointment.ID, "has-preferences", user.LocationPreference != nil)

	//
	// Generate steering query
	//
	internal.SendPusherMessage(psh, user.ID, "Generating your search criteria", nil)
	logger.Debug("generating steering query", "user-id", user.ID, "appointment-id", appointment.ID)
	if err := generateSteeringQuery(ctx, fs, user, transcript); err != nil {
		logger.Error("failed to generate steering query", "error", err, "user-id", user.ID, "appointment-id", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"user_id":        user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "generateSteeringQuery",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}
	logger.Debug("steering query generated", "user-id", user.ID, "appointment-id", appointment.ID, "has-query", user.SteeringQuery != nil)

	//
	// Subscribe the user to the generated SearchCriteria
	//
	logger.Debug("validating user data", "user-id", user.ID, "appointment-id", appointment.ID, "has-steering-query", user.SteeringQuery != nil, "has-work-preferences", user.LocationPreference != nil)
	if user.SteeringQuery == nil || user.LocationPreference == nil {
		logger.Error("user steering query or work preferences not found", "user-id", user.ID, "has-steering-query", user.SteeringQuery != nil, "has-work-preferences", user.LocationPreference != nil)
		internal.SentryReportError(ctx, fmt.Errorf("user steering query or work preferences not found"), sentry.LevelError, map[string]string{
			"user_id":              user.ID,
			"appointment_id":       appointment.ID,
			"user_email":           user.Email,
			"call_id":              *appointment.CallID,
			"has_steering_query":   fmt.Sprintf("%t", user.SteeringQuery != nil),
			"has_work_preferences": fmt.Sprintf("%t", user.LocationPreference != nil),
			"step":                 "validate user data",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, fmt.Errorf("user steering query or work preferences not found"))
		return
	}

	//
	// Get or create search criteria
	//
	logger.Debug("getting or creating search criteria", "user-id", user.ID, "appointment-id", appointment.ID)
	criteria, isNewCriteria, err := getOrCreateSearchCriteria(ctx, fs, user)
	if err != nil {
		logger.Error("failed to get or create search criteria", "error", err, "user-id", user.ID, "appointment-id", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"user_id":        user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "getOrCreateSearchCriteria",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}
	logger.Debug("search criteria resolved", "user-id", user.ID, "appointment-id", appointment.ID, "criteria-id", criteria.ID, "is-new", isNewCriteria)

	//
	// Subscribe the user to the search criteria
	//
	logger.Debug("subscribing user to search criteria", "user-id", user.ID, "criteria-id", criteria.ID)
	if err := internal.SubscribeToCriteria(ctx, fs, user.ID, criteria.ID); err != nil {
		logger.Error("failed to subscribe user to search criteria", "error", err, "user-id", user.ID, "criteria-id", criteria.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"user_id":        user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"criteria_id":    criteria.ID,
			"step":           "SubscribeToCriteria",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}
	logger.Debug("user subscribed to criteria", "user-id", user.ID, "criteria-id", criteria.ID)

	if isNewCriteria {
		if queue == nil {
			logger.Error("queue is nil", "user-id", user.ID, "appointment-id", appointment.ID)
			internal.SentryReportError(ctx, fmt.Errorf("queue is nil"), sentry.LevelError, map[string]string{
				"user_id":        user.ID,
				"appointment_id": appointment.ID,
				"user_email":     user.Email,
				"call_id":        *appointment.CallID,
				"step":           "validate_queue",
			})
			internal.ProcessAppointmentFailed(ctx, fs, user, appointment, fmt.Errorf("queue is nil"))
			return
		}

		if dispatch == nil {
			logger.Error("dispatch is nil", "user-id", user.ID, "appointment-id", appointment.ID)
			internal.SentryReportError(ctx, fmt.Errorf("dispatch is nil"), sentry.LevelError, map[string]string{
				"user_id":        user.ID,
				"appointment_id": appointment.ID,
				"user_email":     user.Email,
				"call_id":        *appointment.CallID,
				"step":           "validate_dispatch",
			})
			internal.ProcessAppointmentFailed(ctx, fs, user, appointment, fmt.Errorf("dispatch is nil"))
			return
		}

		//
		// If a new criteria was created, we need to queue an immediate search.
		// We also increase / broaden the first search for a new criteria.
		//
		maxPages := internal.DefaultMaxPages * 5 // Increase the first search to get more results
		testing := os.Getenv("TESTING_MODE") == "true"
		if testing {
			maxPages = 2
		}
		criteria.MaxPages = maxPages
		criteria.ListingAge = internal.ListingAgeWeek // Do a week instead of a day

		logger.Debug("creating search task", "user-id", user.ID, "criteria-id", criteria.ID, "max-pages", maxPages, "testing-mode", testing)
		jobSearch, err := internal.CreateSearchTask(ctx, fs, *criteria, "scrapingdog")
		if err != nil {
			logger.Error("failed to create search task", "error", err, "user-id", user.ID, "criteria-id", criteria.ID)
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"user_id":        user.ID,
				"appointment_id": appointment.ID,
				"user_email":     user.Email,
				"call_id":        *appointment.CallID,
				"criteria_id":    criteria.ID,
				"step":           "CreateSearchTask",
			})
			internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
			return
		}
		logger.Debug("search task created", "user-id", user.ID, "job-search-id", jobSearch.ID)

		notifier := &userNotifier{userID: user.ID, dispatch: dispatch, publisher: psh}
		dispatch.Register(user.ID, notifier)

		logger.Debug("queueing search", "user-id", user.ID, "job-search-id", jobSearch.ID)
		if err := queue.QueueSearch(ctx, *jobSearch); err != nil {
			logger.Error("failed to queue search", "error", err, "user-id", user.ID, "job-search-id", jobSearch.ID)
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"user_id":        user.ID,
				"appointment_id": appointment.ID,
				"user_email":     user.Email,
				"call_id":        *appointment.CallID,
				"job_search_id":  jobSearch.ID,
				"step":           "QueueSearch",
			})
			internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
			return
		}
		logger.Debug("search queued successfully", "user-id", user.ID, "job-search-id", jobSearch.ID)
	}

	logger.Debug(internal.Blue("⏱️ appointment processing took"), "duration", time.Since(start).String())

	//
	// Set the apppointment status to completed
	//
	internal.SendPusherMessage(psh, user.ID, "Starting your first search (this takes a few minutes)", nil)
	logger.Debug("marking appointment as completed", "user-id", user.ID, "appointment-id", appointment.ID)
	if err := internal.ProcessAppointmentComplete(ctx, fs, user, appointment); err != nil {
		logger.Error("failed to mark appointment as completed", "error", err, "user-id", user.ID, "appointment-id", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"user_id":        user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "mark_appointment_completed",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}
	logger.Debug("appointment marked as completed", "user-id", user.ID, "appointment-id", appointment.ID)

	//
	// Refresh user jobs - queries Pinecone for relevant jobs using an existing criteria
	//
	if !isNewCriteria {
		logger.Debug("refreshing user jobs", "user-id", user.ID, "criteria-id", criteria.ID)
		internal.SendPusherMessage(psh, user.ID, "Refreshing your results", nil)
		// if err := internal.RefreshUserJobs(ctx, fs, user, *criteria, 0); err != nil {
		// 	logger.Error("failed to refresh user jobs", "error", err)
		// 	hub.CaptureException(err)
		// 	return
		// }
	}

	if smtpCfg != nil {
		logger.Debug("generating recap email", "user-id", user.ID, "appointment-id", appointment.ID)
		if err := GenerateRecapEmail(ctx, fs, user, transcript, *smtpCfg, logger); err != nil {
			logger.Error("failed to generate recap email", "error", err, "user-id", user.ID, "appointment-id", appointment.ID)
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"user_id":        user.ID,
				"appointment_id": appointment.ID,
				"user_email":     user.Email,
				"call_id":        *appointment.CallID,
				"step":           "GenerateRecapEmail",
			})
			internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
			return
		}

		appointment.RecapSent = true
		if err := internal.UpdateAppointment(ctx, fs, appointment); err != nil {
			logger.Error("failed to update appointment with recap sent status", "error", err, "user-id", user.ID, "appointment-id", appointment.ID)
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"user_id":        user.ID,
				"appointment_id": appointment.ID,
				"user_email":     user.Email,
				"call_id":        *appointment.CallID,
				"step":           "UpdateAppointment-RecapSent",
			})
			internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
			return
		}
		logger.Debug("recap email sent", "user-id", user.ID, "appointment-id", appointment.ID)
	} else {
		logger.Debug("skipping recap email - no SMTP config", "user-id", user.ID, "appointment-id", appointment.ID)
	}

	internal.SendPusherRedirect(psh, user.ID, "/dashboard")
	logger.Debug(internal.Green("✅ appointment processing completed"), "user-id", user.ID, "appointment-id", appointment.ID)
}

func GenerateRecapEmail(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, transcript *internal.Transcript, cfg SMTPConfig, logger *slog.Logger) error {
	mailer, err := smtp.NewMailer(cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.From)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "smtp_mailer_new",
		})
		return err
	}

	promptData := map[string]any{
		"FullName":   user.Name,
		"FirstName":  user.FirstName,
		"Transcript": transcript,
	}

	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/recap-email.001.txt")
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "assets_read_file",
		})
		return fmt.Errorf("failed to read recap email template: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, promptData)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "tools_generate_prompt",
		})
		return fmt.Errorf("failed to generate prompt for recap email: %w", err)
	}

	resp, err := tools.Ask(ctx, prompt, tools.WithModel(tools.DeepSeekChat))
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "tools_ask",
		})
		return fmt.Errorf("failed to ask LLM for recap email body: %w", err)
	}

	bodyData, err := extractEmailData(ctx, resp)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "extract_email_data",
		})
		logger.Error("failed to extract email data", "error", err, "user-id", user.ID, "resp", resp)
		return fmt.Errorf("failed to extract email data: %w", err)
	}

	err = mailer.Send(user.Email, bodyData, "recap-email.html")
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "smtp_mailer_send",
		})
		return fmt.Errorf("failed to send recap email: %w", err)
	}

	return nil
}

func extractEmailData(_ context.Context, data string) (map[string]any, error) {
	if !strings.Contains(data, "<plain_email_body>") {
		return nil, fmt.Errorf("failed to extract email content: <plain_email_body> tag not found")
	}

	start := strings.Index(data, "<plain_email_body>")
	if start == -1 {
		return nil, fmt.Errorf("failed to extract email content: <plain_email_body> tag not found")
	}
	end := strings.Index(data, "</plain_email_body>")
	if end == -1 {
		return nil, fmt.Errorf("failed to extract email content: </plain_email_body> tag not found")
	}

	plainBody := data[start+len("<plain_email_body>") : end]

	if plainBody == "" {
		return nil, fmt.Errorf("failed to extract email content: body is empty")
	}

	// If HTML body is present, extract it as well
	if !strings.Contains(data, "<html_email_body>") {
		return nil, fmt.Errorf("failed to extract email content: <html_email_body> tag not found")
	}

	htmlStart := strings.Index(data, "<html_email_body>")
	if htmlStart == -1 {
		return nil, fmt.Errorf("failed to extract email content: <html_email_body> tag not found")
	}
	htmlEnd := strings.Index(data, "</html_email_body>")
	if htmlEnd == -1 {
		return nil, fmt.Errorf("failed to extract email content: </html_email_body> tag not found")
	}
	htmlBody := data[htmlStart+len("<html_email_body>") : htmlEnd]
	if htmlBody == "" {
		return nil, fmt.Errorf("failed to extract email content: HTML body is empty")
	}
	return map[string]any{
		"PlainBody": plainBody,
		"HtmlBody":  htmlBody,
	}, nil
}

// RecoverProcessingAppointments finds and reprocesses any appointments that were interrupted during processing
func RecoverProcessingAppointments(hub *sentry.Hub, fs *internal.FirestoreClient, psh *pusher.Client, logger *slog.Logger, dispatch *internal.JobEventDispatch, queue internal.SearchQueue, smtpCfg *SMTPConfig) error {
	ctx := context.Background()

	logger = logger.With("pkg", "transcript.RecoverProcessingAppointments")

	logger.Info("starting recovery process",
		"has-pusher", psh != nil,
		"has-dispatch", dispatch != nil,
		"has-queue", queue != nil,
		"has-smtp-config", smtpCfg != nil)

	appointments, err := internal.ListAppointmentsNeedingProcessing(ctx, fs, logger)
	if err != nil {
		logger.Error("failed to list appointments needing processing", "error", err)
		return fmt.Errorf("failed to list appointments needing processing: %w", err)
	}

	if len(appointments) == 0 {
		logger.Info("no appointments needing processing found")
		return nil
	}

	logger.Info("found appointments needing processing", "count", len(appointments))

	// Log appointment details for debugging
	for i, appointment := range appointments {
		logger.Debug("appointment needing processing",
			"index", i,
			"appointment-id", appointment.ID,
			"user-id", appointment.UserID,
			"status", appointment.Status,
			"has-call-id", appointment.CallID != nil)
	}

	// Collect unique user IDs from appointments
	userMap := make(map[string]*internal.User)
	for _, appointment := range appointments {
		userMap[appointment.UserID] = nil
	}

	userIDs := lo.Keys(userMap)
	logger.Debug("getting users for appointments", "user-count", len(userIDs))

	// Get all users in one call
	users, err := internal.GetUsersByIDs(ctx, fs, userIDs)
	if err != nil {
		logger.Error("failed to get users by ids", "error", err, "requested-count", len(userIDs))
		return fmt.Errorf("failed to get users by ids: %w", err)
	}
	logger.Debug("retrieved users", "found-count", len(users), "requested-count", len(userIDs))

	// Populate userMap with actual user data
	validUserCount := 0
	for _, user := range users {
		if _, exists := userMap[user.ID]; exists {
			userMap[user.ID] = user
			validUserCount++
		} else {
			// If the user does not exist, skip the appointment
			logger.Error("user not found for appointment",
				"user-id", user.ID,
			)
			internal.SentryReportError(ctx, fmt.Errorf("user not found for appointment: %s", user.ID), sentry.LevelError, map[string]string{
				"user_id": user.ID,
				"step":    "populate_user_map",
			})
			// remove the appointment user from the list
			delete(userMap, user.ID)
			continue
		}
	}
	logger.Debug("user mapping completed", "valid-users", validUserCount, "total-appointments", len(appointments))

	// Process each appointment with its corresponding user
	processedCount := 0
	skippedCount := 0
	for _, appointment := range appointments {
		user, ok := userMap[appointment.UserID]
		if !ok {
			logger.Error("user not found for appointment",
				"user-id", appointment.UserID,
				"appointment-id", appointment.ID)
			internal.SentryReportError(ctx, fmt.Errorf("user not found for appointment: %s", appointment.UserID), sentry.LevelError, map[string]string{
				"user_id":        appointment.UserID,
				"appointment_id": appointment.ID,
				"step":           "process_appointments",
			})
			skippedCount++
			continue
		}

		logger.Info("reprocessing appointment",
			"appointment-id", appointment.ID,
			"user-id", appointment.UserID,
			"user-email", user.Email)

		go ProcessCompletedCall(hub, fs, psh, queue, logger, dispatch, user, appointment, smtpCfg)
		processedCount++
	}

	logger.Info("recovery process completed",
		"processed-count", processedCount,
		"skipped-count", skippedCount,
		"total-appointments", len(appointments))

	return nil
}

func getOrCreateSearchCriteria(ctx context.Context, fs *internal.FirestoreClient, user *internal.User) (*internal.SearchCriteria, bool, error) {
	if user.SteeringQuery == nil || user.LocationPreference == nil {
		return nil, false, fmt.Errorf("user steering query or work preferences not found")
	}

	sq := user.SteeringQuery
	if sq.Query == "" || sq.WorkType == "" || sq.ExperienceLevel == "" {
		return nil, false, fmt.Errorf("steering query is incomplete: %v", sq)
	}
	q := user.SteeringQuery.Query
	locPref := *user.LocationPreference

	geoID, err := internal.ExtractGeoID(ctx, fs, locPref)
	if err != nil {
		return nil, false, fmt.Errorf("failed to extract geo id: %w", err)
	}

	id := internal.MakeSearchCriteriaID(q, geoID, sq.WorkType, sq.ExperienceLevel)
	criteria, found, err := internal.GetSearchCriteria(ctx, fs, id)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get search criteria: %w", err)
	}

	if found {
		return criteria, false, nil
	}

	maxPages := internal.DefaultMaxPages
	testing := os.Getenv("TESTING_MODE") == "true"
	if testing {
		maxPages = 2
	}

	criteria, err = internal.CreateSearchCriteria(ctx, fs, q, geoID, sq.WorkType, sq.ExperienceLevel, maxPages, internal.DefaultListingAge)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create search criteria: %w", err)
	}

	return criteria, true, nil
}

// generateSteeringQuery generates a steering query based on the transcript
func generateSteeringQuery(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, transcript *internal.Transcript) error {
	resume, found, err := internal.GetLatestResume(ctx, fs, user.ID)
	if err != nil {
		return fmt.Errorf("generateSteeringQuery: failed to get user resume: %w", err)
	}
	if !found {
		return fmt.Errorf("generateSteeringQuery: no resume found for user %s", user.ID)
	}

	query, err := internal.GenerateSteeringQuery(ctx, *resume, *transcript)
	if err != nil {
		return fmt.Errorf("generateSteeringQuery: failed to ask LLM for steering query: %w", err)
	}

	user.SteeringQuery = query

	if err := internal.SetUserSteeringQuery(ctx, fs, user.ID, *query); err != nil {
		return fmt.Errorf("generateSteeringQuery: failed to update user with steering query: %w", err)
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
		return fmt.Errorf("getWorkPreferences: failed to read work location prompt: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"Transcript": transcript.String(),
		"Locations":  locations,
	})
	if err != nil {
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	data, err := tools.Ask(ctx, prompt, tools.WithModel(tools.DeepSeekChat))
	if err != nil {
		return fmt.Errorf("getWorkPreferences: failed to ask LLM: %w", err)
	}

	prefs, err := extractWorkPreferences(ctx, data)
	if err != nil {
		return fmt.Errorf("getWorkPreferences: failed to extract work preferences: %w", err)
	}

	user.LocationPreference = prefs
	if err := internal.SetUserWorkPreferences(ctx, fs, user.ID, *prefs); err != nil {
		return fmt.Errorf("getWorkPreferences: failed to update appointment with work preferences: %w", err)
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
