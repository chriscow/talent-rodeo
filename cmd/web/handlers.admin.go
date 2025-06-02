package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/cloudflare"
	"talent-rodeo/internal/feedback"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/search"
	"talent-rodeo/internal/search/providers/scrapingdog"
	"talent-rodeo/internal/subscription"
	"time"

	"github.com/getsentry/sentry-go"
)

func (app *application) adminIndex(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	if err := response.Page(w, http.StatusOK, data, "pages/admin/index.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) adminVoice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminUser := contextGetAuthenticatedUser(r)
	data := app.newTemplateData(r)
	data["VapiAPIKey"] = os.Getenv("VAPI_PUBLIC_API_KEY")
	data["AssistantID"] = os.Getenv("VAPI_ASSISTANT_ID")

	// Fetch all users for the dropdown
	users, err := internal.ListUsers(ctx, app.fs, 100, 0) // Limiting to 100 users for now
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data["Users"] = users

	// Handle POST request for user selection
	if r.Method == http.MethodPost {
		app.logger.Info("handling POST request", "url", r.URL.String())

		if err := r.ParseForm(); err != nil {
			app.logger.Error("failed to parse form", "error", err)
			app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
			return
		}

		selectedUserID := r.PostForm.Get("user_id")
		app.logger.Info("received form data", "user_id", selectedUserID)

		if selectedUserID == "" {
			app.logger.Error("no user selected")
			http.Error(w, "No user selected", http.StatusBadRequest)
			return
		}

		data["SelectedUserID"] = selectedUserID

		// Handle the "no user" case
		if selectedUserID == "_none" {
			app.logger.Info("handling _none user case")
			// Build assistant overrides with nil user and nil resume
			overrides, err := internal.BuildAssistantOverrides(ctx, nil, nil, app.fs, app.logger)
			if err != nil {
				app.logger.Error("failed to build assistant overrides", "error", err)
				app.serverError(w, r, fmt.Errorf("failed to build assistant overrides: %w", err))
				return
			}
			data["AssistantOverrides"] = overrides
		} else {
			app.logger.Info("fetching user details", "user_id", selectedUserID)
			// Fetch the selected user's details
			selectedUser, found, err := internal.GetUser(ctx, app.fs, selectedUserID)
			if err != nil {
				app.logger.Error("failed to get user", "error", err, "user_id", selectedUserID)
				app.serverError(w, r, fmt.Errorf("failed to get user: %w", err))
				return
			}
			if !found {
				app.logger.Error("user not found", "user_id", selectedUserID)
				http.Error(w, "User not found", http.StatusNotFound)
				return
			}

			// Fetch the user's latest resume
			resume, _, err := internal.GetLatestResume(ctx, app.fs, selectedUserID)
			if err != nil {
				app.logger.Warn("failed to get resume", "error", err, "user_id", selectedUserID)
				// Continue without resume - it's optional
			}

			// Build assistant overrides for the selected user
			overrides, err := internal.BuildAssistantOverrides(ctx, selectedUser, resume, app.fs, app.logger)
			if err != nil {
				app.logger.Error("failed to build assistant overrides", "error", err, "user_id", selectedUserID)
				app.serverError(w, r, fmt.Errorf("failed to build assistant overrides: %w", err))
				return
			}

			// call.name is the appointment ID
			// call.customer.name is the customer ID
			apptName := funcs.Slugify(fmt.Sprintf("admin-%s", adminUser.Name))
			appt := &internal.Appointment{
				UserID:        selectedUser.ID,
				Name:          apptName,
				ScheduledTime: time.Now(),
				Status:        internal.AppointmentStatusRunning,
			}
			if err := internal.CreateAppointment(ctx, app.fs, selectedUser.ID, appt); err != nil {
				app.logger.Error("failed to create appointment", "error", err, "user_id", selectedUserID)
				app.serverError(w, r, fmt.Errorf("failed to create appointment: %w", err))
				return
			}

			data["CallName"] = appt.ID
			data["CallCustomerName"] = selectedUser.ID
			data["AssistantOverrides"] = overrides
			data["Prompt"] = overrides.Model.Messages[0].Content
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/voice.html"); err != nil {
		app.logger.Error("failed to render page", "error", err)
		app.serverError(w, r, err)
	}
}

func (app *application) adminFeatureFlags(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	// Initialize Cloudflare KV client
	kv, err := cloudflare.NewKV()
	if err != nil {
		app.logger.Error("failed to create Cloudflare KV client", "error", err)
		data["Error"] = "Failed to initialize Cloudflare KV: " + err.Error()
		if err := response.Page(w, http.StatusOK, data, "pages/admin/feature-flags.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	// Initialize default settings
	var ss search.SchedulerSettings = search.DefaultSchedulerSettings
	var sp subscription.ProcessorSettings = subscription.DefaultSettings
	var sd scrapingdog.WorkerSettings
	var fp feedback.ProcessorSettings = feedback.DefaultSettings

	// Read search-scheduler flag
	if err := kv.Get("search-scheduler", &ss); err != nil {
		if err != cloudflare.ErrNotFound {
			app.logger.Error("failed to get search-scheduler flag", "error", err)
		}
		ss = search.DefaultSchedulerSettings
	}

	// Read subscription-processor flag
	if err := kv.Get("subscription-processor", &sp); err != nil {
		if err != cloudflare.ErrNotFound {
			app.logger.Error("failed to get subscription-processor flag", "error", err)
		}
		sp = subscription.DefaultSettings
	}

	// Read scrapingdog workers flag
	if err := kv.Get("scrapingdog", &sd); err != nil {
		if err != cloudflare.ErrNotFound {
			app.logger.Error("failed to get scrapingdog flag", "error", err)
		}
		// Keep default value if error or not found
	}

	// Read feedback processor flag
	if err := kv.Get("feedback-processor", &fp); err != nil {
		if err != cloudflare.ErrNotFound {
			app.logger.Error("failed to get feedback-processor flag", "error", err)
		}
		fp = feedback.DefaultSettings
	}

	data["SearchScheduler"] = ss
	data["SubscriptionProcessor"] = sp
	data["ScrapingdogWorkers"] = sd
	data["FeedbackProcessor"] = fp

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.logger.Error("failed to parse form", "error", err)
			data["Error"] = "Failed to parse form: " + err.Error()
		} else {
			newSearchPaused := r.PostForm.Get("search-scheduler-paused") == "true"
			newSubscriptionPaused := r.PostForm.Get("subscription-processor-paused") == "true"
			newScrapingdogPaused := r.PostForm.Get("scrapingdog-paused") == "true"
			newFeedbackPaused := r.PostForm.Get("feedback-processor-paused") == "true"

			searchStartHour, _ := strconv.Atoi(r.PostForm.Get("search-scheduler-start-hour"))
			searchEndHour, _ := strconv.Atoi(r.PostForm.Get("search-scheduler-end-hour"))
			subscriptionStartHour, _ := strconv.Atoi(r.PostForm.Get("subscription-processor-start-hour"))
			subscriptionEndHour, _ := strconv.Atoi(r.PostForm.Get("subscription-processor-end-hour"))
			feedbackStartHour, _ := strconv.Atoi(r.PostForm.Get("feedback-processor-start-hour"))
			feedbackEndHour, _ := strconv.Atoi(r.PostForm.Get("feedback-processor-end-hour"))

			var errors []string

			ss.Paused = newSearchPaused
			ss.StartHour = searchStartHour
			ss.EndHour = searchEndHour
			if err := kv.Set("search-scheduler", ss, 0); err != nil {
				app.logger.Error("failed to set search-scheduler settings", "error", err)
				errors = append(errors, "Failed to update search-scheduler: "+err.Error())
			} else {
				app.logger.Info("updated search-scheduler settings", "paused", newSearchPaused, "start_hour", searchStartHour, "end_hour", searchEndHour)
			}

			sp.Paused = newSubscriptionPaused
			sp.StartHour = subscriptionStartHour
			sp.EndHour = subscriptionEndHour
			if err := kv.Set("subscription-processor", sp, 0); err != nil {
				app.logger.Error("failed to set subscription-processor settings", "error", err)
				errors = append(errors, "Failed to update subscription-processor: "+err.Error())
			} else {
				app.logger.Info("updated subscription-processor settings", "paused", newSubscriptionPaused, "start_hour", subscriptionStartHour, "end_hour", subscriptionEndHour)
			}

			sd.Paused = newScrapingdogPaused
			if err := kv.Set("scrapingdog", sd, 0); err != nil {
				app.logger.Error("failed to set scrapingdog flag", "error", err)
				errors = append(errors, "Failed to update scrapingdog: "+err.Error())
			} else {
				app.logger.Info("updated scrapingdog flag", "paused", newScrapingdogPaused)
			}

			fp.Paused = newFeedbackPaused
			fp.StartHour = feedbackStartHour
			fp.EndHour = feedbackEndHour
			if err := kv.Set("feedback-processor", fp, 0); err != nil {
				app.logger.Error("failed to set feedback-processor flag", "error", err)
				errors = append(errors, "Failed to update feedback-processor: "+err.Error())
			} else {
				app.logger.Info("updated feedback-processor flag", "paused", newFeedbackPaused, "start_hour", feedbackStartHour, "end_hour", feedbackEndHour)
			}

			if len(errors) > 0 {
				data["Error"] = strings.Join(errors, "; ")
			} else {
				data["Success"] = "Feature flags updated successfully"
				data["SearchScheduler"] = ss
				data["SubscriptionProcessor"] = sp
				data["ScrapingdogWorkers"] = sd
				data["FeedbackProcessor"] = fp
			}
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/feature-flags.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) adminUserDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// Fetch all users for the dropdown
	users, err := internal.ListUsers(ctx, app.fs, 100, 0) // Limiting to 100 users
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data["Users"] = users

	if r.Method == http.MethodGet {
		if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	if r.Method != http.MethodPost {
		app.logger.Error("adminUserDashboard: unsupported method", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Default params for hybrid search
	params := internal.DefaultHybridSearchParams()

	// Handle POST request for user selection
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
		return
	}

	selectedUserID := r.PostForm.Get("user_id")
	previousUserID := r.PostForm.Get("previous_user_id") // Hidden field to track previous selection

	// Determine if the user selection changed
	userChanged := selectedUserID != previousUserID
	app.logger.Debug("adminUserDashboard: user selection check", "userChanged", userChanged, "selectedUserID", selectedUserID, "previousUserID", previousUserID)

	if selectedUserID == "" {
		data["Error"] = "No user selected"
		if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}
	// Fetch the selected user's details
	selectedUser, found, err := internal.GetUser(ctx, app.fs, selectedUserID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get user: %w", err))
		return
	}
	if !found {
		data["Error"] = "User not found"
		if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	data["SelectedUser"] = selectedUser
	data["SelectedUserID"] = selectedUserID

	steeringQuery := selectedUser.SteeringQuery
	if steeringQuery == nil {
		data["Error"] = "User does not have a steering query set"
		if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render page: %w", err))
		}
		return
	}

	// Handle steering query and hybrid search parameters
	// If a user is selected, get their steering query and subscriptions
	// Only use user's steering query if user changed, otherwise keep form values
	if userChanged {
		steeringQuery = selectedUser.SteeringQuery
		app.logger.Debug("adminUserDashboard: using user steering query due to user change")
	}

	// Parse SteeringQuery fields from form
	steeringQueryText := strings.TrimSpace(r.PostForm.Get("steering_query"))
	rerankQueryText := strings.TrimSpace(r.PostForm.Get("rerank_query"))
	moveTowardText := strings.TrimSpace(r.PostForm.Get("move_toward"))
	moveAwayText := strings.TrimSpace(r.PostForm.Get("move_away"))

	if !userChanged {
		// Same user with form values - use form values to override
		app.logger.Debug("adminUserDashboard: using form steering query values - same user with edits")
		steeringQuery.Query = steeringQueryText
		steeringQuery.RerankQuery = rerankQueryText
		steeringQuery.MoveToward = moveTowardText
		steeringQuery.MoveAway = moveAwayText
		// Parse force values
		forceToward := steeringQuery.ForceToward
		if v := r.PostForm.Get("force_toward"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				forceToward = f
			}
		}
		forceAway := steeringQuery.ForceAway
		if v := r.PostForm.Get("force_away"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				forceAway = f
			}
		}
		// Update steering query with form values
		steeringQuery.ForceToward = forceToward
		steeringQuery.ForceAway = forceAway

		if err := steeringQuery.Validate(); err != nil {
			data["Error"] = fmt.Sprintf("Invalid steering query: %s", err.Error())
			if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
				app.serverError(w, r, fmt.Errorf("failed to render page: %w", err))
			}
			return
		}
	}

	// Parse HybridSearchParams fields from form
	if v := r.PostForm.Get("dense_weight"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			params.DenseWeight = f
		}
	}
	if v := r.PostForm.Get("sparse_weight"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			params.SparseWeight = f
		}
	}
	if v := r.PostForm.Get("score_fusion"); v != "" {
		params.ScoreFusion = v
	}
	if v := r.PostForm.Get("recency_weight"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			params.RecencyWeight = f
		}
	}
	if v := r.PostForm.Get("recency_half_life"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			params.RecencyHalfLife = f
		}
	}
	if v := r.PostForm.Get("score_threshold"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			params.ScoreThreshold = f
		}
	}

	// Get top_k from form
	topK := 10
	if topKStr := r.PostForm.Get("top_k"); topKStr != "" {
		if k, err := strconv.Atoi(topKStr); err == nil && k > 0 {
			topK = k
		}
	}

	// Get enable_reranking from form
	enableReranking := r.PostForm.Get("enable_reranking") == "on"

	// Add parameters to template data
	data["SteeringQuery"] = steeringQuery
	data["HybridParams"] = params
	data["TopK"] = topK
	data["EnableReranking"] = enableReranking

	// Get user's dashboard data similar to showDashboard
	status := internal.JobStatusNew
	if statusParam := r.PostForm.Get("status"); statusParam != "" {
		if parsedStatus, isValid := internal.ParseJobStatus(statusParam); isValid {
			status = parsedStatus
		}
	}

	showAllRatings := r.PostForm.Get("show_all") == "true"
	pageSize := 10 // Show more jobs in admin view

	// Parse current page for semantic search pagination
	currentPage := 1
	pageToken := r.PostForm.Get("page")
	if pageToken != "" {
		if parsedPage, err := strconv.Atoi(pageToken); err == nil && parsedPage > 0 {
			currentPage = parsedPage
		}
	}

	// Hybrid Approach: Use semantic search for "New" jobs, existing queries for other statuses
	jobs := map[string]JobWithDetails{}
	var semanticJobsSlice []JobWithDetails // Ordered slice for semantic search results
	var nextPageToken string

	if status != internal.JobStatusNew {
		// unsupported status, return error
		data["Error"] = fmt.Sprintf("Unsupported job status: %s", status)
		if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render page: %w", err))
			return
		}
		return
	}

	// For "New" status, use semantic search with jobs that have no UserJob record
	start := time.Now()

	// Get user's subscriptions to find criteriaID for semantic search
	subscriptions, err := internal.GetSubscriptionsForUser(ctx, app.fs, selectedUser.ID)
	if err != nil {
		wrappedErr := fmt.Errorf("adminUserDashboard: failed to get user subscriptions: %w", err)
		app.logger.Error("adminUserDashboard: failed to get user subscriptions", "error", err, "user_id", selectedUserID)
		internal.SentryReportError(ctx, wrappedErr, sentry.LevelError, map[string]string{
			"error_type":  "database_retrieval",
			"entity_type": "user_subscriptions",
			"user_id":     selectedUserID,
		})
		app.serverError(w, r, wrappedErr)
		return
	}

	if len(subscriptions) == 0 {
		app.logger.Info("adminUserDashboard: user has no subscriptions, showing empty new jobs", "user_id", selectedUserID)
		data["Error"] = "No subscriptions found for user, cannot perform semantic search"
		if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render page: %w", err))
			return
		}
		return
	}

	// Use the first subscription's criteria for semantic search
	criteriaID := subscriptions[0].CriteriaID

	// Use custom topK from form instead of hardcoded value
	semanticSearchTopK := topK * 10 // Use larger search to get good results for filtering

	var allFilteredJobs []internal.ScoredJobDetail

	if err := steeringQuery.Validate(); err != nil {
		wrappedErr := fmt.Errorf("adminUserDashboard: invalid steering query: %w", err)
		app.logger.Error("adminUserDashboard: invalid steering query", "error", err, "user_id", selectedUserID)
		if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
			app.serverError(w, r, wrappedErr)
			return
		}
		return
	}

	allFilteredJobs, err = internal.GetSemanticJobsForUser(ctx, app.fs, selectedUser.ID, selectedUser.SteeringQuery, criteriaID, semanticSearchTopK, params)
	if err != nil {
		wrappedErr := fmt.Errorf("adminUserDashboard: failed to get semantic jobs: %w", err)
		app.logger.Error("adminUserDashboard: failed to get semantic jobs", "error", err, "user_id", selectedUserID)
		app.serverError(w, r, wrappedErr)
		return
	}

	// Calculate memory usage for debug logging
	memoryUsage := calculateSemanticJobsMemory(allFilteredJobs)
	app.logger.Debug("adminUserDashboard: semantic search memory usage",
		"total_jobs", len(allFilteredJobs),
		"memory_bytes", memoryUsage,
		"memory_mb", memoryUsage/(1024*1024),
		"user_id", selectedUserID)

	// Implement pagination through the filtered results
	startIdx := (currentPage - 1) * pageSize
	endIdx := startIdx + pageSize

	if startIdx < len(allFilteredJobs) {
		if endIdx > len(allFilteredJobs) {
			endIdx = len(allFilteredJobs)
		}

		// Convert the paginated slice to JobWithDetails
		for _, scoredJob := range allFilteredJobs[startIdx:endIdx] {
			semanticJobsSlice = append(semanticJobsSlice, JobWithDetails{
				ScoredJobDetail: scoredJob,
				IsSemanticOnly:  true,
			})
		}
	}

	// Update statusJobCount to reflect actual filtered semantic results
	statusJobCount := int64(len(allFilteredJobs))

	app.logger.Debug("adminUserDashboard: semantic search completed",
		"duration", time.Since(start).String(),
		"total_semantic_jobs", len(allFilteredJobs),
		"filtered_jobs", len(allFilteredJobs),
		"page_jobs", len(semanticJobsSlice),
		"current_page", currentPage,
		"user_id", selectedUserID)

	// Add dashboard data to template
	data["HasJobs"] = len(jobs) > 0 || statusJobCount > 0
	data["ActiveStatus"] = string(status)
	data["NextPageToken"] = nextPageToken
	data["HasNextPage"] = nextPageToken != ""
	data["TotalJobsForStatus"] = statusJobCount
	data["JobStatuses"] = internal.GetJobStatuses()
	data["StatusDisplayNames"] = map[string]string{
		"new":      "New Jobs",
		"saved":    "Saved Jobs",
		"applied":  "Applied Jobs",
		"archived": "Archived Jobs",
	}
	data["ShowAllRatings"] = showAllRatings
	data["TestMode"] = true // Enable test mode features in dashboard view

	// Set jobs data based on status type (matching main dashboard approach)
	data["HasJobs"] = len(semanticJobsSlice) > 0 || statusJobCount > 0
	data["Jobs"] = semanticJobsSlice

	if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
		app.serverError(w, r, err)
	}
}
