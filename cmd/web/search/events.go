package search

import (
	"context"
	"fmt"
	"talent-rodeo/internal"
)

const (
	userStateUpdateEvent = "state-update"
	newJobEvent          = "new-job"
)

// PusherInterface defines the interface for Pusher client
type PusherInterface interface {
	Trigger(channel string, eventName string, data interface{}) error
}

// OnNewJob is called by the scrapingdog provider when a new job is found and saved
func (s *Service) OnNewJob(userID, jobID string) error {
	s.logger.Debug(internal.Blue("received new-job callback"),
		"user_id", userID,
		"job_id", jobID,
		"channel", fmt.Sprintf("user-%s", userID))

	// Create a bare-bones record first
	job := &internal.UserJob{
		ID:     jobID,
		Status: internal.JobStatusUnprocessed,
		JobRef: s.fs.Collection("job_details").Doc(jobID),
	}

	if err := internal.SaveUserJob(context.Background(), s.fs, userID, job); err != nil {
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

	return nil
}

// notifyUserSearchCompleted sends a search completed event through Pusher
func (s *Service) notifyUserSearchCompleted(userID string, totalJobs int, errors []error) error {
	s.logger.Debug("notifyUserSearchCompleted - sending pusher event",
		"user_id", userID,
		"total_jobs", totalJobs,
		"error_count", len(errors),
		"channel", fmt.Sprintf("user-%s", userID))

	data := map[string]interface{}{
		"total_jobs":  totalJobs,
		"status_text": "Job Search Complete",
		"description": fmt.Sprintf("Search completed with %d jobs found", totalJobs),
	}

	// If there were errors, include them in the notification
	if len(errors) > 0 {
		errorMsgs := make([]string, len(errors))
		for i, err := range errors {
			errorMsgs[i] = err.Error()
		}
		data["errors"] = errorMsgs
	}

	if err := s.pusher.Trigger(fmt.Sprintf("user-%s", userID), "state-update", data); err != nil {
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
