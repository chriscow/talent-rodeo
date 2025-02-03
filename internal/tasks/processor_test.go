package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/models"
)

// MockTaskHandler is a test handler that records task executions
type MockTaskHandler struct {
	processedTasks []*models.Task
	shouldError    bool
	taskType       string
}

func (h *MockTaskHandler) ProcessTask(ctx context.Context, task *models.Task) error {
	if h.shouldError {
		return errors.New("mock error")
	}
	h.processedTasks = append(h.processedTasks, task)
	return nil
}

func (h *MockTaskHandler) TaskType() string {
	return h.taskType
}

func setupTestProcessor(t *testing.T) (*Processor, *database.DB) {
	t.Helper()
	// Create an in-memory database for testing
	db, err := database.New(":memory:", true)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Enable foreign key support
	_, err = db.DB.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	// Create the tasks table
	_, err = db.DB.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			payload BLOB,
			status TEXT NOT NULL CHECK (
				status IN (
					'pending',
					'running',
					'complete',
					'failed',
					'retrying',
					'cancelled'
				)
			),
			attempts INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			run_at DATETIME NOT NULL,
			error TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_tasks_status_run_at ON tasks (status, run_at)
		WHERE
			status IN ('pending', 'retrying');

		CREATE TRIGGER IF NOT EXISTS tasks_updated_at 
			AFTER UPDATE ON tasks
		BEGIN
			UPDATE tasks 
			SET updated_at = CURRENT_TIMESTAMP
			WHERE id = NEW.id;
		END;
	`)
	if err != nil {
		t.Fatalf("failed to create tasks table: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	processor := NewProcessor(db, logger)
	return processor, db
}

func TestProcessor_RegisterHandler(t *testing.T) {
	processor, _ := setupTestProcessor(t)

	handler := &MockTaskHandler{taskType: "test_task"}
	processor.RegisterHandler(handler)

	got := len(processor.handlers)
	want := 1
	if got != want {
		t.Errorf("handler count = %d; want %d", got, want)
	}

	gotHandler := processor.handlers["test_task"]
	if gotHandler != handler {
		t.Errorf("registered handler = %v; want %v", gotHandler, handler)
	}
}

func TestProcessor_EnqueueTask(t *testing.T) {
	t.Run("enqueue task with unregistered type", func(t *testing.T) {
		processor, _ := setupTestProcessor(t)
		ctx := context.Background()

		_, err := processor.EnqueueTask(ctx, "unknown_type", []byte("{}"), time.Now())
		if err == nil {
			t.Error("expected error for unregistered handler, got nil")
		}
		if err != nil && err.Error() != "no handler registered for task type: unknown_type" {
			t.Errorf("got error = %v; want 'no handler registered for task type: unknown_type'", err)
		}
	})

	t.Run("enqueue task successfully", func(t *testing.T) {
		processor, _ := setupTestProcessor(t)
		ctx := context.Background()

		handler := &MockTaskHandler{taskType: "test_task"}
		processor.RegisterHandler(handler)

		payload := map[string]string{"test": "data"}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		runAt := time.Now().Add(time.Hour)
		task, err := processor.EnqueueTask(ctx, "test_task", payloadBytes, runAt)
		if err != nil {
			t.Fatalf("failed to enqueue task: %v", err)
		}

		if task.ID == "" {
			t.Error("task ID is empty")
		}
		if got := task.Type; got != "test_task" {
			t.Errorf("task type = %q; want %q", got, "test_task")
		}
		if got := task.Status; got != models.TaskStatusPending {
			t.Errorf("task status = %v; want %v", got, models.TaskStatusPending)
		}
		got := string(task.Payload)
		if got != string(payloadBytes) {
			t.Errorf("task payload = %s; want %s", got, payloadBytes)
		}
		if got := task.RunAt.Unix(); got != runAt.Unix() {
			t.Errorf("task runAt = %v; want %v", got, runAt.Unix())
		}
	})
}

func TestProcessor_ProcessPendingTasks(t *testing.T) {
	processor, db := setupTestProcessor(t)
	ctx := context.Background()

	handler := &MockTaskHandler{taskType: "test_task"}
	processor.RegisterHandler(handler)

	// Create a test task
	task := &models.Task{
		ID:          "test-id",
		Type:        "test_task",
		Payload:     []byte("{}"),
		Status:      models.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       time.Now().Add(-time.Hour), // In the past so it's ready to process
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.CreateTask(task); err != nil {
		t.Fatalf("failed to create test task: %v", err)
	}

	t.Run("process pending task successfully", func(t *testing.T) {
		if err := processor.processPendingTasks(ctx); err != nil {
			t.Fatalf("failed to process pending tasks: %v", err)
		}

		// Wait for goroutines to complete
		processor.wg.Wait()

		// Verify the task was processed
		processedTask, err := db.GetTask(task.ID)
		if err != nil {
			t.Fatalf("failed to get processed task: %v", err)
		}

		if got := processedTask.Status; got != models.TaskStatusComplete {
			t.Errorf("task status = %v; want %v", got, models.TaskStatusComplete)
		}
		if got := processedTask.Attempts; got != 1 {
			t.Errorf("task attempts = %d; want %d", got, 1)
		}
		if got := processedTask.Error; got != "" {
			t.Errorf("task error = %q; want empty string", got)
		}

		// Verify the handler processed the task
		if got := len(handler.processedTasks); got != 1 {
			t.Errorf("processed tasks count = %d; want %d", got, 1)
		}
		if got := handler.processedTasks[0].ID; got != task.ID {
			t.Errorf("processed task ID = %q; want %q", got, task.ID)
		}
	})

	t.Run("handle task processing error", func(t *testing.T) {
		// Create a new task that will fail
		failingTask := &models.Task{
			ID:          "failing-task",
			Type:        "test_task",
			Payload:     []byte("{}"),
			Status:      models.TaskStatusPending,
			MaxAttempts: 2,
			RunAt:       time.Now().Add(-time.Hour),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		if err := db.CreateTask(failingTask); err != nil {
			t.Fatalf("failed to create failing task: %v", err)
		}

		// Make the handler return an error
		handler.shouldError = true

		if err := processor.processPendingTasks(ctx); err != nil {
			t.Fatalf("failed to process pending tasks: %v", err)
		}

		// Wait for goroutines to complete
		processor.wg.Wait()

		// Verify the task was marked for retry
		processedTask, err := db.GetTask(failingTask.ID)
		if err != nil {
			t.Fatalf("failed to get processed task: %v", err)
		}

		if got := processedTask.Status; got != models.TaskStatusRetrying {
			t.Errorf("task status = %v; want %v", got, models.TaskStatusRetrying)
		}
		if got := processedTask.Attempts; got != 1 {
			t.Errorf("task attempts = %d; want %d", got, 1)
		}
		if got := processedTask.Error; got == "" {
			t.Error("task error is empty, want non-empty error message")
		}

		// Process again to exceed max attempts
		// First, update the task's RunAt time to the past so it will be picked up
		processedTask.RunAt = time.Now().Add(-time.Hour)
		if err := db.UpdateTask(processedTask); err != nil {
			t.Fatalf("failed to update task run_at: %v", err)
		}

		if err := processor.processPendingTasks(ctx); err != nil {
			t.Fatalf("failed to process pending tasks: %v", err)
		}

		// Wait for goroutines to complete
		processor.wg.Wait()

		// Verify the task was marked as failed
		processedTask, err = db.GetTask(failingTask.ID)
		if err != nil {
			t.Fatalf("failed to get processed task: %v", err)
		}

		if got := processedTask.Status; got != models.TaskStatusFailed {
			t.Errorf("task status = %v; want %v", got, models.TaskStatusFailed)
		}
		if got := processedTask.Attempts; got != 2 {
			t.Errorf("task attempts = %d; want %d", got, 2)
		}
		if got := processedTask.Error; got == "" {
			t.Error("task error is empty, want non-empty error message")
		}
	})
}

func TestProcessor_StartStop(t *testing.T) {
	processor, _ := setupTestProcessor(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the processor in a goroutine
	errCh := make(chan error)
	go func() {
		errCh <- processor.Start(ctx)
	}()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Stop the processor
	processor.Stop()

	// Verify it stopped without error
	if err := <-errCh; err != nil {
		t.Errorf("processor stopped with error: %v", err)
	}
}
