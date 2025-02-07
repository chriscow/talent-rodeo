package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"talent-rodeo/internal"
)

func TestTaskOperations(t *testing.T) {
	db := SetupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Test payload struct
	type testPayload struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}

	// Create test task
	payload := testPayload{
		Message: "test message",
		Count:   42,
	}

	task, err := internal.NewTask("test_task", payload)
	if err != nil {
		t.Fatalf("NewTask() error = %v", err)
	}
	if task == nil {
		t.Fatal("NewTask() returned nil task")
	}

	// Set run_at to a future time to satisfy the constraint
	task.RunAt = time.Now().Add(time.Hour)

	// Test CreateTask
	err = db.CreateTask(ctx, task)
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if task.ID == 0 {
		t.Error("CreateTask() did not set task ID")
	}

	// Test GetTask
	fetchedTask, err := db.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if fetchedTask == nil {
		t.Fatal("GetTask() returned nil task")
	}
	if fetchedTask.ID != task.ID {
		t.Errorf("GetTask() got ID = %v, want %v", fetchedTask.ID, task.ID)
	}
	if fetchedTask.Type != task.Type {
		t.Errorf("GetTask() got Type = %v, want %v", fetchedTask.Type, task.Type)
	}
	if fetchedTask.Status != task.Status {
		t.Errorf("GetTask() got Status = %v, want %v", fetchedTask.Status, task.Status)
	}

	// Test payload unmarshaling
	var fetchedPayload testPayload
	err = json.Unmarshal(fetchedTask.Payload, &fetchedPayload)
	if err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if fetchedPayload.Message != payload.Message {
		t.Errorf("Payload Message = %v, want %v", fetchedPayload.Message, payload.Message)
	}
	if fetchedPayload.Count != payload.Count {
		t.Errorf("Payload Count = %v, want %v", fetchedPayload.Count, payload.Count)
	}

	// Test UpdateTask
	fetchedTask.Status = internal.TaskStatusRunning
	fetchedTask.Attempts++
	err = db.UpdateTask(ctx, fetchedTask)
	if err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}

	// Verify update
	updatedTask, err := db.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask() after update error = %v", err)
	}
	if updatedTask.Status != internal.TaskStatusRunning {
		t.Errorf("UpdateTask() got Status = %v, want %v", updatedTask.Status, internal.TaskStatusRunning)
	}
	if updatedTask.Attempts != 1 {
		t.Errorf("UpdateTask() got Attempts = %v, want 1", updatedTask.Attempts)
	}

	// Test GetPendingTasks
	pendingTasks, err := db.GetPendingTasks(ctx)
	if err != nil {
		t.Fatalf("GetPendingTasks() error = %v", err)
	}
	if len(pendingTasks) != 0 {
		t.Errorf("GetPendingTasks() got %v tasks, want 0", len(pendingTasks))
	}

	// Create a pending task
	pendingTask, err := internal.NewTask("pending_task", testPayload{Message: "pending", Count: 1})
	if err != nil {
		t.Fatalf("NewTask() error = %v", err)
	}
	// Set run_at to a time slightly in the future to satisfy the constraint
	pendingTask.RunAt = time.Now().Add(time.Millisecond)
	err = db.CreateTask(ctx, pendingTask)
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	// Sleep briefly to ensure run_at is in the past when we check
	time.Sleep(2 * time.Millisecond)

	// Verify pending tasks
	pendingTasks, err = db.GetPendingTasks(ctx)
	if err != nil {
		t.Fatalf("GetPendingTasks() error = %v", err)
	}
	if len(pendingTasks) != 1 {
		t.Errorf("GetPendingTasks() got %v tasks, want 1", len(pendingTasks))
	}
	if len(pendingTasks) > 0 && pendingTasks[0].ID != pendingTask.ID {
		t.Errorf("GetPendingTasks() got ID = %v, want %v", pendingTasks[0].ID, pendingTask.ID)
	}

	// Test DeleteTask
	err = db.DeleteTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("DeleteTask() error = %v", err)
	}

	// Verify deletion
	_, err = db.GetTask(ctx, task.ID)
	if err == nil {
		t.Error("GetTask() after delete succeeded, want error")
	}
}

func TestTaskValidation(t *testing.T) {
	db := SetupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		task        *internal.Task
		expectError bool
	}{
		{
			name: "valid task",
			task: &internal.Task{
				Type:        "test_task",
				Payload:     json.RawMessage(`{"test": "data"}`),
				Status:      internal.TaskStatusPending,
				MaxAttempts: 3,
				RunAt:       time.Now().Add(time.Hour),
			},
			expectError: false,
		},
		{
			name: "missing type",
			task: &internal.Task{
				Payload:     json.RawMessage(`{"test": "data"}`),
				Status:      internal.TaskStatusPending,
				MaxAttempts: 3,
				RunAt:       time.Now().Add(time.Hour),
			},
			expectError: true,
		},
		{
			name: "missing payload",
			task: &internal.Task{
				Type:        "test_task",
				Status:      internal.TaskStatusPending,
				MaxAttempts: 3,
				RunAt:       time.Now().Add(time.Hour),
			},
			expectError: true,
		},
		{
			name: "past run_at",
			task: &internal.Task{
				Type:        "test_task",
				Payload:     json.RawMessage(`{"test": "data"}`),
				Status:      internal.TaskStatusPending,
				MaxAttempts: 3,
				RunAt:       time.Now().Add(-time.Hour),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.CreateTask(ctx, tt.task)
			if tt.expectError && err == nil {
				t.Error("CreateTask() succeeded, want error")
			}
			if !tt.expectError && err != nil {
				t.Errorf("CreateTask() error = %v, want nil", err)
			}
		})
	}
}

