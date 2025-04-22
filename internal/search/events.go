package search

// JobEventHandler defines the interface for handling job-related events
type JobEventHandler interface {
	OnNewJob(userID string, criteria Criteria, jobID string, details JobDetail) error
	OnSearchCompleted(userID string, criteria Criteria, totalJobs int, errors []error) error
}
