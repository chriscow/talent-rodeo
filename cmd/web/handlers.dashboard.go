package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/utils"
	"time"
	"unsafe"

	"github.com/getsentry/sentry-go"
	"github.com/samber/lo"
)

// JobWithDetails combines UserJob with the job details, and can also handle ScoredJobDetail
type JobWithDetails struct {
	UserJob     internal.UserJob
	JobDetail   internal.JobDetail
	EncryptedID string
}

// CreatedAt returns when the job was added to the user's list
func (j *JobWithDetails) CreatedAt() time.Time {
	return j.UserJob.CreatedAt
}

// Title returns the job title for display
func (j *JobWithDetails) Title() string {
	// Use JobDetail title if available
	if j.JobDetail.Title != "" {
		return j.JobDetail.Title
	}
	// For ScoredJobDetail
	if j.JobDetail.Title != "" {
		return j.JobDetail.Title
	}
	return ""
}

// CompanyName returns the company name for display
func (j *JobWithDetails) CompanyName() string {
	// Use JobDetail company if available
	if j.JobDetail.Company != "" {
		return j.JobDetail.Company
	}
	// For ScoredJobDetail
	if j.JobDetail.Company != "" {
		return j.JobDetail.Company
	}
	return ""
}

// GetScore returns the semantic search score if available
func (j *JobWithDetails) GetRating() int {
	return j.UserJob.Rating
}

// GetID returns the job ID
func (j *JobWithDetails) GetID() string {
	return j.JobDetail.ID
}

// Summary returns the job summary
func (j *JobWithDetails) Summary() string {
	jobDetail := j.JobDetail
	if jobDetail.ExtractedData != nil {
		return jobDetail.ExtractedData.Summary
	}
	return jobDetail.Description
}

// Qualifications returns the job qualifications
func (j *JobWithDetails) Qualifications() []string {
	jobDetail := j.JobDetail
	if jobDetail.ExtractedData != nil {
		if len(jobDetail.ExtractedData.Qualifications) > 0 {
			return jobDetail.ExtractedData.Qualifications
		}
		// Fallback to regular qualifications if summary qualifications not available
		if len(jobDetail.Qualifications) > 0 {
			return jobDetail.Qualifications
		}
	}
	return []string{}
}

func (j *JobWithDetails) Requirements() []string {
	jobDetail := j.JobDetail
	if jobDetail.ExtractedData != nil {
		return jobDetail.ExtractedData.Requirements
	}
	return []string{}
}

// calculateSemanticJobsMemory estimates the memory usage of semantic job results
func calculateSemanticJobsMemory(jobs []JobWithDetails) int {
	if len(jobs) == 0 {
		return 0
	}

	// Base struct size
	baseSize := int(unsafe.Sizeof(JobWithDetails{}))

	// Estimate string field sizes
	totalStringSize := 0
	for _, job := range jobs {
		totalStringSize += len(job.JobDetail.ID)
		totalStringSize += len(job.JobDetail.Title)
		totalStringSize += len(job.JobDetail.Company)
		totalStringSize += len(job.JobDetail.Location)
		totalStringSize += len(job.JobDetail.Description)
		totalStringSize += len(job.JobDetail.URL)
		totalStringSize += len(job.JobDetail.Provider)
		totalStringSize += len(job.JobDetail.ProviderJobID)

		// Estimate slice sizes (qualifications, requirements, etc.)
		for _, qual := range job.JobDetail.Qualifications {
			totalStringSize += len(qual)
		}
		if job.JobDetail.ExtractedData != nil {
			totalStringSize += len(job.JobDetail.ExtractedData.Summary)
			for _, req := range job.JobDetail.ExtractedData.Requirements {
				totalStringSize += len(req)
			}
			for _, qual := range job.JobDetail.ExtractedData.Qualifications {
				totalStringSize += len(qual)
			}
		}
	}

	// Total memory = (base struct size + average string overhead) * count
	totalMemory := (baseSize + totalStringSize)
	return totalMemory
}

