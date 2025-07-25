package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/cloudflare"
	"talent-rodeo/internal/feedback"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/nudge"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/search"
	"talent-rodeo/internal/search/providers/scrapingdog"
	"talent-rodeo/internal/subscription"
	"time"

	"github.com/alexedwards/flow"
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
	var np nudge.NudgeProcessorSettings = nudge.DefaultNudgeSettings

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

	// Read nudge processor flag
	if err := kv.Get(nudge.NudgeProcessorKey, &np); err != nil {
		if err != cloudflare.ErrNotFound {
			app.logger.Error("failed to get nudge-processor flag", "error", err)
		}
		np = nudge.DefaultNudgeSettings
	}

	data["SearchScheduler"] = ss
	data["SubscriptionProcessor"] = sp
	data["ScrapingdogWorkers"] = sd
	data["FeedbackProcessor"] = fp
	data["NudgeProcessor"] = np

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.logger.Error("failed to parse form", "error", err)
			data["Error"] = "Failed to parse form: " + err.Error()
		} else {
			newSearchPaused := r.PostForm.Get("search-scheduler-paused") == "true"
			newSubscriptionPaused := r.PostForm.Get("subscription-processor-paused") == "true"
			newScrapingdogPaused := r.PostForm.Get("scrapingdog-paused") == "true"
			newFeedbackPaused := r.PostForm.Get("feedback-processor-paused") == "true"
			newNudgePaused := r.PostForm.Get("nudge-processor-paused") == "true"
			newNudgeDryRun := r.PostForm.Get("nudge-processor-dry-run") == "true"
			newNudgeTestMode := r.PostForm.Get("nudge-processor-test-mode") == "true"
			newNudgeStartHour, _ := strconv.Atoi(r.PostForm.Get("nudge-processor-start-hour"))
			newNudgeEndHour, _ := strconv.Atoi(r.PostForm.Get("nudge-processor-end-hour"))
			newNudgeIntervalMinutes, _ := strconv.Atoi(r.PostForm.Get("nudge-processor-interval-minutes"))
			newNudgeMaxEmails, _ := strconv.Atoi(r.PostForm.Get("nudge-processor-max-emails"))
			newNudgeNoResumeDays, _ := strconv.Atoi(r.PostForm.Get("nudge-processor-no-resume-days"))
			newNudgeNoCallDays, _ := strconv.Atoi(r.PostForm.Get("nudge-processor-no-call-days"))
			newNudgeMatchAlertDays, _ := strconv.Atoi(r.PostForm.Get("nudge-processor-match-alert-days"))
			newNudgeMaxNoResume, _ := strconv.Atoi(r.PostForm.Get("nudge-max-no-resume"))
			newNudgeMaxNoCall, _ := strconv.Atoi(r.PostForm.Get("nudge-max-no-call"))
			newNudgeMaxFiveStar, _ := strconv.Atoi(r.PostForm.Get("nudge-max-five-star"))
			newNudgeMaxFollowUp, _ := strconv.Atoi(r.PostForm.Get("nudge-max-follow-up"))
			newNudgeWeeklyRateLimitDays, _ := strconv.Atoi(r.PostForm.Get("nudge-weekly-rate-limit-days"))
			limitActivityNoResume := r.PostForm.Get("nudge-limit-activity-no-resume") == "true"
			limitActivityNoCall := r.PostForm.Get("nudge-limit-activity-no-call") == "true"
			limitActivityFiveStar := r.PostForm.Get("nudge-limit-activity-five-star") == "true"
			limitActivityFollowUp := r.PostForm.Get("nudge-limit-activity-follow-up") == "true"

			searchStartHour, _ := strconv.Atoi(r.PostForm.Get("search-scheduler-start-hour"))
			searchEndHour, _ := strconv.Atoi(r.PostForm.Get("search-scheduler-end-hour"))
			searchIntervalMinutes, _ := strconv.Atoi(r.PostForm.Get("search-scheduler-interval-minutes"))
			subscriptionStartHour, _ := strconv.Atoi(r.PostForm.Get("subscription-processor-start-hour"))
			subscriptionEndHour, _ := strconv.Atoi(r.PostForm.Get("subscription-processor-end-hour"))
			subscriptionIntervalMinutes, _ := strconv.Atoi(r.PostForm.Get("subscription-processor-interval-minutes"))
			feedbackStartHour, _ := strconv.Atoi(r.PostForm.Get("feedback-processor-start-hour"))
			feedbackEndHour, _ := strconv.Atoi(r.PostForm.Get("feedback-processor-end-hour"))
			feedbackIntervalMinutes, _ := strconv.Atoi(r.PostForm.Get("feedback-processor-interval-minutes"))

			var errors []string

			ss.Paused = newSearchPaused
			ss.StartHour = searchStartHour
			ss.EndHour = searchEndHour
			if searchIntervalMinutes > 0 {
				ss.Interval = time.Duration(searchIntervalMinutes) * time.Minute
			}
			if err := kv.Set("search-scheduler", ss, 0); err != nil {
				app.logger.Error("failed to set search-scheduler settings", "error", err)
				errors = append(errors, "Failed to update search-scheduler: "+err.Error())
			} else {
				app.logger.Info("updated search-scheduler settings", "paused", newSearchPaused, "start_hour", searchStartHour, "end_hour", searchEndHour, "interval_minutes", searchIntervalMinutes)
			}

			sp.Paused = newSubscriptionPaused
			sp.StartHour = subscriptionStartHour
			sp.EndHour = subscriptionEndHour
			if subscriptionIntervalMinutes > 0 {
				sp.Interval = time.Duration(subscriptionIntervalMinutes) * time.Minute
			}
			if err := kv.Set("subscription-processor", sp, 0); err != nil {
				app.logger.Error("failed to set subscription-processor settings", "error", err)
				errors = append(errors, "Failed to update subscription-processor: "+err.Error())
			} else {
				app.logger.Info("updated subscription-processor settings", "paused", newSubscriptionPaused, "start_hour", subscriptionStartHour, "end_hour", subscriptionEndHour, "interval_minutes", subscriptionIntervalMinutes)
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
			if feedbackIntervalMinutes > 0 {
				fp.Interval = time.Duration(feedbackIntervalMinutes) * time.Minute
			}
			if err := kv.Set("feedback-processor", fp, 0); err != nil {
				app.logger.Error("failed to set feedback-processor flag", "error", err)
				errors = append(errors, "Failed to update feedback-processor: "+err.Error())
			} else {
				app.logger.Info("updated feedback-processor flag", "paused", newFeedbackPaused, "start_hour", feedbackStartHour, "end_hour", feedbackEndHour, "interval_minutes", feedbackIntervalMinutes)
			}

			np.Paused = newNudgePaused
			np.DryRun = newNudgeDryRun
			np.TestModeEnabled = newNudgeTestMode
			np.StartHour = newNudgeStartHour
			np.EndHour = newNudgeEndHour
			if newNudgeIntervalMinutes > 0 {
				np.Interval = time.Duration(newNudgeIntervalMinutes) * time.Minute
			}
			np.MaxEmailsPerRun = newNudgeMaxEmails
			np.NoResumeDelayDays = newNudgeNoResumeDays
			np.NoCallDelayDays = newNudgeNoCallDays
			np.MatchAlertDelayDays = newNudgeMatchAlertDays
			np.MaxNudgesPerType.NoResume = newNudgeMaxNoResume
			np.MaxNudgesPerType.NoCall = newNudgeMaxNoCall
			np.MaxNudgesPerType.FiveStarMatches = newNudgeMaxFiveStar
			np.MaxNudgesPerType.FollowUp = newNudgeMaxFollowUp
			if newNudgeWeeklyRateLimitDays > 0 {
				np.WeeklyRateLimit = time.Duration(newNudgeWeeklyRateLimitDays) * 24 * time.Hour
			}
			np.LimitOnRecentActivityPerType = nudge.LimitOnRecentActivitySettings{
				NoResume:        limitActivityNoResume,
				NoCall:          limitActivityNoCall,
				FiveStarMatches: limitActivityFiveStar,
				FollowUp:        limitActivityFollowUp,
			}

			if err := kv.Set(nudge.NudgeProcessorKey, np, 0); err != nil {
				app.logger.Error("failed to set nudge-processor settings", "error", err)
				errors = append(errors, "Failed to update nudge-processor: "+err.Error())
			} else {
				app.logger.Info("updated nudge-processor settings")
			}

			if len(errors) > 0 {
				data["Error"] = strings.Join(errors, "; ")
			} else {
				data["Success"] = "Feature flags updated successfully"
				data["SearchScheduler"] = ss
				data["SubscriptionProcessor"] = sp
				data["ScrapingdogWorkers"] = sd
				data["FeedbackProcessor"] = fp
				data["NudgeProcessor"] = np
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
	allUsers, err := internal.ListUsers(ctx, app.fs, 10000, 0)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Fetch all subscriptions to determine which users have subscriptions
	allSubscriptions, err := internal.ListSubscriptions(ctx, app.fs)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Build map of users with subscriptions
	userSubscriptionCounts := make(map[string]int)
	for _, subscription := range allSubscriptions {
		userSubscriptionCounts[subscription.UserID]++
	}

	// Group users according to the requirements
	now := time.Now()
	var activeUsers []*internal.User  // Group 1: Active subscriptions or future SubscriptionEnd
	var expiredUsers []*internal.User // Group 2: Subscriptions but expired SubscriptionEnd
	var otherUsers []*internal.User   // Group 3: All other users

	for _, user := range allUsers {
		hasSubscription := userSubscriptionCounts[user.ID] > 0
		hasActiveSubscriptionEnd := user.SubscriptionEnd != nil && user.SubscriptionEnd.After(now)

		if hasSubscription || hasActiveSubscriptionEnd {
			activeUsers = append(activeUsers, user)
		} else if user.SubscriptionEnd != nil && user.SubscriptionEnd.Before(now) {
			expiredUsers = append(expiredUsers, user)
		} else {
			otherUsers = append(otherUsers, user)
		}
	}

	data["Users"] = allUsers // Keep for backward compatibility
	data["ActiveUsers"] = activeUsers
	data["ExpiredUsers"] = expiredUsers
	data["OtherUsers"] = otherUsers
	data["UserSubscriptionCounts"] = userSubscriptionCounts

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

	// Add daily job evaluation statistics for the selected user
	data["LastJobEvaluationRun"] = selectedUser.LastJobEvaluationRun
	data["LastEvaluationDate"] = selectedUser.LastEvaluationDate
	data["TotalJobsEvaluated"] = selectedUser.TotalJobsEvaluated
	data["NewJobsFound"] = selectedUser.NewJobsFound
	data["RejectedJobsCount"] = selectedUser.RejectedJobsCount
	data["QualityJobsFound"] = selectedUser.QualityJobsFound

	// Get all user jobs and aggregate rating statistics
	allUserJobs, err := internal.ListUserJobs(ctx, app.fs, selectedUserID)
	if err != nil {
		app.logger.Error("failed to list user jobs for rating stats", "user_id", selectedUserID, "error", err)
		// Continue with empty stats rather than failing the whole page
		allUserJobs = []internal.UserJob{}
	}

	// Aggregate rating counts (0-5)
	ratingCounts := make(map[int]int)
	for rating := 0; rating <= 5; rating++ {
		ratingCounts[rating] = 0
	}

	for _, job := range allUserJobs {
		if job.Rating >= 0 && job.Rating <= 5 {
			ratingCounts[job.Rating]++
		}
	}

	data["RatingCounts"] = ratingCounts
	data["TotalJobs"] = len(allUserJobs)

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
			UserJob: *job,
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
				job.JobDetail = detail
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
		return jobsSlice[i].UserJob.Rating > jobsSlice[j].UserJob.Rating
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

	startTime := time.Now()
	app.logger.Info("adminSubscriptions: Starting request processing")

	// Fetch all users for stats and dropdown
	usersStart := time.Now()
	allUsers, err := internal.ListUsers(ctx, app.fs, 10000, 0) // Using a high limit to approximate all users
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to list users for stats: %w", err))
		return
	}
	app.logger.Info("adminSubscriptions: Fetched all users", "count", len(allUsers), "duration", time.Since(usersStart))
	data["Users"] = allUsers // for the dropdown

	// Fetch all criteria
	criteriaStart := time.Now()
	allCriteria, err := internal.ListSearchCriteria(ctx, app.fs)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to list search criteria for stats: %w", err))
		return
	}
	app.logger.Info("adminSubscriptions: Fetched all criteria", "count", len(allCriteria), "duration", time.Since(criteriaStart))

	// Build subscription counts using optimized single query approach
	subscriptionCountStart := time.Now()

	allSubscriptions, err := internal.ListSubscriptions(ctx, app.fs)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get all subscriptions: %w", err))
		return
	}

	app.logger.Info("adminSubscriptions: Fetched all subscriptions", "count", len(allSubscriptions), "duration", time.Since(subscriptionCountStart))

	userSubscriptionCounts := make(map[string]int)
	subscribedUserIDs := make(map[string]struct{})

	for _, subscription := range allSubscriptions {
		userSubscriptionCounts[subscription.UserID]++
		subscribedUserIDs[subscription.UserID] = struct{}{}
	}

	subscriptionCountDuration := time.Since(subscriptionCountStart)
	app.logger.Info("adminSubscriptions: Completed subscription count calculation (optimized)",
		"total_subscriptions", len(allSubscriptions),
		"unique_subscribed_users", len(subscribedUserIDs),
		"duration", subscriptionCountDuration)

	data["UserSubscriptionCounts"] = userSubscriptionCounts

	// Group users according to the requirements (same as adminUserDashboard)
	now := time.Now()
	var activeUsers []*internal.User  // Group 1: Active subscriptions or future SubscriptionEnd
	var expiredUsers []*internal.User // Group 2: Subscriptions but expired SubscriptionEnd
	var otherUsers []*internal.User   // Group 3: All other users

	for _, user := range allUsers {
		hasSubscription := userSubscriptionCounts[user.ID] > 0
		hasActiveSubscriptionEnd := user.SubscriptionEnd != nil && user.SubscriptionEnd.After(now)

		if hasSubscription || hasActiveSubscriptionEnd {
			activeUsers = append(activeUsers, user)
		} else if user.SubscriptionEnd != nil && user.SubscriptionEnd.Before(now) {
			expiredUsers = append(expiredUsers, user)
		} else {
			otherUsers = append(otherUsers, user)
		}
	}

	// Keep backward compatibility data
	data["UsersWithSubscriptions"] = len(subscribedUserIDs)
	data["UsersWithoutSubscriptions"] = len(allUsers) - len(subscribedUserIDs)
	data["SortedUsersWithSubs"] = activeUsers                            // Update to use new grouping
	data["SortedUsersWithoutSubs"] = append(expiredUsers, otherUsers...) // Combine expired and other

	// Add new grouped data
	data["ActiveUsers"] = activeUsers
	data["ExpiredUsers"] = expiredUsers
	data["OtherUsers"] = otherUsers

	selectedUserID := r.FormValue("user_id")
	if selectedUserID != "" {
		var selectedUser *internal.User
		for _, u := range allUsers {
			if u.ID == selectedUserID {
				selectedUser = u
				break
			}
		}

		if selectedUser == nil {
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
			// Handle extend subscription action
			if extendDaysStr := r.PostForm.Get("extend_days"); extendDaysStr != "" {
				extendDays, err := strconv.Atoi(extendDaysStr)
				if err != nil || extendDays < 0 {
					subscriptionError = "Invalid number of days to extend"
				} else {
					// Get the current user to check their subscription end date
					user, found, err := internal.GetUser(ctx, app.fs, selectedUserID)
					if err != nil {
						subscriptionError = fmt.Sprintf("Failed to get user: %v", err)
					} else if !found {
						subscriptionError = "User not found"
					} else {
						// Calculate new subscription end date
						var newSubscriptionEnd time.Time
						now := time.Now()

						if user.SubscriptionEnd != nil && user.SubscriptionEnd.After(now) {
							// If subscription is still active, extend from current end date
							newSubscriptionEnd = user.SubscriptionEnd.Add(time.Duration(extendDays) * 24 * time.Hour)
						} else {
							// If subscription is expired or not set, extend from today
							newSubscriptionEnd = now.Add(time.Duration(extendDays) * 24 * time.Hour)
						}

						// Handle tier change if provided
						updateFields := map[string]any{
							"SubscriptionEnd": newSubscriptionEnd,
						}

						if tierStr := r.PostForm.Get("tier"); tierStr != "" {
							tier := internal.UserTier(tierStr)
							if tier == internal.UserTierFree || tier == internal.UserTierPro {
								updateFields["Tier"] = tier
							}
						}

						err = internal.UpdateUser(ctx, app.fs, selectedUserID, updateFields)
						if err != nil {
							subscriptionError = fmt.Sprintf("Failed to extend subscription: %v", err)
						} else {
							successMsg := fmt.Sprintf("Extended subscription by %d days to %s",
								extendDays, newSubscriptionEnd.Format("2006-01-02 15:04:05"))
							if tierStr := r.PostForm.Get("tier"); tierStr != "" {
								successMsg += fmt.Sprintf(" and updated tier to %s", tierStr)
							}
							subscriptionSuccess = successMsg
						}
					}
				}
			}

			if refreshID := r.PostForm.Get("refresh_criteria_id"); refreshID != "" {
				criteria, found, err := internal.GetSearchCriteria(ctx, app.fs, refreshID)
				if err != nil {
					subscriptionError = fmt.Sprintf("Failed to get search criteria: %v", err)
				} else if !found {
					subscriptionError = "Search criteria not found."
				} else {
					app.backgroundTask(r, func() error {
						return subscription.RefreshUserJobs(context.Background(), app.fs, nil, app.logger, *selectedUser, *criteria)
					})
				}
			}
		}

		// Fetch subscriptions and all criteria
		userSubscriptionsStart := time.Now()
		userSubscriptions, _ := internal.GetSubscriptionsForUser(ctx, app.fs, selectedUserID)
		app.logger.Info("adminSubscriptions: Fetched user subscriptions",
			"user_id", selectedUserID,
			"subscription_count", len(userSubscriptions),
			"duration", time.Since(userSubscriptionsStart))
		// We already have allCriteria

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

	renderStart := time.Now()
	if err := response.Page(w, http.StatusOK, data, "pages/admin/subscriptions.html"); err != nil {
		app.serverError(w, r, err)
	}
	app.logger.Info("adminSubscriptions: Completed request",
		"total_duration", time.Since(startTime),
		"render_duration", time.Since(renderStart))
}

