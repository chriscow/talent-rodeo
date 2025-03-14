package handlers

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
	"talent-rodeo/internal/handlers/search"

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
	scheduler := &SearchScheduler{}
	now := time.Now().UTC()

	tests := []struct {
		name       string
		tier       string
		lastSearch time.Time
		want       time.Duration // Duration from now
	}{
		{
			name:       "free tier with no previous search",
			tier:       "free",
			lastSearch: time.Time{},
			want:       0,
		},
		{
			name:       "free tier with recent search",
			tier:       "free",
			lastSearch: now.Add(-24 * time.Hour),
			want:       48 * time.Hour, // 72h - 24h = 48h remaining
		},
		{
			name:       "mid tier with recent search",
			tier:       "mid",
			lastSearch: now.Add(-12 * time.Hour),
			want:       12 * time.Hour, // 24h - 12h = 12h remaining
		},
		{
			name:       "top tier with recent search",
			tier:       "top",
			lastSearch: now.Add(-2 * time.Hour),
			want:       4 * time.Hour, // 6h - 2h = 4h remaining
		},
		{
			name:       "overdue search",
			tier:       "free",
			lastSearch: now.Add(-96 * time.Hour),
			want:       0, // Should schedule immediately
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scheduler.getNextSearchTime(tt.tier, tt.lastSearch)
			wantTime := now.Add(tt.want)

			// Allow 1 second tolerance for timing differences
			if diff := got.Sub(wantTime); diff > time.Second || diff < -time.Second {
				t.Errorf("getNextSearchTime() = %v, want %v (±1s)", got, wantTime)
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
	taskProc := &MockTaskProcessor{}
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

	// Verify that two tasks were enqueued (search and next scheduler)
	if got := len(taskProc.enqueuedTasks); got != 2 {
		t.Errorf("got %d enqueued tasks, want 2", got)
	}

	// Verify the search task
	var searchTask *internal.Task
	var nextSchedulerTask *internal.Task
	for _, task := range taskProc.enqueuedTasks {
		if task.Type == "nats_message_scheduler" {
			searchTask = task
		} else if task.Type == "search_scheduler" {
			nextSchedulerTask = task
		}
	}

	if searchTask == nil {
		t.Fatal("search task not found")
	}
	if nextSchedulerTask == nil {
		t.Fatal("next scheduler task not found")
	}

	// Verify search task payload
	var msgPayload NATSMessagePayload
	if err := json.Unmarshal(searchTask.Payload, &msgPayload); err != nil {
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

	// Verify task timing
	if searchTask.RunAt.After(nextSchedulerTask.RunAt) {
		t.Error("search task should run before next scheduler task")
	}
	if diff := nextSchedulerTask.RunAt.Sub(searchTask.RunAt); diff < time.Minute {
		t.Errorf("scheduler task should be at least 1 minute after search task, got %v", diff)
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
	taskProc := &MockTaskProcessor{}
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
