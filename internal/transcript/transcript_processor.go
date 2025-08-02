package transcript

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"talent-rodeo/internal/smtp"
	"talent-rodeo/internal/subscription"
	"talent-rodeo/internal/utils"

	// Removing self-import of internal package
	"talent-rodeo/assets"
	"talent-rodeo/internal"
	"time"

	"github.com/chriscow/minds/tools"
	"github.com/getsentry/sentry-go"
	"github.com/pusher/pusher-http-go/v5"
	"github.com/samber/lo"
	"github.com/sashabaranov/go-openai"
)

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
func ProcessCompletedCall(hub *sentry.Hub, fs *internal.FirestoreClient, psh *pusher.Client, queue internal.SearchQueue, logger *slog.Logger, dispatch *internal.JobEventDispatch, user *internal.User, appointment *internal.Appointment, smtpCfg *smtp.Config) {
	ctx := context.Background()
	sentry.SetHubOnContext(ctx, hub)

	// Log initial state for debugging
	logger.Info("starting call processing",
		"userID", user.ID,
		"appointmentID", appointment.ID,
		"user-email", user.Email,
		"appointment-status", appointment.Status,
		"has-call-id", appointment.CallID != nil,
		"has-smtp-config", smtpCfg != nil,
		"has-queue", queue != nil,
		"has-dispatch", dispatch != nil)

	if appointment.CallID == nil {
		logger.Error("call ID is nil", "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, fmt.Errorf("call ID is nil for appointment %s", appointment.ID), sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"step":           "validation",
		})
		return
	}

	//
	// Mark the appointment as processing and update user status
	//
	logger.Debug("marking appointment as processing started", "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID)
	if err := internal.ProcessAppointmentStarted(ctx, fs, user, appointment); err != nil {
		logger.Error("failed to mark appointment as processing", "error", err, "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
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
	logger.Debug("getting supported locations", "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID)
	locations, err := internal.GetGeoIDKeys(ctx, fs)
	if err != nil {
		logger.Error("failed to get supported locations", "error", err, "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"step":           "GetGeoIDKeys",
		})
		return
	}
	logger.Debug("retrieved supported locations", "count", len(locations), "userID", user.ID, "appointmentID", appointment.ID)

	//
	// Get the transcript from the VAPI call
	//
	logger.Debug("getting transcript from VAPI call", "name", user.Name, "call-id", *appointment.CallID, "userID", user.ID, "appointmentID", appointment.ID)
	if appointment.CallID == nil || *appointment.CallID == "" {
		logger.Error("call ID is nil", "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, fmt.Errorf("call ID is nil for appointment %s", appointment.ID), sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"step":           "GetAppointmentTranscript",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, fmt.Errorf("call ID is nil for appointment %s", appointment.ID))
		return
	}

	transcript, err := internal.GetAppointmentTranscript(ctx, fs, *appointment.CallID)
	if err != nil {
		logger.Error("failed to get transcript from VAPI call", "error", err, "call-id", *appointment.CallID, "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "GetAppointmentTranscript",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}
	logger.Debug("retrieved transcript", "transcript-messages", len(transcript.Messages), "userID", user.ID, "appointmentID", appointment.ID)

	//
	// Get work preferences
	//
	internal.SendPusherMessage(psh, user.ID, "Determining your work preferences (~10s)", nil)
	logger.Debug("extracting GeoID from transcript", "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID, "locations-count", len(locations))
	if err := getWorkPreferences(ctx, fs, logger, transcript, locations, user); err != nil {
		logger.Error("failed to get work preferences", "error", err, "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "getWorkPreferences",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}
	logger.Debug("work preferences extracted", "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID, "has-preferences", user.LocationPreference != nil)

	//
	// Generate steering query
	//
	internal.SendPusherMessage(psh, user.ID, "Generating your search criteria (~30s)", nil)
	logger.Debug("generating steering query", "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID)
	if err := generateSteeringQuery(ctx, fs, user, transcript); err != nil {
		logger.Error("failed to generate steering query", "error", err, "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "generateSteeringQuery",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}
	logger.Debug("steering query generated", "userID", user.ID, "appointmentID", appointment.ID, "has-query", user.SteeringQuery != nil)

	//
	// Subscribe the user to the generated SearchCriteria
	//
	logger.Debug("validating user data", "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID, "has-steering-query", user.SteeringQuery != nil, "has-work-preferences", user.LocationPreference != nil)
	if user.SteeringQuery == nil || user.LocationPreference == nil {
		logger.Error("user steering query or work preferences not found", "userID", user.ID, "has-steering-query", user.SteeringQuery != nil, "has-work-preferences", user.LocationPreference != nil)
		internal.SentryReportError(ctx, fmt.Errorf("user steering query or work preferences not found"), sentry.LevelError, map[string]string{
			"userID":               user.ID,
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
	logger.Debug("getting or creating search criteria", "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID)
	searchCriteriaList, err := getOrCreateSearchCriteria(ctx, fs, user)
	if err != nil {
		logger.Error("failed to get or create search criteria", "error", err, "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "getOrCreateSearchCriteria",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	//
	// Subscribe user to all search criteria at once (atomic transaction)
	//
	criteriaIDs := make([]string, len(searchCriteriaList))
	for i, searchCriteria := range searchCriteriaList {
		criteriaIDs[i] = searchCriteria.Criteria.ID
	}
	logger.Debug("subscribing user to all search criteria", "userID", user.ID, "criteria-count", len(criteriaIDs))
	if err := internal.SubscribeToMultipleCriteria(ctx, fs, user.ID, criteriaIDs); err != nil {
		logger.Error("failed to subscribe user to search criteria", "error", err, "userID", user.ID, "appointmentID", appointment.ID, "criteria-count", len(criteriaIDs))
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"criteria_count": fmt.Sprintf("%d", len(criteriaIDs)),
			"step":           "SubscribeToMultipleCriteria",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}
	logger.Debug("user subscribed to all criteria", "name", user.Name, "userID", user.ID, "criteria-count", len(criteriaIDs))

	//
	// Get user statement
	//
	logger.Debug("getting user statement", "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID)
	internal.SendPusherMessage(psh, user.ID, "Analyzing your call", nil)
	if err := GetUserStatement(ctx, user, transcript, fs, logger); err != nil {
		logger.Error("failed to get user statement", "error", err, "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "getUserStatement",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	SendRecapEmail(ctx, fs, user, appointment, transcript, smtpCfg, logger)

	//
	// We set the user status to call completed so they will be redirected to the dashboard on the next refresh.
	//
	logger.Debug("marking appointment as completed", "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID)
	if err := internal.ProcessAppointmentComplete(ctx, fs, user, appointment); err != nil {
		logger.Error("failed to mark appointment as completed", "error", err, "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "mark_appointment_completed",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return
	}

	for i, searchCriteria := range searchCriteriaList {

		which := "first"
		count := "search"
		switch i {
		case 0:
			which = "first"
		case 1:
			which = "second"
		case 2:
			which = "third"
		case 3:
			which = "fourth"
		case 4:
			which = "last"
		default:
			which = "first"
		}
		switch len(searchCriteriaList) {
		case 2:
			count = "of two searches"
		case 3:
			count = "of three searches"
		case 4:
			count = "of four searches"
		case 5:
			count = "of five searches"
		default:
			count = "search"
		}
		//
		// User is already subscribed to all criteria - no need to subscribe individually
		//
		logger.Debug("processing criteria", "userID", user.ID, "criteria-id", searchCriteria.Criteria.ID, "is-new", searchCriteria.IsNew)

		if searchCriteria.IsNew {
			if queue == nil {
				logger.Error("queue is nil", "userID", user.ID, "appointmentID", appointment.ID)
				internal.SentryReportError(ctx, fmt.Errorf("queue is nil"), sentry.LevelError, map[string]string{
					"userID":         user.ID,
					"appointment_id": appointment.ID,
					"user_email":     user.Email,
					"call_id":        *appointment.CallID,
					"step":           "validate_queue",
				})
				internal.ProcessAppointmentFailed(ctx, fs, user, appointment, fmt.Errorf("queue is nil"))
				return
			}
			internal.SendPusherProgress(psh, user.ID, "server-progress", fmt.Sprintf("Starting your %s %s (phase 1 of 2,this takes a few minutes)", which, count), 0, 100)
			if dispatch == nil {
				logger.Error("dispatch is nil", "userID", user.ID, "appointmentID", appointment.ID)
				internal.SentryReportError(ctx, fmt.Errorf("dispatch is nil"), sentry.LevelError, map[string]string{
					"userID":         user.ID,
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
			maxPages := internal.DefaultMaxPages * 3 // Increase the first search to get more results
			testing := os.Getenv("TESTING_MODE") == "true"
			if testing {
				maxPages = 2
			}
			searchCriteria.Criteria.MaxPages = maxPages
			searchCriteria.Criteria.ListingAge = internal.ListingAgeWeek // Do a week instead of a day

			logger.Debug("creating search task", "name", user.Name, "userID", user.ID, "criteria-id", searchCriteria.Criteria.ID, "max-pages", maxPages, "testing-mode", testing)
			jobSearch, err := internal.CreateSearchTask(ctx, fs, searchCriteria.Criteria, "scrapingdog", &user.ID)
			if err != nil {
				logger.Error("failed to create search task", "error", err, "userID", user.ID, "criteria-id", searchCriteria.Criteria.ID)
				internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
					"userID":         user.ID,
					"appointment_id": appointment.ID,
					"user_email":     user.Email,
					"call_id":        *appointment.CallID,
					"criteria_id":    searchCriteria.Criteria.ID,
					"step":           "CreateSearchTask",
				})
				internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
				return
			}
			logger.Debug("search task created", "name", user.Name, "userID", user.ID, "job-search-id", jobSearch.ID)

			handlerID := fmt.Sprintf("search-completion-%s", jobSearch.ID)
			completionHandler := newSearchCompletionHandler(jobSearch.ID, user.ID, logger)
			if err := dispatch.Register(handlerID, completionHandler); err != nil {
				logger.Error("failed to register search completion handler", "error", err, "userID", user.ID, "job-search-id", jobSearch.ID)
				internal.ProcessAppointmentFailed(ctx, fs, user, appointment, fmt.Errorf("failed to register search completion handler: %w", err))
				return
			}
			defer dispatch.Deregister(handlerID)

			logger.Debug("queueing search", "name", user.Name, "userID", user.ID, "job-search-id", jobSearch.ID)
			if err := queue.QueueSearch(ctx, *jobSearch); err != nil {
				logger.Error("failed to queue search", "error", err, "userID", user.ID, "job-search-id", jobSearch.ID)
				internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
					"userID":         user.ID,
					"appointment_id": appointment.ID,
					"user_email":     user.Email,
					"call_id":        *appointment.CallID,
					"job_search_id":  jobSearch.ID,
					"step":           "QueueSearch",
				})
				internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
				return
			}
			logger.Debug("search queued successfully", "name", user.Name, "userID", user.ID, "job-search-id", jobSearch.ID)
			internal.SendPusherMessage(psh, user.ID, "", nil)

			// Block here until the initial search is complete.
			logger.Info("waiting for initial search to complete", "userID", user.ID, "job-search-id", jobSearch.ID, "timeout", "10m")
			select {
			case <-completionHandler.Wait():
				logger.Info("initial search completed", "userID", user.ID, "job-search-id", jobSearch.ID)
			case <-time.After(30 * time.Minute):
				logger.Error("timed out waiting for initial search to complete", "userID", user.ID, "job-search-id", jobSearch.ID)
				internal.SentryReportError(ctx, fmt.Errorf("timed out waiting for search %s", jobSearch.ID), sentry.LevelError, map[string]string{
					"userID":         user.ID,
					"appointment_id": appointment.ID,
					"job_search_id":  jobSearch.ID,
					"step":           "WaitForInitialSearch",
				})
			}
		}

		internal.SendPusherMessage(psh, user.ID, "Refreshing your results", nil)
		internal.SendPusherRedirect(psh, user.ID, "/dashboard")

		// give time for the user to see the redirect
		time.Sleep(5 * time.Second)
		internal.SendPusherMessage(psh, user.ID, "Refreshing your results (phase 2 of 2, this takes a few minutes)", nil)

		logger.Debug("refreshing user jobs", "name", user.Name, "userID", user.ID, "criteria-id", searchCriteria.Criteria.ID)
		if err := subscription.RefreshUserJobs(ctx, fs, psh, logger, *user, searchCriteria.Criteria); err != nil {
			logger.Error("failed to refresh user jobs", "error", err)
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"userID":     user.ID,
				"user_email": user.Email,
				"step":       "refreshUserJobs",
			})
		}
	}

	logger.Debug(internal.Blue("⏱️ appointment processing took"), "name", user.Name, "duration", time.Since(start).String())
	logger.Debug(internal.Green("✅ appointment processing completed"), "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID)
}

func SendRecapEmail(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, appointment *internal.Appointment, transcript *internal.Transcript, cfg *smtp.Config, logger *slog.Logger) error {
	if cfg == nil {
		logger.Debug("skipping recap email - no SMTP config", "userID", user.ID, "appointmentID", appointment.ID)
		return nil
	}

	logger.Debug("generating recap email", "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID)
	if err := GenerateRecapEmail(ctx, fs, user, transcript, *cfg, logger); err != nil {
		logger.Error("failed to generate recap email", "error", err, "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "GenerateRecapEmail",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return fmt.Errorf("SendRecapEmail: GenerateRecapEmail: %w", err)
	}

	appointment.RecapSent = true
	if err := internal.UpdateAppointment(ctx, fs, appointment); err != nil {
		logger.Error("failed to update appointment with recap sent status", "error", err, "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id":        *appointment.CallID,
			"step":           "UpdateAppointment-RecapSent",
		})
		internal.ProcessAppointmentFailed(ctx, fs, user, appointment, err)
		return fmt.Errorf("SendRecapEmail: UpdateAppointment: %w", err)
	}
	logger.Debug("recap email sent", "userID", user.ID, "appointmentID", appointment.ID)

	return nil
}

func SendFailureEmail(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, appointment *internal.Appointment, failureReason string, cfg *smtp.Config, logger *slog.Logger) error {
	if cfg == nil {
		logger.Debug("skipping failure email - no SMTP config", "userID", user.ID, "appointmentID", appointment.ID)
		return nil
	}

	logger.Debug("generating failure email", "name", user.Name, "userID", user.ID, "appointmentID", appointment.ID, "failureReason", failureReason)
	if err := GenerateFailureEmail(ctx, fs, user, failureReason, *cfg, logger); err != nil {
		logger.Error("failed to generate failure email", "error", err, "userID", user.ID, "appointmentID", appointment.ID)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"userID":         user.ID,
			"appointment_id": appointment.ID,
			"user_email":     user.Email,
			"call_id": func() string {
				if appointment.CallID != nil {
					return *appointment.CallID
				} else {
					return "nil"
				}
			}(),
			"step": "GenerateFailureEmail",
		})
		return fmt.Errorf("SendFailureEmail: GenerateFailureEmail: %w", err)
	}

	logger.Debug("failure email sent", "userID", user.ID, "appointmentID", appointment.ID)
	return nil
}

func GenerateFailureEmail(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, failureReason string, cfg smtp.Config, logger *slog.Logger) error {
	mailer, err := smtp.NewMailSender(cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.From)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "smtp_mailer_new",
		})
		return fmt.Errorf("GenerateFailureEmail: NewMailSender: %w", err)
	}

	promptData := map[string]any{
		"FullName":      user.Name,
		"FirstName":     user.FirstName,
		"FailureReason": failureReason,
	}

	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/call-failure-email.001.txt")
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "assets_read_file",
		})
		return fmt.Errorf("GenerateFailureEmail: ReadFile: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, promptData)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "tools_generate_prompt",
		})
		return fmt.Errorf("GenerateFailureEmail: GeneratePrompt: %w", err)
	}

	resp := openai.ChatCompletionResponse{}
	llmResp, err := tools.Ask(ctx, prompt, tools.WithModel(tools.DeepSeekChat), tools.WantCompletionResponse(&resp))
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "tools_ask",
		})
		return fmt.Errorf("GenerateFailureEmail: Ask: %w", err)
	}

	// Record LLM cost for failure email generation
	if err := internal.RecordLLMCost(ctx, fs, &user.ID, tools.DeepSeekChat, resp, "failure_email_generation", nil); err != nil {
		// Log error but don't fail the operation
		logger.Error("failed to record LLM cost for failure email generation", "error", err)
	}

	bodyData, err := extractEmailData(ctx, llmResp)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "extract_email_data",
		})
		logger.Error("failed to extract email data", "error", err, "userID", user.ID, "resp", llmResp)
		return fmt.Errorf("GenerateFailureEmail: extractEmailData: %w", err)
	}

	err = mailer.SendFromTemplate(user.Email, bodyData, "call-failure-email.html")
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "smtp_mailer_send",
			"component":  "transcript_processor_send_failure",
		})
		return fmt.Errorf("GenerateFailureEmail: SendFromTemplate: %w", err)
	}

	return nil
}

func GenerateRecapEmail(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, transcript *internal.Transcript, cfg smtp.Config, logger *slog.Logger) error {
	mailer, err := smtp.NewMailSender(cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.From)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "smtp_mailer_new",
		})
		return fmt.Errorf("SendRecapEmail: tools.GeneratePrompt: %w", err)
	}

	promptData := map[string]any{
		"FullName":   user.Name,
		"FirstName":  user.FirstName,
		"Transcript": transcript,
	}

	// Check if user has trial information and timezone to include trial context
	var templateFile string
	if user.SubscriptionEnd != nil && user.Timezone != nil {
		// User is in trial period - add trial context
		trialStartFormatted, _ := utils.FormatTimeInUserTimezone(
			time.Now(),
			*user.Timezone,
			"January 2, 2006 at 3:04 PM MST",
		)

		trialEndFormatted, _ := utils.FormatTimeInUserTimezone(
			*user.SubscriptionEnd,
			*user.Timezone,
			"January 2, 2006 at 11:59 PM MST",
		)

		daysRemaining := utils.CalculateDaysRemaining(*user.SubscriptionEnd, *user.Timezone)

		// Add trial context to prompt data
		promptData["TrialStartDateUser"] = trialStartFormatted
		promptData["TrialEndDateUser"] = trialEndFormatted
		promptData["DaysRemaining"] = daysRemaining

		templateFile = "prompts/recap-email-with-trial.001.txt"
	} else {
		// Use original template for users without trial context
		templateFile = "prompts/recap-email.001.txt"
	}

	tmpl, err := assets.EmbeddedFiles.ReadFile(templateFile)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "assets_read_file",
		})
		return fmt.Errorf("SendRecapEmail: tools.GeneratePrompt: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, promptData)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "tools_generate_prompt",
		})
		return fmt.Errorf("SendRecapEmail: tools.GeneratePrompt: %w", err)
	}

	resp := openai.ChatCompletionResponse{}
	llmResp, err := tools.Ask(ctx, prompt, tools.WithModel(tools.DeepSeekChat), tools.WantCompletionResponse(&resp))
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "tools_ask",
		})
		return fmt.Errorf("SendRecapEmail: tools.Ask: %w", err)
	}

	// Record LLM cost for recap email generation
	if err := internal.RecordLLMCost(ctx, fs, &user.ID, tools.DeepSeekChat, resp, "recap_email_generation", nil); err != nil {
		// Log error but don't fail the operation
		logger.Error("failed to record LLM cost for recap email generation", "error", err)
	}

	bodyData, err := extractEmailData(ctx, llmResp)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "extract_email_data",
		})
		logger.Error("failed to extract email data", "error", err, "userID", user.ID, "resp", llmResp)
		return fmt.Errorf("SendRecapEmail: extractEmailData: %w", err)
	}

	err = mailer.SendFromTemplate(user.Email, bodyData, "recap-email.html")
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "smtp_mailer_send",
			"component":  "transcript_processor_send_recap",
		})
		return fmt.Errorf("SendRecapEmail: mailer.Send: %w", err)
	}

	return nil
}