// --- Admin Upload Resume for User ---
func (app *application) adminUploadResume(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// Fetch all users for the dropdown
	allUsers, err := internal.ListUsers(ctx, app.fs, 10000, 0)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Fetch all subscriptions to determine which users have subscriptions
	allSubscriptions, err := internal.ListSubscriptions(ctx, app.fs)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Build map of users with subscriptions
	userSubscriptionCounts := make(map[string]int)
	for _, subscription := range allSubscriptions {
		userSubscriptionCounts[subscription.UserID]++
	}

	// Group users according to the requirements
	now := time.Now()
	var activeUsers []*internal.User  // Group 1: Active subscriptions or future SubscriptionEnd
	var expiredUsers []*internal.User // Group 2: Subscriptions but expired SubscriptionEnd
	var otherUsers []*internal.User   // Group 3: All other users

	for _, user := range allUsers {
		hasSubscription := userSubscriptionCounts[user.ID] > 0
		hasActiveSubscriptionEnd := user.SubscriptionEnd != nil && user.SubscriptionEnd.After(now)

		if hasSubscription || hasActiveSubscriptionEnd {
			activeUsers = append(activeUsers, user)
		} else if user.SubscriptionEnd != nil && user.SubscriptionEnd.Before(now) {
			expiredUsers = append(expiredUsers, user)
		} else {
			otherUsers = append(otherUsers, user)
		}
	}

	data["Users"] = allUsers // Keep for backward compatibility
	data["ActiveUsers"] = activeUsers
	data["ExpiredUsers"] = expiredUsers
	data["OtherUsers"] = otherUsers
	data["UserSubscriptionCounts"] = userSubscriptionCounts

	if r.Method == http.MethodGet {
		if err := response.Page(w, http.StatusOK, data, "pages/admin/upload-resume.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	// POST: handle upload
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		data["Error"] = "File too large. Maximum size is 10MB."
		response.Page(w, http.StatusUnprocessableEntity, data, "pages/admin/upload-resume.html")
		return
	}

	selectedUserID := r.FormValue("user_id")
	phone := r.FormValue("phone")
	data["SelectedUserID"] = selectedUserID
	data["Phone"] = phone

	if selectedUserID == "" {
		data["Error"] = "Please select a user."
		response.Page(w, http.StatusUnprocessableEntity, data, "pages/admin/upload-resume.html")
		return
	}
	selectedUser, found, err := internal.GetUser(ctx, app.fs, selectedUserID)
	if err != nil || !found {
		data["Error"] = "User not found."
		response.Page(w, http.StatusUnprocessableEntity, data, "pages/admin/upload-resume.html")
		return
	}

	file, header, err := r.FormFile("resume")
	if err != nil {
		data["Error"] = "Please select a file."
		response.Page(w, http.StatusUnprocessableEntity, data, "pages/admin/upload-resume.html")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".pdf" && ext != ".docx" && ext != ".txt" {
		data["Error"] = "Invalid file type. Please upload a DOCX, PDF, or TXT file."
		response.Page(w, http.StatusUnprocessableEntity, data, "pages/admin/upload-resume.html")
		return
	}

	if phone != "" {
		phone = strings.TrimSpace(phone)
		if !strings.HasPrefix(phone, "+1") && !strings.HasPrefix(phone, "1") {
			phone = "+1" + phone
		}
		phoneRegex := regexp.MustCompile(`^\+1\d{10}$`)
		if !phoneRegex.MatchString(phone) {
			data["Error"] = "Invalid phone number. Please use the format +1XXXXXXXXXX."
			response.Page(w, http.StatusUnprocessableEntity, data, "pages/admin/upload-resume.html")
			return
		}
	}

	tempFile, err := os.CreateTemp("", "resume-*"+ext)
	if err != nil {
		data["Error"] = "Failed to process file. Please try again."
		response.Page(w, http.StatusUnprocessableEntity, data, "pages/admin/upload-resume.html")
		return
	}
	defer os.Remove(tempFile.Name())

	_, err = io.Copy(tempFile, file)
	if err != nil {
		data["Error"] = "Failed to process file. Please try again."
		response.Page(w, http.StatusUnprocessableEntity, data, "pages/admin/upload-resume.html")
		return
	}

	resumeText, err := app.extractResumeText(tempFile, header.Filename)
	if err != nil {
		data["Error"] = fmt.Sprintf("Failed to extract text from resume: %v", err)
		response.Page(w, http.StatusUnprocessableEntity, data, "pages/admin/upload-resume.html")
		return
	}

	resume := internal.Resume{
		RawText: resumeText,
	}
	if err := internal.CreateResume(ctx, app.fs, *selectedUser, &resume); err != nil {
		data["Error"] = "Failed to save resume. Please try again."
		response.Page(w, http.StatusUnprocessableEntity, data, "pages/admin/upload-resume.html")
		return
	}

	if phone != "" {
		selectedUser.Phone = &phone
		if err := internal.UpdateUser(ctx, app.fs, selectedUser.ID, map[string]any{
			"Phone": phone,
		}); err != nil {
			data["Error"] = "Failed to update user phone."
			response.Page(w, http.StatusUnprocessableEntity, data, "pages/admin/upload-resume.html")
			return
		}
	}

	// Start resume processing in background, but do not show waiting screen
	go func() {
		_ = internal.AnnotateResume(context.Background(), app.fs, app.logger, selectedUser, &resume)
	}()

	data["Success"] = "Resume uploaded and processing started for user."
	if err := response.Page(w, http.StatusOK, data, "pages/admin/upload-resume.html"); err != nil {
		app.serverError(w, r, err)
	}
}

// --- Admin Search Criteria Management ---
func (app *application) adminSearchCriteria(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// POST handling comes first
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
			return
		}

		action := r.PostForm.Get("action")
		id := r.PostForm.Get("id")

		switch action {
		case "create":
			maxPages, _ := strconv.Atoi(r.PostForm.Get("maxPages"))
			query := r.PostForm.Get("query")
			geoID := r.PostForm.Get("geoId")
			workType := internal.WorkType(r.PostForm.Get("workType"))
			experienceLevel := internal.ExperienceLevel(r.PostForm.Get("experienceLevel"))
			listingAge := internal.ListingAge(r.PostForm.Get("listingAge"))

			newCriteria, err := internal.CreateSearchCriteria(ctx, app.fs, query, geoID, workType, experienceLevel, maxPages, listingAge)
			if err != nil {
				data["Error"] = fmt.Sprintf("Failed to create criteria: %v", err)
			} else {
				data["Success"] = fmt.Sprintf("Successfully created criteria %s.", newCriteria.ID)
				// To select the newly created item in the dropdown
				r.URL.RawQuery = "id=" + newCriteria.ID
			}
		case "update":
			maxPages, _ := strconv.Atoi(r.PostForm.Get("maxPages"))
			updatedCriteria := internal.SearchCriteria{
				ID:              id,
				Query:           r.PostForm.Get("query"),
				GeoID:           r.PostForm.Get("geoId"),
				WorkType:        internal.WorkType(r.PostForm.Get("workType")),
				ExperienceLevel: internal.ExperienceLevel(r.PostForm.Get("experienceLevel")),
				ListingAge:      internal.ListingAge(r.PostForm.Get("listingAge")),
				MaxPages:        maxPages,
			}
			if err := internal.SetSearchCriteria(ctx, app.fs, &updatedCriteria); err != nil {
				data["Error"] = fmt.Sprintf("Failed to update criteria: %v", err)
			} else {
				data["Success"] = "Successfully updated criteria."
			}
		case "delete":
			if r.PostForm.Get("confirm_delete") != "on" {
				data["Error"] = "You must check the confirmation box to delete."
			} else {
				if err := internal.DeleteSearchCriteria(ctx, app.fs, id); err != nil {
					data["Error"] = fmt.Sprintf("Failed to delete criteria: %v", err)
				} else {
					data["Success"] = "Successfully deleted criteria."
					// Clear selection
					r.URL.RawQuery = ""
				}
			}
		default:
			data["Error"] = "Invalid action."
		}
	}

	// --- GET Data Loading ---

	// Fetch all criteria for the dropdown
	allCriteria, err := internal.ListSearchCriteria(ctx, app.fs)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	sort.Slice(allCriteria, func(i, j int) bool {
		return allCriteria[i].ID < allCriteria[j].ID
	})
	data["AllCriteria"] = allCriteria

	// Fetch GeoIDs for the form dropdown
	geoIDs, err := internal.ListGeoIDs(ctx, app.fs)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	sort.Slice(geoIDs, func(i, j int) bool {
		return geoIDs[i].Value < geoIDs[j].Value
	})
	data["GeoIDs"] = geoIDs

	// Pass enum values to the template
	data["WorkTypes"] = []internal.WorkType{internal.WorkTypeAtWork, internal.WorkTypeRemote, internal.WorkTypeHybrid}
	data["ExperienceLevels"] = []internal.ExperienceLevel{internal.ExperienceLevelInternship, internal.ExperienceLevelEntryLevel, internal.ExperienceLevelAssociate, internal.ExperienceLevelMidSeniorLevel, internal.ExperienceLevelDirector}
	data["ListingAges"] = []internal.ListingAge{internal.ListingAgeDay, internal.ListingAgeWeek, internal.ListingAgeMonth}

	// Check for a selected criteria from the dropdown
	selectedID := r.URL.Query().Get("id")
	duplicateID := r.URL.Query().Get("duplicate_id")

	if duplicateID != "" {
		// Handle duplication: load data into form for creation
		dupCriteria, found, err := internal.GetSearchCriteria(ctx, app.fs, duplicateID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if found {
			data["SelectedCriteria"] = dupCriteria // To pre-fill form
			data["IsDuplicate"] = true             // To show "Create" button
		}
	} else if selectedID != "" && selectedID != "new" {
		// Handle editing
		selectedCriteria, found, err := internal.GetSearchCriteria(ctx, app.fs, selectedID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if found {
			data["SelectedCriteria"] = selectedCriteria
		}
	} else if selectedID == "new" {
		data["IsNew"] = true
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/search-criteria.html"); err != nil {
		app.serverError(w, r, err)
	}
}

// --- Admin Appointment Management ---
func (app *application) adminAppointments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)
	data["AppointmentStatuses"] = []internal.AppointmentStatus{
		internal.AppointmentStatusScheduled,
		internal.AppointmentStatusRunning,
		internal.AppointmentStatusProcessing,
		internal.AppointmentStatusCompleted,
		internal.AppointmentStatusCancelled,
		internal.AppointmentStatusFailed,
		internal.AppointmentStatusOther,
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
			return
		}

		userID := r.PostForm.Get("user_id")
		appointmentID := r.PostForm.Get("appointment_id")
		status := internal.AppointmentStatus(r.PostForm.Get("status"))

		if userID == "" || appointmentID == "" {
			data["Error"] = "User ID and Appointment ID are required for update."
		} else {
			appointment, err := internal.GetAppointment(ctx, app.fs, userID, appointmentID)
			if err != nil {
				data["Error"] = "Failed to retrieve appointment: " + err.Error()
			} else {
				appointment.Status = status
				appointment.UpdatedAt = time.Now()
				if err := internal.UpdateAppointment(ctx, app.fs, appointment); err != nil {
					data["Error"] = "Failed to update appointment: " + err.Error()
				} else {
					data["Success"] = "Appointment status updated successfully."
				}
			}
		}
	}

	// --- GET Data Loading ---
	// Fetch all users for the dropdown
	allUsers, err := internal.ListUsers(ctx, app.fs, 10000, 0)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Fetch all subscriptions to determine which users have subscriptions
	allSubscriptions, err := internal.ListSubscriptions(ctx, app.fs)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Build map of users with subscriptions
	userSubscriptionCounts := make(map[string]int)
	for _, subscription := range allSubscriptions {
		userSubscriptionCounts[subscription.UserID]++
	}

	// Group users according to the requirements
	now := time.Now()
	var activeUsers []*internal.User  // Group 1: Active subscriptions or future SubscriptionEnd
	var expiredUsers []*internal.User // Group 2: Subscriptions but expired SubscriptionEnd
	var otherUsers []*internal.User   // Group 3: All other users

	for _, user := range allUsers {
		hasSubscription := userSubscriptionCounts[user.ID] > 0
		hasActiveSubscriptionEnd := user.SubscriptionEnd != nil && user.SubscriptionEnd.After(now)

		if hasSubscription || hasActiveSubscriptionEnd {
			activeUsers = append(activeUsers, user)
		} else if user.SubscriptionEnd != nil && user.SubscriptionEnd.Before(now) {
			expiredUsers = append(expiredUsers, user)
		} else {
			otherUsers = append(otherUsers, user)
		}
	}

	data["Users"] = allUsers // Keep for backward compatibility
	data["ActiveUsers"] = activeUsers
	data["ExpiredUsers"] = expiredUsers
	data["OtherUsers"] = otherUsers
	data["UserSubscriptionCounts"] = userSubscriptionCounts

	// Check for a selected user
	selectedUserID := r.URL.Query().Get("user_id")
	if r.Method == http.MethodPost {
		selectedUserID = r.PostForm.Get("user_id")
	}
	if selectedUserID != "" {
		data["SelectedUserID"] = selectedUserID
		appointments, err := internal.ListAppointments(ctx, app.fs, selectedUserID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		sort.Slice(appointments, func(i, j int) bool {
			return appointments[i].ScheduledTime.After(appointments[j].ScheduledTime)
		})
		data["Appointments"] = appointments

		// Check for a selected appointment
		selectedAppointmentID := r.URL.Query().Get("appointment_id")
		if r.Method == http.MethodPost {
			selectedAppointmentID = r.PostForm.Get("appointment_id")
		}
		if selectedAppointmentID != "" {
			data["SelectedAppointmentID"] = selectedAppointmentID
			appointment, err := internal.GetAppointment(ctx, app.fs, selectedUserID, selectedAppointmentID)
			if err != nil {
				app.serverError(w, r, err)
				return
			}
			data["SelectedAppointment"] = appointment

			if appointment.CallID != nil && *appointment.CallID != "" {
				transcript, err := internal.GetAppointmentTranscript(ctx, app.fs, *appointment.CallID)
				if err != nil {
					app.logger.Error("failed to get transcript", "error", err, "call_id", *appointment.CallID)
					data["Error"] = "Failed to fetch transcript: " + err.Error()
				} else if transcript != nil {
					data["Transcript"] = transcript.String()
				}
			}
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/appointments.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) showSearchTasksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
			return
		}

		taskID := r.PostForm.Get("task_id")
		action := r.PostForm.Get("action")

		if action == "cancel" && taskID != "" {
			err := internal.SetSearchTaskStatus(ctx, app.fs, taskID, internal.SearchTaskStatusCancelled)
			if err != nil {
				data["Error"] = "Failed to cancel task: " + err.Error()
			} else {
				data["Success"] = "Task cancelled successfully."
			}
		}
	}

	tasks, err := internal.GetIncompleteSearchTasks(ctx, app.fs)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})

	data["Tasks"] = tasks

	if err := response.Page(w, http.StatusOK, data, "pages/admin/search-tasks.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getSearchTasksPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	tasks, err := internal.GetIncompleteSearchTasks(ctx, app.fs)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})

	data["Tasks"] = tasks

	if err := response.Partial(w, http.StatusOK, data, "partials/search-task-list.html"); err != nil {
		app.serverError(w, r, err)
	}
}

