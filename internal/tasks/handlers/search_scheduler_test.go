// Package handlers provides task handlers for processing various types of scheduled jobs.
package handlers

/*
Important Notes for Search Scheduler Unit Tests:

1. Time Handling:
   - Use fixed time functions (h.now) for deterministic testing
   - Always truncate times to microsecond precision
   - Test both UTC and non-UTC input times
   - Zero time (time.Time{}) indicates no previous search

2. Tier-based Scheduling:
   - Free tier: 72 hour interval
   - Mid tier: 24 hour interval
   - Top tier: 6 hour interval
   - Default to free tier (72h) for unknown tiers
   - Test both exact and inexact intervals

3. Search Time Calculation:
   - Immediate scheduling if no previous search
   - Immediate scheduling if next calculated time is in the past
   - Future scheduling based on tier interval
   - Time calculations should handle DST transitions

4. Task Processing:
   - Search tasks and scheduler tasks are created in pairs
   - Scheduler task runs 1 minute after its corresponding search task
   - All tasks should have proper CreatedAt timestamps
   - RunAt must never be before CreatedAt

5. Mock Components:
   - Use MockTaskProcessor for task creation verification
   - Use mockPublisher for NATS message testing
   - Ensure mocks properly record all interactions
*/

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/services/search"

	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// searchTestEnv provides a test environment with all dependencies
type searchTestEnv struct {
	db         *database.DB
	js         jetstream.JetStream
	nc         *nats.Conn
	logger     *slog.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	cleanups   []func()
	stream     jetstream.Stream
	streamName string
}

func newSearchTestEnv(t *testing.T) *searchTestEnv {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create NATS server with JetStream enabled
	opts := test.DefaultTestOptions
	opts.JetStream = true
	srv := test.RunServer(&opts)
	t.Cleanup(func() {
		logger.Debug("cleaning up test NATS server")
		srv.Shutdown()
	})

	// Wait for server to be ready
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server failed to start")
	}

	// Connect to NATS
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}

	// Create JetStream
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("Failed to create JetStream: %v", err)
	}

	// Create unique stream name for this test
	streamName := fmt.Sprintf("TEST_STREAM_%d", time.Now().UnixNano())

	// Create stream
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      streamName,
		Subjects:  []string{fmt.Sprintf("%s.>", streamName)},
		Retention: jetstream.InterestPolicy,
	})
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	db := database.SetupTestDB(t)

	env := &searchTestEnv{
		db:         db,
		js:         js,
		nc:         nc,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		cleanups:   make([]func(), 0),
		stream:     stream,
		streamName: streamName,
	}

	// Add cleanup functions
	env.cleanups = append(env.cleanups, func() {
		logger.Debug("cleaning up test environment", "component", "test_env")
		if err := js.DeleteStream(ctx, streamName); err != nil {
			logger.Error("error deleting stream", "error", err)
		}
		cancel()
		nc.Close()
		if err := db.Close(); err != nil {
			logger.Error("error closing database", "error", err)
		}
	})

	return env
}

func (env *searchTestEnv) cleanup() {
	for _, cleanup := range env.cleanups {
		cleanup()
	}
}

// mockPublisher adapts JetStream to the Publisher interface
type mockPublisher struct {
	js         jetstream.JetStream
	stream     jetstream.Stream
	streamName string
}

func (m *mockPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	// Prefix the subject with the stream name to avoid conflicts
	streamSubject := fmt.Sprintf("%s.%s", m.streamName, subject)
	_, err := m.js.Publish(ctx, streamSubject, data)
	return err
}

