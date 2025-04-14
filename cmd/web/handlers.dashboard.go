package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/search"
	"time"

	"github.com/getsentry/sentry-go"
)

// JobWithDetails combines UserJob with the job details
type JobWithDetails struct {
	*internal.UserJob
	*search.JobDetail
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
	if j.UserJob.CompanyName != "" {
		return j.UserJob.CompanyName
	}
	if j.JobDetail != nil && j.JobDetail.Company != "" {
		return j.JobDetail.Company
	}
	return ""
}

// Summary returns the job summary
func (j *JobWithDetails) Summary() string {
	if j.JobDetail != nil && j.JobDetail.ExtractedData != nil {
		return j.JobDetail.ExtractedData.Summary
	}
	// Fallback to full description if summary is not available
	if j.JobDetail != nil {
		return j.JobDetail.FullDescription
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

// showDashboard handles GET requests to /dashboard.
func (app *application) showDashboard(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.logger.Warn("showDashboard: user not in context")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if user.ID == "" {
		app.logger.Warn("showDashboard: user ID is empty")
		internal.SentryReportError(r.Context(), fmt.Errorf("user ID is empty"), sentry.LevelError, map[string]string{
			"error_type":  "user_id_empty",
			"entity_type": "user",
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if user.State.Status < internal.UserStatusCallScheduled {
		app.logger.Warn("showDashboard: user has not onboarded", "user_id", user.ID, "status", user.State.Status.String())
		internal.SentryReportError(r.Context(), fmt.Errorf("user has not onboarded"), sentry.LevelError, map[string]string{
			"user_status": user.State.Status.String(),
			"error_type":  "user_not_onboarded",
			"entity_type": "user",
		})
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}

	internal.SentrySetScope(r.Context(), map[string]any{
		"user_status": user.State.Status.String(),
	})

	app.logger.Debug("showDashboard", "user_id", user.ID, "status", user.State.Status.String())

	// If user is in call_scheduled state, check if they have any jobs
	if user.State.Status == internal.UserStatusCallScheduled {
		// Use the more efficient UserHasAnyJobs function instead of retrieving all counts
		start := time.Now()
		hasJobs, err := internal.UserHasAnyJobs(r.Context(), app.fs, user.ID)
		if err != nil {
			wrappedErr := fmt.Errorf("showDashboard: failed to check if user has jobs: %w", err)
			app.logger.Error("showDashboard: failed to check if user has jobs", "error", err)
			internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
				"error_type":  "database_retrieval",
				"entity_type": "user_jobs_check",
			})
			app.serverError(w, r, wrappedErr)
			return
		}
		app.logger.Debug("showDashboard: UserHasAnyJobs took", "duration", time.Since(start).String())

		// If they don't have any jobs, show the waiting for call page
		if !hasJobs {
			app.showWaitingForCall(w, r, user)
			return
		}
	}

	// Get search status information
	nextSearchTime, isSearching := app.search.GetNextSearchTime(user)

	// TEMPORARY: For testing search states
	testMode := r.URL.Query().Get("test_mode")
	if testMode != "" {
		switch testMode {
		case "searching":
			isSearching = true
			app.logger.Info("TEST MODE: Setting isSearching to true")
		case "scheduled_soon":
			isSearching = false
			nextSearchTime = time.Now().Add(2 * time.Minute)
			app.logger.Info("TEST MODE: Setting nextSearchTime to 2 minutes from now", "time", nextSearchTime)
		case "scheduled_future":
			isSearching = false
			nextSearchTime = time.Now().Add(24 * time.Hour)
			app.logger.Info("TEST MODE: Setting nextSearchTime to 24 hours from now", "time", nextSearchTime)
		case "no_schedule":
			isSearching = false
			nextSearchTime = time.Time{} // Zero time
			app.logger.Info("TEST MODE: Setting nextSearchTime to zero time")
		}

		// Add test mode to template data for links
		data["TestMode"] = testMode
	}

	// Use our helper function to parse and validate the status
	status, isValid := internal.ParseJobStatus(r.URL.Query().Get("status"))
	if !isValid {
		status = internal.JobStatusNew
	}

	// Get the showAllRatings parameter from the query string
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
	jobs, nextPageToken, err := internal.GetUserJobsSorted(r.Context(), app.fs, user.ID, internal.JobStatus(status), pageSize, pageToken, showAllRatings)
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

	// Fetch job details for the filtered jobs
	var jobsWithDetails []*JobWithDetails
	for _, job := range jobs {
		// Fetch job details from job_ref
		var details JobWithDetails
		details.UserJob = job

		docSnap, err := job.JobRef.Get(r.Context())
		if err != nil {
			wrappedErr := fmt.Errorf("showDashboard: failed to get job details: %w", err)
			app.logger.Error("showDashboard: failed to get job details", "error", err)

			internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
				"error_type":  "database_retrieval",
				"entity_type": "job_details",
			})

			app.serverError(w, r, wrappedErr)
			return
		}

		var jobDetail search.JobDetail
		if err := docSnap.DataTo(&jobDetail); err != nil {
			wrappedErr := fmt.Errorf("showDashboard: failed to unmarshal job details: %w", err)
			app.logger.Error("showDashboard: failed to unmarshal job details", "error", err)

			internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
				"error_type":  "data_processing",
				"entity_type": "job_details",
			})

			app.serverError(w, r, wrappedErr)
			return
		}
		details.JobDetail = &jobDetail

		jobsWithDetails = append(jobsWithDetails, &details)
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
	data["IsSearching"] = isSearching
	data["NextSearchTime"] = nextSearchTime
	data["CurrentTime"] = time.Now()
	data["HasJobs"] = len(jobsWithDetails) > 0 || statusJobCount > 0
	data["ActiveStatus"] = string(status)
	data["Jobs"] = jobsWithDetails
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

	// Find the most recent active appointment
	var activeAppointment *internal.Appointment
	for _, appt := range appointments {
		if appt.Status == internal.AppointmentStatusScheduled || appt.Status == internal.AppointmentStatusRunning {
			if activeAppointment == nil || appt.ScheduledTime.After(activeAppointment.ScheduledTime) {
				activeAppointment = appt
			}
		}
	}

	if activeAppointment == nil {
		user.State.Status = internal.UserStatusResumeUploaded
		if err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusResumeUploaded); err != nil {
			wrappedErr := fmt.Errorf("showDashboard: failed to update user status: %w", err)
			app.logger.Error("showDashboard: failed to update user status", "error", err)

			internal.SentryReportError(r.Context(), wrappedErr, sentry.LevelError, map[string]string{
				"error_type":  "database_update",
				"entity_type": "user_status",
			})

			app.serverError(w, r, wrappedErr)
			return
		}

		// we expected the user to have an appointment, but they don't
		// so we redirect to the profile page
		app.logger.Error(internal.Red("showDashboard: no active appointment found when showing dashboard"), "user_id", user.ID)
		internal.SentryReportError(r.Context(), fmt.Errorf("no active appointment found when showing dashboard"), sentry.LevelError, map[string]string{
			"error_type":  "no_active_appointment",
			"entity_type": "appointments",
		})
		// we expected the user to have an appointment, but they don't
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