// --- Admin Users Management ---
func (app *application) adminUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// Check if filtering by Call Completed status
	showCallCompletedOnly := r.URL.Query().Get("call_completed") == "true"
	data["ShowCallCompletedOnly"] = showCallCompletedOnly

	// Fetch all users - using a high limit to get all users
	allUsers, err := internal.ListUsers(ctx, app.fs, 10000, 0)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to list users: %w", err))
		return
	}

	// Filter users if needed
	var users []*internal.User
	if showCallCompletedOnly {
		for _, user := range allUsers {
			if user.Status == internal.UserStatusCallCompleted {
				users = append(users, user)
			}
		}
	} else {
		users = allUsers
	}

	// Sort users by LastAccessedAt, most recent first
	sort.Slice(users, func(i, j int) bool {
		return users[i].LastAccessedAt.After(users[j].LastAccessedAt)
	})

	data["Users"] = users
	data["TotalUsers"] = len(users)
	data["TotalAllUsers"] = len(allUsers)

	if err := response.Page(w, http.StatusOK, data, "pages/admin/users.html"); err != nil {
		app.serverError(w, r, err)
	}
}

// --- Admin Search Task Analytics ---
func (app *application) adminSearchTaskAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// Fetch aggregated search task analytics
	analytics, err := internal.GetSearchTaskAnalyticsByCriteria(ctx, app.fs)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get search task analytics: %w", err))
		return
	}

	// Handle sorting
	sortBy := r.URL.Query().Get("sort")
	switch sortBy {
	case "task_count":
		sort.Slice(analytics, func(i, j int) bool {
			return analytics[i].TaskCount > analytics[j].TaskCount
		})
	case "total_listings":
		sort.Slice(analytics, func(i, j int) bool {
			return analytics[i].TotalListings > analytics[j].TotalListings
		})
	case "duplicates":
		sort.Slice(analytics, func(i, j int) bool {
			return analytics[i].TotalDuplicates > analytics[j].TotalDuplicates
		})
	case "duplicate_percentage":
		sort.Slice(analytics, func(i, j int) bool {
			return analytics[i].DuplicatePercentage > analytics[j].DuplicatePercentage
		})
	case "last_search":
		sort.Slice(analytics, func(i, j int) bool {
			if analytics[i].LastSearchAt == nil && analytics[j].LastSearchAt == nil {
				return false
			}
			if analytics[i].LastSearchAt == nil {
				return false
			}
			if analytics[j].LastSearchAt == nil {
				return true
			}
			return analytics[i].LastSearchAt.After(*analytics[j].LastSearchAt)
		})
	default:
		// Default sort by criteria ID
		sort.Slice(analytics, func(i, j int) bool {
			return analytics[i].CriteriaID < analytics[j].CriteriaID
		})
	}

	// Calculate summary statistics
	totalCriteria := len(analytics)
	var totalTasks, totalListings, totalDuplicates int
	var maxTaskCount, maxDuplicatePercentage float64
	var mostActiveCriteria, highestDuplicateCriteria string

	for _, a := range analytics {
		totalTasks += a.TaskCount
		totalListings += a.TotalListings
		totalDuplicates += a.TotalDuplicates

		if float64(a.TaskCount) > maxTaskCount {
			maxTaskCount = float64(a.TaskCount)
			mostActiveCriteria = a.CriteriaID
		}

		if a.DuplicatePercentage > maxDuplicatePercentage {
			maxDuplicatePercentage = a.DuplicatePercentage
			highestDuplicateCriteria = a.CriteriaID
		}
	}

	var overallDuplicatePercentage float64
	if totalListings > 0 {
		overallDuplicatePercentage = float64(totalDuplicates) / float64(totalListings) * 100
	}

	// Set template data
	data["Analytics"] = analytics
	data["TotalCriteria"] = totalCriteria
	data["TotalTasks"] = totalTasks
	data["TotalListings"] = totalListings
	data["TotalDuplicates"] = totalDuplicates
	data["OverallDuplicatePercentage"] = overallDuplicatePercentage
	data["MostActiveCriteria"] = mostActiveCriteria
	data["HighestDuplicateCriteria"] = highestDuplicateCriteria
	data["MaxDuplicatePercentage"] = maxDuplicatePercentage
	data["CurrentSort"] = sortBy

	if err := response.Page(w, http.StatusOK, data, "pages/admin/search-task-analytics.html"); err != nil {
		app.serverError(w, r, err)
	}
}