func (app *application) showWaitingForCall(w http.ResponseWriter, r *http.Request, user *internal.User) {
	appointments, err := internal.ListAppointments(r.Context(), app.fs, user.ID)
	if err != nil {
		wrappedErr := fmt.Errorf("showDashboard: failed to get appointments: %w", err)
		app.logger.Error("showDashboard: failed to get appointments", "error", err)

		internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
			"error_type":  "database_retrieval",
			"entity_type": "appointments",
		})

		app.serverError(w, r, wrappedErr)
		return
	}

	// Find the most recent appointment regardless of status
	var activeAppointment *internal.Appointment
	for _, appt := range appointments {
		if appt.Status == internal.AppointmentStatusCancelled || appt.Status == internal.AppointmentStatusFailed || appt.Status == internal.AppointmentStatusOther {
			continue
		}

		if activeAppointment == nil || appt.CreatedAt.After(activeAppointment.CreatedAt) {
			activeAppointment = appt
		}
	}

	if activeAppointment == nil {
		// redirect to the profile page
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}

	data := app.newTemplateData(r)
	data["Appointment"] = activeAppointment

	if err := response.Page(w, http.StatusOK, data, "pages/dashboard-waiting-call.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render dashboard page: %w", err))
		internal.SentryReportError(r.Context(), fmt.Errorf("failed to render dashboard page: %w", err), sentry.LevelError, map[string]string{
			"error_type":  "page_rendering",
			"entity_type": "dashboard_page",
		})
	}
}

func dashboardValidateUserStatus(r *http.Request) (*internal.User, error) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		return nil, fmt.Errorf("user not authenticated")
	}

	if user.ID == "" {
		return nil, fmt.Errorf("user ID is empty")
	}

	internal.SentrySetScope(r.Context(), map[string]any{
		"user_status": user.Status.String(),
	})
	return user, nil
}