func extractEmailData(_ context.Context, data string) (map[string]any, error) {
	if !strings.Contains(data, "<plain_email_body>") {
		return nil, fmt.Errorf("extractEmailData: <plain_email_body> tag not found")
	}

	start := strings.Index(data, "<plain_email_body>")
	if start == -1 {
		return nil, fmt.Errorf("extractEmailData: <plain_email_body> tag not found")
	}
	end := strings.Index(data, "</plain_email_body>")
	if end == -1 {
		return nil, fmt.Errorf("extractEmailData: </plain_email_body> tag not found")
	}

	plainBody := data[start+len("<plain_email_body>") : end]

	if plainBody == "" {
		return nil, fmt.Errorf("extractEmailData: body is empty")
	}

	// If HTML body is present, extract it as well
	if !strings.Contains(data, "<html_email_body>") {
		return nil, fmt.Errorf("extractEmailData: <html_email_body> tag not found")
	}

	htmlStart := strings.Index(data, "<html_email_body>")
	if htmlStart == -1 {
		return nil, fmt.Errorf("extractEmailData: <html_email_body> tag not found")
	}
	htmlEnd := strings.Index(data, "</html_email_body>")
	if htmlEnd == -1 {
		return nil, fmt.Errorf("extractEmailData: </html_email_body> tag not found")
	}
	htmlBody := data[htmlStart+len("<html_email_body>") : htmlEnd]
	if htmlBody == "" {
		return nil, fmt.Errorf("extractEmailData: HTML body is empty")
	}
	return map[string]any{
		"PlainBody": plainBody,
		"HtmlBody":  htmlBody,
	}, nil
}

