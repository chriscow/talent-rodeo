package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"time"
	"unsafe"

	"github.com/getsentry/sentry-go"
	"github.com/samber/lo"
)

// calculateSemanticJobsMemory estimates the memory usage of semantic job results
func calculateSemanticJobsMemory(jobs []internal.ScoredJobDetail) int {
	if len(jobs) == 0 {
		return 0
	}

	// Base struct size
	baseSize := int(unsafe.Sizeof(internal.ScoredJobDetail{}))

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

// JobWithDetails combines UserJob with the job details, and can also handle ScoredJobDetail
type JobWithDetails struct {
	UserJob                  *internal.UserJob
	internal.ScoredJobDetail      // For semantic search results without UserJob
	IsSemanticOnly           bool // True if this comes from semantic search without UserJob
}

// CreatedAt returns when the job was added to the user's list
func (j *JobWithDetails) CreatedAt() time.Time {
	if j.UserJob != nil {
		return j.UserJob.CreatedAt
	}

	// For semantic-only jobs, use the job's posted date or current time
	if !j.ScoredJobDetail.JobDetail.PostedDate.IsZero() {
		return j.ScoredJobDetail.JobDetail.PostedDate
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
		return j.ScoredJobDetail.JobDetail.Title
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
	if j.ScoredJobDetail.JobDetail.Company != "" {
		return j.ScoredJobDetail.JobDetail.Company
	}
	return ""
}

// GetScore returns the semantic search score if available
func (j *JobWithDetails) GetScore() float64 {
	return j.ScoredJobDetail.Score
}

// GetID returns the job ID
func (j *JobWithDetails) GetID() string {
	return j.ScoredJobDetail.JobDetail.ID
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
	jobDetail := j.ScoredJobDetail.JobDetail
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
	jobDetail := j.ScoredJobDetail.JobDetail
	if jobDetail.ExtractedData != nil {
		return jobDetail.ExtractedData.Requirements
	}
	return []string{}
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

	// Parse current page for semantic search pagination
	currentPage := 1
	if pageToken != "" {
		if parsedPage, err := strconv.Atoi(pageToken); err == nil && parsedPage > 0 {
			currentPage = parsedPage
		}
	}

	// Get the total count for the current status for pagination calculations
	start := time.Now()
	statusJobCount, err := internal.GetJobsCount(r.Context(), app.fs, user.ID, status)
	if err != nil {
		wrappedErr := fmt.Errorf("showDashboard: failed to get job count for status %s: %w", status, err)
		app.logger.Error("showDashboard: failed to get job count", "error", err, "status", status)

		internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
			"error_type":  "database_retrieval",
			"entity_type": "user_jobs_count",
			"job_status":  string(status),
		})

		// Check if this is the missing collection/index error for new users
		if strings.Contains(err.Error(), "failed to iterate jobs") ||
			strings.Contains(err.Error(), "failed to get job count") {
			// For new users without any jobs collection yet, set count to 0
			statusJobCount = 0
			data["FirestoreError"] = true
		} else {
			// For other errors, return server error
			app.serverError(w, r, wrappedErr)
			return
		}
	} else {
		app.logger.Debug("showDashboard: GetJobsCount took", "duration", time.Since(start).String())
	}

	// Hybrid Approach: Use semantic search for "New" jobs, existing queries for other statuses
	jobs := map[string]JobWithDetails{}
	var semanticJobsSlice []JobWithDetails // Ordered slice for semantic search results
	var nextPageToken string

	if status == internal.JobStatusNew {
		// For "New" status, use semantic search with jobs that have no UserJob record
		start = time.Now()

		// Get user's subscriptions to find criteriaID for semantic search
		subscriptions, err := internal.GetSubscriptionsForUser(r.Context(), app.fs, user.ID)
		if err != nil {
			wrappedErr := fmt.Errorf("showDashboard: failed to get user subscriptions: %w", err)
			app.logger.Error("showDashboard: failed to get user subscriptions", "error", err)
			internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
				"error_type":  "database_retrieval",
				"entity_type": "user_subscriptions",
			})
			app.serverError(w, r, wrappedErr)
			return
		}

		if len(subscriptions) > 0 {
			// Use a large topK for comprehensive semantic search results (cached for 1 hour)
			const semanticSearchTopK = 50 // we use 50 because SearchJobDetails will double it and rerank, which has a limit of 100

			var params *internal.HybridSearchParams = nil // use the default params

			allSemanticJobs, err := internal.SearchJobDetails(r.Context(), app.fs, user.ID, *user.SteeringQuery, semanticSearchTopK, params)
			if err != nil {
				wrappedErr := fmt.Errorf("showDashboard: failed to get semantic jobs: %w", err)
				app.logger.Error("showDashboard: failed to get semantic jobs", "error", err)
				internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
					"error_type":  "semantic_search",
					"entity_type": "semantic_jobs",
				})
				app.serverError(w, r, wrappedErr)
				return
			}

			// Calculate memory usage for debug logging
			memoryUsage := calculateSemanticJobsMemory(allSemanticJobs)
			app.logger.Debug("showDashboard: semantic search memory usage",
				"total_jobs", len(allSemanticJobs),
				"memory_bytes", memoryUsage,
				"memory_mb", memoryUsage/(1024*1024))

			// Filter semantic jobs to only include those without existing UserJob records
			existingUserJobs, err := internal.ListUserJobs(r.Context(), app.fs, user.ID)
			if err != nil {
				app.logger.Warn("showDashboard: failed to get existing user jobs for filtering", "error", err)
				existingUserJobs = []internal.UserJob{} // Continue with empty list
			}

			// Create a set of existing job IDs for fast lookup
			existingJobIDs := make(map[string]bool)
			for _, userJob := range existingUserJobs {
				existingJobIDs[userJob.JobRef.ID] = true
			}

			// Filter all semantic jobs to exclude those with UserJob records
			var allFilteredJobs []internal.ScoredJobDetail
			for _, scoredJob := range allSemanticJobs {
				if !existingJobIDs[scoredJob.JobDetail.ID] {
					allFilteredJobs = append(allFilteredJobs, scoredJob)
				}
			}

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
			statusJobCount = int64(len(allFilteredJobs))

			app.logger.Debug("showDashboard: semantic search completed",
				"duration", time.Since(start).String(),
				"total_semantic_jobs", len(allSemanticJobs),
				"filtered_jobs", len(allFilteredJobs),
				"page_jobs", len(semanticJobsSlice),
				"current_page", currentPage)
		} else {
			app.logger.Info("showDashboard: user has no subscriptions, showing empty new jobs")
		}
	} else {
		// For other statuses, use existing UserJob queries
		start = time.Now()
		userJobs, _, err := internal.GetUserJobsSorted(r.Context(), app.fs, user.ID, internal.JobStatus(status), pageSize, pageToken)
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
		} else {
			app.logger.Debug("showDashboard: GetUserJobsSorted took", "duration", time.Since(start).String())
		}

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
					job.JobDetail = detail
					jobs[detail.ID] = job
				} else {
					app.logger.Warn("showDashboard: job detail found without corresponding user job", "job_id", detail.ID)
				}
			}
		}
	}

	// Calculate pagination information for non-semantic results
	if status != internal.JobStatusNew {
		// Use the simple approach for UserJob pagination
		if pageToken != "" {
			currentPage = 2 // This is a simplification, actual page number would need more tracking
		}
	}

	totalPages := (int(statusJobCount) + pageSize - 1) / pageSize // Ceiling division

	// Calculate next page info for semantic search
	if status == internal.JobStatusNew {
		hasNextPage := currentPage*pageSize < int(statusJobCount)
		if hasNextPage {
			nextPageToken = strconv.Itoa(currentPage + 1)
		}
	}

	// Add data to template
	data["CurrentStatus"] = user.State.Status.Description()
	data["CurrentTime"] = time.Now()

	// Set jobs data and HasJobs based on status type
	if status == internal.JobStatusNew {
		data["HasJobs"] = len(semanticJobsSlice) > 0 || statusJobCount > 0
		data["Jobs"] = semanticJobsSlice
	} else {
		// Convert jobs map to slice for template consumption
		jobsSlice := make([]JobWithDetails, 0, len(jobs))
		for _, job := range jobs {
			jobsSlice = append(jobsSlice, job)
		}
		data["HasJobs"] = len(jobs) > 0 || statusJobCount > 0
		data["Jobs"] = jobsSlice
	}

	data["ActiveStatus"] = string(status)
	data["NextPageToken"] = nextPageToken
	data["CurrentPageToken"] = pageToken

	// Set HasNextPage based on status type
	if status == internal.JobStatusNew {
		data["HasNextPage"] = nextPageToken != ""
	} else {
		data["HasNextPage"] = nextPageToken != ""
	}

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
	var jobIDs []string
	if status == internal.JobStatusNew {
		for _, job := range semanticJobsSlice {
			jobIDs = append(jobIDs, job.GetID())
		}
	} else {
		for jobID := range jobs {
			jobIDs = append(jobIDs, jobID)
		}
	}

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
