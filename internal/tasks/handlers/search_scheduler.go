// Package handlers provides task handlers for processing various types of scheduled jobs.
// It includes implementations for search scheduling, message handling, and task processing.
package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/services/search"
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
	publisher Publisher        // Used to publish search messages
	logger    *slog.Logger     // Structured logger for the scheduler
	db        *database.DB     // Database connection for persistence
	taskProc  TaskProcessor    // Processor for enqueueing new tasks
	now       func() time.Time // Function to get current time (for testing)
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
		now:       time.Now,
	}
}

// TaskType returns the type of task this handler processes.
// This is used to identify which tasks should be routed to this handler.
//
// Returns the string identifier for the task type.
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
//
// Parameters:
//   - tier: The subscription tier of the candidate
//   - lastSearch: The time of the last search performed
//
// Returns the calculated time for the next search.
func (h *SearchScheduler) getNextSearchTime(tier string, lastSearch time.Time) time.Time {
	now := h.now().UTC().Truncate(time.Microsecond)

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

	// Calculate next search time based on the last search time
	nextSearch := lastSearch.Add(interval).Truncate(time.Microsecond)

	// If next search time is in the past, schedule immediately
	if nextSearch.Before(now) {
		return now
	}

	return nextSearch
}

// getMaxPages returns the maximum number of pages allowed for a given tier.
//
// Parameters:
//   - tier: The subscription tier of the candidate
//
// Returns the maximum number of pages allowed for the specified tier.
func (h *SearchScheduler) getMaxPages(tier string) int {
	switch tier {
	case "free":
		return 3
	case "mid":
		return 50
	case "top":
		return 200
	default:
		return 3 // Default to free tier limit
	}
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
// Parameters:
//   - ctx: Context for the operation, which can be used to cancel it
//   - task: The task to be processed, containing the candidate's information
//
// Returns an error if any step in the process fails.
func (h *SearchScheduler) ProcessTask(ctx context.Context, task *internal.Task) error {
	var payload SearchSchedulerPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	var nextSearchTime time.Time
	var nextSchedulerTime time.Time

	err := h.db.WithTx(ctx, func(tx *database.DB) error {
		// Get candidate with exclusive lock
		var candidate struct {
			ID              string
			Tier            string     `db:"tier"`
			LastSearchAt    *time.Time `db:"last_search_at"`
			SubscriptionEnd *time.Time `db:"subscription_end_date"`
		}
		err := tx.GetContext(ctx, &candidate,
			`SELECT id, tier::text as tier, last_search_at, subscription_end_date
			FROM candidates 
			WHERE id = $1 
			FOR UPDATE NOWAIT`,
			payload.CandidateID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("candidate not found: %s", payload.CandidateID)
			}
			if strings.Contains(err.Error(), "could not obtain lock") {
				return &internal.TaskError{
					Err:         fmt.Errorf("candidate %s is already being processed", payload.CandidateID),
					ShouldRetry: false,
				}
			}
			return fmt.Errorf("failed to get candidate: %w", err)
		}

		// Check for tier changes
		var previousTier string
		err = tx.GetContext(ctx, &previousTier, `
			SELECT tier::text
			FROM candidates_tier_history
			WHERE candidate_id = $1
			ORDER BY created_at DESC
			LIMIT 1`,
			candidate.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to get previous tier: %w", err)
		}

		h.logger.Debug("checking tier changes",
			"candidate_id", candidate.ID,
			"current_tier", candidate.Tier,
			"previous_tier", previousTier,
			"last_search_at", candidate.LastSearchAt)

		// Log initial state
		h.logger.Info("processing candidate",
			"candidate_id", candidate.ID,
			"tier", candidate.Tier,
			"previous_tier", previousTier,
			"subscription_end", candidate.SubscriptionEnd)

		// Validate tier
		switch candidate.Tier {
		case "free", "mid", "top":
			// Valid tier, continue processing
		default:
			return fmt.Errorf("invalid tier: %s", candidate.Tier)
		}

		// Get max pages for this tier
		maxPages := h.getMaxPages(candidate.Tier)
		h.logger.Debug("initial tier limits",
			"tier", candidate.Tier,
			"max_pages", maxPages)

		// Check if subscription has ended
		now := h.now().UTC()
		if candidate.SubscriptionEnd != nil && now.After(*candidate.SubscriptionEnd) {
			h.logger.Info("subscription expired",
				"candidate_id", candidate.ID,
				"subscription_end", candidate.SubscriptionEnd,
				"current_time", now,
				"tier", candidate.Tier,
				"max_pages", maxPages)

			// After subscription expires, use free tier limit
			maxPages = h.getMaxPages("free")
			h.logger.Info("using free tier limit after subscription expiry",
				"candidate_id", candidate.ID,
				"new_max_pages", maxPages)
		}

		// Calculate next search time
		var lastSearchTime time.Time
		if candidate.LastSearchAt != nil {
			lastSearchTime = *candidate.LastSearchAt
		}

		// If tier has changed, force an immediate search
		if previousTier != "" && previousTier != candidate.Tier {
			h.logger.Info("tier changed, forcing immediate search",
				"candidate_id", candidate.ID,
				"previous_tier", previousTier,
				"current_tier", candidate.Tier)
			lastSearchTime = time.Time{} // Reset last search time to force immediate search
		}

		nextSearchTime = h.getNextSearchTime(candidate.Tier, lastSearchTime)
		nextSchedulerTime = nextSearchTime.Add(time.Minute)

		h.logger.Debug("search timing",
			"candidate_id", candidate.ID,
			"last_search_time", lastSearchTime,
			"next_search_time", nextSearchTime,
			"next_scheduler_time", nextSchedulerTime)

		// Update candidate with new search time
		_, err = tx.ExecContext(ctx, `
			UPDATE candidates 
			SET last_search_at = $1
			WHERE id = $2`,
			nextSearchTime,
			candidate.ID)
		if err != nil {
			return fmt.Errorf("failed to update candidate: %w", err)
		}

		// Create NATS messages for all pages
		seenPages := make(map[int]bool)
		for page := 1; page <= maxPages; page++ {
			h.logger.Debug("scheduling search task",
				"candidate_id", candidate.ID,
				"tier", candidate.Tier,
				"page", page,
				"max_pages", maxPages)

			searchInput := search.SearchInput{
				CandidateID: candidate.ID,
				Page:        page,
			}
			searchInputBytes, err := json.Marshal(searchInput)
			if err != nil {
				return fmt.Errorf("failed to marshal search input: %w", err)
			}

			messagePayload := NATSMessagePayload{
				Subject: search.TopicSearchInput,
				Data:    searchInputBytes,
			}
			messagePayloadBytes, err := json.Marshal(messagePayload)
			if err != nil {
				return fmt.Errorf("failed to marshal message payload: %w", err)
			}

			searchTask := &internal.Task{
				Type:        "nats_message_scheduler",
				Payload:     messagePayloadBytes,
				Status:      internal.TaskStatusPending,
				MaxAttempts: 3,
				RunAt:       nextSearchTime,
			}
			if err := h.taskProc.EnqueueTask(ctx, searchTask); err != nil {
				return fmt.Errorf("failed to enqueue search task for page %d: %w", page, err)
			}
			seenPages[page] = true
		}

		h.logger.Debug("scheduled pages summary",
			"candidate_id", candidate.ID,
			"tier", candidate.Tier,
			"max_pages", maxPages,
			"pages_scheduled", len(seenPages),
			"seen_pages", seenPages)

		// Schedule next scheduler task
		nextTask := &internal.Task{
			Type:        h.TaskType(),
			Payload:     task.Payload,
			Status:      internal.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       nextSchedulerTime,
		}
		if err := h.taskProc.EnqueueTask(ctx, nextTask); err != nil {
			return fmt.Errorf("failed to schedule next task: %w", err)
		}

		h.logger.Info("search scheduled successfully",
			"component", "search_scheduler",
			"candidate_id", payload.CandidateID,
			"next_search", nextSearchTime,
			"next_scheduler", nextSchedulerTime,
			"pages_scheduled", maxPages)

		return nil
	})

	return err
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

	now := h.now().UTC().Truncate(time.Microsecond)
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
		CreatedAt:   now,
		RunAt:       now,
	}

	if err := h.taskProc.EnqueueTask(ctx, task); err != nil {
		return fmt.Errorf("failed to schedule initial search: %w", err)
	}

	return nil
}