// RecoverProcessingAppointments finds and reprocesses any appointments that were interrupted during processing
func RecoverProcessingAppointments(hub *sentry.Hub, fs *internal.FirestoreClient, psh *pusher.Client, logger *slog.Logger, dispatch *internal.JobEventDispatch, queue internal.SearchQueue, smtpCfg *smtp.Config) error {
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
		return fmt.Errorf("RecoverProcessingAppointments: ListAppointmentsNeedingProcessing: %w", err)
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
			"appointmentID", appointment.ID,
			"userID", appointment.UserID,
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
		return fmt.Errorf("RecoverProcessingAppointments: GetUsersByIDs: %w", err)
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
				"userID", user.ID,
			)
			internal.SentryReportError(ctx, fmt.Errorf("user not found for appointment: %s", user.ID), sentry.LevelError, map[string]string{
				"userID": user.ID,
				"step":   "populate_user_map",
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
				"userID", appointment.UserID,
				"appointmentID", appointment.ID)
			internal.SentryReportError(ctx, fmt.Errorf("user not found for appointment: %s", appointment.UserID), sentry.LevelError, map[string]string{
				"userID":         appointment.UserID,
				"appointment_id": appointment.ID,
				"step":           "process_appointments",
			})
			skippedCount++
			continue
		}

		logger.Info("reprocessing appointment",
			"appointmentID", appointment.ID,
			"userID", appointment.UserID,
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

type searchCriteriaResult struct {
	Criteria internal.SearchCriteria
	IsNew    bool
}

func getOrCreateSearchCriteria(ctx context.Context, fs *internal.FirestoreClient, user *internal.User) ([]searchCriteriaResult, error) {
	if user.SteeringQuery == nil || user.LocationPreference == nil {
		return nil, fmt.Errorf("getOrCreateSearchCriteria: user steering query or work preferences not found")
	}

	sq := user.SteeringQuery
	if err := sq.Validate(); err != nil {
		return nil, fmt.Errorf("getOrCreateSearchCriteria: steering query is invalid: %w", err)
	}

	locPref := *user.LocationPreference

	geoID, err := internal.ExtractGeoID(ctx, fs, &user.ID, locPref)
	if err != nil {
		return nil, fmt.Errorf("getOrCreateSearchCriteria: ExtractGeoID: %w", err)
	}

	maxPages := internal.DefaultMaxPages
	testing := os.Getenv("TESTING_MODE") == "true"
	if testing {
		maxPages = 2
	}

	results := []searchCriteriaResult{}

	for _, query := range sq.Queries {
		for _, workType := range sq.WorkTypes {
			id := internal.MakeSearchCriteriaID(query, geoID, workType, sq.ExperienceLevel)
			criteria, found, err := internal.GetSearchCriteria(ctx, fs, id)
			if err != nil {
				return nil, fmt.Errorf("getOrCreateSearchCriteria: GetSearchCriteria: %w", err)
			}

			if found {
				results = append(results, searchCriteriaResult{
					Criteria: *criteria,
					IsNew:    false,
				})
				continue
			}

			criteria, err = internal.CreateSearchCriteria(ctx, fs, query, geoID, workType, sq.ExperienceLevel, maxPages, internal.DefaultListingAge)
			if err != nil {
				return nil, fmt.Errorf("getOrCreateSearchCriteria: CreateSearchCriteria: %w", err)
			}

			results = append(results, searchCriteriaResult{
				Criteria: *criteria,
				IsNew:    true,
			})
		}
	}
	return results, nil
}

// generateSteeringQuery generates a steering query based on the transcript
func generateSteeringQuery(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, transcript *internal.Transcript) error {
	resume, found, err := internal.GetLatestResume(ctx, fs, user.ID)
	if err != nil {
		return fmt.Errorf("generateSteeringQuery: GetLatestResume: %w", err)
	}
	if !found {
		return fmt.Errorf("generateSteeringQuery: GetLatestResume: no resume found for user %s", user.ID)
	}

	query, err := internal.GenerateSteeringQuery(ctx, *resume, *transcript)
	if err != nil {
		return fmt.Errorf("generateSteeringQuery: GenerateSteeringQuery: %w", err)
	}

	user.SteeringQuery = query

	if err := internal.SetUserSteeringQuery(ctx, fs, user.ID, *query); err != nil {
		return fmt.Errorf("generateSteeringQuery: SetUserSteeringQuery: %w", err)
	}
	return nil
}

func extractWorkPreferences(ctx context.Context, data string) (*string, error) {
	if strings.Contains(data, "<final_determinations>") {
		start := strings.Index(data, "<final_determinations>")
		if start == -1 {
			return nil, fmt.Errorf("extractWorkPreferences: <final_determinations> tag not found")
		}
		end := strings.Index(data, "</final_determinations>")
		if end == -1 {
			return nil, fmt.Errorf("extractWorkPreferences: </final_determinations> tag not found")
		}
		data = data[start+len("<final_determinations>") : end]
	} else {
		return nil, fmt.Errorf("extractWorkPreferences: <final_determinations> tag not found")
	}

	return &data, nil
}
func getWorkPreferences(ctx context.Context, fs *internal.FirestoreClient, logger *slog.Logger, transcript *internal.Transcript, locations []string, user *internal.User) error {
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript.work-location.txt")
	if err != nil {
		return fmt.Errorf("getWorkPreferences: ReadFile: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"Transcript": transcript.String(),
		"Locations":  locations,
	})
	if err != nil {
		return fmt.Errorf("getWorkPreferences: GeneratePrompt: %w", err)
	}

	resp := openai.ChatCompletionResponse{}
	data, err := tools.Ask(ctx, prompt, tools.WithModel(tools.DeepSeekChat), tools.WantCompletionResponse(&resp))
	if err != nil {
		return fmt.Errorf("getWorkPreferences: Ask: %w", err)
	}

	// Record LLM cost for work preferences extraction
	if err := internal.RecordLLMCost(ctx, fs, &user.ID, tools.DeepSeekChat, resp, "work_preferences_extraction", nil); err != nil {
		// Log error but don't fail the operation
		// Note: we don't have a logger here, so using default
		logger.Error("failed to record LLM cost for work preferences extraction", "error", err)
	}

	prefs, err := extractWorkPreferences(ctx, data)
	if err != nil {
		return fmt.Errorf("getWorkPreferences: extractWorkPreferences: %w", err)
	}

	user.LocationPreference = prefs
	if err := internal.SetUserWorkPreferences(ctx, fs, user.ID, *prefs); err != nil {
		return fmt.Errorf("getWorkPreferences: SetUserWorkPreferences: %w", err)
	}

	return nil
}

// func (p *processor) getSearchQueries(ctx context.Context, resume *internal.Resume, appointment *internal.Appointment) error {
// 	p.logger.Debug("generating search criteria", "userID", appointment.UserID, "appointmentID", appointment.ID)

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

func GetUserStatement(ctx context.Context, user *internal.User, transcript *internal.Transcript, fs *internal.FirestoreClient, logger *slog.Logger) error {
	// Check if user already has statement versions
	existing, err := internal.GetLatestStatement(ctx, fs, user.ID)
	if err == nil && existing != nil {
		logger.Debug("user already has statement, skipping transcript generation", "userID", user.ID, "currentVersion", existing.VersionNumber)
		return nil
	}
	
	logger.Debug("generating initial user statement using versioning system", "userID", user.ID)
	
	// Use versioning system to create initial statement
	promptFile := "transcript.user-statement.v001.txt"
	version, err := internal.CreateInitialStatement(ctx, fs, user.ID, transcript, promptFile)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, nil)
		return fmt.Errorf("failed to create initial statement: %w", err)
	}
	
	// Update user object with new statement for immediate use in this session
	user.Statement = &version.StatementText
	
	logger.Debug("created initial statement version", "userID", user.ID, "versionID", version.ID, "versionNumber", version.VersionNumber)
	return nil
}

