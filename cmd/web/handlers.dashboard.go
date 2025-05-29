package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/samber/lo"
)

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

	if user.State.Status < internal.UserStatusCallScheduled {
		internal.SentryReportError(r.Context(), fmt.Errorf("user has not onboarded"), sentry.LevelError, map[string]string{
			"user_status": user.State.Status.String(),
			"error_type":  "user_not_onboarded",
			"entity_type": "user",
		})
		return nil, fmt.Errorf("user has not onboarded")
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

	// If this is true, we show all ratings not just the "IsRecommended" ones
	showAllRatings := r.URL.Query().Get("show_all") == "true"

	pageToken := r.URL.Query().Get("page")
	pageSize := 5 // Fixed page size

	// Get the total count for the current status for pagination calculations
	start := time.Now()
	statusJobCount, err := internal.GetJobsCount(r.Context(), app.fs, user.ID, status, showAllRatings)
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
			// Use the first subscription's criteria for semantic search
			criteriaID := subscriptions[0].CriteriaID
			topK := pageSize // Use pageSize as topK for semantic search

			semanticJobs, err := internal.GetSemanticJobsForUser(r.Context(), app.fs, user, criteriaID, topK)
			if err != nil {
				wrappedErr := fmt.Errorf("showDashboard: failed to get semantic jobs: %w", err)
				app.logger.Error("showDashboard: failed to get semantic jobs", "error", err)
				internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
					"error_type":  "semantic_search",
					"entity_type": "semantic_jobs",
					"criteria_id": criteriaID,
				})
				app.serverError(w, r, wrappedErr)
				return
			}

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

			// Add semantic jobs that don't have UserJob records, preserving order
			for _, scoredJob := range semanticJobs {
				if !existingJobIDs[scoredJob.JobDetail.ID] {
					semanticJobsSlice = append(semanticJobsSlice, JobWithDetails{
						ScoredJobDetail: scoredJob,
						IsSemanticOnly:  true,
					})
				}
			}

			app.logger.Debug("showDashboard: semantic search completed", "duration", time.Since(start).String(), "found", len(semanticJobs), "new", len(semanticJobsSlice))
		} else {
			app.logger.Info("showDashboard: user has no subscriptions, showing empty new jobs")
		}
	} else {
		// For other statuses, use existing UserJob queries
		start = time.Now()
		userJobs, _, err := internal.GetUserJobsSorted(r.Context(), app.fs, user.ID, internal.JobStatus(status), pageSize, pageToken, showAllRatings)
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
					job.JobDetail = *detail
					jobs[detail.ID] = job
				} else {
					app.logger.Warn("showDashboard: job detail found without corresponding user job", "job_id", detail.ID)
				}
			}
		}
	}

	// Calculate pagination information
	currentPage := 1
	if pageToken != "" {
		// If we have a page token, we're not on the first page
		// The exact page number would require additional queries, so we just indicate > 1
		currentPage = 2 // This is a simplification, actual page number would need more tracking
	}

	totalPages := (int(statusJobCount) + pageSize - 1) / pageSize // Ceiling division

	// Add data to template
	data["CurrentStatus"] = user.State.Status.Description()
	data["CurrentTime"] = time.Now()

	// Set jobs data and HasJobs based on status type
	if status == internal.JobStatusNew {
		data["HasJobs"] = len(semanticJobsSlice) > 0 || statusJobCount > 0
		data["Jobs"] = semanticJobsSlice
	} else {
		data["HasJobs"] = len(jobs) > 0 || statusJobCount > 0
		data["Jobs"] = jobs
	}

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
		"new": "New Jobs",
		// "viewed":   "Viewed Jobs",
		"saved":    "Saved Jobs",
		"applied":  "Applied Jobs",
		"rejected": "Rejected Jobs",
	}

	// Add showAllRatings to template data
	data["ShowAllRatings"] = showAllRatings

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
