package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"talent-rodeo/internal"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/search"
	"talent-rodeo/internal/tasks"
)

// Publisher defines the minimal interface needed for publishing messages.
// Implementations must be able to publish messages to a specific subject
// with arbitrary byte data.
type Publisher interface {
	// Publish sends a message with the given data to the specified subject.
	// The context can be used to cancel the publish operation.
	Publish(ctx context.Context, subject string, data []byte) error
}

// SearchInput represents the input message for job searches
type SearchInput struct {
	CandidateID     string   `json:"candidate_id"`
	MustHaveSkills  []string `json:"must_have_skills"`
	PreferredSkills []string `json:"preferred_skills"`
	Page            int      `json:"page"`
}

// MockTaskProcessor implements TaskProcessor for testing
type MockTaskProcessor struct {
	enqueuedTasks []*internal.Task
	publisher     Publisher
	db            *database.DB
	logger        *slog.Logger
}

func NewMockTaskProcessor(publisher Publisher, db *database.DB) *MockTaskProcessor {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	return &MockTaskProcessor{
		publisher: publisher,
		db:        db,
		logger:    logger.With(slog.String("component", "mock_task_processor")),
	}
}

func (m *MockTaskProcessor) EnqueueTask(ctx context.Context, task *internal.Task) error {
	// If this is a nats_message_scheduler task, publish the message directly
	if task.Type == "nats_message_scheduler" && m.publisher != nil {
		var payload tasks.NATSMessagePayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			return fmt.Errorf("failed to unmarshal message payload: %w", err)
		}

		// If this is a search message, we need to handle it specially by publishing multiple messages
		if payload.Subject == search.TopicSearchInput {
			var searchInput SearchInput
			if err := json.Unmarshal(payload.Data, &searchInput); err != nil {
				return fmt.Errorf("failed to unmarshal search input: %w", err)
			}

			// Get max pages for validation
			maxPages := 3 // Default to free tier
			if m.db != nil {
				type candidateInfo struct {
					Tier            string  `db:"tier"`
					SubscriptionEnd *string `db:"subscription_end_date"`
				}

				var candidate candidateInfo
				err := m.db.GetContext(ctx, &candidate,
					`SELECT tier::text as tier, subscription_end_date 
					FROM candidates 
					WHERE id = $1`,
					searchInput.CandidateID)

				if err != nil && err != sql.ErrNoRows {
					m.logger.Warn("failed to get candidate tier", "error", err, "candidate_id", searchInput.CandidateID)
				} else if err == nil {
					m.logger.Debug("found candidate tier",
						"candidate_id", searchInput.CandidateID,
						"tier", candidate.Tier,
						"subscription_end", candidate.SubscriptionEnd)

					// Set max pages based on tier
					switch candidate.Tier {
					case "free":
						maxPages = 3
					case "mid":
						maxPages = 50
					case "top":
						maxPages = 200
					default:
						maxPages = 3 // Default to free tier
					}
				}
			}

			// Validate the page number is within bounds
			if searchInput.Page < 1 || searchInput.Page > maxPages {
				return fmt.Errorf("invalid page number %d for max pages %d", searchInput.Page, maxPages)
			}

			m.logger.Debug("publishing search message",
				"page", searchInput.Page,
				"max_pages", maxPages,
				"candidate_id", searchInput.CandidateID)

			// Publish just this single page
			if err := m.publisher.Publish(ctx, payload.Subject, payload.Data); err != nil {
				m.logger.Error("failed to publish message", "error", err, "page", searchInput.Page)
				// On any publish error, clear all enqueued tasks before returning the error
				m.enqueuedTasks = nil
				return fmt.Errorf("failed to publish message: %w", err)
			}

			m.logger.Debug("published message",
				"page", searchInput.Page,
				"candidate_id", searchInput.CandidateID)
		} else {
			// For non-search messages, just publish directly
			if err := m.publisher.Publish(ctx, payload.Subject, payload.Data); err != nil {
				// On any publish error, clear all enqueued tasks before returning the error
				m.enqueuedTasks = nil
				return fmt.Errorf("failed to publish message: %w", err)
			}
		}
	}

	// Only add to enqueuedTasks if we get here (no errors occurred)
	m.enqueuedTasks = append(m.enqueuedTasks, task)
	return nil
}