func extractUserStatement(_ context.Context, data string) (string, error) {
	if !strings.Contains(data, "<first_person_prompt>") {
		return "", fmt.Errorf("extractUserStatement: <first_person_prompt> tag not found")
	}

	start := strings.Index(data, "<first_person_prompt>")
	if start == -1 {
		return "", fmt.Errorf("extractUserStatement: <first_person_prompt> tag not found")
	}
	end := strings.Index(data, "</first_person_prompt>")
	if end == -1 {
		return "", fmt.Errorf("extractUserStatement: </first_person_prompt> tag not found")
	}

	statement := data[start+len("<first_person_prompt>") : end]
	if statement == "" {
		return "", fmt.Errorf("extractUserStatement: user statement is empty")
	}

	return statement, nil
}

// searchCompletionHandler waits for a specific search task to complete.
type searchCompletionHandler struct {
	doneCh       chan struct{}
	searchTaskID string
	userID       string
	logger       *slog.Logger
}

// newSearchCompletionHandler creates a new searchCompletionHandler.
func newSearchCompletionHandler(searchTaskID string, userID string, logger *slog.Logger) *searchCompletionHandler {
	return &searchCompletionHandler{
		doneCh:       make(chan struct{}),
		searchTaskID: searchTaskID,
		userID:       userID,
		logger:       logger.With("component", "searchCompletionHandler", "searchTaskID", searchTaskID, "userID", userID),
	}
}

