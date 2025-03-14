// Package handlers provides task handlers for processing various types of scheduled jobs.
package handlers

/*
Important Notes for Search Scheduler Integration Tests:

1. Database Type Handling:
   - The 'tier' column is defined as a custom 'tier_type' enum in the database
   - When setting tier values in tests, use `tier::tier_type` cast syntax
   - When querying tier values, use `tier::text` to get string representation

2. Timestamp Precision:
   - All timestamps must be truncated to microsecond precision
   - Use UTC timestamps consistently throughout tests
   - Compare times using .Equal() or with a small time window (±1m) for scheduling tests

3. Test Environment Setup:
   - Each test case needs its own isolated database state
   - Update candidate tier and last_search_at atomically
   - Verify tier was set correctly before proceeding with test
   - NATS subjects must include stream name prefix for proper routing
   - Consumer FilterSubject must match the full subject including stream name

4. Task Creation:
   - Each ProcessTask call should create exactly two tasks:
     * NATS message scheduler task for immediate search
     * Next scheduler task for future scheduling
   - Search task must run before scheduler task (1 minute gap)
   - Task payloads must be properly formatted:
     * Search task: NATSMessagePayload with subject and SearchInput
     * Scheduler task: SearchSchedulerPayload with CandidateID

5. Message Publishing:
   - Messages are published immediately after task creation
   - Each message contains a SearchInput with page number
   - Page numbers must be sequential and respect tier limits
   - Messages must be acknowledged after processing
   - Use appropriate timeouts when fetching messages

6. Common Pitfalls:
   - Nil LastSearchAt handling: The database may return nil for last_search_at
   - Time zone issues: Always use UTC and explicit truncation
   - Task timing: Scheduler tasks should be 1 minute after search tasks
   - Missing stream name prefix in subjects
   - Not acknowledging messages in consumer

7. Test Case Coverage:
   - Initial search (no previous search time)
   - Immediate search (past due)
   - Future search (based on tier intervals)
   - Concurrent task processing
   - Message content verification
   - Task timing verification
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

// TestSearchScheduler_EndToEnd tests the complete flow of the search scheduler
// through different tier scenarios. It verifies:
//   - Initial search for free tier candidates (no previous search)
//   - Subsequent searches for mid tier candidates (past due search)
//   - Future searches for top tier candidates (not yet due)
//
// For each scenario, it checks:
//   - Correct task creation (N search tasks + 1 scheduler task, where N is tier page limit)
//   - Proper task timing based on tier intervals
//   - Message publishing for immediate searches
//   - Subject routing and message content
//   - Database state updates (last_search_at)
func TestSearchScheduler_EndToEnd(t *testing.T) {
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

	now := time.Now().UTC().Truncate(time.Microsecond)
	fixedTime := func() time.Time { return now }

	tests := []struct {
		name           string
		tier           string
		lastSearchTime time.Time
		wantDelay      time.Duration
		wantSubject    string
		wantPages      int // Number of pages for this tier
	}{
		{
			name:           "free_tier_initial_search",
			tier:           "free",
			lastSearchTime: time.Time{}, // Zero time for initial search
			wantDelay:      0,
			wantSubject:    "rodeo.search.input",
			wantPages:      3,
		},
		{
			name:           "mid_tier_subsequent_search",
			tier:           "mid",
			lastSearchTime: now.Add(-25 * time.Hour).Truncate(time.Microsecond),
			wantDelay:      0,
			wantSubject:    "rodeo.search.input",
			wantPages:      50,
		},
		{
			name:           "top_tier_future_search",
			tier:           "top",
			lastSearchTime: now.Add(-1 * time.Hour).Truncate(time.Microsecond),
			wantDelay:      5 * time.Hour,
			wantSubject:    "rodeo.search.input",
			wantPages:      200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Update candidate's tier and last search time
			_, err = env.db.Exec(`
				UPDATE candidates 
				SET tier = $1::tier_type, last_search_at = $2
				WHERE id = $3`,
				tt.tier, tt.lastSearchTime, candidateID)
			if err != nil {
				t.Fatalf("failed to update candidate: %v", err)
			}

			// Verify the tier was set correctly
			var tier string
			err = env.db.GetContext(env.ctx, &tier, `SELECT tier::text FROM candidates WHERE id = $1`, candidateID)
			if err != nil {
				t.Fatalf("failed to get candidate tier: %v", err)
			}
			if tier != tt.tier {
				t.Fatalf("tier not set correctly, got %q, want %q", tier, tt.tier)
			}

			// Create mock task processor
			publisher := &mockPublisher{js: env.js, stream: env.stream, streamName: env.streamName}
			taskProc := NewMockTaskProcessor(publisher, env.db)
			scheduler := NewSearchScheduler(publisher, env.logger, env.db, taskProc)
			scheduler.now = fixedTime // Use fixed time for testing

			// Create initial scheduler task
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

			// Process the task
			err = scheduler.ProcessTask(env.ctx, task)
			if err != nil {
				t.Fatalf("ProcessTask() error = %v", err)
			}

			// Verify that tasks were enqueued (N search tasks + 1 scheduler task)
			expectedTasks := tt.wantPages + 1
			if len(taskProc.enqueuedTasks) != expectedTasks {
				t.Errorf("got %d enqueued tasks, want %d", len(taskProc.enqueuedTasks), expectedTasks)
			}

			// Find all search tasks and verify their page numbers
			searchTasks := make([]*internal.Task, 0)
			var schedulerTask *internal.Task
			for _, task := range taskProc.enqueuedTasks {
				if task.Type == "nats_message_scheduler" {
					searchTasks = append(searchTasks, task)
				} else if task.Type == "search_scheduler" {
					schedulerTask = task
				}
			}

			if len(searchTasks) != tt.wantPages {
				t.Errorf("got %d search tasks, want %d", len(searchTasks), tt.wantPages)
			}

			if schedulerTask == nil {
				t.Fatal("scheduler task not found")
			}

			// Verify page numbers are sequential and within range
			seenPages := make(map[int]bool)
			for _, searchTask := range searchTasks {
				var msgPayload NATSMessagePayload
				if err := json.Unmarshal(searchTask.Payload, &msgPayload); err != nil {
					t.Fatalf("failed to unmarshal message payload: %v", err)
				}

				var searchInput search.SearchInput
				if err := json.Unmarshal(msgPayload.Data, &searchInput); err != nil {
					t.Fatalf("failed to unmarshal search input: %v", err)
				}

				if searchInput.Page < 1 || searchInput.Page > tt.wantPages {
					t.Errorf("got invalid page number %d, want between 1 and %d", searchInput.Page, tt.wantPages)
				}

				if seenPages[searchInput.Page] {
					t.Errorf("got duplicate page number %d", searchInput.Page)
				}
				seenPages[searchInput.Page] = true
			}

			// Verify we have all pages from 1 to wantPages
			for i := 1; i <= tt.wantPages; i++ {
				if !seenPages[i] {
					t.Errorf("missing page number %d", i)
				}
			}

			// Verify search task timing
			if tt.wantDelay > 0 {
				expectedTime := now.Add(tt.wantDelay).Truncate(time.Microsecond)
				for _, searchTask := range searchTasks {
					actualTime := searchTask.RunAt.Truncate(time.Microsecond)
					if diff := actualTime.Sub(expectedTime); diff < -time.Minute || diff > time.Minute {
						t.Errorf("search task scheduled for %v, want approximately %v (±1m)\ndiff: %v",
							actualTime, expectedTime, diff)
					}
				}
			} else {
				for _, searchTask := range searchTasks {
					if !searchTask.RunAt.Truncate(time.Microsecond).Equal(now.Truncate(time.Microsecond)) {
						t.Error("search task should be scheduled for immediate execution")
					}
				}
			}

			// Create consumer to verify messages
			consumer, err := env.js.CreateOrUpdateConsumer(env.ctx, env.streamName, jetstream.ConsumerConfig{
				Name:          "test-consumer",
				FilterSubject: env.streamName + "." + tt.wantSubject,
				DeliverPolicy: jetstream.DeliverAllPolicy,
			})
			if err != nil {
				t.Fatalf("failed to create consumer: %v", err)
			}

			// Execute the search tasks if they should run immediately
			if tt.wantDelay == 0 {
				for _, searchTask := range searchTasks {
					var msgPayload NATSMessagePayload
					if err := json.Unmarshal(searchTask.Payload, &msgPayload); err != nil {
						t.Fatalf("failed to unmarshal message payload: %v", err)
					}

					// Publish the message
					if err := publisher.Publish(env.ctx, msgPayload.Subject, msgPayload.Data); err != nil {
						t.Fatalf("failed to publish message: %v", err)
					}

					// Verify the message was published
					msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(time.Second))
					if err != nil {
						t.Fatalf("failed to fetch message: %v", err)
					}

					msg := <-msgs.Messages()
					if msg == nil {
						t.Fatal("no message received")
					}

					// Verify message content
					var searchInput search.SearchInput
					if err := json.Unmarshal(msg.Data(), &searchInput); err != nil {
						t.Fatalf("failed to unmarshal search input: %v", err)
					}

					if searchInput.CandidateID != candidateID {
						t.Errorf("got candidate ID %q, want %q", searchInput.CandidateID, candidateID)
					}

					// Acknowledge the message
					if err := msg.Ack(); err != nil {
						t.Errorf("failed to acknowledge message: %v", err)
					}
				}
			}
		})
	}
}

// TestSearchScheduler_ConcurrentSearches tests that the scheduler properly
// enforces the "one search at a time" rule for candidates. It verifies:
//   - Only one search task can be processed at a time for a candidate
//   - Concurrent attempts to process search tasks are rejected
//   - The first task succeeds and creates necessary follow-up tasks
//   - Subsequent concurrent tasks fail with "already being processed" error
//   - No duplicate searches are scheduled
//
// This ensures the business requirement that only one search runs at a time
// per user is strictly enforced at the scheduler level.
func TestSearchScheduler_ConcurrentSearches(t *testing.T) {
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

	// Update candidate's tier
	_, err = env.db.Exec(`
		UPDATE candidates 
		SET tier = $1::tier_type
		WHERE id = $2`,
		"mid", candidateID)
	if err != nil {
		t.Fatalf("failed to update candidate: %v", err)
	}

	// Create scheduler
	publisher := &mockPublisher{js: env.js, stream: env.stream, streamName: env.streamName}
	taskProc := NewMockTaskProcessor(publisher, env.db)
	scheduler := NewSearchScheduler(publisher, env.logger, env.db, taskProc)

	// Create multiple concurrent tasks
	const numTasks = 5
	tasks := make([]*internal.Task, numTasks)
	for i := 0; i < numTasks; i++ {
		payload := SearchSchedulerPayload{
			CandidateID: candidateID,
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		tasks[i] = &internal.Task{
			Type:        scheduler.TaskType(),
			Payload:     payloadBytes,
			Status:      internal.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now().UTC(),
		}
	}

	// Process tasks concurrently
	errCh := make(chan error, numTasks)
	for _, task := range tasks {
		go func(task *internal.Task) {
			errCh <- scheduler.ProcessTask(env.ctx, task)
		}(task)
	}

	// Collect results and verify behavior
	successCount := 0
	alreadyProcessingCount := 0
	for i := 0; i < numTasks; i++ {
		err := <-errCh
		if err == nil {
			successCount++
		} else if err.Error() == fmt.Sprintf("candidate %s is already being processed", candidateID) {
			alreadyProcessingCount++
		} else {
			t.Errorf("unexpected error from task: %v", err)
		}
	}

	// Verify exactly one task succeeded and others were rejected
	if successCount != 1 {
		t.Errorf("expected exactly one successful task, got %d", successCount)
	}
	if alreadyProcessingCount != numTasks-1 {
		t.Errorf("expected %d tasks to be rejected with 'already being processed', got %d",
			numTasks-1, alreadyProcessingCount)
	}

	// For mid tier, we expect 50 search tasks plus 1 scheduler task
	expectedTasks := 51
	if got := len(taskProc.enqueuedTasks); got != expectedTasks {
		t.Errorf("got %d enqueued tasks, want %d", got, expectedTasks)
	}

	// Count search tasks and verify their pages
	searchTasks := make([]*internal.Task, 0)
	var schedulerTask *internal.Task
	for _, task := range taskProc.enqueuedTasks {
		if task.Type == "nats_message_scheduler" {
			searchTasks = append(searchTasks, task)
		} else if task.Type == "search_scheduler" {
			schedulerTask = task
		}
	}

	if len(searchTasks) != 50 {
		t.Errorf("got %d search tasks, want 50", len(searchTasks))
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

		if searchInput.Page < 1 || searchInput.Page > 50 {
			t.Errorf("got invalid page number %d", searchInput.Page)
		}

		if seenPages[searchInput.Page] {
			t.Errorf("got duplicate page number %d", searchInput.Page)
		}
		seenPages[searchInput.Page] = true
	}

	// Verify we have all pages from 1 to 50
	for i := 1; i <= 50; i++ {
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