func TestTaskTransactions(t *testing.T) {
	db := SetupTestDB(t)
	defer db.Close()

	t.Run("basic_CRUD_operations", func(t *testing.T) {
		task := &internal.Task{
			Type:        "test_task",
			Payload:     json.RawMessage(`{"test": "data"}`),
			Status:      "pending",
			RunAt:       time.Now().Add(time.Hour),
			MaxAttempts: 3,
		}

		// Test Create
		err := db.CreateTask(context.Background(), task)
		if err != nil {
			t.Fatalf("CreateTask() error = %v", err)
		}
		if task.ID == 0 {
			t.Error("CreateTask() did not set task ID")
		}

		// Test Get
		retrieved, err := db.GetTask(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("GetTask() error = %v", err)
		}
		if retrieved.Type != task.Type {
			t.Errorf("GetTask() got Type = %v, want %v", retrieved.Type, task.Type)
		}
		if retrieved.Status != task.Status {
			t.Errorf("GetTask() got Status = %v, want %v", retrieved.Status, task.Status)
		}
		if retrieved.MaxAttempts != task.MaxAttempts {
			t.Errorf("GetTask() got MaxAttempts = %v, want %v", retrieved.MaxAttempts, task.MaxAttempts)
		}

		// Test Update
		task.Status = "complete"
		err = db.UpdateTask(context.Background(), task)
		if err != nil {
			t.Fatalf("UpdateTask() error = %v", err)
		}

		// Verify Update
		updated, err := db.GetTask(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("GetTask() after update error = %v", err)
		}
		if updated.Status != "complete" {
			t.Errorf("UpdateTask() got Status = %v, want complete", updated.Status)
		}

		// Test Delete
		err = db.DeleteTask(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("DeleteTask() error = %v", err)
		}

		// Verify Delete
		_, err = db.GetTask(context.Background(), task.ID)
		if err == nil {
			t.Error("GetTask() after delete succeeded, want error")
		}
	})

	t.Run("basic_transaction", func(t *testing.T) {
		task := &internal.Task{
			Type:        "test_task",
			Payload:     json.RawMessage(`{"test": "data"}`),
			Status:      "pending",
			RunAt:       time.Now().Add(time.Hour),
			MaxAttempts: 3,
		}

		err := db.WithTx(context.Background(), func(tx *DB) error {
			if err := tx.CreateTask(context.Background(), task); err != nil {
				return fmt.Errorf("failed to create task in transaction: %w", err)
			}

			retrieved, err := tx.GetTask(context.Background(), task.ID)
			if err != nil {
				return fmt.Errorf("failed to get task in transaction: %w", err)
			}

			if retrieved.Type != task.Type {
				return fmt.Errorf("unexpected task type in transaction: got %s, want %s", retrieved.Type, task.Type)
			}

			return nil
		})

		if err != nil {
			t.Fatalf("WithTx() error = %v", err)
		}

		// Verify the task was created
		retrieved, err := db.GetTask(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("GetTask() after transaction error = %v", err)
		}
		if retrieved.Type != task.Type {
			t.Errorf("GetTask() after transaction got Type = %v, want %v", retrieved.Type, task.Type)
		}
	})

	t.Run("rollback", func(t *testing.T) {
		task := &internal.Task{
			Type:        "test_task",
			Payload:     json.RawMessage(`{"test": "data"}`),
			Status:      "pending",
			RunAt:       time.Now().Add(time.Hour),
			MaxAttempts: 3,
		}

		err := db.WithTx(context.Background(), func(tx *DB) error {
			if err := tx.CreateTask(context.Background(), task); err != nil {
				return fmt.Errorf("failed to create task in transaction: %w", err)
			}

			return errors.New("rollback test")
		})

		if err == nil {
			t.Error("WithTx() succeeded, want error")
		}
		if err != nil && err.Error() != "rollback test" {
			t.Errorf("WithTx() got error = %v, want 'rollback test'", err)
		}

		// Verify the task was not created
		_, err = db.GetTask(context.Background(), task.ID)
		if err == nil {
			t.Error("GetTask() after rollback succeeded, want error")
		}
	})

	t.Run("validation", func(t *testing.T) {
		// Test invalid task type
		task := &internal.Task{
			Type:        "",
			Payload:     json.RawMessage(`{"test": "data"}`),
			Status:      "pending",
			RunAt:       time.Now().Add(time.Hour),
			MaxAttempts: 3,
		}
		err := db.CreateTask(context.Background(), task)
		if err == nil {
			t.Error("CreateTask() with empty type succeeded, want error")
		}

		// Test invalid status
		task = &internal.Task{
			Type:        "test_task",
			Payload:     json.RawMessage(`{"test": "data"}`),
			Status:      "invalid_status",
			RunAt:       time.Now().Add(time.Hour),
			MaxAttempts: 3,
		}
		err = db.CreateTask(context.Background(), task)
		if err == nil {
			t.Error("CreateTask() with invalid status succeeded, want error")
		}

		// Test past run_at time
		task = &internal.Task{
			Type:        "test_task",
			Payload:     json.RawMessage(`{"test": "data"}`),
			Status:      "pending",
			RunAt:       time.Now().Add(-time.Hour),
			MaxAttempts: 3,
		}
		err = db.CreateTask(context.Background(), task)
		if err == nil {
			t.Error("CreateTask() with past run_at succeeded, want error")
		}
	})
}
