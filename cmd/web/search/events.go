package search

import (
	"context"
	"fmt"
	"sync"
	"talent-rodeo/internal"
	"time"
)

const (
	userStateUpdateEvent = "state-update"
	newJobEvent          = "new-job"
	jobAnalysisEvent     = "job-analysis"
)

// State update constants
const (
	StatusSearchScheduled  = "Job Search Scheduled"
	StatusSearchQueued     = "Job Search Queued"
	StatusSearchStarted    = "Job Search Started"
	StatusSearchInProgress = "Job Search In Progress"
	StatusAnalysisStarted  = "Job Analysis Started"
	StatusAnalysisProgress = "Job Analysis In Progress"
	StatusAnalysisComplete = "Job Analysis Complete"
	StatusSearchComplete   = "Job Search Complete"
)

// PusherInterface defines the interface for Pusher client
type PusherInterface interface {
	Trigger(channel string, eventName string, data interface{}) error
}

// SearchProgress tracks the progress of a user's search and analysis
type SearchProgress struct {
	UserID             string
	TotalJobsFound     int
	JobsAnalyzed       int
	JobsMatched        int
	JobsNotMatched     int
	SearchCompleted    bool
	AnalysisInProgress bool
	mu                 sync.Mutex // Protects concurrent access
}

// User search progress tracking
var userSearchProgress = make(map[string]*SearchProgress)
var progressMutex sync.Mutex

// getOrCreateProgress gets or creates a progress tracker for a user
func (s *Service) getOrCreateProgress(userID string) *SearchProgress {
	// If cache is available, use it
	if s.progressCache != nil {
		return s.progressCache.GetOrCreate(userID)
	}

	// Fallback to map-based implementation if cache is not available
	progressMutex.Lock()
	defer progressMutex.Unlock()

	if progress, ok := userSearchProgress[userID]; ok {
		return progress
	}

	progress := &SearchProgress{
		UserID:             userID,
		TotalJobsFound:     0,
		JobsAnalyzed:       0,
		JobsMatched:        0,
		JobsNotMatched:     0,
		SearchCompleted:    false,
		AnalysisInProgress: false,
	}
	userSearchProgress[userID] = progress
	return progress
}

// updateProgress safely updates the search progress for a user
func (s *Service) updateProgress(userID string, updateFn func(*SearchProgress)) {
	// If cache is available, use it
	if s.progressCache != nil {
		// This handles getting/creating the progress, locking, and saving back to cache
		progress := s.progressCache.UpdateProgress(userID, updateFn)
		// Send notification after update
		s.notifyAnalysisProgressUpdate(userID, progress)
		return
	}

	// Fallback to map-based implementation
	progressMutex.Lock()
	defer progressMutex.Unlock()

	progress, ok := userSearchProgress[userID]
	if !ok {
		progress = &SearchProgress{
			UserID:             userID,
			TotalJobsFound:     0,
			JobsAnalyzed:       0,
			JobsMatched:        0,
			JobsNotMatched:     0,
			SearchCompleted:    false,
			AnalysisInProgress: false,
		}
		userSearchProgress[userID] = progress
	}

	progress.mu.Lock()
	defer progress.mu.Unlock()

	updateFn(progress)

	// Send notification if needed
	s.notifyAnalysisProgressUpdate(userID, progress)
}

// cleanupProgress removes progress tracking for a user when search and analysis are complete
func (s *Service) cleanupProgress(userID string) {
	// If cache is available, use it
	if s.progressCache != nil {
		s.progressCache.Delete(userID)
		s.logger.Debug("cleaned up progress tracking data from cache", "user_id", userID)
		return
	}

	// Fallback to map-based implementation
	progressMutex.Lock()
	defer progressMutex.Unlock()

	delete(userSearchProgress, userID)
	s.logger.Debug("cleaned up progress tracking data from map", "user_id", userID)
}

