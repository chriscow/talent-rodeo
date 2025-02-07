package search

import "context"

// JobProvider defines the interface that all job listing providers must implement.
// This interface is designed to be generic enough to support multiple providers
// while maintaining consistency in how we interact with them.
//
// Future Considerations:
// - Phase 5 will introduce async processing via NATS/Watermill
// - Rate limiting and throttling will be needed per provider
// - Caching layer may be added for frequently accessed data
// - Error handling will need to account for provider-specific error types
type JobProvider interface {
	// SearchJobs performs a search for jobs based on the provided criteria.
	// This is a synchronous operation for Phase 1, but will be converted to
	// async in Phase 5 using NATS/Watermill for better scalability.
	SearchJobs(ctx context.Context, criteria Criteria) ([]Job, error)

	// GetJobDetails retrieves detailed information about a specific job.
	// The jobID format may vary by provider, so implementations should handle
	// provider-specific ID formats internally.
	GetJobDetails(ctx context.Context, jobID string) (*JobDetail, error)

	// Name returns the unique identifier for this provider.
	// This is used for logging, metrics, and provider-specific configurations.
	Name() string

	// Validate verifies that the provider is properly configured and can make
	// API calls. This should check API keys, connection settings, etc.
	Validate(ctx context.Context) error
}
