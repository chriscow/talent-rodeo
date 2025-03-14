// Package handlers provides task handlers for processing various types of scheduled jobs.
package handlers

/*
Important Notes for Search Scheduler Edge Case Tests:

1. Concurrency and Task Processing:
   - Only one search task can be processed at a time per candidate
   - Concurrent attempts are rejected with a non-retryable TaskError
   - Database locking (FOR UPDATE NOWAIT) ensures exclusive access
   - Failed concurrent attempts should not affect the successful task's state

2. Error Handling:
   - Provider API failures (timeouts, rate limits, invalid responses)
   - Database connection issues
   - NATS connection/publishing failures
   - Invalid message payloads
   - Non-retryable errors for concurrent processing
   - Tasks should not update state (last_search_at, last_page) on failure

3. Data Validation:
   - Invalid tier values are rejected with clear error messages
   - Only 'free', 'mid', and 'top' are valid tier values
   - Tier validation happens before any task processing
   - Nil timestamps and last_page are handled gracefully

4. State Management:
   - last_search_at and last_page are only updated after successful message publishing
   - State updates are atomic within the transaction
   - Page continuity is maintained across searches
   - Page limits are enforced per tier (free: 3, mid: 50, top: 200)

5. Task Scheduling:
   - Search frequency varies by tier (free: 72h, mid: 24h, top: 6h)
   - Next scheduler task is created after successful processing
   - Future searches start from page 1
   - Interrupted searches can resume from last_page + 1

6. Testing Guidelines:
   - Use MockTaskProcessor to verify task creation
   - Use mockFailingPublisher to simulate NATS failures
   - Verify database state after task processing
   - Check for correct error types and messages
   - Ensure cleanup after test failures
*/

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"talent-rodeo/internal"

	"github.com/nats-io/nats.go/jetstream"
)

// mockFailingPublisher simulates various NATS publishing failures
type mockFailingPublisher struct {
	js         jetstream.JetStream
	stream     jetstream.Stream
	streamName string
	failMode   string
}

func (p *mockFailingPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	switch p.failMode {
	case "timeout":
		return context.DeadlineExceeded
	case "connection":
		return errors.New("nats: connection closed")
	case "stream":
		return errors.New("nats: stream not found")
	default:
		return nil
	}
}

func TestSearchScheduler_ProviderFailures(t *testing.T) {
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

	tests := []struct {
		name      string
		failMode  string
		wantError bool
		setup     func(t *testing.T)
		verify    func(t *testing.T, taskProc *MockTaskProcessor)
	}{
		{
			name:      "nats_timeout",
			failMode:  "timeout",
			wantError: true,
			setup: func(t *testing.T) {
				// Set up a candidate ready for search
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
			verify: func(t *testing.T, taskProc *MockTaskProcessor) {
				// Verify last_search_at and last_page weren't updated
				var candidate struct {
					LastSearchAt *time.Time `db:"last_search_at"`
					LastPage     *int       `db:"last_page"`
				}
				err := env.db.GetContext(env.ctx, &candidate,
					`SELECT last_search_at, last_page 
					FROM candidates 
					WHERE id = $1`,
					candidateID)
				if err != nil {
					t.Fatalf("failed to get candidate: %v", err)
				}
				if candidate.LastSearchAt != nil {
					t.Error("last_search_at should not be updated on failure")
				}
				if candidate.LastPage != nil {
					t.Error("last_page should not be updated on failure")
				}

				// Verify no tasks were enqueued
				if len(taskProc.enqueuedTasks) != 0 {
					t.Errorf("expected no enqueued tasks on failure, got %d", len(taskProc.enqueuedTasks))
				}
			},
		},
		{
			name:      "invalid_tier",
			failMode:  "",
			wantError: true,
			setup: func(t *testing.T) {
				// First add the invalid tier type
				_, err := env.db.Exec(`ALTER TYPE tier_type ADD VALUE IF NOT EXISTS 'invalid'`)
				if err != nil {
					t.Fatalf("failed to add invalid tier type: %v", err)
				}

				// Then update the candidate's tier
				_, err = env.db.Exec(`UPDATE candidates SET tier = 'invalid'::tier_type WHERE id = $1`, candidateID)
				if err != nil {
					t.Fatalf("failed to set invalid tier: %v", err)
				}
			},
			verify: func(t *testing.T, taskProc *MockTaskProcessor) {
				// Verify no tasks were enqueued
				if len(taskProc.enqueuedTasks) != 0 {
					t.Errorf("expected no enqueued tasks on failure, got %d", len(taskProc.enqueuedTasks))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run setup
			tt.setup(t)

			// Create failing publisher and task processor
			publisher := &mockFailingPublisher{
				js:         env.js,
				stream:     env.stream,
				streamName: env.streamName,
				failMode:   tt.failMode,
			}
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
			err = scheduler.ProcessTask(env.ctx, task)
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Run verification
			tt.verify(t, taskProc)
		})
	}
}

func TestSearchScheduler_ConcurrencyEdgeCases(t *testing.T) {
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

	// Set up initial candidate state
	_, err = env.db.Exec(`
		UPDATE candidates 
		SET tier = 'mid'::tier_type, 
		    last_search_at = $1
		WHERE id = $2`,
		time.Now().UTC().Add(-25*time.Hour),
		candidateID)
	if err != nil {
		t.Fatalf("failed to update candidate: %v", err)
	}

	// Create multiple concurrent tasks for the same candidate
	publisher := &mockPublisher{js: env.js, stream: env.stream, streamName: env.streamName}
	taskProc := NewMockTaskProcessor(publisher, env.db)
	scheduler := NewSearchScheduler(publisher, env.logger, env.db, taskProc)

	payload := SearchSchedulerPayload{
		CandidateID: candidateID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	// Create and process multiple tasks concurrently
	taskCount := 5
	errCh := make(chan error, taskCount)
	for i := 0; i < taskCount; i++ {
		go func() {
			task := &internal.Task{
				Type:        scheduler.TaskType(),
				Payload:     payloadBytes,
				Status:      internal.TaskStatusPending,
				MaxAttempts: 3,
				CreatedAt:   time.Now().UTC(),
				RunAt:       time.Now().UTC(),
			}
			errCh <- scheduler.ProcessTask(env.ctx, task)
		}()
	}

	// Collect results
	successCount := 0
	for i := 0; i < taskCount; i++ {
		if err := <-errCh; err == nil {
			successCount++
		}
	}

	// Verify only one task succeeded
	if successCount != 1 {
		t.Errorf("expected exactly one successful task, got %d", successCount)
	}

	// Verify last_search_at was updated exactly once
	var updateCount int
	err = env.db.GetContext(env.ctx, &updateCount, `
		SELECT COUNT(*) 
		FROM candidates 
		WHERE id = $1 
		AND last_search_at > $2`,
		candidateID,
		time.Now().UTC().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("failed to count updates: %v", err)
	}
	if updateCount != 1 {
		t.Errorf("expected exactly one last_search_at update, got %d", updateCount)
	}
}