// OnNewJob is called by the scrapingdog provider when a new job is found and saved
func (s *Service) OnNewJob(userID, jobID string) error {
	s.logger.Debug(internal.Blue("received new-job callback"),
		"user_id", userID,
		"job_id", jobID,
		"channel", fmt.Sprintf("user-%s", userID))

	// Update the search progress tracking
	s.updateProgress(userID, func(progress *SearchProgress) {
		progress.TotalJobsFound++

		// If this is the first job found, notify that analysis has started
		if progress.TotalJobsFound == 1 || !progress.AnalysisInProgress {
			progress.AnalysisInProgress = true
		}
	})

	// Create a bare-bones record first
	job := &internal.UserJob{
		ID:     jobID,
		Status: internal.JobStatusUnprocessed,
		JobRef: s.fs.Collection("job_details").Doc(jobID),
	}

	if err := internal.CreateUserJob(context.Background(), s.fs, userID, job); err != nil {
		s.logger.Error("failed to create bare-bones job record",
			"error", err,
			"user_id", userID,
			"job_id", jobID)
		return fmt.Errorf("failed to create bare-bones job record: %w", err)
	}

	// Queue the job for processing
	s.jobQueue <- QueuedJob{
		UserID: userID,
		JobID:  jobID,
	}

	return nil
}

// notifyUserSearchQueued sends a notification that the search has been queued
func (s *Service) notifyUserSearchQueued(userID string) error {
	s.logger.Debug("notifying user search queued", "user_id", userID)

	data := map[string]interface{}{
		"status_text": StatusSearchQueued,
		"description": "Your job search has been queued and will begin processing shortly.",
		"phase":       "search_queued",
	}

	if err := s.pusher.Trigger(fmt.Sprintf("user-%s", userID), userStateUpdateEvent, data); err != nil {
		s.logger.Error("failed to send search queued event", "error", err, "user_id", userID)
		return fmt.Errorf("failed to send search queued event: %w", err)
	}

	return nil
}

// notifyUserSearchStarted sends a notification that the search has started
func (s *Service) notifyUserSearchStarted(userID string) error {
	s.logger.Debug("notifying user search started", "user_id", userID)

	data := map[string]interface{}{
		"status_text": StatusSearchStarted,
		"description": "Your job search has started. We're looking for jobs that match your criteria.",
		"phase":       "search_started",
	}

	if err := s.pusher.Trigger(fmt.Sprintf("user-%s", userID), userStateUpdateEvent, data); err != nil {
		s.logger.Error("failed to send search started event", "error", err, "user_id", userID)
		return fmt.Errorf("failed to send search started event: %w", err)
	}

	return nil
}

// notifyUserSearchInProgress sends an update on search progress
func (s *Service) notifyUserSearchInProgress(userID string) error {
	s.logger.Debug("notifying user of search progress", "user_id", userID)

	// Get current progress
	progress := s.getOrCreateProgress(userID)
	progress.mu.Lock()
	jobsFound := progress.TotalJobsFound
	progress.mu.Unlock()

	data := map[string]interface{}{
		"status_text":   StatusSearchInProgress,
		"description":   fmt.Sprintf("Searching for jobs... Found %d jobs so far.", jobsFound),
		"phase":         "search_in_progress",
		"jobs_found":    jobsFound,
		"jobs_analyzed": 0, // None analyzed yet
	}

	if err := s.pusher.Trigger(fmt.Sprintf("user-%s", userID), userStateUpdateEvent, data); err != nil {
		s.logger.Error("failed to send search progress event", "error", err, "user_id", userID)
		return fmt.Errorf("failed to send search progress event: %w", err)
	}

	return nil
}

// notifyUserSearchError sends a notification about search errors through Pusher
func (s *Service) notifyUserSearchError(userID string, errorMsg string) error {
	s.logger.Debug("notifying user of search error",
		"user_id", userID,
		"error", errorMsg)

	data := map[string]interface{}{
		"status_text": "Job Search Error",
		"description": "We've encountered an error with your job search. Our team has been notified and is working to resolve it.",
		"phase":       "search_error",
		"error":       errorMsg,
	}

	if err := s.pusher.Trigger(fmt.Sprintf("user-%s", userID), userStateUpdateEvent, data); err != nil {
		s.logger.Error("failed to send search error event", "error", err, "user_id", userID)
		return fmt.Errorf("failed to send search error event: %w", err)
	}

	return nil
}

// notifyUserAnalysisStarted sends notification that job analysis has started
func (s *Service) notifyUserAnalysisStarted(userID string, jobsFound int) error {
	s.logger.Debug("notifying user analysis started", "user_id", userID, "total_jobs", jobsFound)

	data := map[string]interface{}{
		"status_text":      StatusAnalysisStarted,
		"description":      fmt.Sprintf("Analysis started on %d jobs to find your best matches.", jobsFound),
		"phase":            "analysis_started",
		"jobs_found":       jobsFound,
		"jobs_analyzed":    0,
		"jobs_matched":     0,
		"jobs_not_matched": 0,
	}

	if err := s.pusher.Trigger(fmt.Sprintf("user-%s", userID), userStateUpdateEvent, data); err != nil {
		s.logger.Error("failed to send analysis started event", "error", err, "user_id", userID)
		return fmt.Errorf("failed to send analysis started event: %w", err)
	}

	return nil
}

