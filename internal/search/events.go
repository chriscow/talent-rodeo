package search

// JobEventHandler defines the interface for handling job-related events
type JobEventHandler interface {
	OnNewJob(userID, jobID string) error
	OnSearchCompleted(userID string, totalJobs int, errors []error) error
}
