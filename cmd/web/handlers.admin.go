package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
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
			overrides, err := internal.BuildAssistantOverrides(ctx, nil, app.fs, app.logger)
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

			// Build assistant overrides for the selected user
			overrides, err := internal.BuildAssistantOverrides(ctx, selectedUser, app.fs, app.logger)
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

	// Set common template data
	data["JobStatuses"] = internal.GetJobStatuses()
	data["StatusDisplayNames"] = map[string]string{
		"new":      "New Jobs",
		"saved":    "Saved Jobs",
		"applied":  "Applied Jobs",
		"archived": "Archived Jobs",
	}
	data["ActiveStatus"] = string(internal.JobStatusNew)

	// Fetch all users for the dropdown
	users, err := internal.ListUsers(ctx, app.fs, 100, 0)
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

	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
		return
	}

	status := internal.JobStatusNew
	if statusParam := r.PostForm.Get("status"); statusParam != "" {
		if parsedStatus, isValid := internal.ParseJobStatus(statusParam); isValid {
			status = parsedStatus
		}
	}
	data["ActiveStatus"] = string(status)

	selectedUserID := r.PostForm.Get("user_id")
	if selectedUserID == "" {
		data["Error"] = "No user selected"
		if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}

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

	// --- Subscription Management Logic ---
	subscriptionError := ""
	subscriptionSuccess := ""

	// Handle unsubscribe action
	if unsubID := r.PostForm.Get("unsubscribe_criteria_id"); unsubID != "" {
		err := internal.UnsubscribeFromCriteria(ctx, app.fs, selectedUserID, unsubID)
		if err != nil {
			subscriptionError = fmt.Sprintf("Failed to unsubscribe: %v", err)
		} else {
			subscriptionSuccess = "Unsubscribed successfully."
		}
	}

	// Handle subscribe action
	if subID := r.PostForm.Get("subscribe_criteria_id"); subID != "" {
		err := internal.SubscribeToCriteria(ctx, app.fs, selectedUserID, subID)
		if err != nil {
			subscriptionError = fmt.Sprintf("Failed to subscribe: %v", err)
		} else {
			subscriptionSuccess = "Subscribed successfully."
		}
	}

	// Fetch subscriptions and all criteria
	userSubscriptions, _ := internal.GetSubscriptionsForUser(ctx, app.fs, selectedUserID)
	allCriteria, _ := internal.ListSearchCriteria(ctx, app.fs)

	// Build set of subscribed criteria IDs
	subscribedIDs := make(map[string]struct{})
	for _, sub := range userSubscriptions {
		subscribedIDs[sub.CriteriaID] = struct{}{}
	}
	// Build list of criteria the user is NOT subscribed to
	var availableCriteria []internal.SearchCriteria
	for _, crit := range allCriteria {
		if _, ok := subscribedIDs[crit.ID]; !ok {
			availableCriteria = append(availableCriteria, crit)
		}
	}

	// Build a map for fast lookup in the template
	criteriaMap := make(map[string]internal.SearchCriteria)
	for _, crit := range allCriteria {
		criteriaMap[crit.ID] = crit
	}

	data["UserSubscriptions"] = userSubscriptions
	data["AvailableCriteria"] = availableCriteria
	data["AllCriteria"] = allCriteria
	data["CriteriaMap"] = criteriaMap
	if subscriptionError != "" {
		data["SubscriptionError"] = subscriptionError
	}
	if subscriptionSuccess != "" {
		data["SubscriptionSuccess"] = subscriptionSuccess
	}

	// Pagination
	pageToken := r.PostForm.Get("page")
	pageSize := 10
	currentPage := 1
	if pageToken != "" {
		if parsedPage, err := strconv.Atoi(pageToken); err == nil && parsedPage > 0 {
			currentPage = parsedPage
		}
	}

	// Fetch UserJobs for the selected user and status
	jobs := map[string]JobWithDetails{}
	nextPageToken := ""
	statusJobCount := int64(0)

	userJobs, nextPage, err := internal.GetUserJobsSorted(ctx, app.fs, selectedUserID, status, pageSize, pageToken)
	if err != nil {
		wrappedErr := fmt.Errorf("adminUserDashboard: failed to get sorted user jobs: %w", err)
		app.logger.Error("adminUserDashboard: failed to get sorted user jobs", "error", err)
		if strings.Contains(err.Error(), "failed to iterate user jobs") &&
			(strings.Contains(err.Error(), "The query requires an index") ||
				strings.Contains(err.Error(), "FailedPrecondition")) {
			data["FirestoreError"] = true
			data["HasJobs"] = false
			data["Jobs"] = nil
		} else {
			app.serverError(w, r, wrappedErr)
		}
		return
	}
	app.logger.Debug("adminUserDashboard: GetUserJobsSorted took", "count", len(userJobs))

	for _, job := range userJobs {
		jobs[job.JobRef.ID] = JobWithDetails{
			UserJob: job,
		}
	}

	// Fetch JobDetails for these jobs
	jobIDs := make([]string, 0, len(jobs))
	for id := range jobs {
		jobIDs = append(jobIDs, id)
	}
	if len(jobIDs) > 0 {
		jobDetails, err := internal.GetJobDetailsByIDs(ctx, app.fs, jobIDs)
		if err != nil {
			wrappedErr := fmt.Errorf("adminUserDashboard: failed to get job details by IDs: %w", err)
			app.logger.Error("adminUserDashboard: failed to get job details by IDs", "error", err)
			app.serverError(w, r, wrappedErr)
			return
		}
		for _, detail := range jobDetails {
			if job, exists := jobs[detail.ID]; exists {
				job.JobDetail = &detail
				jobs[detail.ID] = job
			}
		}
	}

	// Get the total count for the current status for pagination calculations
	statusJobCount, _ = internal.GetJobsCount(ctx, app.fs, selectedUserID, status)
	totalPages := (int(statusJobCount) + pageSize - 1) / pageSize
	if nextPage != "" {
		nextPageToken = nextPage
	}

	// Prepare jobs slice for template
	jobsSlice := make([]JobWithDetails, 0, len(jobs))
	for _, job := range jobs {
		jobsSlice = append(jobsSlice, job)
	}
	// Resort jobsSlice by UserJob.Rating descending
	sort.Slice(jobsSlice, func(i, j int) bool {
		if jobsSlice[i].UserJob != nil && jobsSlice[j].UserJob != nil {
			return jobsSlice[i].UserJob.Rating > jobsSlice[j].UserJob.Rating
		}
		return false
	})
	data["HasJobs"] = len(jobs) > 0 || statusJobCount > 0
	data["Jobs"] = jobsSlice
	data["NextPageToken"] = nextPageToken
	data["HasNextPage"] = nextPageToken != ""
	data["TotalJobsForStatus"] = statusJobCount
	data["CurrentPage"] = currentPage
	data["TotalPages"] = totalPages
	data["TestMode"] = true

	if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
		app.serverError(w, r, err)
	}
}

