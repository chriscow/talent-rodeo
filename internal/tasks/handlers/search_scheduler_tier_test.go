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
	"talent-rodeo/internal/services/search"

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
			name:          "free_tier_active",
			tier:          "free",
			expectedPages: 3,
			setup: func(t *testing.T) {
				_, err := env.db.Exec(`
					UPDATE candidates 
					SET tier = 'free'::tier_type, 
					    last_search_at = NULL
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
				seenPages := make(map[int]bool)
				for _, msg := range messages {
					var input search.SearchInput
					if err := json.Unmarshal(msg, &input); err != nil {
						t.Fatalf("failed to unmarshal message: %v", err)
					}
					if input.Page < 1 || input.Page > 3 {
						t.Errorf("got invalid page number %d", input.Page)
					}
					if seenPages[input.Page] {
						t.Errorf("got duplicate page number %d", input.Page)
					}
					seenPages[input.Page] = true
				}
				// Verify we have all pages from 1 to 3
				for i := 1; i <= 3; i++ {
					if !seenPages[i] {
						t.Errorf("missing page number %d", i)
					}
				}
			},
		},
		{
			name:          "mid_tier_active",
			tier:          "mid",
			expectedPages: 50,
			setup: func(t *testing.T) {
				_, err := env.db.Exec(`
					UPDATE candidates 
					SET tier = 'mid'::tier_type, 
					    last_search_at = NULL
					WHERE id = $1`,
					candidateID)
				if err != nil {
					t.Fatalf("failed to update candidate: %v", err)
				}
			},
			verify: func(t *testing.T, messages [][]byte) {
				if len(messages) != 50 {
					t.Errorf("got %d messages, want 50", len(messages))
				}
				// Verify each message has correct page number
				seenPages := make(map[int]bool)
				for _, msg := range messages {
					var input search.SearchInput
					if err := json.Unmarshal(msg, &input); err != nil {
						t.Fatalf("failed to unmarshal message: %v", err)
					}
					if input.Page < 1 || input.Page > 50 {
						t.Errorf("got invalid page number %d", input.Page)
					}
					if seenPages[input.Page] {
						t.Errorf("got duplicate page number %d", input.Page)
					}
					seenPages[input.Page] = true
				}
				// Verify we have all pages from 1 to 50
				for i := 1; i <= 50; i++ {
					if !seenPages[i] {
						t.Errorf("missing page number %d", i)
					}
				}
			},
		},
		{
			name:          "top_tier_active",
			tier:          "top",
			expectedPages: 200,
			setup: func(t *testing.T) {
				_, err := env.db.Exec(`
					UPDATE candidates 
					SET tier = 'top'::tier_type, 
					    last_search_at = NULL
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
				seenPages := make(map[int]bool)
				for _, msg := range messages {
					var input search.SearchInput
					if err := json.Unmarshal(msg, &input); err != nil {
						t.Fatalf("failed to unmarshal message: %v", err)
					}
					if input.Page < 1 || input.Page > 200 {
						t.Errorf("got invalid page number %d", input.Page)
					}
					if seenPages[input.Page] {
						t.Errorf("got duplicate page number %d", input.Page)
					}
					seenPages[input.Page] = true
				}
				// Verify we have all pages from 1 to 200
				for i := 1; i <= 200; i++ {
					if !seenPages[i] {
						t.Errorf("missing page number %d", i)
					}
				}
			},
		},
		{
			name:          "expired_subscription_defaults_to_free",
			tier:          "top",
			expectedPages: 3,
			setup: func(t *testing.T) {
				_, err := env.db.Exec(`
					UPDATE candidates 
					SET tier = 'top'::tier_type, 
					    last_search_at = NULL
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
				seenPages := make(map[int]bool)
				for _, msg := range messages {
					var input search.SearchInput
					if err := json.Unmarshal(msg, &input); err != nil {
						t.Fatalf("failed to unmarshal message: %v", err)
					}
					if input.Page < 1 || input.Page > 3 {
						t.Errorf("got invalid page number %d", input.Page)
					}
					if seenPages[input.Page] {
						t.Errorf("got duplicate page number %d", input.Page)
					}
					seenPages[input.Page] = true
				}
				// Verify we have all pages from 1 to 3
				for i := 1; i <= 3; i++ {
					if !seenPages[i] {
						t.Errorf("missing page number %d", i)
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

			// Process the task
			if err := scheduler.ProcessTask(env.ctx, task); err != nil {
				t.Fatalf("ProcessTask() error = %v", err)
			}

			// Verify the number of tasks created (search tasks + 1 scheduler task)
			expectedTasks := tt.expectedPages + 1
			if got := len(taskProc.enqueuedTasks); got != expectedTasks {
				t.Errorf("got %d enqueued tasks, want %d", got, expectedTasks)
			}

			// Extract all search task messages
			var messages [][]byte
			for _, task := range taskProc.enqueuedTasks {
				if task.Type == "nats_message_scheduler" {
					var msgPayload NATSMessagePayload
					if err := json.Unmarshal(task.Payload, &msgPayload); err != nil {
						t.Fatalf("failed to unmarshal message payload: %v", err)
					}
					messages = append(messages, msgPayload.Data)
				}
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
		seenPages := make(map[int]bool)
		for _, msg := range initialMessages {
			var input search.SearchInput
			if err := json.Unmarshal(msg, &input); err != nil {
				t.Fatalf("failed to unmarshal message: %v", err)
			}
			if input.Page < 1 || input.Page > 3 {
				t.Errorf("got invalid page number %d", input.Page)
			}
			if seenPages[input.Page] {
				t.Errorf("got duplicate page number %d", input.Page)
			}
			seenPages[input.Page] = true
		}

		// Verify we have all pages from 1 to 3
		for i := 1; i <= 3; i++ {
			if !seenPages[i] {
				t.Errorf("missing page number %d", i)
			}
		}

		// Reset seen pages for next test
		seenPages = make(map[int]bool)

		// Upgrade to mid tier
		_, err = env.db.Exec(`
			UPDATE candidates 
			SET tier = 'mid'::tier_type,
			    last_search_at = NULL
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
		msgs, err = consumer.Fetch(50, jetstream.FetchMaxWait(time.Second))
		if err != nil {
			t.Fatalf("failed to fetch messages: %v", err)
		}

		var additionalMessages [][]byte
		for msg := range msgs.Messages() {
			additionalMessages = append(additionalMessages, msg.Data())
			msg.Ack()
		}

		if len(additionalMessages) != 50 {
			t.Errorf("got %d additional messages, want 50", len(additionalMessages))
		}

		// Verify additional messages have correct page numbers
		for _, msg := range additionalMessages {
			var input search.SearchInput
			if err := json.Unmarshal(msg, &input); err != nil {
				t.Fatalf("failed to unmarshal message: %v", err)
			}
			if input.Page < 1 || input.Page > 50 {
				t.Errorf("got invalid page number %d", input.Page)
			}
			if seenPages[input.Page] {
				t.Errorf("got duplicate page number %d", input.Page)
			}
			seenPages[input.Page] = true
		}

		// Verify we have all pages from 1 to 50
		for i := 1; i <= 50; i++ {
			if !seenPages[i] {
				t.Errorf("missing page number %d", i)
			}
		}
	})

	// Add new test for tier skipping
	t.Run("skip_tier_free_to_top", func(t *testing.T) {
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

		// Create consumer with unique name
		consumerName := "test-consumer-tier-skip-free-to-top"

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

		// Start initial free tier search
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

		// Process task for free tier
		if err := scheduler.ProcessTask(env.ctx, task); err != nil {
			t.Fatalf("ProcessTask() error = %v", err)
		}

		// Collect free tier messages
		msgs, err := consumer.Fetch(3, jetstream.FetchMaxWait(time.Second))
		if err != nil {
			t.Fatalf("failed to fetch messages: %v", err)
		}

		var freeMessages [][]byte
		for msg := range msgs.Messages() {
			freeMessages = append(freeMessages, msg.Data())
			msg.Ack()
		}

		if len(freeMessages) != 3 {
			t.Errorf("got %d free tier messages, want 3", len(freeMessages))
		}

		// Verify free tier messages have correct page numbers
		seenPages := make(map[int]bool)
		for _, msg := range freeMessages {
			var input search.SearchInput
			if err := json.Unmarshal(msg, &input); err != nil {
				t.Fatalf("failed to unmarshal message: %v", err)
			}
			if input.Page < 1 || input.Page > 3 {
				t.Errorf("got invalid page number %d", input.Page)
			}
			if seenPages[input.Page] {
				t.Errorf("got duplicate page number %d", input.Page)
			}
			seenPages[input.Page] = true
		}

		// Verify we have all pages from 1 to 3
		for i := 1; i <= 3; i++ {
			if !seenPages[i] {
				t.Errorf("missing page number %d", i)
			}
		}

		// Reset seen pages for next test
		seenPages = make(map[int]bool)

		// Upgrade directly to top tier
		_, err = env.db.Exec(`
			UPDATE candidates 
			SET tier = 'top'::tier_type,
			    last_search_at = NULL
			WHERE id = $1`,
			candidateID)
		if err != nil {
			t.Fatalf("failed to upgrade tier: %v", err)
		}

		// Process task again
		if err := scheduler.ProcessTask(env.ctx, task); err != nil {
			t.Fatalf("ProcessTask() error = %v", err)
		}

		// Collect top tier messages
		msgs, err = consumer.Fetch(200, jetstream.FetchMaxWait(time.Second))
		if err != nil {
			t.Fatalf("failed to fetch messages: %v", err)
		}

		var topMessages [][]byte
		for msg := range msgs.Messages() {
			topMessages = append(topMessages, msg.Data())
			msg.Ack()
		}

		if len(topMessages) != 200 {
			t.Errorf("got %d top tier messages, want 200", len(topMessages))
		}

		// Verify top tier messages have correct page numbers
		for _, msg := range topMessages {
			var input search.SearchInput
			if err := json.Unmarshal(msg, &input); err != nil {
				t.Fatalf("failed to unmarshal message: %v", err)
			}
			if input.Page < 1 || input.Page > 200 {
				t.Errorf("got invalid page number %d", input.Page)
			}
			if seenPages[input.Page] {
				t.Errorf("got duplicate page number %d", input.Page)
			}
			seenPages[input.Page] = true
		}

		// Verify we have all pages from 1 to 200
		for i := 1; i <= 200; i++ {
			if !seenPages[i] {
				t.Errorf("missing page number %d", i)
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
		wantSearchEnqueued bool
		wantPageCount      int
	}{
		{
			name:               "downgrade_mid_to_free",
			previousTier:       "mid",
			currentTier:        "free",
			lastPage:           intPtr(45),
			wantSearchEnqueued: true,
			wantPageCount:      3,
		},
		{
			name:               "downgrade_top_to_mid",
			previousTier:       "top",
			currentTier:        "mid",
			lastPage:           intPtr(150),
			wantSearchEnqueued: true,
			wantPageCount:      50,
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

			// Verify search tasks were enqueued
			if tt.wantSearchEnqueued {
				// We expect N search tasks + 1 scheduler task
				expectedTasks := tt.wantPageCount + 1
				if len(mockProc.enqueuedTasks) != expectedTasks {
					t.Errorf("expected %d tasks to be enqueued, got %d", expectedTasks, len(mockProc.enqueuedTasks))
				}

				// Count search tasks and verify their pages
				searchTasks := make([]*internal.Task, 0)
				var schedulerTask *internal.Task
				for _, task := range mockProc.enqueuedTasks {
					if task.Type == "nats_message_scheduler" {
						searchTasks = append(searchTasks, task)
					} else if task.Type == "search_scheduler" {
						schedulerTask = task
					}
				}

				if len(searchTasks) != tt.wantPageCount {
					t.Errorf("got %d search tasks, want %d", len(searchTasks), tt.wantPageCount)
				}

				if schedulerTask == nil {
					t.Fatal("scheduler task not found")
				}

				// Verify each search task has the correct page number
				seenPages := make(map[int]bool)
				for _, task := range searchTasks {
					var msgPayload NATSMessagePayload
					if err := json.Unmarshal(task.Payload, &msgPayload); err != nil {
						t.Fatalf("failed to unmarshal search task payload: %v", err)
					}

					// Strip stream name prefix from subject for comparison
					subject := msgPayload.Subject
					if subject != "rodeo.search.input" {
						t.Errorf("got subject %q, want %q", subject, "rodeo.search.input")
					}

					var searchInput search.SearchInput
					if err := json.Unmarshal(msgPayload.Data, &searchInput); err != nil {
						t.Fatalf("failed to unmarshal search input: %v", err)
					}

					if searchInput.CandidateID != candidateID {
						t.Errorf("got candidate ID %q, want %q", searchInput.CandidateID, candidateID)
					}

					if searchInput.Page < 1 || searchInput.Page > tt.wantPageCount {
						t.Errorf("got invalid page number %d", searchInput.Page)
					}

					if seenPages[searchInput.Page] {
						t.Errorf("got duplicate page number %d", searchInput.Page)
					}
					seenPages[searchInput.Page] = true
				}

				// Verify we have all pages from 1 to wantPageCount
				for i := 1; i <= tt.wantPageCount; i++ {
					if !seenPages[i] {
						t.Errorf("missing page number %d", i)
					}
				}

				// Verify task timing
				for _, searchTask := range searchTasks {
					if searchTask.RunAt.After(schedulerTask.RunAt) {
						t.Error("search task should run before next scheduler task")
					}
					if diff := schedulerTask.RunAt.Sub(searchTask.RunAt); diff < time.Minute {
						t.Errorf("scheduler task should be at least 1 minute after search task, got %v", diff)
					}
				}
			}
		})
	}
}
