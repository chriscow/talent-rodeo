// Package handlers provides task handlers for processing various types of scheduled jobs.
// It includes implementations for search scheduling, message handling, and task processing.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/handlers/search"
)

// Publisher defines the minimal interface needed for publishing messages.
// Implementations must be able to publish messages to a specific subject
// with arbitrary byte data.
type Publisher interface {
	// Publish sends a message with the given data to the specified subject.
	// The context can be used to cancel the publish operation.
	Publish(ctx context.Context, subject string, data []byte) error
}

// SearchSchedulerPayload represents the data needed for scheduling a search.
// It contains the unique identifier of the candidate for whom the search
// should be scheduled.
type SearchSchedulerPayload struct {
	CandidateID string `json:"candidate_id"` // Unique identifier of the candidate
}

// SearchScheduler processes scheduled job search tasks. It manages the scheduling
// and execution of automated job searches for candidates based on their subscription tier.
// The scheduler ensures that searches are performed at appropriate intervals and
// maintains the scheduling state in the database.
type SearchScheduler struct {
	publisher Publisher     // Used to publish search messages
	logger    *slog.Logger  // Structured logger for the scheduler
	db        *database.DB  // Database connection for persistence
	taskProc  TaskProcessor // Processor for enqueueing new tasks
}

// NewSearchScheduler creates a new search scheduler with the provided dependencies.
// It initializes a scheduler that can process search tasks and manage search schedules
// for candidates based on their subscription tiers.
//
// Parameters:
//   - publisher: Interface for publishing messages to the message queue
//   - logger: Structured logger for recording scheduler operations
//   - db: Database connection for persisting state
//   - taskProc: Task processor for enqueueing new tasks
//
// Returns a configured SearchScheduler instance ready to process tasks.
func NewSearchScheduler(publisher Publisher, logger *slog.Logger, db *database.DB, taskProc TaskProcessor) *SearchScheduler {
	return &SearchScheduler{
		publisher: publisher,
		logger:    logger.With(slog.String("component", "search_scheduler")),
		db:        db,
		taskProc:  taskProc,
	}
}

// TaskType returns the type of task this handler processes.
// This is used to identify which tasks should be routed to this handler.
func (h *SearchScheduler) TaskType() string {
	return "search_scheduler"
}

// getNextSearchTime calculates when the next search should run based on the tier
// and the time of the last search. The calculation follows these rules:
//   - For new candidates (no previous search): Schedule immediately
//   - Free tier: Every 72 hours
//   - Mid tier: Every 24 hours
//   - Top tier: Every 6 hours
//
// If the calculated next search time is in the past, the search is scheduled
// immediately. All times are handled in UTC to avoid timezone issues.
func (h *SearchScheduler) getNextSearchTime(tier string, lastSearch time.Time) time.Time {
	now := time.Now().UTC()

	// If no previous search, schedule immediately
	if lastSearch.IsZero() {
		return now
	}

	// Calculate interval based on tier
	var interval time.Duration
	switch tier {
	case "free":
		interval = 72 * time.Hour // Every three days
	case "mid":
		interval = 24 * time.Hour // Daily
	case "top":
		interval = 6 * time.Hour // Four times daily
	default:
		interval = 72 * time.Hour // Default to free tier interval
	}

	nextSearch := lastSearch.Add(interval)
	if nextSearch.Before(now) {
		return now
	}
	return nextSearch
}

// ProcessTask handles the execution of a search scheduler task. It performs the following steps:
//  1. Retrieves the candidate's current tier and last search time
//  2. Calculates the next search time based on the tier
//  3. Creates and schedules a search task
//  4. Schedules the next scheduler task
//
// The function ensures that searches are performed at appropriate intervals based
// on the candidate's subscription tier and maintains the scheduling chain.
//
// Returns an error if any step in the process fails.
func (h *SearchScheduler) ProcessTask(ctx context.Context, task *internal.Task) error {
	h.logger.Debug("processing search scheduler task",
		"task_id", task.ID,
		"run_at", task.RunAt,
	)

	// Parse payload
	var payload SearchSchedulerPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Validate payload
	if payload.CandidateID == "" {
		return fmt.Errorf("candidate_id is required")
	}

	// Get candidate's current tier and last search time
	var tier string
	var lastSearchAt time.Time
	err := h.db.GetContext(ctx, &struct {
		Tier         *string    `db:"tier"`
		LastSearchAt *time.Time `db:"last_search_at"`
	}{
		Tier:         &tier,
		LastSearchAt: &lastSearchAt,
	}, `SELECT tier, last_search_at FROM candidates WHERE id = $1`, payload.CandidateID)
	if err != nil {
		return fmt.Errorf("failed to get candidate info: %w", err)
	}

	// Calculate next search time
	nextSearchTime := h.getNextSearchTime(tier, lastSearchAt)

	// Create search input message
	searchInput := search.SearchInput{
		CandidateID: payload.CandidateID,
	}
	searchInputBytes, err := json.Marshal(searchInput)
	if err != nil {
		return fmt.Errorf("failed to marshal search input: %w", err)
	}

	// Schedule the search message
	msgPayload := NATSMessagePayload{
		Subject: "rodeo.search.input",
		Data:    searchInputBytes,
		Metadata: map[string]string{
			"candidate_id": payload.CandidateID,
			"tier":         tier,
		},
	}
	msgPayloadBytes, err := json.Marshal(msgPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal message payload: %w", err)
	}

	// Create task for the search
	searchTask := &internal.Task{
		Type:        "nats_message_scheduler",
		Payload:     msgPayloadBytes,
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       nextSearchTime,
	}

	// Enqueue the search task
	if err := h.taskProc.EnqueueTask(ctx, searchTask); err != nil {
		return fmt.Errorf("failed to schedule search: %w", err)
	}

	// Schedule the next scheduler task
	nextSchedulerTime := nextSearchTime.Add(time.Minute) // Schedule slightly after the search
	nextSchedulerTask := &internal.Task{
		Type:        h.TaskType(),
		Payload:     task.Payload, // Reuse the same payload
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       nextSchedulerTime,
	}

	if err := h.taskProc.EnqueueTask(ctx, nextSchedulerTask); err != nil {
		return fmt.Errorf("failed to schedule next scheduler task: %w", err)
	}

	h.logger.Info("search scheduled successfully",
		"candidate_id", payload.CandidateID,
		"tier", tier,
		"next_search", nextSearchTime,
		"next_scheduler", nextSchedulerTime,
	)

	return nil
}

// ScheduleInitialSearch schedules the first search for a candidate. This is typically
// called when a candidate is first created or when their subscription is activated.
// The initial search is scheduled to run immediately.
//
// Parameters:
//   - ctx: Context for the operation, which can be used to cancel it
//   - candidateID: The unique identifier of the candidate
//
// Returns an error if scheduling the initial search fails.
func (h *SearchScheduler) ScheduleInitialSearch(ctx context.Context, candidateID string) error {
	h.logger.Debug("scheduling initial search", "candidate_id", candidateID)

	payload := SearchSchedulerPayload{
		CandidateID: candidateID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := &internal.Task{
		Type:        h.TaskType(),
		Payload:     payloadBytes,
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       time.Now().UTC(),
	}

	if err := h.taskProc.EnqueueTask(ctx, task); err != nil {
		return fmt.Errorf("failed to schedule initial search: %w", err)
	}

	return nil
}