// --- Subscription Management Tool ---
func (app *application) adminSubscriptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// Fetch all users for the dropdown
	users, err := internal.ListUsers(ctx, app.fs, 100, 0)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data["Users"] = users

	selectedUserID := r.FormValue("user_id")
	if selectedUserID != "" {
		selectedUser, found, err := internal.GetUser(ctx, app.fs, selectedUserID)
		if err != nil {
			app.serverError(w, r, fmt.Errorf("failed to get user: %w", err))
			return
		}
		if !found {
			data["Error"] = "User not found"
			if err := response.Page(w, http.StatusOK, data, "pages/admin/subscriptions.html"); err != nil {
				app.serverError(w, r, err)
			}
			return
		}
		data["SelectedUser"] = selectedUser
		data["SelectedUserID"] = selectedUserID

		subscriptionError := ""
		subscriptionSuccess := ""

		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
				return
			}
			// Handle unsubscribe action
			if unsubID := r.PostForm.Get("unsubscribe_criteria_id"); unsubID != "" {
				err := internal.UnsubscribeFromCriteria(ctx, app.fs, selectedUserID, unsubID)
				if err != nil {
					subscriptionError = fmt.Sprintf("Failed to unsubscribe: %v", err)
				} else {
					subscriptionSuccess = "Unsubscribed successfully."
				}
			}
			// Handle subscribe action
			if subID := r.PostForm.Get("subscribe_criteria_id"); subID != "" {
				err := internal.SubscribeToCriteria(ctx, app.fs, selectedUserID, subID)
				if err != nil {
					subscriptionError = fmt.Sprintf("Failed to subscribe: %v", err)
				} else {
					subscriptionSuccess = "Subscribed successfully."
				}
			}
		}

		// Fetch subscriptions and all criteria
		userSubscriptions, _ := internal.GetSubscriptionsForUser(ctx, app.fs, selectedUserID)
		allCriteria, _ := internal.ListSearchCriteria(ctx, app.fs)

		// Build set of subscribed criteria IDs
		subscribedIDs := make(map[string]struct{})
		for _, sub := range userSubscriptions {
			subscribedIDs[sub.CriteriaID] = struct{}{}
		}
		// Build list of criteria the user is NOT subscribed to
		var availableCriteria []internal.SearchCriteria
		for _, crit := range allCriteria {
			if _, ok := subscribedIDs[crit.ID]; !ok {
				availableCriteria = append(availableCriteria, crit)
			}
		}

		// Build a map for fast lookup in the template
		criteriaMap := make(map[string]internal.SearchCriteria)
		for _, crit := range allCriteria {
			criteriaMap[crit.ID] = crit
		}

		data["UserSubscriptions"] = userSubscriptions
		data["AvailableCriteria"] = availableCriteria
		data["AllCriteria"] = allCriteria
		data["CriteriaMap"] = criteriaMap
		if subscriptionError != "" {
			data["SubscriptionError"] = subscriptionError
		}
		if subscriptionSuccess != "" {
			data["SubscriptionSuccess"] = subscriptionSuccess
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/subscriptions.html"); err != nil {
		app.serverError(w, r, err)
	}
}
