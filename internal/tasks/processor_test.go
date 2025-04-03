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
     task := &internal.Task{
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
	"sync"
	"testing"
	"time"

	"talent-rodeo/internal"

	"cloud.google.com/go/firestore"
)

// MockTaskHandler is a test handler that records task executions
type MockTaskHandler struct {
	processedTasks []*internal.Task
	shouldError    bool
	taskType       string
}

func (h *MockTaskHandler) ProcessTask(ctx context.Context, task *internal.Task) error {
	if h.shouldError {
		return errors.New("mock error")
	}
	h.processedTasks = append(h.processedTasks, task)
	return nil
}

func (h *MockTaskHandler) TaskType() string {
	return h.taskType
}

func setupTestProcessor(t *testing.T) *Processor {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create a mock storage implementation instead of using real Firestore
	storage := NewMemoryTaskStorage()

	return NewProcessor(storage, logger)
}

// MemoryTaskStorage is a mock implementation of TaskStorage for testing
type MemoryTaskStorage struct {
	tasks map[string]*internal.Task
	mu    sync.RWMutex
}

func NewMemoryTaskStorage() *MemoryTaskStorage {
	return &MemoryTaskStorage{
		tasks: make(map[string]*internal.Task),
	}
}

func (m *MemoryTaskStorage) CreateTask(ctx context.Context, task *internal.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[task.ID] = task
	return nil
}

func (m *MemoryTaskStorage) GetTask(ctx context.Context, id string) (*internal.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, exists := m.tasks[id]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	return task, nil
}

func (m *MemoryTaskStorage) UpdateTask(ctx context.Context, task *internal.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[task.ID] = task
	return nil
}

func (m *MemoryTaskStorage) DeleteTask(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tasks, id)
	return nil
}

func (m *MemoryTaskStorage) GetPendingTasks(ctx context.Context) ([]*internal.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var pendingTasks []*internal.Task
	now := time.Now()
	for _, task := range m.tasks {
		// Include both pending and retrying tasks that are ready to run
		if (task.Status == internal.TaskStatusPending || task.Status == internal.TaskStatusRetrying) && task.RunAt.Before(now) {
			pendingTasks = append(pendingTasks, task)
		}
	}
	return pendingTasks, nil
}

func (m *MemoryTaskStorage) RunTransaction(ctx context.Context, fn func(context.Context, *firestore.Transaction) error) error {
	// For testing purposes, we just execute the function without a real transaction
	return fn(ctx, nil)
}

func cleanupProcessorTestTasks(t *testing.T, processor *Processor) {
	t.Helper()
	ctx := context.Background()

	// Get all tasks
	tasks, err := processor.storage.GetPendingTasks(ctx)
	if err != nil {
		t.Logf("failed to get tasks for cleanup: %v", err)
		return
	}

	// Delete all tasks
	for _, task := range tasks {
		if err := processor.storage.DeleteTask(ctx, task.ID); err != nil {
			t.Logf("failed to delete task %s during cleanup: %v", task.ID, err)
		}
	}
}

