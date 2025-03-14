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
	"talent-rodeo/internal/handlers/search"
)

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
		var payload NATSMessagePayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			return fmt.Errorf("failed to unmarshal message payload: %w", err)
		}

		// If this is a search message, we need to handle it specially by publishing multiple messages
		if payload.Subject == "rodeo.search.input" {
			var searchInput search.SearchInput
			if err := json.Unmarshal(payload.Data, &searchInput); err != nil {
				return fmt.Errorf("failed to unmarshal search input: %w", err)
			}

			// Default values
			startPage := searchInput.Page
			maxPages := 3 // Default to free tier

			// If we have a database connection, get the candidate's tier to determine max pages
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
			} else {
				// Fallback to determining max pages based on start page
				if startPage <= 3 {
					maxPages = 3 // Free tier
				} else if startPage <= 50 {
					maxPages = 50 // Mid tier
				} else {
					maxPages = 200 // Top tier
				}
				m.logger.Debug("no database connection, using fallback logic",
					"start_page", startPage,
					"max_pages", maxPages)
			}

			m.logger.Debug("publishing search messages",
				"start_page", startPage,
				"max_pages", maxPages,
				"candidate_id", searchInput.CandidateID)

			// Publish a message for each page from startPage to maxPages
			for page := startPage; page <= maxPages; page++ {
				// Create a new search input for this page
				pageInput := search.SearchInput{
					CandidateID: searchInput.CandidateID,
					Page:        page,
				}

				// Marshal the page input
				pageData, err := json.Marshal(pageInput)
				if err != nil {
					return fmt.Errorf("failed to marshal page input: %w", err)
				}

				// Publish the message
				if err := m.publisher.Publish(ctx, payload.Subject, pageData); err != nil {
					m.logger.Error("failed to publish message", "error", err, "page", page)
					return fmt.Errorf("failed to publish message: %w", err)
				}

				m.logger.Debug("published message", "page", page, "candidate_id", searchInput.CandidateID)
			}

			// Only add to enqueuedTasks if publishing succeeds
			m.enqueuedTasks = append(m.enqueuedTasks, task)
			return nil
		}

		// For non-search messages, just publish directly
		if err := m.publisher.Publish(ctx, payload.Subject, payload.Data); err != nil {
			return fmt.Errorf("failed to publish message: %w", err)
		}
	}

	// Add to enqueuedTasks after successful publishing or for non-publishing tasks
	m.enqueuedTasks = append(m.enqueuedTasks, task)
	return nil
}
