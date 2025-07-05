package internal

import "context"

// JobProvider interface defines methods that must be implemented by all job providers
type JobProvider interface {
	Name() string
	Validate(ctx context.Context) error
	SearchJobListings(ctx context.Context, criteria SearchCriteria) ([]JobListing, error)
	GetJobDetails(ctx context.Context, userID *string, jobID string) (*JobDetail, error)
}
