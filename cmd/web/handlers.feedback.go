package main

import (
	"net/http"
	"talent-rodeo/internal"
	"time"
)

// handleJobFeedback handles POST requests to /feedback/job.
// It processes job feedback submissions via HTMX from the dashboard.
//
// Expected form parameters:
//   - job_id: The ID of the job being rated
//   - item_id: The item identifier (e.g., "resp_0", "skill_1", "req_2")
//   - item_text: The text content of the feedback item
//   - feedback_type: Either "like" or "dislike"
//
// Flow:
//  1. Validates user authentication
//  2. Extracts and validates form parameters
//  3. Logs the feedback data
//  4. Returns success response for HTMX
//
// Routes:
//   - POST /feedback/job
//
// Requires: Authenticated user (handled by middleware)
func (app *application) handleJobFeedback(w http.ResponseWriter, r *http.Request) {
	app.logger.Debug("handleJobFeedback", "method", r.Method, "url", r.URL.Path)
	// Get authenticated user
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.clientError(w, r, http.StatusUnauthorized)
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		app.logger.Error("Failed to parse feedback form", "error", err)
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	// Extract form parameters
	jobID := r.FormValue("job_id")
	itemID := r.FormValue("item_id")
	itemText := r.FormValue("item_text")
	feedbackType := r.FormValue("feedback_type")

	// Validate required parameters
	if jobID == "" || itemID == "" || itemText == "" || feedbackType == "" {
		app.logger.Error("Missing required feedback parameters",
			"job_id", jobID,
			"item_id", itemID,
			"item_text_length", len(itemText),
			"feedback_type", feedbackType)
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	// Validate feedback type
	if feedbackType != string(internal.LikeFeedbackType) && feedbackType != string(internal.DislikeFeedbackType) {
		app.logger.Error("Invalid feedback type", "feedback_type", feedbackType)
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	// Convert feedback type to boolean for logging clarity
	liked := feedbackType == string(internal.LikeFeedbackType)

	// Check if feedback already exists
	feedbackID := user.ID + "-" + jobID + "-" + itemID
	existingFeedback, exists, err := internal.GetUserFeedback(r.Context(), app.fs, feedbackID)
	if err != nil {
		app.logger.Error("Failed to check existing feedback",
			"error", err,
			"feedback_id", feedbackID)
		app.serverError(w, r, err)
		return
	}

	// If feedback exists and is the same type, delete it (deselect)
	if exists && existingFeedback.FeedbackType == internal.FeedbackType(feedbackType) {
		if err := internal.DeleteUserFeedbackByID(r.Context(), app.fs, feedbackID); err != nil {
			app.logger.Error("Failed to delete user feedback",
				"error", err,
				"feedback_id", feedbackID)
			app.serverError(w, r, err)
			return
		}

		// Special handling for job title feedback deselection - delete UserJob record
		if itemID == "job-title" {
			// Check if UserJob exists for this job
			existingUserJob, userJobExists, err := internal.GetUserJob(r.Context(), app.fs, user.ID, jobID)
			if err != nil {
				app.logger.Error("Failed to check existing UserJob for deselection",
					"error", err,
					"user_id", user.ID,
					"job_id", jobID)
				// Don't fail the whole request for this, continue with feedback deletion
			} else if userJobExists {
				// Determine the expected status based on the feedback type being deselected
				var expectedStatus internal.JobStatus
				if feedbackType == string(internal.LikeFeedbackType) {
					expectedStatus = internal.JobStatusSaved
				} else if feedbackType == string(internal.DislikeFeedbackType) {
					expectedStatus = internal.JobStatusArchived
				}

				// Only delete the UserJob if it has the expected status
				// This prevents accidentally deleting UserJob records that were manually changed to other statuses
				if existingUserJob.Status == expectedStatus {
					if err := internal.DeleteUserJob(r.Context(), app.fs, user.ID, jobID); err != nil {
						app.logger.Error("Failed to delete UserJob for job title feedback deselection",
							"error", err,
							"user_id", user.ID,
							"job_id", jobID,
							"status", expectedStatus)
						// Don't fail the whole request for this
					} else {
						app.logger.Info("Deleted UserJob record for job title feedback deselection",
							"user_id", user.ID,
							"job_id", jobID,
							"status", expectedStatus,
							"action", "untoggled")
					}
				} else {
					app.logger.Info("UserJob status doesn't match expected status for deselection, keeping record",
						"user_id", user.ID,
						"job_id", jobID,
						"expected_status", expectedStatus,
						"actual_status", existingUserJob.Status)
				}
			}
		}

		app.logger.Info("Job feedback deselected",
			"user_id", user.ID,
			"job_id", jobID,
			"item_id", itemID,
			"feedback_type", feedbackType,
			"feedback_id", feedbackID)

		// For HTMX requests, return a simple success response that doesn't trigger navigation
		// Using 204 No Content to prevent HTMX from updating the DOM
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Create or update feedback record
	feedback := &internal.UserFeedback{
		UserID:       user.ID,
		JobID:        jobID,
		ItemID:       itemID,
		ItemText:     itemText,
		Liked:        liked,
		FeedbackType: internal.FeedbackType(feedbackType),
	}

	// Store the feedback in the database
	if err := internal.UpsertUserFeedback(r.Context(), app.fs, feedback); err != nil {
		app.logger.Error("Failed to store user feedback",
			"error", err,
			"user_id", user.ID,
			"job_id", jobID,
			"item_id", itemID)
		app.serverError(w, r, err)
		return
	}

	// Special handling for job title feedback - create UserJob record with appropriate status
	if itemID == "job-title" {
		var targetStatus internal.JobStatus
		var statusAction string
		var timeField *time.Time

		if feedbackType == string(internal.LikeFeedbackType) {
			targetStatus = internal.JobStatusSaved
			statusAction = "saved"
			now := time.Now()
			timeField = &now
		} else if feedbackType == string(internal.DislikeFeedbackType) {
			targetStatus = internal.JobStatusArchived
			statusAction = "archived"
			now := time.Now()
			timeField = &now
		}

		// Check if UserJob already exists
		_, exists, err := internal.GetUserJob(r.Context(), app.fs, user.ID, jobID)
		if err != nil {
			app.logger.Error("Failed to check existing UserJob",
				"error", err,
				"user_id", user.ID,
				"job_id", jobID)
			// Don't fail the whole request for this
		} else if !exists {
			// Create UserJob with appropriate status since it doesn't exist
			userJob := &internal.UserJob{
				ID:        jobID,
				Status:    targetStatus,
				JobRef:    app.fs.Collection("job_details").Doc(jobID),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			// Set the appropriate timestamp field
			if targetStatus == internal.JobStatusSaved {
				userJob.SavedAt = timeField
			} else if targetStatus == internal.JobStatusArchived {
				userJob.ArchivedAt = timeField
			}

			// Save the UserJob record
			_, err := app.fs.Collection("users").Doc(user.ID).Collection("user_jobs").Doc(jobID).Set(r.Context(), userJob)
			if err != nil {
				app.logger.Error("Failed to create UserJob for job title feedback",
					"error", err,
					"user_id", user.ID,
					"job_id", jobID,
					"status", targetStatus)
				// Don't fail the whole request for this
			} else {
				app.logger.Info("Created UserJob record for job title feedback",
					"user_id", user.ID,
					"job_id", jobID,
					"status", targetStatus,
					"action", statusAction,
					"item_text", itemText)
			}
		} else {
			// UserJob exists, update it to the new status
			if err := internal.UpdateUserJobStatus(r.Context(), app.fs, user.ID, jobID, targetStatus); err != nil {
				app.logger.Error("Failed to update UserJob status",
					"error", err,
					"user_id", user.ID,
					"job_id", jobID,
					"status", targetStatus)
				// Don't fail the whole request for this
			} else {
				app.logger.Info("Updated UserJob status for job title feedback",
					"user_id", user.ID,
					"job_id", jobID,
					"status", targetStatus,
					"action", statusAction,
					"item_text", itemText)
			}
		}
	}

	// Log the feedback data
	app.logger.Info("Job feedback received and stored",
		"user_id", user.ID,
		"job_id", jobID,
		"item_id", itemID,
		"item_text", itemText,
		"liked", liked,
		"feedback_type", feedbackType,
		"feedback_id", feedback.ID)

	// For HTMX requests, return a simple success response that doesn't trigger navigation
	// Using 204 No Content to prevent HTMX from updating the DOM
	w.WriteHeader(http.StatusNoContent)
}

// Add new handler for dismissing a job
func (app *application) handleJobDismiss(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.clientError(w, r, http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}
	jobID := r.FormValue("job_id")
	if jobID == "" {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}
	// Create or update UserJob with Dismissed status
	_, exists, err := internal.GetUserJob(r.Context(), app.fs, user.ID, jobID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !exists {
		userJob := &internal.UserJob{
			ID:        jobID,
			Status:    internal.JobStatusDismissed,
			JobRef:    app.fs.Collection("job_details").Doc(jobID),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_, err := app.fs.Collection("users").Doc(user.ID).Collection("user_jobs").Doc(jobID).Set(r.Context(), userJob)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
	} else {
		err := internal.UpdateUserJobStatus(r.Context(), app.fs, user.ID, jobID, internal.JobStatusDismissed)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Add new handler for job-title save/archive (no feedback record)
func (app *application) handleJobTitleAction(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.clientError(w, r, http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}
	jobID := r.FormValue("job_id")
	action := r.FormValue("action") // 'like', 'dislike', or 'unset'
	typeVal := r.FormValue("type")  // for unset
	if jobID == "" || action == "" {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}
	var targetStatus internal.JobStatus
	if action == "like" {
		targetStatus = internal.JobStatusSaved
	} else if action == "dislike" {
		targetStatus = internal.JobStatusArchived
	} else if action == "unset" {
		// Remove UserJob if it matches the status being unset
		existingUserJob, exists, err := internal.GetUserJob(r.Context(), app.fs, user.ID, jobID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		var expectedStatus internal.JobStatus
		if typeVal == "like" {
			expectedStatus = internal.JobStatusSaved
		} else if typeVal == "dislike" {
			expectedStatus = internal.JobStatusArchived
		}
		if exists && existingUserJob.Status == expectedStatus {
			err := internal.DeleteUserJob(r.Context(), app.fs, user.ID, jobID)
			if err != nil {
				app.serverError(w, r, err)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return
	} else {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}
	// Create or update UserJob
	_, exists, err := internal.GetUserJob(r.Context(), app.fs, user.ID, jobID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	now := time.Now()
	if !exists {
		userJob := &internal.UserJob{
			ID:        jobID,
			Status:    targetStatus,
			JobRef:    app.fs.Collection("job_details").Doc(jobID),
			CreatedAt: now,
			UpdatedAt: now,
		}
		if targetStatus == internal.JobStatusSaved {
			userJob.SavedAt = &now
		} else if targetStatus == internal.JobStatusArchived {
			userJob.ArchivedAt = &now
		}
		_, err := app.fs.Collection("users").Doc(user.ID).Collection("user_jobs").Doc(jobID).Set(r.Context(), userJob)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
	} else {
		err := internal.UpdateUserJobStatus(r.Context(), app.fs, user.ID, jobID, targetStatus)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
