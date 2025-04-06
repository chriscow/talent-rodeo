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
	"talent-rodeo/internal/search"
	"talent-rodeo/internal/tasks"

	"cloud.google.com/go/firestore"
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
	UserID string `json:"user_id"` // Unique identifier of the candidate
}

// SearchScheduler processes scheduled job search tasks. It manages the scheduling
// and execution of automated job searches for candidates based on their subscription tier.
// The scheduler ensures that searches are performed at appropriate intervals and
// maintains the scheduling state in the database.
type SearchScheduler struct {
	publisher Publisher         // Used to publish search messages
	logger    *slog.Logger      // Structured logger for the scheduler
	fs        *firestore.Client // Database connection for persistence
	taskProc  *tasks.Processor  // Processor for enqueueing new tasks
	now       func() time.Time  // Function to get current time (for testing)
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
func NewSearchScheduler(publisher Publisher, logger *slog.Logger, fs *firestore.Client, taskProc *tasks.Processor) *SearchScheduler {
	return &SearchScheduler{
		publisher: publisher,
		logger:    logger.With(slog.String("component", "search_scheduler")),
		fs:        fs,
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
func (h *SearchScheduler) getNextSearchTime(tier internal.UserTier, lastSearch time.Time) time.Time {
	now := h.now().UTC().Truncate(time.Microsecond)

	// If no previous search, schedule immediately
	if lastSearch.IsZero() {
		return now
	}

	// Calculate interval based on tier
	var interval time.Duration
	switch tier {
	case internal.UserTierFree:
		interval = 72 * time.Hour // Every three days
	case internal.UserTierPro:
		interval = 24 * time.Hour // Daily
	case internal.UserTierEnterprise:
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
func (h *SearchScheduler) getMaxPages(tier internal.UserTier) int {
	switch tier {
	case internal.UserTierFree:
		return 3
	case internal.UserTierPro:
		return 50
	case internal.UserTierEnterprise:
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

	user, found, err := internal.GetUser(ctx, h.fs, payload.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if !found {
		return fmt.Errorf("user not found: %s", payload.UserID)
	}

	// Get max pages for this tier
	maxPages := h.getMaxPages(user.Tier)
	h.logger.Debug("initial tier limits",
		"tier", user.Tier,
		"max_pages", maxPages)

	// Check if subscription has ended
	now := h.now().UTC()
	if user.SubscriptionEnd != nil && now.After(*user.SubscriptionEnd) {
		h.logger.Info("subscription expired",
			"user_id", user.ID,
			"subscription_end", user.SubscriptionEnd,
			"current_time", now,
			"tier", user.Tier,
			"max_pages", maxPages)

		// After subscription expires, use free tier limit
		maxPages = h.getMaxPages("free")
		h.logger.Info("using free tier limit after subscription expiry",
			"user_id", user.ID,
			"new_max_pages", maxPages)
	}

	// Calculate next search time
	var lastSearchTime time.Time
	if user.CompletedAt != nil {
		lastSearchTime = *user.CompletedAt
	}

	// TODO: If tier has changed, force an immediate search

	nextSearchTime = h.getNextSearchTime(user.Tier, lastSearchTime)
	nextSchedulerTime = nextSearchTime.Add(time.Minute)

	h.logger.Debug("search timing",
		"user_id", user.ID,
		"last_search_time", lastSearchTime,
		"next_search_time", nextSearchTime,
		"next_scheduler_time", nextSchedulerTime)

	// Update candidate with new search time
	err = internal.UpdateUser(ctx, h.fs, user.ID, map[string]any{
		"last_search_at": nextSearchTime,
	})
	if err != nil {
		return fmt.Errorf("failed to update candidate: %w", err)
	}

	// Create NATS messages for all pages
	seenPages := make(map[int]bool)
	for page := 1; page <= maxPages; page++ {
		h.logger.Debug("scheduling search task",
			"user_id", user.ID,
			"tier", user.Tier,
			"page", page,
			"max_pages", maxPages)

		searchInput := search.SearchInput{
			CandidateID: user.ID,
			Page:        page,
		}
		searchInputBytes, err := json.Marshal(searchInput)
		if err != nil {
			return fmt.Errorf("failed to marshal search input: %w", err)
		}

		messagePayload := tasks.NATSMessagePayload{
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
		"user_id", user.ID,
		"tier", user.Tier,
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
		"user_id", payload.UserID,
		"next_search", nextSearchTime,
		"next_scheduler", nextSchedulerTime,
		"pages_scheduled", maxPages)

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
func (h *SearchScheduler) ScheduleInitialSearch(ctx context.Context, userID string) error {
	h.logger.Debug("scheduling initial search", "user_id", userID)

	now := h.now().UTC().Truncate(time.Microsecond)
	payload := SearchSchedulerPayload{
		UserID: userID,
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
