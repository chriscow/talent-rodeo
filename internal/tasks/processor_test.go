package tasks

/*
DO NOT DELETE THIS COMMENT

Important Database Constraints and Timing Requirements for Tasks:

1. Database Constraint on run_at vs created_at:
   - The tasks table has a check constraint: run_at >= created_at
   - This means run_at must be equal to or later than created_at
   - You cannot set run_at to a time before created_at

2. Task Processing Requirements:
   - For a task to be processed immediately in tests:
     * created_at should be set to time.Now()
     * run_at should be set to a small duration after created_at (e.g., 10ms)
     * Example:
       created_at := time.Now()
       run_at := created_at.Add(10 * time.Millisecond)

3. Task Retry Behavior:
   - When a task fails and needs to retry:
     * The new run_at time must be after the original created_at
     * Typically set to: time.Now().Add(retryBackoff * attempts)
     * The retryBackoff duration is multiplied by the number of attempts

4. Test Setup Best Practices:
   - Always set created_at to time.Now()
   - Set run_at slightly in the future from created_at
   - Wait for the duration to pass before processing (time.Sleep)
   - Example pattern:
     now := time.Now()
     task := &models.Task{
         CreatedAt: now,
         RunAt: now.Add(10 * time.Millisecond),
     }
     time.Sleep(20 * time.Millisecond) // Wait for task to be ready

5. Common Mistakes to Avoid:
   - Don't set run_at to a time before created_at
   - Don't set created_at in the past without adjusting run_at accordingly
   - Don't expect immediate processing without waiting for run_at time

Remember: These constraints exist to ensure task scheduling integrity and proper retry behavior.
*/

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/models"
	"talent-rodeo/internal/tasks/handlers"
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

	// Use the test database helper
	db := database.SetupTestDB(t)

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

		task := &models.Task{
			ID:          time.Now().UnixNano(),
			Type:        "unknown_type",
			Payload:     []byte("{}"),
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err := processor.EnqueueTask(ctx, task)
		if err == nil {
			t.Error("expected error for unregistered handler, got nil")
		}
		wantErr := "no handler registered for task type: unknown_type"
		if err != nil && err.Error() != wantErr {
			t.Errorf("got error = %v; want %q", err, wantErr)
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
		task := &models.Task{
			ID:          time.Now().UnixNano(),
			Type:        "test_task",
			Payload:     payloadBytes,
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       runAt,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err = processor.EnqueueTask(ctx, task)
		if err != nil {
			t.Fatalf("failed to enqueue task: %v", err)
		}

		if task.ID == 0 {
			t.Error("task ID is zero")
		}
		if got := task.Type; got != "test_task" {
			t.Errorf("task type = %q; want %q", got, "test_task")
		}
		if got := task.Status; got != models.TaskStatusPending {
			t.Errorf("task status = %v; want %v", got, models.TaskStatusPending)
		}
		if got := string(task.Payload); got != string(payloadBytes) {
			t.Errorf("task payload = %s; want %s", got, payloadBytes)
		}
		if got := task.RunAt.Unix(); got != runAt.Unix() {
			t.Errorf("task runAt = %v; want %v", got, runAt.Unix())
		}
	})
}