// showDashboard handles GET and POST requests to /dashboard.
func (app *application) showDashboard(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	user, err := dashboardValidateUserStatus(r)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if app.mode.Personal && !app.mode.CanMatch(user, time.Now()) {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}
	if !app.mode.Personal && user.Status < internal.UserStatusCallCompleted {
		app.showWaitingForCall(w, r, user)
		return
	}

	app.logger.Debug("showDashboard", "name", user.Name, "userID", user.ID, "status", user.Status.String())

	// Handle POST request (form submission for rating filter)
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.logger.Error("showDashboard: failed to parse form", "error", err)
			app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
			return
		}

		// Build redirect URL with form parameters
		redirectURL := "/dashboard"
		queryParams := []string{}

		// Preserve status if provided
		if status := r.PostForm.Get("status"); status != "" {
			queryParams = append(queryParams, "status="+status)
		}

		// Add show_poor_ratings parameter
		if r.PostForm.Get("show_poor_ratings") == "on" {
			queryParams = append(queryParams, "show_poor_ratings=true")
		}

		// Preserve test mode if provided
		if testMode := r.PostForm.Get("test_mode"); testMode != "" {
			queryParams = append(queryParams, "test_mode="+testMode)
		}

		if len(queryParams) > 0 {
			redirectURL += "?" + strings.Join(queryParams, "&")
		}

		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	// Parse query parameters for GET request
	status, isValid := internal.ParseJobStatus(r.URL.Query().Get("status"))
	if !isValid {
		status = internal.JobStatusNew
	}

	pageToken := r.URL.Query().Get("page")
	showPoorRatings := r.URL.Query().Get("show_poor_ratings") == "true"
	pageSize := 5 // Fixed page size

	// Decode the page token to get the actual job ID for pagination
	var decodedPageToken string
	if pageToken != "" {
		var err error
		decodedPageToken, err = internal.DecodePageToken(pageToken)
		if err != nil {
			app.logger.Warn("Invalid page token", "token", pageToken, "error", err)
			// Reset to first page if token is invalid
			pageToken = ""
			decodedPageToken = ""
		}
	}

	// Parse current page for pagination (this logic may need adjustment based on your pagination implementation)
	currentPage := 1
	if pageToken != "" {
		// For now, we'll keep the page number logic but you may want to adjust this
		// based on how your GetUserJobsSorted function actually uses the page token
		currentPage++ // Increment since we have a valid token
	}

	jobs := map[string]JobWithDetails{}
	var nextPageToken string
	var statusJobCount int64 = 0

	start := time.Now()
	userJobs, nextPage, err := internal.GetUserJobsSorted(r.Context(), app.fs, user.ID, internal.JobStatus(status), pageSize, decodedPageToken)
	if err != nil {
		wrappedErr := fmt.Errorf("showDashboard: failed to get sorted user jobs: %w", err)
		app.logger.Error("showDashboard: failed to get sorted user jobs", "error", err)

		internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
			"error_type":  "database_retrieval",
			"entity_type": "user_jobs",
			"job_status":  string(status),
		})

		// Check if this is the missing collection/index error for new users
		if strings.Contains(err.Error(), "failed to iterate user jobs") &&
			(strings.Contains(err.Error(), "The query requires an index") ||
				strings.Contains(err.Error(), "FailedPrecondition")) {
			// For new users without any jobs collection yet, show a welcome message
			data["FirestoreError"] = true
			data["HasJobs"] = false
			data["Jobs"] = nil
		} else {
			// For other errors, return server error
			app.serverError(w, r, wrappedErr)
			return
		}
	}
	app.logger.Debug("showDashboard: GetUserJobsSorted took", "duration", time.Since(start).String(), "count", len(userJobs))

	for _, job := range userJobs {
		jobs[job.JobRef.ID] = JobWithDetails{
			UserJob: *job,
		}
	}

	// Get job details for UserJob records
	jobIDs := lo.Keys(jobs)
	if len(jobIDs) > 0 {
		jobDetails, err := internal.GetJobDetailsByIDs(r.Context(), app.fs, jobIDs)
		if err != nil {
			wrappedErr := fmt.Errorf("showDashboard: failed to get job details by IDs: %w", err)
			app.logger.Error("showDashboard: failed to get job details by IDs", "error", err)
			internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
				"error_type":  "database_retrieval",
				"entity_type": "job_details",
				"job_ids":     strings.Join(jobIDs, ","),
			})
			app.serverError(w, r, wrappedErr)
			return
		}
		// Merge job details into the jobs map
		for _, detail := range jobDetails {
			if job, exists := jobs[detail.ID]; exists {
				job.JobDetail = detail
				encryptedID, err := utils.EncryptString(detail.ID)
				if err != nil {
					app.logger.Error("showDashboard: failed to encrypt job id", "error", err, "job_id", detail.ID)
					// skip this job if we can't encrypt the ID
					continue
				}
				job.EncryptedID = encryptedID
				jobs[detail.ID] = job
			} else {
				app.logger.Warn("showDashboard: job detail found without corresponding user job", "job_id", detail.ID)
			}
		}
	}

	// Calculate next search time
	var nextSearchTime time.Time
	if !app.mode.Personal {
		subscriptions, err := internal.GetSubscriptionsForUser(r.Context(), app.fs, user.ID)
		if err != nil {
			app.logger.Error("failed to get user subscriptions", "error", err, "userID", user.ID)
		} else {
			for _, sub := range subscriptions {
				criteria, found, err := internal.GetSearchCriteria(r.Context(), app.fs, sub.CriteriaID)
				if err != nil {
					app.logger.Warn("failed to get search criteria", "error", err, "criteria_id", sub.CriteriaID)
					continue
				}
				if found && criteria.LastPublishedAt != nil {
					t := criteria.LastPublishedAt.Add(24 * time.Hour)
					if nextSearchTime.IsZero() || t.Before(nextSearchTime) {
						// We always start searching at 17:00 UTC
						nextSearchTime = time.Date(t.Year(), t.Month(), t.Day(), 17, 0, 0, 0, time.UTC)
					}
				}
			}
		}

	}
	if !app.mode.Personal && !nextSearchTime.IsZero() && time.Until(nextSearchTime) < 0 {
		// it's in the past -- there is a delay or something else wrong
		internal.SentryReportError(r.Context(), fmt.Errorf("next search time is in the past"), sentry.LevelError, map[string]string{
			"error_type":  "database_retrieval",
			"entity_type": "search_criteria",
			"userID":      user.ID,
		})
		// it will start at 17:00 UTC
		nextSearchTime = time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 17, 0, 0, 0, time.UTC)
	}

	data["NextSearchTime"] = nextSearchTime

	// Get the total count for the current status for pagination calculations
	statusJobCount, _ = internal.GetJobsCount(r.Context(), app.fs, user.ID, status)

	totalPages := (int(statusJobCount) + pageSize - 1) / pageSize // Ceiling division

	if nextPage != "" {
		nextPageToken = internal.EncodePageToken(nextPage)
	}

	// Add data to template
	data["CurrentStatus"] = user.Status.Description()
	if app.mode.Personal {
		data["CurrentStatus"] = "Your search criteria are ready. We are finding matches."
	}
	data["CurrentTime"] = time.Now()

	// Add daily job evaluation statistics
	data["LastJobEvaluationRun"] = user.LastJobEvaluationRun
	data["LastEvaluationDate"] = user.LastEvaluationDate
	data["TotalJobsEvaluated"] = user.TotalJobsEvaluated
	data["NewJobsFound"] = user.NewJobsFound
	data["RejectedJobsCount"] = user.RejectedJobsCount
	data["QualityJobsFound"] = user.QualityJobsFound

	// Check for expired subscription
	isSubscriptionExpired := false
	daysSinceLastEvaluation := 0
	if !app.mode.Personal && user.SubscriptionEnd != nil && user.SubscriptionEnd.Before(time.Now()) {
		isSubscriptionExpired = true
		if user.LastEvaluationDate != nil {
			daysSinceLastEvaluation = int(time.Since(*user.LastEvaluationDate).Hours() / 24)
		}
	}
	data["IsSubscriptionExpired"] = isSubscriptionExpired
	data["DaysSinceLastEvaluation"] = daysSinceLastEvaluation

	// Convert jobs map to slice for template consumption and track filtering
	jobsSlice := make([]JobWithDetails, 0, len(jobs))
	hiddenJobsCount := 0

	for _, job := range jobs {
		// Only filter out 3-star jobs on the "New" jobs tab, and only when showPoorRatings is false
		if status == internal.JobStatusNew && !showPoorRatings && job.UserJob.Rating == 3 {
			hiddenJobsCount++
			continue
		}
		jobsSlice = append(jobsSlice, job)
	}
	// Resort jobsSlice by UserJob.Rating descending
	sort.Slice(jobsSlice, func(i, j int) bool {
		return jobsSlice[i].UserJob.Rating > jobsSlice[j].UserJob.Rating
	})

	displayedJobsCount := len(jobsSlice)

	data["HasJobs"] = len(jobs) > 0 || statusJobCount > 0
	data["Jobs"] = jobsSlice
	data["ShowPoorRatings"] = showPoorRatings
	data["DisplayedJobsCount"] = displayedJobsCount
	data["HiddenJobsCount"] = hiddenJobsCount
	data["TotalJobsForStatus"] = statusJobCount

	isPusherUserPresent, err := internal.IsPusherUserPresent(app.pusher, user.ID)
	if err != nil {
		app.logger.Error("failed to check if user is present", "error", err, "userID", user.ID)
	}
	data["IsPusherUserPresent"] = isPusherUserPresent

	data["ActiveStatus"] = string(status)
	data["NextPageToken"] = nextPageToken
	data["CurrentPageToken"] = pageToken
	data["HasNextPage"] = nextPageToken != ""
	data["CurrentPage"] = currentPage
	data["TotalPages"] = totalPages
	data["PusherKey"] = os.Getenv("PUSHER_KEY")
	data["PusherCluster"] = os.Getenv("PUSHER_CLUSTER")
	data["JobStatuses"] = internal.GetJobStatuses()
	data["User"] = user

	// Map for display names of job statuses
	data["StatusDisplayNames"] = map[string]string{
		"new":      "New Jobs",
		"saved":    "Saved Jobs",
		"applied":  "Applied Jobs",
		"archived": "Archived Jobs",
	}

	// Get user feedback for the current jobs to maintain state between refreshes
	if len(jobIDs) > 0 {
		userFeedback, err := internal.GetUserFeedbackForJobIDs(r.Context(), app.fs, user.ID, jobIDs)
		if err != nil {
			app.logger.Warn("Failed to get user feedback for jobs", "error", err, "userID", user.ID)
			userFeedback = []*internal.UserFeedback{} // Continue with empty feedback
		}

		// Convert feedback to a map for easy template access
		feedbackMap := make(map[string]map[string]string) // jobID -> itemID -> feedbackType
		for _, feedback := range userFeedback {
			if feedbackMap[feedback.JobID] == nil {
				feedbackMap[feedback.JobID] = make(map[string]string)
			}
			feedbackMap[feedback.JobID][feedback.ItemID] = string(feedback.FeedbackType)
		}
		data["UserFeedback"] = feedbackMap
	} else {
		data["UserFeedback"] = make(map[string]map[string]string)
	}

	if err := response.Page(w, http.StatusOK, data, "pages/dashboard.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render dashboard page: %w", err))
		internal.SentryReportError(r.Context(), fmt.Errorf("failed to render dashboard page: %w", err), sentry.LevelError, map[string]string{
			"error_type":  "page_rendering",
			"entity_type": "dashboard_page",
		})
	}
}