// notifyAnalysisProgressUpdate sends an update on analysis progress based on current state
func (s *Service) notifyAnalysisProgressUpdate(userID string, progress *SearchProgress) error {
	// Don't send a notification if no jobs have been found yet
	if progress.TotalJobsFound == 0 {
		return nil
	}

	// Determine the appropriate status text and phase
	statusText := StatusAnalysisProgress
	phase := "analysis_in_progress"

	if progress.SearchCompleted && progress.JobsAnalyzed >= progress.TotalJobsFound {
		// If all jobs have been analyzed and search is completed
		statusText = StatusAnalysisComplete
		phase = "analysis_complete"

		// Check if we should clean up the progress data
		// We can clean up when:
		// 1. Search is completed
		// 2. All jobs have been analyzed
		// 3. Total jobs found is greater than 0
		if progress.SearchCompleted && progress.JobsAnalyzed >= progress.TotalJobsFound &&
			progress.TotalJobsFound > 0 {
			// Use a goroutine to clean up after notification is sent
			go func() {
				// Add a small delay to ensure any final notifications are processed
				time.Sleep(5 * time.Second)

				// Check if there are any pending jobs for this user before cleanup
				ctx := context.Background()
				jobsCount, err := s.countPendingJobsForUser(ctx, userID)
				if err != nil {
					s.logger.Error("failed to count pending jobs before cleanup",
						"error", err,
						"user_id", userID)
					return
				}

				// Only clean up if there are no pending jobs
				if jobsCount == 0 {
					s.cleanupProgress(userID)
					s.logger.Debug("cleaned up progress tracking data after completion",
						"user_id", userID,
						"analyzed", progress.JobsAnalyzed,
						"total", progress.TotalJobsFound)
				} else {
					s.logger.Debug("skipped progress cleanup - user still has pending jobs",
						"user_id", userID,
						"pending_jobs", jobsCount)
				}
			}()
		}
	} else if !progress.AnalysisInProgress {
		// If analysis hasn't started yet
		statusText = StatusSearchInProgress
		phase = "search_in_progress"
	} else if !progress.SearchCompleted {
		// If jobs are still being found and analyzed simultaneously
		statusText = StatusAnalysisProgress
		phase = "analysis_in_progress"
	}

	// Create a description based on the current state
	var description string
	switch phase {
	case "search_in_progress":
		description = fmt.Sprintf("Searching for jobs... Found %d jobs so far.", progress.TotalJobsFound)
	case "analysis_in_progress":
		if progress.SearchCompleted {
			description = fmt.Sprintf("Analyzing jobs (%d of %d complete)...", progress.JobsAnalyzed, progress.TotalJobsFound)
		} else {
			description = fmt.Sprintf("Found %d jobs so far. Analyzed %d jobs (%d matches).",
				progress.TotalJobsFound, progress.JobsAnalyzed,
				progress.JobsMatched)
		}
	case "analysis_complete":
		description = fmt.Sprintf("Analysis complete! Found %d good matches out of %d jobs.",
			progress.JobsMatched, progress.TotalJobsFound)
	}

	// Calculate progress percentage
	progressPercentage := 0
	if progress.TotalJobsFound > 0 {
		progressPercentage = (progress.JobsAnalyzed * 100) / progress.TotalJobsFound
		// Cap at 99% if search is still in progress
		if !progress.SearchCompleted && progressPercentage >= 99 {
			progressPercentage = 99
		}
	}

	data := map[string]interface{}{
		"status_text":         statusText,
		"description":         description,
		"phase":               phase,
		"jobs_found":          progress.TotalJobsFound,
		"jobs_analyzed":       progress.JobsAnalyzed,
		"jobs_matched":        progress.JobsMatched,
		"jobs_not_matched":    progress.JobsNotMatched,
		"progress_percentage": progressPercentage,
		"search_completed":    progress.SearchCompleted,
	}

	s.logger.Debug("sending analysis progress update",
		"user_id", userID,
		"status", statusText,
		"analyzed", progress.JobsAnalyzed,
		"total", progress.TotalJobsFound,
		"matched", progress.JobsMatched)

	if err := s.pusher.Trigger(fmt.Sprintf("user-%s", userID), userStateUpdateEvent, data); err != nil {
		s.logger.Error("failed to send analysis progress event", "error", err, "user_id", userID)
		return fmt.Errorf("failed to send analysis progress event: %w", err)
	}

	return nil
}

