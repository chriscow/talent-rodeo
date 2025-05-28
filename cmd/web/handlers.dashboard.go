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

// JobWithDetails combines UserJob with the job details
type JobWithDetails struct {
	*internal.UserJob
	*internal.JobDetail
}

// CreatedAt returns when the job was added to the user's list
func (j *JobWithDetails) CreatedAt() time.Time {
	return j.UserJob.CreatedAt
}

// Title returns the job title for display
func (j *JobWithDetails) Title() string {
	// Use UserJob title first if available (from job search),
	// otherwise fallback to JobDetail title
	if j.JobDetail != nil && j.JobDetail.Title != "" {
		return j.JobDetail.Title
	}
	return ""
}

// CompanyName returns the company name for display
func (j *JobWithDetails) CompanyName() string {
	// Use UserJob company name if available,
	// otherwise fallback to JobDetail company

	return ""
}

// Summary returns the job summary
func (j *JobWithDetails) Summary() string {
	if j.JobDetail != nil && j.JobDetail.ExtractedData != nil {
		return j.JobDetail.ExtractedData.Summary
	}
	// Fallback to full description if summary is not available
	if j.JobDetail != nil {
		return j.JobDetail.Description
	}
	return ""
}

// Qualifications returns the job qualifications
func (j *JobWithDetails) Qualifications() []string {
	if j.JobDetail != nil && j.JobDetail.ExtractedData != nil {
		if len(j.JobDetail.ExtractedData.Qualifications) > 0 {
			return j.JobDetail.ExtractedData.Qualifications
		}
		// Fallback to regular qualifications if summary qualifications not available
		if len(j.JobDetail.Qualifications) > 0 {
			return j.JobDetail.Qualifications
		}
	}
	return []string{}
}

func (j *JobWithDetails) Requirements() []string {
	if j.JobDetail != nil && j.JobDetail.ExtractedData != nil {
		return j.JobDetail.ExtractedData.Requirements
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

	// Get paginated jobs by status
	start = time.Now()
	userJobs, nextPageToken, err := internal.GetUserJobsSorted(r.Context(), app.fs, user.ID, internal.JobStatus(status), pageSize, pageToken, showAllRatings)
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

	jobs := map[string]JobWithDetails{}
	for _, job := range userJobs {
		jobs[job.JobRef.ID] = JobWithDetails{
			UserJob: job,
		}
	}

	jobIDs := lo.Keys(jobs)

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
	data["HasJobs"] = len(jobs) > 0 || statusJobCount > 0
	data["ActiveStatus"] = string(status)
	data["Jobs"] = jobs
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
		"viewed":   "Viewed Jobs",
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