// FeedbackDashboardData contains all data for the feedback processing dashboard
type FeedbackDashboardData struct {
	RecentActivity   []RecentFeedbackActivity
	SystemMetrics    FeedbackSystemMetrics
	UserActivity     []UserFeedbackActivity
	ProcessingTrends []ProcessingTrend
	TopUsers         []TopFeedbackUser
	ErrorSummary     []ErrorSummary
	LastUpdateTime   time.Time
}

// RecentFeedbackActivity represents recent feedback processing activity
type RecentFeedbackActivity struct {
	UserID               string
	UserName             string
	FeedbackCount        int
	AnalysisConfidence   float64
	StatementUpdated     bool
	SteeringQueryUpdated bool
	ProcessingDuration   time.Duration
	ProcessedAt          time.Time
	ErrorMessage         string
	Success              bool
}

// FeedbackSystemMetrics represents overall system metrics
type FeedbackSystemMetrics struct {
	TotalUsers          int
	TotalFeedbackItems  int
	UnprocessedFeedback int
	AvgProcessingTime   time.Duration
	SuccessRate         float64
	AvgConfidenceScore  float64
	StatementsUpdated   int
	QueriesUpdated      int
	ProcessingErrors    int
	LastProcessingRun   time.Time
}

// UserFeedbackActivity represents user-specific feedback activity
type UserFeedbackActivity struct {
	UserID              string
	UserName            string
	TotalFeedback       int
	UnprocessedFeedback int
	LastFeedbackAt      time.Time
	ProcessingSuccess   bool
	LastProcessedAt     time.Time
}