func TestProcessor_ProcessPendingTasks(t *testing.T) {
	processor, db := setupTestProcessor(t)
	// Set a short retry interval for testing
	processor.SetRetryBackoff(10 * time.Millisecond)
	ctx := context.Background()

	handler := &MockTaskHandler{taskType: "test_task"}
	processor.RegisterHandler(handler)

	// Create a test task
	task := &models.Task{
		ID:          time.Now().UnixNano(),
		Type:        "test_task",
		Payload:     []byte("{}"),
		Status:      models.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       time.Now().Add(10 * time.Millisecond), // Set to very near future
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create test task: %v", err)
	}

	// Wait for the task to be ready
	time.Sleep(20 * time.Millisecond)

	t.Run("process pending task successfully", func(t *testing.T) {
		if err := processor.processPendingTasks(ctx); err != nil {
			t.Fatalf("failed to process pending tasks: %v", err)
		}

		// Wait for goroutines to complete
		processor.wg.Wait()

		// Verify the task was processed
		processedTask, err := db.GetTask(ctx, task.ID)
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
			t.Errorf("processed task ID = %d; want %d", got, task.ID)
		}
	})

	t.Run("handle task processing error", func(t *testing.T) {
		// Create a new task that will fail
		failingTask := &models.Task{
			ID:          time.Now().UnixNano(),
			Type:        "test_task",
			Payload:     []byte("{}"),
			Status:      models.TaskStatusPending,
			MaxAttempts: 2,
			RunAt:       time.Now().Add(10 * time.Millisecond), // Set to very near future
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		if err := db.CreateTask(ctx, failingTask); err != nil {
			t.Fatalf("failed to create failing task: %v", err)
		}

		// Wait for the task to be ready
		time.Sleep(20 * time.Millisecond)

		// Make the handler return an error
		handler.shouldError = true

		if err := processor.processPendingTasks(ctx); err != nil {
			t.Fatalf("failed to process pending tasks: %v", err)
		}

		// Wait for goroutines to complete
		processor.wg.Wait()

		// Verify the task was marked for retry
		processedTask, err := db.GetTask(ctx, failingTask.ID)
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
		// Wait for the retry time to pass (10ms since it's the first attempt)
		time.Sleep(20 * time.Millisecond)

		if err := processor.processPendingTasks(ctx); err != nil {
			t.Fatalf("failed to process pending tasks: %v", err)
		}

		// Wait for goroutines to complete
		processor.wg.Wait()

		// Verify the task was marked as failed
		processedTask, err = db.GetTask(ctx, failingTask.ID)
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

func TestProcessor_CreateAndEnqueueTask(t *testing.T) {
	t.Run("create and enqueue task with unregistered type", func(t *testing.T) {
		processor, _ := setupTestProcessor(t)
		ctx := context.Background()

		_, err := processor.CreateAndEnqueueTask(ctx, "unknown_type", []byte("{}"), time.Now())
		if err == nil {
			t.Error("expected error for unregistered handler, got nil")
		}
		wantErr := "no handler registered for task type: unknown_type"
		if err != nil && err.Error() != wantErr {
			t.Errorf("got error = %v; want %q", err, wantErr)
		}
	})

	t.Run("create and enqueue task successfully", func(t *testing.T) {
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
		task, err := processor.CreateAndEnqueueTask(ctx, "test_task", payloadBytes, runAt)
		if err != nil {
			t.Fatalf("failed to create and enqueue task: %v", err)
		}

		if task.ID == 0 {
			t.Error("task ID is zero")
		}
		if got := task.Type; got != "test_task" {
			t.Errorf("task type = %q; want %q", got, "test_task")
		}
		if got := task.Status; got != models.TaskStatusPending {
			t.Errorf("task status = %v; want %v", got, models.TaskStatusPending)
		}
		if got := string(task.Payload); got != string(payloadBytes) {
			t.Errorf("task payload = %s; want %s", got, payloadBytes)
		}
		if got := task.RunAt.Unix(); got != runAt.Unix() {
			t.Errorf("task runAt = %v; want %v", got, runAt.Unix())
		}
		if got := task.MaxAttempts; got != 3 {
			t.Errorf("task max attempts = %d; want %d", got, 3)
		}
	})
}

// TestTaskProcessorInterface verifies that Processor implements the TaskProcessor interface
func TestTaskProcessorInterface(t *testing.T) {
	t.Run("verify processor implements TaskProcessor interface", func(t *testing.T) {
		processor, _ := setupTestProcessor(t)

		// This is a compile-time check that Processor implements TaskProcessor
		var _ handlers.TaskProcessor = processor

		// Test the interface method
		ctx := context.Background()
		task := &models.Task{
			ID:          time.Now().UnixNano(),
			Type:        "test_task",
			Payload:     []byte("{}"),
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now().Add(time.Hour), // Set to future time
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		handler := &MockTaskHandler{taskType: "test_task"}
		processor.RegisterHandler(handler)

		err := processor.EnqueueTask(ctx, task)
		if err != nil {
			t.Errorf("EnqueueTask failed: %v", err)
		}
	})
}

func TestProcessor_ConcurrentTaskProcessing(t *testing.T) {
	processor, db := setupTestProcessor(t)
	ctx := context.Background()

	// Register handler
	handler := &MockTaskHandler{taskType: "test_task"}
	processor.RegisterHandler(handler)

	// Create multiple tasks
	numTasks := 5
	tasks := make([]*models.Task, numTasks)
	for i := 0; i < numTasks; i++ {
		now := time.Now()
		tasks[i] = &models.Task{
			ID:          time.Now().UnixNano() + int64(i),
			Type:        "test_task",
			Payload:     []byte(fmt.Sprintf(`{"index": %d}`, i)),
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       now.Add(10 * time.Millisecond), // Set run_at slightly in the future
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := db.CreateTask(ctx, tasks[i]); err != nil {
			t.Fatalf("failed to create task %d: %v", i, err)
		}
	}

	// Wait for tasks to be ready to run
	time.Sleep(20 * time.Millisecond)

	// Process tasks
	if err := processor.processPendingTasks(ctx); err != nil {
		t.Fatalf("failed to process pending tasks: %v", err)
	}

	// Wait for all goroutines to complete
	processor.wg.Wait()

	// Verify all tasks were processed
	for i, task := range tasks {
		processedTask, err := db.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("failed to get processed task %d: %v", i, err)
		}

		if got := processedTask.Status; got != models.TaskStatusComplete {
			t.Errorf("task %d status = %v; want %v", i, got, models.TaskStatusComplete)
		}
		if got := processedTask.Attempts; got != 1 {
			t.Errorf("task %d attempts = %d; want %d", i, got, 1)
		}
	}

	// Verify handler processed all tasks
	if got := len(handler.processedTasks); got != numTasks {
		t.Errorf("processed tasks count = %d; want %d", got, numTasks)
	}
}

func TestProcessor_TaskRetryBackoff(t *testing.T) {
	processor, db := setupTestProcessor(t)
	ctx := context.Background()

	// Set custom retry backoff
	customBackoff := 50 * time.Millisecond
	processor.SetRetryBackoff(customBackoff)

	// Register failing handler
	handler := &MockTaskHandler{
		taskType:    "test_task",
		shouldError: true,
	}
	processor.RegisterHandler(handler)

	// Create a task
	now := time.Now()
	task := &models.Task{
		ID:          time.Now().UnixNano(),
		Type:        "test_task",
		Payload:     []byte("{}"),
		Status:      models.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       now.Add(10 * time.Millisecond), // Set run_at slightly in the future
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := db.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Wait for task to be ready to run
	time.Sleep(20 * time.Millisecond)

	// Process task first time
	if err := processor.processPendingTasks(ctx); err != nil {
		t.Fatalf("failed to process pending tasks: %v", err)
	}
	processor.wg.Wait()

	// Verify task is retrying
	processedTask, err := db.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get processed task: %v", err)
	}

	if got := processedTask.Status; got != models.TaskStatusRetrying {
		t.Errorf("task status = %v; want %v", got, models.TaskStatusRetrying)
	}

	// Verify retry time is set correctly
	expectedRetryTime := task.RunAt.Add(customBackoff)
	if processedTask.RunAt.Sub(expectedRetryTime) > time.Second {
		t.Errorf("retry time = %v; want close to %v", processedTask.RunAt, expectedRetryTime)
	}

	// Wait for retry backoff
	time.Sleep(customBackoff * 2)

	// Process task second time
	if err := processor.processPendingTasks(ctx); err != nil {
		t.Fatalf("failed to process pending tasks second time: %v", err)
	}
	processor.wg.Wait()

	// Verify task is retrying with increased backoff
	processedTask, err = db.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get processed task after retry: %v", err)
	}

	if got := processedTask.Status; got != models.TaskStatusRetrying {
		t.Errorf("task status after retry = %v; want %v", got, models.TaskStatusRetrying)
	}

	// Verify increased retry time
	expectedRetryTime = task.RunAt.Add(customBackoff * 2)
	if processedTask.RunAt.Sub(expectedRetryTime) > time.Second {
		t.Errorf("retry time after retry = %v; want close to %v", processedTask.RunAt, expectedRetryTime)
	}
}