func TestSearchScheduler_GetNextSearchTime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	fixedTime := func() time.Time { return now }

	tests := []struct {
		name       string
		tier       string
		lastSearch time.Time
		want       time.Time
	}{
		{
			name:       "free_tier_with_no_previous_search",
			tier:       "free",
			lastSearch: time.Time{},
			want:       now,
		},
		{
			name:       "free_tier_with_past_search",
			tier:       "free",
			lastSearch: now.Add(-73 * time.Hour),
			want:       now,
		},
		{
			name:       "free_tier_with_recent_search",
			tier:       "free",
			lastSearch: now.Add(-71 * time.Hour),
			want:       now.Add(-71 * time.Hour).Add(72 * time.Hour),
		},
		{
			name:       "mid_tier_with_past_search",
			tier:       "mid",
			lastSearch: now.Add(-25 * time.Hour),
			want:       now,
		},
		{
			name:       "mid_tier_with_recent_search",
			tier:       "mid",
			lastSearch: now.Add(-23 * time.Hour),
			want:       now.Add(-23 * time.Hour).Add(24 * time.Hour),
		},
		{
			name:       "top_tier_with_past_search",
			tier:       "top",
			lastSearch: now.Add(-7 * time.Hour),
			want:       now,
		},
		{
			name:       "top_tier_with_recent_search",
			tier:       "top",
			lastSearch: now.Add(-5 * time.Hour),
			want:       now.Add(-5 * time.Hour).Add(6 * time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheduler := &SearchScheduler{
				now: fixedTime,
			}
			got := scheduler.getNextSearchTime(tt.tier, tt.lastSearch)
			if !got.Equal(tt.want) {
				t.Errorf("getNextSearchTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSearchScheduler_ProcessTask(t *testing.T) {
	env := newSearchTestEnv(t)
	defer env.cleanup()

	// Create test user and candidate using helpers
	user, err := env.db.CreateTestUser(t)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	candidateID, err := env.db.CreateTestCandidate(t, user.ID)
	if err != nil {
		t.Fatalf("failed to create test candidate: %v", err)
	}

	// Update candidate tier and last search time
	_, err = env.db.Exec(`
		UPDATE candidates 
		SET tier = $1, last_search_at = $2
		WHERE id = $3`,
		"mid", time.Now().Add(-25*time.Hour), candidateID)
	if err != nil {
		t.Fatalf("failed to update candidate: %v", err)
	}

	// Create dependencies
	publisher := &mockPublisher{js: env.js, stream: env.stream, streamName: env.streamName}
	taskProc := NewMockTaskProcessor(publisher, env.db)
	scheduler := NewSearchScheduler(publisher, env.logger, env.db, taskProc)

	// Create and process a task
	payload := SearchSchedulerPayload{
		CandidateID: candidateID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	task := &internal.Task{
		ID:          time.Now().UnixNano(),
		Type:        scheduler.TaskType(),
		Payload:     payloadBytes,
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       time.Now(),
	}

	// Process the task
	err = scheduler.ProcessTask(env.ctx, task)
	if err != nil {
		t.Fatalf("ProcessTask() error = %v", err)
	}

	// For mid tier, we expect 50 search tasks plus 1 scheduler task
	expectedTasks := 51
	if got := len(taskProc.enqueuedTasks); got != expectedTasks {
		t.Errorf("got %d enqueued tasks, want %d", got, expectedTasks)
	}

	// Count search tasks and verify their pages
	searchTasks := make([]*internal.Task, 0)
	var nextSchedulerTask *internal.Task
	for _, task := range taskProc.enqueuedTasks {
		if task.Type == "nats_message_scheduler" {
			searchTasks = append(searchTasks, task)
		} else if task.Type == "search_scheduler" {
			nextSchedulerTask = task
		}
	}

	if len(searchTasks) != 50 {
		t.Errorf("got %d search tasks, want 50", len(searchTasks))
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

	if nextSchedulerTask == nil {
		t.Fatal("next scheduler task not found")
	}

	// Verify task timing
	for _, searchTask := range searchTasks {
		if searchTask.RunAt.After(nextSchedulerTask.RunAt) {
			t.Error("search task should run before next scheduler task")
		}
		if diff := nextSchedulerTask.RunAt.Sub(searchTask.RunAt); diff < time.Minute {
			t.Errorf("scheduler task should be at least 1 minute after search task, got %v", diff)
		}
	}
}

func TestSearchScheduler_ScheduleInitialSearch(t *testing.T) {
	env := newSearchTestEnv(t)
	defer env.cleanup()

	// Create test user and candidate using helpers
	user, err := env.db.CreateTestUser(t)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	candidateID, err := env.db.CreateTestCandidate(t, user.ID)
	if err != nil {
		t.Fatalf("failed to create test candidate: %v", err)
	}

	// Update candidate tier
	_, err = env.db.Exec(`
		UPDATE candidates 
		SET tier = $1
		WHERE id = $2`,
		"free", candidateID)
	if err != nil {
		t.Fatalf("failed to update candidate: %v", err)
	}

	// Create dependencies
	publisher := &mockPublisher{js: env.js, stream: env.stream, streamName: env.streamName}
	taskProc := NewMockTaskProcessor(publisher, env.db)
	scheduler := NewSearchScheduler(publisher, env.logger, env.db, taskProc)

	// Schedule initial search
	err = scheduler.ScheduleInitialSearch(env.ctx, candidateID)
	if err != nil {
		t.Fatalf("ScheduleInitialSearch() error = %v", err)
	}

	// Verify that one task was enqueued
	if got := len(taskProc.enqueuedTasks); got != 1 {
		t.Errorf("got %d enqueued tasks, want 1", got)
	}

	// Verify the task
	task := taskProc.enqueuedTasks[0]
	if task.Type != "search_scheduler" {
		t.Errorf("got task type %q, want %q", task.Type, "search_scheduler")
	}

	var payload SearchSchedulerPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if payload.CandidateID != candidateID {
		t.Errorf("got candidate ID %q, want %q", payload.CandidateID, candidateID)
	}

	// Verify task timing
	now := time.Now().UTC()
	if diff := task.RunAt.Sub(now); diff > time.Second {
		t.Errorf("task should run immediately, got delay of %v", diff)
	}
}