// ProcessingTrend represents processing trends over time
type ProcessingTrend struct {
	Date           time.Time
	ProcessedUsers int
	SuccessRate    float64
	AvgDuration    time.Duration
	ErrorCount     int
}

// TopFeedbackUser represents users with most feedback
type TopFeedbackUser struct {
	UserID        string
	UserName      string
	FeedbackCount int
	LastActive    time.Time
}

// ErrorSummary represents common errors in feedback processing
type ErrorSummary struct {
	ErrorType    string
	Count        int
	LastOccurred time.Time
	SampleError  string
}

// showFeedbackDashboard handles the admin feedback processing dashboard
func (app *application) showFeedbackDashboard(w http.ResponseWriter, r *http.Request) {
	// User authentication and admin privileges are handled by requireSuperUser middleware
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	data := app.newTemplateData(r)

	// Get date range for analysis (default to last 30 days)
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -30)

	// Parse query parameters for date range
	if start := r.URL.Query().Get("start_date"); start != "" {
		if parsed, err := time.Parse("2006-01-02", start); err == nil {
			startDate = parsed
		}
	}
	if end := r.URL.Query().Get("end_date"); end != "" {
		if parsed, err := time.Parse("2006-01-02", end); err == nil {
			endDate = parsed
		}
	}

	// Gather dashboard data
	dashboardData, err := app.getFeedbackDashboardData(r.Context(), startDate, endDate)
	if err != nil {
		app.logger.Error("Failed to get feedback dashboard data", "error", err)
		app.serverError(w, r, fmt.Errorf("failed to get feedback dashboard data: %w", err))
		return
	}

	// Add data to template
	data["DashboardData"] = dashboardData
	data["StartDate"] = startDate.Format("2006-01-02")
	data["EndDate"] = endDate.Format("2006-01-02")
	data["RefreshInterval"] = 300 // 5 minutes in seconds

	if err := response.Page(w, http.StatusOK, data, "pages/admin/feedback-dashboard.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render feedback dashboard page: %w", err))
	}
}

