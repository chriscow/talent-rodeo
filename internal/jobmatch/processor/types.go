package processor

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// JobProcessor handles the processing of jobs from various providers.
// In Phase 1, this will be a simple synchronous processor, but it's designed
// to evolve into an async distributed system in Phase 5.
//
// Future Considerations (Phase 5):
// - Will integrate with NATS/Watermill for message handling
// - Will implement retry mechanisms with backoff
// - Will support distributed processing across multiple instances
// - Will include sophisticated error handling and dead letter queues
type JobProcessor interface {
	// Process handles a job processing request. In Phase 1 this is synchronous,
	// but in Phase 5 it will return immediately and process asynchronously.
	Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error)

	// Status retrieves the current status of a job processing request.
	// This will become more important in Phase 5 with async processing.
	Status(ctx context.Context, id uuid.UUID) (*ProcessStatus, error)

	// Cancel attempts to cancel a running job process.
	// In Phase 5, this will coordinate cancellation across distributed processes.
	Cancel(ctx context.Context, id uuid.UUID) error
}

// ProcessRequest represents a job processing request.
// This structure is designed to be extensible for future requirements.
type ProcessRequest struct {
	ID       uuid.UUID
	Type     ProcessType
	Provider string
	Params   map[string]interface{}
	Priority Priority
	Deadline *time.Time
	// Phase 5 additions:
	// - RetryPolicy
	// - Circuit breaker settings
	// - Resource requirements
}

// ProcessResponse contains the result of a process request.
// In Phase 1 this will be returned immediately, in Phase 5 it will
// be retrieved via message queue.
type ProcessResponse struct {
	RequestID uuid.UUID
	Status    ProcessStatus
	Result    interface{}
	Error     error
	StartTime time.Time
	EndTime   time.Time
	// Phase 5 additions:
	// - Resource utilization
	// - Distributed tracing info
	// - Performance metrics
}

// ProcessStatus represents the current state of a process request.
// This will be especially important in Phase 5 for tracking async jobs.
type ProcessStatus struct {
	State        State
	Progress     float64
	CurrentStage string
	Error        error
	LastUpdated  time.Time
	// Phase 5 additions:
	// - Node information
	// - Queue position
	// - Resource allocation
}

// ProcessType defines the type of processing to be performed.
type ProcessType string

const (
	ProcessTypeSearch   ProcessType = "search"   // Search for jobs
	ProcessTypeAnalyze  ProcessType = "analyze"  // Analyze job details
	ProcessTypeMatch    ProcessType = "match"    // Match jobs to candidates
	ProcessTypeValidate ProcessType = "validate" // Validate job data
	// Future types for Phase 5:
	// - Batch processing
	// - Data enrichment
	// - Analytics
)

// State represents the current state of a process.
type State string

const (
	StateQueued     State = "queued"     // Request is queued
	StateProcessing State = "processing" // Request is being processed
	StateCompleted  State = "completed"  // Request completed successfully
	StateFailed     State = "failed"     // Request failed
	StateCancelled  State = "cancelled"  // Request was cancelled
	// Phase 5 states:
	// StateRetrying   State = "retrying"    // Request is being retried
	// StateScheduled  State = "scheduled"   // Request is scheduled for future
	// StatePaused     State = "paused"      // Request is paused
)

// Priority defines the processing priority of a request.
type Priority int

const (
	PriorityLow    Priority = 1
	PriorityNormal Priority = 2
	PriorityHigh   Priority = 3
	// Phase 5 priorities:
	// PriorityCritical Priority = 4
	// PriorityBatch    Priority = 0
)
