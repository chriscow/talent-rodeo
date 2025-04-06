package search

import "context"

type JobQueue interface {
	QueueSearch(ctx context.Context, userID string, criteria Criteria) error
	GetQueueDepth(ctx context.Context) (int, error)
}

// JobProvider defines the interface that all job listing providers must implement.
// This interface is designed to be generic enough to support multiple providers
// while maintaining consistency in how we interact with them.
//
// Future Considerations:
// - Phase 5 will introduce async processing via NATS/Watermill
// - Rate limiting and throttling will be needed per provider
// - Caching layer may be added for frequently accessed data
// - Error handling will need to account for provider-specific error types
type JobStore interface {
	// GetJobDetails retrieves detailed information about a specific job.
	// The jobID format may vary by provider, so implementations should handle
	// provider-specific ID formats internally.
	GetJobDetails(ctx context.Context, jobID string) (*JobDetail, error)
}