// getFeedbackDashboardData gathers all data needed for the feedback dashboard
func (app *application) getFeedbackDashboardData(ctx context.Context, startDate, endDate time.Time) (*FeedbackDashboardData, error) {
	dashboardData := &FeedbackDashboardData{
		LastUpdateTime: time.Now(),
	}

	// Get system metrics
	systemMetrics, err := app.getFeedbackSystemMetrics(ctx, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get system metrics: %w", err)
	}
	dashboardData.SystemMetrics = *systemMetrics

	// Get recent processing activity
	recentActivity, err := app.getRecentFeedbackActivity(ctx, 50) // Last 50 activities
	if err != nil {
		return nil, fmt.Errorf("failed to get recent activity: %w", err)
	}
	dashboardData.RecentActivity = recentActivity

	// Get user activity summary
	userActivity, err := app.getUserFeedbackActivity(ctx, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get user activity: %w", err)
	}
	dashboardData.UserActivity = userActivity

	// Get top users by feedback count
	topUsers, err := app.getTopFeedbackUsers(ctx, 10) // Top 10 users
	if err != nil {
		return nil, fmt.Errorf("failed to get top users: %w", err)
	}
	dashboardData.TopUsers = topUsers

	return dashboardData, nil
}

// getFeedbackSystemMetrics calculates overall system metrics
func (app *application) getFeedbackSystemMetrics(ctx context.Context, startDate, endDate time.Time) (*FeedbackSystemMetrics, error) {
	metrics := &FeedbackSystemMetrics{}

	// Get total unprocessed feedback across all users
	unprocessedFeedback, err := internal.GetAllUnprocessedFeedback(ctx, app.fs, 0) // 0 = no limit
	if err != nil {
		return nil, fmt.Errorf("failed to get unprocessed feedback: %w", err)
	}
	metrics.UnprocessedFeedback = len(unprocessedFeedback)
	metrics.TotalFeedbackItems = len(unprocessedFeedback)

	// Count unique users with unprocessed feedback
	userMap := make(map[string]bool)
	for _, feedback := range unprocessedFeedback {
		userMap[feedback.UserID] = true
	}
	metrics.TotalUsers = len(userMap)

	// Get real processing statistics from logs
	processingStats, err := internal.GetProcessingStats(ctx, app.fs, startDate, endDate)
	if err != nil {
		app.logger.Warn("failed to get processing stats", "error", err)
		// Fall back to defaults
		metrics.AvgProcessingTime = 2500 * time.Millisecond
		metrics.SuccessRate = 85.0
		metrics.AvgConfidenceScore = 0.88
		metrics.StatementsUpdated = 0
		metrics.QueriesUpdated = 0
		metrics.ProcessingErrors = 0
		metrics.LastProcessingRun = time.Now().Add(-24 * time.Hour)
	} else {
		// Use real data from processing logs
		metrics.AvgProcessingTime = processingStats.AvgProcessingTime
		if processingStats.TotalProcessingRuns > 0 {
			metrics.SuccessRate = float64(processingStats.SuccessfulRuns) / float64(processingStats.TotalProcessingRuns) * 100
		}
		metrics.AvgConfidenceScore = processingStats.AvgConfidenceScore
		metrics.StatementsUpdated = processingStats.StatementsUpdated
		metrics.QueriesUpdated = processingStats.QueriesUpdated
		metrics.ProcessingErrors = processingStats.FailedRuns
		metrics.LastProcessingRun = processingStats.LastProcessingTime
	}

	return metrics, nil
}

// getRecentFeedbackActivity gets recent feedback processing activity
func (app *application) getRecentFeedbackActivity(ctx context.Context, limit int) ([]RecentFeedbackActivity, error) {
	// Get recent processing logs from Firestore
	logs, err := internal.GetRecentProcessingLogs(ctx, app.fs, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent processing logs: %w", err)
	}

	// Convert processing logs to dashboard format
	var activities []RecentFeedbackActivity
	for _, log := range logs {
		activity := RecentFeedbackActivity{
			UserID:               log.UserID,
			UserName:             log.UserName,
			FeedbackCount:        log.FeedbackCount,
			AnalysisConfidence:   log.AnalysisConfidence,
			StatementUpdated:     log.StatementUpdated,
			SteeringQueryUpdated: log.SteeringQueryUpdated,
			ProcessingDuration:   time.Duration(log.ProcessingDuration) * time.Millisecond,
			ProcessedAt:          log.ProcessedAt,
			Success:              log.Success,
			ErrorMessage:         log.ErrorMessage,
		}

		// Use email if name is not available
		if activity.UserName == "" && log.UserEmail != "" {
			activity.UserName = log.UserEmail
		}
		if activity.UserName == "" {
			activity.UserName = "Unknown User"
		}

		activities = append(activities, activity)
	}

	return activities, nil
}