// recordJobAnalyzed updates the progress when a job has been analyzed
func (s *Service) recordJobAnalyzed(userID string, isMatch bool) {
	s.updateProgress(userID, func(progress *SearchProgress) {
		progress.JobsAnalyzed++
		if isMatch {
			progress.JobsMatched++
		} else {
			progress.JobsNotMatched++
		}
	})
}

// recordSearchCompleted marks a search as completed in the progress tracker
func (s *Service) recordSearchCompleted(userID string, totalJobs int) {
	s.updateProgress(userID, func(progress *SearchProgress) {
		progress.SearchCompleted = true
		// Update total if needed (in case we missed some job callbacks)
		if totalJobs > progress.TotalJobsFound {
			progress.TotalJobsFound = totalJobs
		}
	})
}

// After an LLM has concluded that a job is a good fit, we notify the user
func (s *Service) notifyUserNewJob(userID string, errors []error) error {
	s.logger.Debug("notifyUserNewJob - sending pusher event",
		"user_id", userID,
		"error_count", len(errors),
		"channel", fmt.Sprintf("user-%s", userID))

	data := map[string]interface{}{
		"count": 1,
	}
	if err := s.pusher.Trigger(fmt.Sprintf("user-%s", userID), "new-job", data); err != nil {
		s.logger.Error("failed to send new-job event",
			"error", err,
			"user_id", userID)
		return fmt.Errorf("failed to send new-job event: %w", err)
	}
	s.logger.Debug("successfully sent new-job event",
		"user_id", userID)
	return nil
}

// OnSearchCompleted sends a search completed event through Pusher
func (s *Service) OnSearchCompleted(userID string, totalJobs int, errors []error) error {
	s.logger.Debug(internal.BGGreen("received search completed callback"),
		"user_id", userID,
		"total_jobs", totalJobs,
		"error_count", len(errors),
		"channel", fmt.Sprintf("user-%s", userID))

	// Record that the search is completed in our progress tracker
	s.recordSearchCompleted(userID, totalJobs)

	// Notify the user that search is complete
	return s.notifyUserSearchCompleted(userID, totalJobs, errors)
}

// notifyUserSearchCompleted sends a search completed event through Pusher
func (s *Service) notifyUserSearchCompleted(userID string, totalJobs int, errors []error) error {
	s.logger.Debug("notifyUserSearchCompleted - sending pusher event",
		"user_id", userID,
		"total_jobs", totalJobs,
		"error_count", len(errors),
		"channel", fmt.Sprintf("user-%s", userID))

	// Get current progress data for richer notification
	progress := s.getOrCreateProgress(userID)
	progress.mu.Lock()
	jobsAnalyzed := progress.JobsAnalyzed
	jobsMatched := progress.JobsMatched
	progress.mu.Unlock()

	data := map[string]interface{}{
		"total_jobs":  totalJobs,
		"status_text": StatusSearchComplete,
		"description": fmt.Sprintf("Search completed! Found %d jobs. Analyzed %d so far with %d matches.",
			totalJobs, jobsAnalyzed, jobsMatched),
		"phase":         "search_complete",
		"jobs_found":    totalJobs,
		"jobs_analyzed": jobsAnalyzed,
		"jobs_matched":  jobsMatched,
	}

	// If there were errors, include them in the notification
	if len(errors) > 0 {
		errorMsgs := make([]string, len(errors))
		for i, err := range errors {
			errorMsgs[i] = err.Error()
		}
		data["errors"] = errorMsgs
	}

	if err := s.pusher.Trigger(fmt.Sprintf("user-%s", userID), userStateUpdateEvent, data); err != nil {
		s.logger.Error("failed to send search completed event",
			"error", err,
			"user_id", userID,
			"total_jobs", totalJobs)
		return fmt.Errorf("failed to send search completed event: %w", err)
	}

	s.logger.Debug("successfully sent search completed event",
		"user_id", userID,
		"total_jobs", totalJobs)
	return nil
}

// Helper function to calculate percentage
func calculatePercentage(current, total int) int {
	if total == 0 {
		return 0
	}
	return (current * 100) / total
}