// Wait implements the Waiter interface.
func (h *searchCompletionHandler) Wait() <-chan struct{} {
	return h.doneCh
}

func (h *searchCompletionHandler) OnSearchStarted(task internal.SearchTask, userID *string) {}
func (h *searchCompletionHandler) OnJobListing(task *internal.SearchTask, listing internal.JobListing, userID *string) bool {
	return false // returning true will indicate it is a new job listing and it will get fetched. We don't know so return false.
}
func (h *searchCompletionHandler) OnJobDetail(task internal.SearchTask, listing internal.JobListing, details internal.JobDetail, userID *string) {
}
func (h *searchCompletionHandler) OnPageComplete(task internal.SearchTask, page int, jobsFound int, errors []error, userID *string) {
}
func (h *searchCompletionHandler) OnSearchCompleted(task internal.SearchTask, totalJobs int, errors []error, userID *string) {
}
func (h *searchCompletionHandler) OnJobDetailExtractionComplete(task internal.SearchTask, jobDetail internal.JobDetail, userID *string) {
}

func (h *searchCompletionHandler) OnAllExtractionsComplete(task internal.SearchTask, totalJobs int, userID *string) {
	if task.ID != h.searchTaskID {
		return
	}

	uid := "nil"
	if userID != nil {
		uid = *userID
	}
	h.logger.Debug("received OnAllExtractionsComplete", "task.ID", task.ID, "totalJobs", totalJobs, "userID", uid)

	if h.userID != "" && userID != nil && h.userID == *userID {
		h.logger.Debug("all extractions complete event received, closing done channel")
		close(h.doneCh)
	} else {
		h.logger.Warn("user ID mismatch for all extractions complete event", "event-user-id", uid, "handler-user-id", h.userID)
	}
}

// func (p *processor) getJobDescription(ctx context.Context, resume *internal.Resume, appointment *internal.Appointment) error {
// 	p.logger.Debug("getting job description", "userID", appointment.UserID, "appointmentID", appointment.ID)

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