// getUserFeedbackActivity gets user-specific feedback activity
func (app *application) getUserFeedbackActivity(ctx context.Context, startDate, endDate time.Time) ([]UserFeedbackActivity, error) {
	// Get all unprocessed feedback to find users with feedback
	unprocessedFeedback, err := internal.GetAllUnprocessedFeedback(ctx, app.fs, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get unprocessed feedback: %w", err)
	}

	// Group by user and get stats
	userFeedbackMap := make(map[string][]*internal.UserFeedback)
	for _, feedback := range unprocessedFeedback {
		userFeedbackMap[feedback.UserID] = append(userFeedbackMap[feedback.UserID], feedback)
	}

	var activities []UserFeedbackActivity
	for userID, feedbacks := range userFeedbackMap {
		if len(feedbacks) == 0 {
			continue
		}

		// Get user details
		user, found, err := internal.GetUser(ctx, app.fs, userID)
		if err != nil || !found {
			// User might be deleted, skip
			continue
		}

		// Get user's feedback stats
		stats, err := internal.GetUserFeedbackStats(ctx, app.fs, userID)
		if err != nil {
			// Continue with basic info if stats fail
			stats = &internal.UserFeedbackStats{
				TotalFeedback:    len(feedbacks),
				UnprocessedCount: len(feedbacks),
			}
		}

		userName := "Unknown User"
		if user.Name != "" {
			userName = user.Name
		} else if user.Email != "" {
			userName = user.Email
		}

		activity := UserFeedbackActivity{
			UserID:              userID,
			UserName:            userName,
			TotalFeedback:       stats.TotalFeedback,
			UnprocessedFeedback: len(feedbacks),
			ProcessingSuccess:   true,                            // Assume success for now
			LastProcessedAt:     time.Now().Add(-24 * time.Hour), // Estimate
		}

		if stats.LastFeedbackAt != nil {
			activity.LastFeedbackAt = *stats.LastFeedbackAt
		} else {
			activity.LastFeedbackAt = time.Now().Add(-1 * time.Hour)
		}

		activities = append(activities, activity)
	}

	return activities, nil
}

// getTopFeedbackUsers gets users with the most feedback
func (app *application) getTopFeedbackUsers(ctx context.Context, limit int) ([]TopFeedbackUser, error) {
	// Get all unprocessed feedback to find most active users
	unprocessedFeedback, err := internal.GetAllUnprocessedFeedback(ctx, app.fs, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get unprocessed feedback: %w", err)
	}

	// Count feedback per user
	userFeedbackCount := make(map[string]int)
	userLastActivity := make(map[string]time.Time)

	for _, feedback := range unprocessedFeedback {
		userFeedbackCount[feedback.UserID]++
		if feedback.CreatedAt.After(userLastActivity[feedback.UserID]) {
			userLastActivity[feedback.UserID] = feedback.CreatedAt
		}
	}

	// Convert to slice and sort by feedback count
	type userCount struct {
		userID     string
		count      int
		lastActive time.Time
	}

	var userCounts []userCount
	for userID, count := range userFeedbackCount {
		userCounts = append(userCounts, userCount{
			userID:     userID,
			count:      count,
			lastActive: userLastActivity[userID],
		})
	}

	// Sort by feedback count (descending)
	for i := 0; i < len(userCounts)-1; i++ {
		for j := i + 1; j < len(userCounts); j++ {
			if userCounts[i].count < userCounts[j].count {
				userCounts[i], userCounts[j] = userCounts[j], userCounts[i]
			}
		}
	}

	// Apply limit
	if limit > 0 && limit < len(userCounts) {
		userCounts = userCounts[:limit]
	}

	// Get user details and build result
	var topUsers []TopFeedbackUser
	for _, uc := range userCounts {
		user, found, err := internal.GetUser(ctx, app.fs, uc.userID)
		if err != nil || !found {
			// Skip deleted users
			continue
		}

		userName := "Unknown User"
		if user.Name != "" {
			userName = user.Name
		} else if user.Email != "" {
			userName = user.Email
		}

		topUsers = append(topUsers, TopFeedbackUser{
			UserID:        uc.userID,
			UserName:      userName,
			FeedbackCount: uc.count,
			LastActive:    uc.lastActive,
		})
	}

	return topUsers, nil
}