// --- Admin User Detail ---
func (app *application) adminUserDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// Extract user ID from URL path using Flow's parameter extraction
	userID := flow.Param(r.Context(), "id")
	if userID == "" {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Fetch user from database
	user, found, err := internal.GetUser(ctx, app.fs, userID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get user: %w", err))
		return
	}
	if !found {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Add user data to template
	data["User"] = user
	data["UserID"] = userID

	if err := response.Page(w, http.StatusOK, data, "pages/admin/user-detail.html"); err != nil {
		app.serverError(w, r, err)
	}
}

// --- Admin Held Response Management ---
func (app *application) adminHeldResponses(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// Get filter parameters
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	data["SelectedStatus"] = status

	// Define available statuses for filtering
	data["HeldResponseStatuses"] = []string{
		"pending",
		"approved",
		"revision",
		"expired",
	}

	// Get held response service
	heldResponseService := app.heldResponseService
	if heldResponseService == nil {
		data["Error"] = "Held response service not configured"
		if err := response.Page(w, http.StatusOK, data, "pages/admin/held-responses.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	var heldResponses []*internal.HeldResponse
	var err error

	// Fetch responses based on status filter
	switch status {
	case "pending":
		heldResponses, err = heldResponseService.GetPendingResponses(ctx)
	case "approved":
		heldResponses, err = heldResponseService.GetApprovedResponses(ctx)
	case "revision":
		heldResponses, err = heldResponseService.GetRevisionResponses(ctx)
	case "expired":
		heldResponses, err = heldResponseService.GetExpiredResponses(ctx)
	default:
		heldResponses, err = heldResponseService.GetPendingResponses(ctx)
	}

	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get held responses: %w", err))
		return
	}

	// Sort by created date, most recent first
	sort.Slice(heldResponses, func(i, j int) bool {
		return heldResponses[i].CreatedAt.After(heldResponses[j].CreatedAt)
	})

	data["HeldResponses"] = heldResponses
	data["TotalResponses"] = len(heldResponses)

	if err := response.Page(w, http.StatusOK, data, "pages/admin/held-responses.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) adminHeldResponseDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// Extract response ID from URL path
	responseID := flow.Param(r.Context(), "id")
	if responseID == "" {
		http.Error(w, "Response ID is required", http.StatusBadRequest)
		return
	}

	// Get held response service
	heldResponseService := app.heldResponseService
	if heldResponseService == nil {
		data["Error"] = "Held response service not configured"
		if err := response.Page(w, http.StatusOK, data, "pages/admin/held-response-detail.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	// Handle POST actions (approve/revision)
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
			return
		}

		action := r.PostForm.Get("action")
		adminUser := contextGetAuthenticatedUser(r)
		adminUserID := ""
		if adminUser != nil {
			adminUserID = adminUser.ID
		}

		switch action {
		case "approve":
			err := heldResponseService.ApproveResponse(ctx, responseID, adminUserID)
			if err != nil {
				data["Error"] = fmt.Sprintf("Failed to approve response: %v", err)
			} else {
				data["Success"] = "Response approved and sent successfully"
			}
		case "revision":
			feedback := r.PostForm.Get("feedback")
			if feedback == "" {
				data["Error"] = "Feedback is required for revision requests"
			} else {
				err := heldResponseService.RequestRevision(ctx, responseID, adminUserID, feedback)
				if err != nil {
					data["Error"] = fmt.Sprintf("Failed to request revision: %v", err)
				} else {
					data["Success"] = "Revision requested successfully"
				}
			}
		default:
			data["Error"] = "Invalid action"
		}
	}

	// Fetch the held response
	heldResponse, exists, err := internal.GetHeldResponse(ctx, app.fs, responseID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get held response: %w", err))
		return
	}
	if !exists {
		http.Error(w, "Held response not found", http.StatusNotFound)
		return
	}

	data["HeldResponse"] = heldResponse
	data["ResponseID"] = responseID

	if err := response.Page(w, http.StatusOK, data, "pages/admin/held-response-detail.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) adminHeldResponseStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// Get held response service
	heldResponseService := app.heldResponseService
	if heldResponseService == nil {
		data["Error"] = "Held response service not configured"
		if err := response.Page(w, http.StatusOK, data, "pages/admin/held-response-stats.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	// Get statistics for each status
	pendingResponses, err := heldResponseService.GetPendingResponses(ctx)
	if err != nil {
		app.logger.Error("failed to get pending responses", "error", err)
		pendingResponses = []*internal.HeldResponse{}
	}

	approvedResponses, err := heldResponseService.GetApprovedResponses(ctx)
	if err != nil {
		app.logger.Error("failed to get approved responses", "error", err)
		approvedResponses = []*internal.HeldResponse{}
	}

	revisionResponses, err := heldResponseService.GetRevisionResponses(ctx)
	if err != nil {
		app.logger.Error("failed to get revision responses", "error", err)
		revisionResponses = []*internal.HeldResponse{}
	}

	expiredResponses, err := heldResponseService.GetExpiredResponses(ctx)
	if err != nil {
		app.logger.Error("failed to get expired responses", "error", err)
		expiredResponses = []*internal.HeldResponse{}
	}

	// Calculate statistics
	totalResponses := len(pendingResponses) + len(approvedResponses) + len(revisionResponses) + len(expiredResponses)

	var pendingPercentage, approvedPercentage, revisionPercentage, expiredPercentage float64
	if totalResponses > 0 {
		pendingPercentage = float64(len(pendingResponses)) / float64(totalResponses) * 100
		approvedPercentage = float64(len(approvedResponses)) / float64(totalResponses) * 100
		revisionPercentage = float64(len(revisionResponses)) / float64(totalResponses) * 100
		expiredPercentage = float64(len(expiredResponses)) / float64(totalResponses) * 100
	}

	// Count responses by hold criteria
	criteriaCounts := make(map[string]int)
	allResponses := append(append(append(pendingResponses, approvedResponses...), revisionResponses...), expiredResponses...)
	for _, response := range allResponses {
		criteriaCounts[response.HoldCriteria]++
	}

	// Get filtered email statistics
	filteredEmailService := internal.NewFilteredEmailService(app.fs)
	filteredStats, err := filteredEmailService.GetStats(ctx)
	if err != nil {
		app.logger.Error("failed to get filtered email stats", "error", err)
		filteredStats = map[string]interface{}{
			"total_filtered": 0,
			"by_filter":      make(map[string]int),
			"by_reason":      make(map[string]int),
		}
	}

	// Set template data
	data["TotalResponses"] = totalResponses
	data["PendingCount"] = len(pendingResponses)
	data["ApprovedCount"] = len(approvedResponses)
	data["RevisionCount"] = len(revisionResponses)
	data["ExpiredCount"] = len(expiredResponses)
	data["PendingPercentage"] = pendingPercentage
	data["ApprovedPercentage"] = approvedPercentage
	data["RevisionPercentage"] = revisionPercentage
	data["ExpiredPercentage"] = expiredPercentage
	data["CriteriaCounts"] = criteriaCounts

	// Add filtered email stats with calculated percentages
	data["FilteredEmailStats"] = filteredStats
	data["TotalFiltered"] = filteredStats["total_filtered"]

	totalFiltered := filteredStats["total_filtered"].(int)
	filterCounts := filteredStats["by_filter"].(map[string]int)
	reasonCounts := filteredStats["by_reason"].(map[string]int)

	// Calculate percentages for filter counts
	filterCountsWithPercentages := make(map[string]map[string]interface{})
	for filter, count := range filterCounts {
		percentage := 0.0
		if totalFiltered > 0 {
			percentage = float64(count) / float64(totalFiltered) * 100
		}
		filterCountsWithPercentages[filter] = map[string]interface{}{
			"count":      count,
			"percentage": percentage,
		}
	}

	// Calculate percentages for reason counts
	reasonCountsWithPercentages := make(map[string]map[string]interface{})
	for reason, count := range reasonCounts {
		percentage := 0.0
		if totalFiltered > 0 {
			percentage = float64(count) / float64(totalFiltered) * 100
		}
		reasonCountsWithPercentages[reason] = map[string]interface{}{
			"count":      count,
			"percentage": percentage,
		}
	}

	data["FilterCounts"] = filterCounts
	data["ReasonCounts"] = reasonCounts
	data["FilterCountsWithPercentages"] = filterCountsWithPercentages
	data["ReasonCountsWithPercentages"] = reasonCountsWithPercentages

	if err := response.Page(w, http.StatusOK, data, "pages/admin/held-response-stats.html"); err != nil {
		app.serverError(w, r, err)
	}
}
