// Package handlers provides task handlers for processing various types of scheduled jobs.
package handlers

/*
Important Notes for Search Scheduler Tier Tests:

1. Tier Limits:
   - Free tier: 3 pages maximum
   - Mid tier: 50 pages maximum
   - Top tier: 200 pages maximum
   - Each tier must enforce its limit strictly
   - Limits apply per search cycle
   - After subscription end date, enforce new tier limits
   - Before subscription end date, use current tier limits

2. Message Publishing:
   - Messages are published to "{streamName}.rodeo.search.input"
   - Each message contains a SearchInput with CandidateID and Page number
   - Pages are sequential starting from 1 or last_page + 1
   - All messages for a tier are published in a single batch
   - Messages must maintain sequential page numbers across tier changes

3. Test Environment Setup:
   - Uses NATS JetStream for message handling
   - Requires a consumer with FilterSubject matching the stream name prefix
   - Consumer should use DeliverAllPolicy to capture all messages
   - Messages must be acknowledged after processing
   - Each test should reset last_page to ensure clean state

4. Database Considerations:
   - Candidate row is locked with "FOR UPDATE NOWAIT"
   - last_search_at is updated after successful publishing
   - last_page tracks the highest page number processed
   - last_page must persist across tier changes
   - Tier is stored as tier_type enum in the database
   - Tier changes require explicit type casting in SQL
   - subscription_end_date determines when to enforce new tier limits

5. Verification Strategy:
   - Check total number of messages matches tier limit
   - Verify sequential page numbers (1 to limit)
   - Ensure consistent CandidateID across messages
   - Use appropriate timeouts when fetching messages
   - For tier changes:
     * Verify initial tier publishes correct number of pages
     * Verify upgraded tier continues from last page
     * Check page numbers remain sequential across changes
   - For subscription end:
     * Verify old tier limits before subscription end
     * Verify new tier limits after subscription end

6. Error Handling:
   - Invalid tiers are rejected
   - Concurrent updates are prevented by row locking
   - Publishing failures roll back the entire transaction
   - Tier changes mid-search should not lose progress

7. Tier Change Behavior:
   - Upgrading tier should continue from last processed page
   - Downgrading tier should respect new lower limits
   - Multiple tier changes should maintain page continuity
   - Tier changes should not reset search progress
   - last_search_at can be reset to force immediate search
   - Before subscription end: use current tier limits
   - After subscription end: enforce new tier limits
*/

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/handlers/search"

	"github.com/nats-io/nats.go/jetstream"
)