func TestProcessor_RegisterHandler(t *testing.T) {
	processor := setupTestProcessor(t)

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
		processor := setupTestProcessor(t)
		defer cleanupProcessorTestTasks(t, processor)
		ctx := context.Background()

		task := &internal.Task{
			ID:          fmt.Sprintf("test-task-%d", time.Now().UnixNano()),
			Name:        "Test Unknown Task",
			Type:        "unknown_type",
			Payload:     []byte("{}"),
			Status:      internal.TaskStatusPending,
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
		processor := setupTestProcessor(t)
		defer cleanupProcessorTestTasks(t, processor)
		ctx := context.Background()

		handler := &MockTaskHandler{taskType: "test_task"}
		processor.RegisterHandler(handler)

		payload := map[string]string{"test": "data"}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		runAt := time.Now().Add(time.Hour)
		task := &internal.Task{
			ID:          fmt.Sprintf("test-task-%d", time.Now().UnixNano()),
			Name:        "Test Task",
			Type:        "test_task",
			Payload:     payloadBytes,
			Status:      internal.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       runAt,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err = processor.EnqueueTask(ctx, task)
		if err != nil {
			t.Fatalf("failed to enqueue task: %v", err)
		}

		if task.ID == "" {
			t.Error("task ID is empty")
		}
		if got := task.Name; got != "Test Task" {
			t.Errorf("task name = %q; want %q", got, "Test Task")
		}
		if got := task.Type; got != "test_task" {
			t.Errorf("task type = %q; want %q", got, "test_task")
		}
		if got := task.Status; got != internal.TaskStatusPending {
			t.Errorf("task status = %v; want %v", got, internal.TaskStatusPending)
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
	processor := setupTestProcessor(t)
	storage := processor.storage
	defer cleanupProcessorTestTasks(t, processor)
	// Set a short retry interval for testing
	processor.SetRetryBackoff(10 * time.Millisecond)
	ctx := context.Background()

	handler := &MockTaskHandler{taskType: "test_task"}
	processor.RegisterHandler(handler)

	// Create a test task
	task := &internal.Task{
		ID:          fmt.Sprintf("test-task-%d", time.Now().UnixNano()),
		Name:        "Test Processing Task",
		Type:        "test_task",
		Payload:     []byte("{}"),
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       time.Now().Add(10 * time.Millisecond), // Set to very near future
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	t.Logf("Creating test task: id=%s, name=%s, type=%s, status=%s, run_at=%v",
		task.ID, task.Name, task.Type, task.Status, task.RunAt)

	if err := processor.storage.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create test task: %v", err)
	}

	// Wait for the task to be ready
	t.Log("Waiting for task to be ready for processing...")
	time.Sleep(20 * time.Millisecond)

	t.Run("process pending task successfully", func(t *testing.T) {
		t.Logf("Starting to process task: id=%s, name=%s", task.ID, task.Name)

		if err := processor.processPendingTasks(ctx); err != nil {
			t.Fatalf("failed to process pending tasks: %v", err)
		}

		// Wait for goroutines to complete
		t.Log("Waiting for processing goroutines to complete...")
		processor.wg.Wait()

		// Verify the task was processed
		processedTask, err := storage.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("failed to get processed task: %v", err)
		}

		t.Logf("Task processing completed: id=%s, name=%s, status=%s, attempts=%d",
			processedTask.ID, processedTask.Name, processedTask.Status, processedTask.Attempts)

		if got := processedTask.Status; got != internal.TaskStatusComplete {
			t.Errorf("task status = %v; want %v", got, internal.TaskStatusComplete)
		}
		if got := processedTask.Name; got != task.Name {
			t.Errorf("task name = %q; want %q", got, task.Name)
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
			t.Errorf("processed task ID = %s; want %s", got, task.ID)
		}
		if got := handler.processedTasks[0].Name; got != task.Name {
			t.Errorf("processed task name = %q; want %q", got, task.Name)
		}
	})

	t.Run("handle task processing error", func(t *testing.T) {
		// Create a failing task
		failingTask := &internal.Task{
			ID:          fmt.Sprintf("test-task-%d", time.Now().UnixNano()),
			Name:        "Test Failing Task",
			Type:        "test_task",
			Payload:     []byte("{}"),
			Status:      internal.TaskStatusPending,
			MaxAttempts: 2,
			RunAt:       time.Now().Add(10 * time.Millisecond), // Set to very near future
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Attempts:    0,
		}

		t.Logf("Creating failing task: id=%s, name=%s, type=%s, status=%s",
			failingTask.ID, failingTask.Name, failingTask.Type, failingTask.Status)

		if err := storage.CreateTask(ctx, failingTask); err != nil {
			t.Fatalf("failed to create failing task: %v", err)
		}

		// Wait for the task to be ready
		t.Log("Waiting for failing task to be ready...")
		time.Sleep(20 * time.Millisecond)

		// Make the handler return an error
		handler.shouldError = true

		t.Log("Processing failing task with error-generating handler...")
		if err := processor.processPendingTasks(ctx); err != nil {
			t.Fatalf("failed to process pending tasks: %v", err)
		}

		// Wait for goroutines to complete
		processor.wg.Wait()

		// Verify the task was marked for retry
		processedTask, err := storage.GetTask(ctx, failingTask.ID)
		if err != nil {
			t.Fatalf("failed to get processed task: %v", err)
		}

		t.Logf("First attempt completed: status=%s, attempts=%d, error=%s",
			processedTask.Status, processedTask.Attempts, processedTask.Error)

		if got := processedTask.Status; got != internal.TaskStatusRetrying {
			t.Errorf("task status = %v; want %v", got, internal.TaskStatusRetrying)
		}
		if got := processedTask.Name; got != failingTask.Name {
			t.Errorf("task name = %q; want %q", got, failingTask.Name)
		}
		if got := processedTask.Attempts; got != 1 {
			t.Errorf("task attempts = %d; want %d", got, 1)
		}
		if got := processedTask.Error; got == "" {
			t.Error("task error is empty, want non-empty error message")
		}

		// Process again to exceed max attempts
		t.Log("Waiting for retry backoff period...")
		time.Sleep(20 * time.Millisecond)

		t.Log("Processing failing task second time...")
		if err := processor.processPendingTasks(ctx); err != nil {
			t.Fatalf("failed to process pending tasks: %v", err)
		}

		// Wait for goroutines to complete
		processor.wg.Wait()

		// Verify the task was marked as failed
		processedTask, err = storage.GetTask(ctx, failingTask.ID)
		if err != nil {
			t.Fatalf("failed to get processed task: %v", err)
		}

		t.Logf("Second attempt completed: status=%s, attempts=%d, error=%s",
			processedTask.Status, processedTask.Attempts, processedTask.Error)

		if got := processedTask.Status; got != internal.TaskStatusFailed {
			t.Errorf("task status = %v; want %v", got, internal.TaskStatusFailed)
		}
		if got := processedTask.Name; got != failingTask.Name {
			t.Errorf("task name = %q; want %q", got, failingTask.Name)
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
	processor := setupTestProcessor(t)
	defer cleanupProcessorTestTasks(t, processor)
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
		processor := setupTestProcessor(t)
		defer cleanupProcessorTestTasks(t, processor)
		ctx := context.Background()

		_, err := processor.CreateAndEnqueueTask(ctx, "unknown_type", "Test Unknown Task", []byte("{}"), time.Now())
		if err == nil {
			t.Error("expected error for unregistered handler, got nil")
		}
		wantErr := "no handler registered for task type: unknown_type"
		if err != nil && err.Error() != wantErr {
			t.Errorf("got error = %v; want %q", err, wantErr)
		}
	})

	t.Run("create and enqueue task successfully", func(t *testing.T) {
		processor := setupTestProcessor(t)
		defer cleanupProcessorTestTasks(t, processor)
		ctx := context.Background()

		handler := &MockTaskHandler{taskType: "test_task"}
		processor.RegisterHandler(handler)

		payload := map[string]string{"test": "data"}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		runAt := time.Now().Add(time.Hour)
		task, err := processor.CreateAndEnqueueTask(ctx, "test_task", "Test Task", payloadBytes, runAt)
		if err != nil {
			t.Fatalf("failed to create and enqueue task: %v", err)
		}

		if task.ID == "" {
			t.Error("task ID is empty")
		}
		if got := task.Name; got != "Test Task" {
			t.Errorf("task name = %q; want %q", got, "Test Task")
		}
		if got := task.Type; got != "test_task" {
			t.Errorf("task type = %q; want %q", got, "test_task")
		}
		if got := task.Status; got != internal.TaskStatusPending {
			t.Errorf("task status = %v; want %v", got, internal.TaskStatusPending)
		}
		if got := string(task.Payload); got != string(payloadBytes) {
			t.Errorf("task payload = %s; want %s", got, payloadBytes)
		}
		if got := task.RunAt.Unix(); got != runAt.Unix() {
			t.Errorf("task runAt = %v; want %v", got, runAt.Unix())
		}
	})
}

// TestTaskProcessorInterface verifies that Processor implements the TaskProcessor interface
func TestTaskProcessorInterface(t *testing.T) {
	t.Run("verify processor implements TaskProcessor interface", func(t *testing.T) {
		processor := setupTestProcessor(t)
		defer cleanupProcessorTestTasks(t, processor)

		// Test the interface method
		ctx := context.Background()
		task := &internal.Task{
			ID:          fmt.Sprintf("test-task-%d", time.Now().UnixNano()),
			Type:        "test_task",
			Payload:     []byte("{}"),
			Status:      internal.TaskStatusPending,
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
	processor := setupTestProcessor(t)
	defer cleanupProcessorTestTasks(t, processor)
	ctx := context.Background()

	// Register handler
	handler := &MockTaskHandler{taskType: "test_task"}
	processor.RegisterHandler(handler)

	// Create multiple tasks
	numTasks := 5
	tasks := make([]*internal.Task, numTasks)
	for i := 0; i < numTasks; i++ {
		now := time.Now()
		tasks[i] = &internal.Task{
			ID:          fmt.Sprintf("test-task-%d", time.Now().UnixNano()+int64(i)),
			Type:        "test_task",
			Payload:     []byte(fmt.Sprintf(`{"index": %d}`, i)),
			Status:      internal.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       now.Add(10 * time.Millisecond), // Set run_at slightly in the future
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := processor.storage.CreateTask(ctx, tasks[i]); err != nil {
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
		processedTask, err := processor.storage.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("failed to get processed task %d: %v", i, err)
		}

		if got := processedTask.Status; got != internal.TaskStatusComplete {
			t.Errorf("task %d status = %v; want %v", i, got, internal.TaskStatusComplete)
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
	processor := setupTestProcessor(t)
	defer cleanupProcessorTestTasks(t, processor)
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
	task := &internal.Task{
		ID:          fmt.Sprintf("test-task-%d", time.Now().UnixNano()),
		Type:        "test_task",
		Payload:     []byte("{}"),
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       now.Add(10 * time.Millisecond), // Set run_at slightly in the future
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := processor.storage.CreateTask(ctx, task); err != nil {
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
	processedTask, err := processor.storage.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get processed task: %v", err)
	}

	if got := processedTask.Status; got != internal.TaskStatusRetrying {
		t.Errorf("task status = %v; want %v", got, internal.TaskStatusRetrying)
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
	processedTask, err = processor.storage.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get processed task after retry: %v", err)
	}

	if got := processedTask.Status; got != internal.TaskStatusRetrying {
		t.Errorf("task status after retry = %v; want %v", got, internal.TaskStatusRetrying)
	}

	// Verify increased retry time
	expectedRetryTime = task.RunAt.Add(customBackoff * 2)
	if processedTask.RunAt.Sub(expectedRetryTime) > time.Second {
		t.Errorf("retry time after retry = %v; want close to %v", processedTask.RunAt, expectedRetryTime)
	}
}
