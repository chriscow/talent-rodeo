package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"time"
	"unsafe"

	"github.com/getsentry/sentry-go"
	"github.com/samber/lo"
)

// JobWithDetails combines UserJob with the job details, and can also handle ScoredJobDetail
type JobWithDetails struct {
	UserJob   *internal.UserJob
	JobDetail *internal.JobDetail
}

// CreatedAt returns when the job was added to the user's list
func (j *JobWithDetails) CreatedAt() time.Time {
	if j.UserJob != nil {
		return j.UserJob.CreatedAt
	}

	// For semantic-only jobs, use the job's posted date or current time
	if !j.JobDetail.PostedDate.IsZero() {
		return j.JobDetail.PostedDate
	}
	return time.Now()
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
		"user_status": user.State.Status.String(),
	})
	return user, nil
}

// showDashboard handles GET requests to /dashboard.
func (app *application) showDashboard(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	user, err := dashboardValidateUserStatus(r)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if user.State.Status < internal.UserStatusCallCompleted {
		app.showWaitingForCall(w, r, user)
		return
	}

	status, isValid := internal.ParseJobStatus(r.URL.Query().Get("status"))
	if !isValid {
		status = internal.JobStatusNew
	}

	pageToken := r.URL.Query().Get("page")
	pageSize := 5 // Fixed page size

	// Parse current page for pagination
	currentPage := 1
	if pageToken != "" {
		if parsedPage, err := strconv.Atoi(pageToken); err == nil && parsedPage > 0 {
			currentPage = parsedPage
		}
	}

	jobs := map[string]JobWithDetails{}
	var nextPageToken string
	var statusJobCount int64 = 0

	start := time.Now()
	userJobs, nextPage, err := internal.GetUserJobsSorted(r.Context(), app.fs, user.ID, internal.JobStatus(status), pageSize, pageToken)
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
			UserJob: job,
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
				job.JobDetail = &detail
				jobs[detail.ID] = job
			} else {
				app.logger.Warn("showDashboard: job detail found without corresponding user job", "job_id", detail.ID)
			}
		}
	}

	// Calculate next search time
	var nextSearchTime time.Time
	subscriptions, err := internal.GetSubscriptionsForUser(r.Context(), app.fs, user.ID)
	if err != nil {
		app.logger.Error("failed to get user subscriptions", "error", err, "user_id", user.ID)
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

	if time.Until(nextSearchTime) < 0 {
		// it's in the past -- there is a delay or something else wrong
		internal.SentryReportError(r.Context(), fmt.Errorf("next search time is in the past"), sentry.LevelError, map[string]string{
			"error_type":  "database_retrieval",
			"entity_type": "search_criteria",
			"user_id":     user.ID,
		})
		// it will start at 17:00 UTC
		nextSearchTime = time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 17, 0, 0, 0, time.UTC)
	}

	data["NextSearchTime"] = nextSearchTime

	// Get the total count for the current status for pagination calculations
	statusJobCount, _ = internal.GetJobsCount(r.Context(), app.fs, user.ID, status)

	totalPages := (int(statusJobCount) + pageSize - 1) / pageSize // Ceiling division

	if nextPage != "" {
		nextPageToken = nextPage
	}

	// Add data to template
	data["CurrentStatus"] = user.State.Status.Description()
	data["CurrentTime"] = time.Now()

	// Convert jobs map to slice for template consumption
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

	isPusherUserPresent, err := internal.IsPusherUserPresent(app.pusher, user.ID)
	if err != nil {
		app.logger.Error("failed to check if user is present", "error", err, "user_id", user.ID)
	}
	data["IsPusherUserPresent"] = isPusherUserPresent

	data["ActiveStatus"] = string(status)
	data["NextPageToken"] = nextPageToken
	data["CurrentPageToken"] = pageToken
	data["HasNextPage"] = nextPageToken != ""
	data["CurrentPage"] = currentPage
	data["TotalPages"] = totalPages
	data["TotalJobsForStatus"] = statusJobCount
	data["PusherKey"] = os.Getenv("PUSHER_KEY")
	data["PusherCluster"] = os.Getenv("PUSHER_CLUSTER")
	data["JobStatuses"] = internal.GetJobStatuses()

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
			app.logger.Warn("Failed to get user feedback for jobs", "error", err, "user_id", user.ID)
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