func TestSearchScheduler_TierLimits(t *testing.T) {
	env := newSearchTestEnv(t)
	defer env.cleanup()

	// Create test user and candidate
	user, err := env.db.CreateTestUser(t)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	candidateID, err := env.db.CreateTestCandidate(t, user.ID)
	if err != nil {
		t.Fatalf("failed to create test candidate: %v", err)
	}

	// Helper function to set subscription end date
	setSubscriptionEnd := func(t *testing.T, candidateID string, endDate *time.Time) {
		_, err := env.db.Exec(`
			UPDATE candidates 
			SET subscription_end_date = $1
			WHERE id = $2`,
			endDate, candidateID)
		if err != nil {
			t.Fatalf("failed to update subscription end date: %v", err)
		}
	}

	tests := []struct {
		name          string
		tier          string
		expectedPages int
		setup         func(t *testing.T)
		verify        func(t *testing.T, messages [][]byte)
	}{
		{
			name:          "free_tier_limit",
			tier:          "free",
			expectedPages: 3,
			setup: func(t *testing.T) {
				_, err := env.db.Exec(`
					UPDATE candidates 
					SET tier = 'free'::tier_type, 
					    last_search_at = NULL,
					    last_page = NULL,
					    subscription_end_date = NULL
					WHERE id = $1`,
					candidateID)
				if err != nil {
					t.Fatalf("failed to update candidate: %v", err)
				}
			},
			verify: func(t *testing.T, messages [][]byte) {
				if len(messages) != 3 {
					t.Errorf("got %d messages, want 3", len(messages))
				}
				// Verify each message has correct page number
				for i, msg := range messages {
					var input search.SearchInput
					if err := json.Unmarshal(msg, &input); err != nil {
						t.Fatalf("failed to unmarshal message: %v", err)
					}
					if input.Page != i+1 {
						t.Errorf("message %d has page %d, want %d", i, input.Page, i+1)
					}
				}
			},
		},
		{
			name:          "mid_tier_active_subscription",
			tier:          "mid",
			expectedPages: 50,
			setup: func(t *testing.T) {
				_, err := env.db.Exec(`
					UPDATE candidates 
					SET tier = 'mid'::tier_type, 
					    last_search_at = NULL,
					    last_page = NULL
					WHERE id = $1`,
					candidateID)
				if err != nil {
					t.Fatalf("failed to update candidate: %v", err)
				}
				// Set subscription end date in the future
				future := time.Now().Add(24 * time.Hour)
				setSubscriptionEnd(t, candidateID, &future)
			},
			verify: func(t *testing.T, messages [][]byte) {
				if len(messages) != 50 {
					t.Errorf("got %d messages, want 50", len(messages))
				}
				// Verify each message has correct page number
				for i, msg := range messages {
					var input search.SearchInput
					if err := json.Unmarshal(msg, &input); err != nil {
						t.Fatalf("failed to unmarshal message: %v", err)
					}
					if input.Page != i+1 {
						t.Errorf("message %d has page %d, want %d", i, input.Page, i+1)
					}
				}
			},
		},
		{
			name:          "mid_tier_expired_subscription",
			tier:          "mid",
			expectedPages: 3, // Should use free tier limit after expiration
			setup: func(t *testing.T) {
				_, err := env.db.Exec(`
					UPDATE candidates 
					SET tier = 'mid'::tier_type, 
					    last_search_at = NULL,
					    last_page = NULL
					WHERE id = $1`,
					candidateID)
				if err != nil {
					t.Fatalf("failed to update candidate: %v", err)
				}
				// Set subscription end date in the past
				past := time.Now().Add(-24 * time.Hour)
				setSubscriptionEnd(t, candidateID, &past)
			},
			verify: func(t *testing.T, messages [][]byte) {
				if len(messages) != 3 {
					t.Errorf("got %d messages, want 3", len(messages))
				}
				// Verify each message has correct page number
				for i, msg := range messages {
					var input search.SearchInput
					if err := json.Unmarshal(msg, &input); err != nil {
						t.Fatalf("failed to unmarshal message: %v", err)
					}
					if input.Page != i+1 {
						t.Errorf("message %d has page %d, want %d", i, input.Page, i+1)
					}
				}
			},
		},
		{
			name:          "top_tier_active_subscription",
			tier:          "top",
			expectedPages: 200,
			setup: func(t *testing.T) {
				_, err := env.db.Exec(`
					UPDATE candidates 
					SET tier = 'top'::tier_type, 
					    last_search_at = NULL,
					    last_page = NULL
					WHERE id = $1`,
					candidateID)
				if err != nil {
					t.Fatalf("failed to update candidate: %v", err)
				}
				// Set subscription end date in the future
				future := time.Now().Add(24 * time.Hour)
				setSubscriptionEnd(t, candidateID, &future)
			},
			verify: func(t *testing.T, messages [][]byte) {
				if len(messages) != 200 {
					t.Errorf("got %d messages, want 200", len(messages))
				}
				// Verify each message has correct page number
				for i, msg := range messages {
					var input search.SearchInput
					if err := json.Unmarshal(msg, &input); err != nil {
						t.Fatalf("failed to unmarshal message: %v", err)
					}
					if input.Page != i+1 {
						t.Errorf("message %d has page %d, want %d", i, input.Page, i+1)
					}
				}
			},
		},
		{
			name:          "top_tier_expired_subscription",
			tier:          "top",
			expectedPages: 3, // Should use free tier limit after expiration
			setup: func(t *testing.T) {
				_, err := env.db.Exec(`
					UPDATE candidates 
					SET tier = 'top'::tier_type, 
					    last_search_at = NULL,
					    last_page = NULL
					WHERE id = $1`,
					candidateID)
				if err != nil {
					t.Fatalf("failed to update candidate: %v", err)
				}
				// Set subscription end date in the past
				past := time.Now().Add(-24 * time.Hour)
				setSubscriptionEnd(t, candidateID, &past)
			},
			verify: func(t *testing.T, messages [][]byte) {
				if len(messages) != 3 {
					t.Errorf("got %d messages, want 3", len(messages))
				}
				// Verify each message has correct page number
				for i, msg := range messages {
					var input search.SearchInput
					if err := json.Unmarshal(msg, &input); err != nil {
						t.Fatalf("failed to unmarshal message: %v", err)
					}
					if input.Page != i+1 {
						t.Errorf("message %d has page %d, want %d", i, input.Page, i+1)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run setup
			tt.setup(t)

			// Create publisher and task processor
			publisher := &mockPublisher{js: env.js, stream: env.stream, streamName: env.streamName}
			taskProc := NewMockTaskProcessor(publisher, env.db)
			scheduler := NewSearchScheduler(publisher, env.logger, env.db, taskProc)

			// Create scheduler task
			payload := SearchSchedulerPayload{
				CandidateID: candidateID,
			}
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			task := &internal.Task{
				Type:        scheduler.TaskType(),
				Payload:     payloadBytes,
				Status:      internal.TaskStatusPending,
				MaxAttempts: 3,
				CreatedAt:   time.Now().UTC(),
				RunAt:       time.Now().UTC(),
			}

			// Create consumer to capture messages with a unique name for each test
			// Use DeliverNewPolicy instead of DeliverAllPolicy to only get messages from this test
			consumerName := fmt.Sprintf("test-consumer-%s", tt.name)

			// Try to delete consumer if it exists (ignore errors)
			_ = env.js.DeleteConsumer(env.ctx, env.streamName, consumerName)

			consumer, err := env.js.CreateOrUpdateConsumer(env.ctx, env.streamName, jetstream.ConsumerConfig{
				Name:          consumerName,
				FilterSubject: fmt.Sprintf("%s.rodeo.search.input", env.streamName),
				DeliverPolicy: jetstream.DeliverNewPolicy,
			})
			if err != nil {
				t.Fatalf("failed to create consumer: %v", err)
			}

			// Process the task
			if err := scheduler.ProcessTask(env.ctx, task); err != nil {
				t.Fatalf("ProcessTask() error = %v", err)
			}

			// Collect all published messages
			msgs, err := consumer.Fetch(tt.expectedPages, jetstream.FetchMaxWait(time.Second))
			if err != nil {
				t.Fatalf("failed to fetch messages: %v", err)
			}

			var messages [][]byte
			for msg := range msgs.Messages() {
				messages = append(messages, msg.Data())
				msg.Ack()
			}

			// Run verification
			tt.verify(t, messages)
		})
	}
}

func TestSearchScheduler_TierChanges(t *testing.T) {
	env := newSearchTestEnv(t)
	defer env.cleanup()

	// Create test user and candidate
	user, err := env.db.CreateTestUser(t)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	candidateID, err := env.db.CreateTestCandidate(t, user.ID)
	if err != nil {
		t.Fatalf("failed to create test candidate: %v", err)
	}

	// Test upgrading tier during search
	t.Run("upgrade_during_search", func(t *testing.T) {
		// Start with free tier
		_, err := env.db.Exec(`
			UPDATE candidates 
			SET tier = 'free'::tier_type, 
			    last_search_at = NULL,
			    last_page = NULL
			WHERE id = $1`,
			candidateID)
		if err != nil {
			t.Fatalf("failed to update candidate: %v", err)
		}

		publisher := &mockPublisher{js: env.js, stream: env.stream, streamName: env.streamName}
		taskProc := NewMockTaskProcessor(publisher, env.db)
		scheduler := NewSearchScheduler(publisher, env.logger, env.db, taskProc)

		// Create consumer with unique name and DeliverNewPolicy to only get new messages
		// Sanitize the name to avoid invalid characters like '/' in the consumer name
		sanitizedName := "upgrade-during-search"
		consumerName := fmt.Sprintf("test-consumer-tier-changes-%s", sanitizedName)

		// Try to delete consumer if it exists (ignore errors)
		_ = env.js.DeleteConsumer(env.ctx, env.streamName, consumerName)

		consumer, err := env.js.CreateOrUpdateConsumer(env.ctx, env.streamName, jetstream.ConsumerConfig{
			Name:          consumerName,
			FilterSubject: fmt.Sprintf("%s.rodeo.search.input", env.streamName),
			DeliverPolicy: jetstream.DeliverNewPolicy,
		})
		if err != nil {
			t.Fatalf("failed to create consumer: %v", err)
		}

		// Start search
		payload := SearchSchedulerPayload{
			CandidateID: candidateID,
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		task := &internal.Task{
			Type:        scheduler.TaskType(),
			Payload:     payloadBytes,
			Status:      internal.TaskStatusPending,
			MaxAttempts: 3,
			CreatedAt:   time.Now().UTC(),
			RunAt:       time.Now().UTC(),
		}

		// Process task
		if err := scheduler.ProcessTask(env.ctx, task); err != nil {
			t.Fatalf("ProcessTask() error = %v", err)
		}

		// Collect initial messages
		msgs, err := consumer.Fetch(3, jetstream.FetchMaxWait(time.Second))
		if err != nil {
			t.Fatalf("failed to fetch messages: %v", err)
		}

		var initialMessages [][]byte
		for msg := range msgs.Messages() {
			initialMessages = append(initialMessages, msg.Data())
			msg.Ack()
		}

		if len(initialMessages) != 3 {
			t.Errorf("got %d initial messages, want 3", len(initialMessages))
		}

		// Verify initial messages have correct page numbers
		for i, msg := range initialMessages {
			var input search.SearchInput
			if err := json.Unmarshal(msg, &input); err != nil {
				t.Fatalf("failed to unmarshal message: %v", err)
			}
			if input.Page != i+1 {
				t.Errorf("initial message %d has page %d, want %d", i, input.Page, i+1)
			}
		}

		// Upgrade to mid tier while maintaining last_page
		_, err = env.db.Exec(`
			UPDATE candidates 
			SET tier = 'mid'::tier_type,
			    last_page = 3
			WHERE id = $1`,
			candidateID)
		if err != nil {
			t.Fatalf("failed to upgrade tier: %v", err)
		}

		// Process task again
		if err := scheduler.ProcessTask(env.ctx, task); err != nil {
			t.Fatalf("ProcessTask() error = %v", err)
		}

		// Collect additional messages
		msgs, err = consumer.Fetch(47, jetstream.FetchMaxWait(time.Second))
		if err != nil {
			t.Fatalf("failed to fetch messages: %v", err)
		}

		var additionalMessages [][]byte
		for msg := range msgs.Messages() {
			additionalMessages = append(additionalMessages, msg.Data())
			msg.Ack()
		}

		if len(additionalMessages) != 47 {
			t.Errorf("got %d additional messages, want 47", len(additionalMessages))
		}

		// Verify additional messages have correct page numbers
		for i, msg := range additionalMessages {
			var input search.SearchInput
			if err := json.Unmarshal(msg, &input); err != nil {
				t.Fatalf("failed to unmarshal message: %v", err)
			}
			expectedPage := i + 4 // Starting from page 4 (after initial 3 pages)
			if input.Page != expectedPage {
				t.Errorf("additional message %d has page %d, want %d", i, input.Page, expectedPage)
			}
		}

		// Verify all messages together
		allMessages := append(initialMessages, additionalMessages...)
		if len(allMessages) != 50 {
			t.Errorf("got %d total messages, want 50", len(allMessages))
		}

		// Verify all page numbers are sequential
		for i, msg := range allMessages {
			var input search.SearchInput
			if err := json.Unmarshal(msg, &input); err != nil {
				t.Fatalf("failed to unmarshal message: %v", err)
			}
			if input.Page != i+1 {
				t.Errorf("message %d has page %d, want %d", i, input.Page, i+1)
			}
		}
	})
}

func intPtr(i int) *int {
	return &i
}

func TestSearchScheduler_TierDowngrade(t *testing.T) {
	env := newSearchTestEnv(t)
	defer env.cleanup()

	// Create test user and candidate
	user, err := env.db.CreateTestUser(t)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	candidateID, err := env.db.CreateTestCandidate(t, user.ID)
	if err != nil {
		t.Fatalf("failed to create test candidate: %v", err)
	}

	// Create mock task processor
	publisher := &mockPublisher{
		js:         env.js,
		stream:     env.stream,
		streamName: env.streamName,
	}
	mockProc := NewMockTaskProcessor(publisher, env.db)

	// Create scheduler with fixed time
	now := time.Now().UTC().Truncate(time.Microsecond)
	fixedTime := func() time.Time { return now }
	scheduler := &SearchScheduler{
		db:        env.db,
		taskProc:  mockProc,
		publisher: publisher,
		logger:    env.logger,
		now:       fixedTime,
	}

	tests := []struct {
		name               string
		previousTier       string
		currentTier        string
		lastPage           *int
		wantLastPage       *int
		wantStartPage      int
		wantEndPage        int
		wantSearchEnqueued bool
	}{
		{
			name:               "downgrade_mid_to_free_within_limit",
			previousTier:       "mid",
			currentTier:        "free",
			lastPage:           intPtr(2),
			wantLastPage:       intPtr(2),
			wantStartPage:      3,
			wantEndPage:        3,
			wantSearchEnqueued: true,
		},
		{
			name:               "downgrade_mid_to_free_exceeds_limit",
			previousTier:       "mid",
			currentTier:        "free",
			lastPage:           intPtr(45),
			wantLastPage:       nil,
			wantStartPage:      1,
			wantEndPage:        3,
			wantSearchEnqueued: true,
		},
		{
			name:               "downgrade_top_to_mid_within_limit",
			previousTier:       "top",
			currentTier:        "mid",
			lastPage:           intPtr(30),
			wantLastPage:       intPtr(30),
			wantStartPage:      31,
			wantEndPage:        50,
			wantSearchEnqueued: true,
		},
		{
			name:               "downgrade_top_to_mid_exceeds_limit",
			previousTier:       "top",
			currentTier:        "mid",
			lastPage:           intPtr(150),
			wantLastPage:       nil,
			wantStartPage:      1,
			wantEndPage:        50,
			wantSearchEnqueued: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockProc.enqueuedTasks = nil

			// Update candidate tier and last search time
			_, err = env.db.Exec(`
				UPDATE candidates 
				SET tier = $1, last_search_at = $2, last_page = $3
				WHERE id = $4`,
				tt.currentTier,
				now.Add(-25*time.Hour),
				tt.lastPage,
				candidateID)
			if err != nil {
				t.Fatalf("failed to update candidate: %v", err)
			}

			// Insert tier history record
			_, err = env.db.Exec(`
				INSERT INTO candidates_tier_history (candidate_id, tier, created_at)
				VALUES ($1, $2, $3)`,
				candidateID,
				tt.previousTier,
				now.Add(-1*time.Hour))
			if err != nil {
				t.Fatalf("failed to insert tier history: %v", err)
			}

			// Create and process task
			payload := SearchSchedulerPayload{
				CandidateID: candidateID,
			}
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			task := &internal.Task{
				Type:        scheduler.TaskType(),
				Payload:     payloadBytes,
				Status:      internal.TaskStatusPending,
				MaxAttempts: 3,
				CreatedAt:   now,
				RunAt:       now,
			}

			err = scheduler.ProcessTask(env.ctx, task)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify candidate state
			var candidate struct {
				LastPage *int
			}
			err = env.db.QueryRowContext(env.ctx, `
				SELECT last_page
				FROM candidates
				WHERE id = $1`,
				candidateID).Scan(&candidate.LastPage)
			if err != nil {
				t.Fatalf("failed to get candidate: %v", err)
			}

			if (candidate.LastPage == nil && tt.wantLastPage != nil) ||
				(candidate.LastPage != nil && tt.wantLastPage == nil) ||
				(candidate.LastPage != nil && tt.wantLastPage != nil && *candidate.LastPage != *tt.wantLastPage) {
				t.Errorf("expected last_page to be %v, got %v", tt.wantLastPage, candidate.LastPage)
			}

			// Verify search task was enqueued
			if tt.wantSearchEnqueued {
				if len(mockProc.enqueuedTasks) == 0 {
					t.Error("expected search task to be enqueued")
				} else {
					searchTask := mockProc.enqueuedTasks[0]
					var searchPayload NATSMessagePayload
					if err := json.Unmarshal(searchTask.Payload, &searchPayload); err != nil {
						t.Fatalf("failed to unmarshal search task payload: %v", err)
					}

					var searchInput search.SearchInput
					if err := json.Unmarshal(searchPayload.Data, &searchInput); err != nil {
						t.Fatalf("failed to unmarshal search input: %v", err)
					}

					if searchInput.Page != tt.wantStartPage {
						t.Errorf("expected search task to start at page %d, got %d", tt.wantStartPage, searchInput.Page)
					}
				}
			}
		})
	}
}
