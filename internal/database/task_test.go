package database

import (
	"context"
	"testing"
	"time"

	"talent-rodeo/internal/models"
)

func TestTaskTransactions(t *testing.T) {
	db := setupTestDB(t)

	t.Run("successful transaction", func(t *testing.T) {
		ctx := context.Background()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		task := &models.Task{
			ID:          "test-task-1",
			Type:        "test",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Test creating task within transaction
		err = db.CreateTaskTx(ctx, tx, task)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to create task in transaction: %v", err)
		}

		// Update task status within same transaction
		task.Status = models.TaskStatusRunning
		task.Attempts++

		err = db.UpdateTaskTx(ctx, tx, task)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to update task in transaction: %v", err)
		}

		// Commit transaction
		err = tx.Commit()
		if err != nil {
			t.Fatalf("failed to commit transaction: %v", err)
		}

		// Verify task state after commit
		savedTask, err := db.GetTask(task.ID)
		if err != nil {
			t.Fatalf("failed to get task after commit: %v", err)
		}

		if savedTask.Status != models.TaskStatusRunning {
			t.Errorf("task status = %v, want %v", savedTask.Status, models.TaskStatusRunning)
		}
		if savedTask.Attempts != 1 {
			t.Errorf("task attempts = %d, want %d", savedTask.Attempts, 1)
		}
	})

	t.Run("rollback transaction", func(t *testing.T) {
		ctx := context.Background()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		task := &models.Task{
			ID:          "test-task-2",
			Type:        "test",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Create task within transaction
		err = db.CreateTaskTx(ctx, tx, task)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to create task in transaction: %v", err)
		}

		// Rollback transaction
		err = tx.Rollback()
		if err != nil {
			t.Fatalf("failed to rollback transaction: %v", err)
		}

		// Verify task doesn't exist after rollback
		savedTask, err := db.GetTask(task.ID)
		if err != nil {
			t.Fatalf("failed to query task after rollback: %v", err)
		}
		if savedTask != nil {
			t.Error("task exists after rollback, want nil")
		}
	})

	t.Run("concurrent transactions", func(t *testing.T) {
		ctx := context.Background()

		// Create initial task
		task := &models.Task{
			ID:          "test-task-3",
			Type:        "test",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err := db.CreateTask(task)
		if err != nil {
			t.Fatalf("failed to create initial task: %v", err)
		}

		// Start two transactions
		tx1, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction 1: %v", err)
		}

		tx2, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction 2: %v", err)
		}

		// Update task in first transaction
		task.Status = models.TaskStatusRunning
		err = db.UpdateTaskTx(ctx, tx1, task)
		if err != nil {
			tx1.Rollback()
			t.Fatalf("failed to update task in transaction 1: %v", err)
		}

		// Try to update same task in second transaction before first commits
		task.Status = models.TaskStatusFailed
		err = db.UpdateTaskTx(ctx, tx2, task)
		if err == nil {
			tx2.Rollback()
			tx1.Rollback()
			t.Fatal("expected second update to fail due to lock, but it succeeded")
		}

		// Commit first transaction
		err = tx1.Commit()
		if err != nil {
			t.Fatalf("failed to commit transaction 1: %v", err)
		}

		// Verify task state matches first transaction
		savedTask, err := db.GetTask(task.ID)
		if err != nil {
			t.Fatalf("failed to get task after commit: %v", err)
		}

		if savedTask.Status != models.TaskStatusRunning {
			t.Errorf("task status = %v, want %v", savedTask.Status, models.TaskStatusRunning)
		}
	})

	t.Run("transaction isolation", func(t *testing.T) {
		ctx := context.Background()

		// Create initial task
		task := &models.Task{
			ID:          "test-task-4",
			Type:        "test",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err := db.CreateTask(task)
		if err != nil {
			t.Fatalf("failed to create initial task: %v", err)
		}

		// Start a transaction
		tx1, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		// Update task in transaction but don't commit
		task.Status = models.TaskStatusRunning
		err = db.UpdateTaskTx(ctx, tx1, task)
		if err != nil {
			tx1.Rollback()
			t.Fatalf("failed to update task in transaction: %v", err)
		}

		// Try to read task outside transaction - should see old value
		savedTask, err := db.GetTask(task.ID)
		if err != nil {
			tx1.Rollback()
			t.Fatalf("failed to get task: %v", err)
		}

		if savedTask.Status != models.TaskStatusPending {
			tx1.Rollback()
			t.Errorf("task status = %v, want %v (isolation failure)", savedTask.Status, models.TaskStatusPending)
		}

		tx1.Commit()
	})

	t.Run("transaction with invalid task ID", func(t *testing.T) {
		ctx := context.Background()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		task := &models.Task{
			ID:          "", // Invalid empty ID
			Type:        "test",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err = db.CreateTaskTx(ctx, tx, task)
		if err == nil {
			tx.Rollback()
			t.Fatal("expected error for empty task ID, got nil")
		}

		tx.Rollback()
	})

	t.Run("transaction with invalid status", func(t *testing.T) {
		ctx := context.Background()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		task := &models.Task{
			ID:          "test-task-5",
			Type:        "test",
			Status:      "invalid_status", // Invalid status
			MaxAttempts: 3,
			RunAt:       time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err = db.CreateTaskTx(ctx, tx, task)
		if err == nil {
			tx.Rollback()
			t.Fatal("expected error for invalid status, got nil")
		}

		tx.Rollback()
	})

	t.Run("transaction with zero max attempts", func(t *testing.T) {
		ctx := context.Background()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		task := &models.Task{
			ID:          "test-task-6",
			Type:        "test",
			Status:      models.TaskStatusPending,
			MaxAttempts: 0, // Invalid max attempts
			RunAt:       time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err = db.CreateTaskTx(ctx, tx, task)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to create task with zero max attempts: %v", err)
		}

		// Should be able to read task with zero max attempts
		savedTask, err := db.GetTaskTx(ctx, tx, task.ID)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to get task: %v", err)
		}

		if savedTask.MaxAttempts != 0 {
			tx.Rollback()
			t.Errorf("task max_attempts = %d, want %d", savedTask.MaxAttempts, 0)
		}

		tx.Commit()
	})

	t.Run("transaction with past run_at time", func(t *testing.T) {
		ctx := context.Background()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		pastTime := time.Now().Add(-24 * time.Hour)
		task := &models.Task{
			ID:          "test-task-7",
			Type:        "test",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       pastTime,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err = db.CreateTaskTx(ctx, tx, task)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to create task with past run_at: %v", err)
		}

		// Should be able to read task with past run_at
		savedTask, err := db.GetTaskTx(ctx, tx, task.ID)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to get task: %v", err)
		}

		if !savedTask.RunAt.Equal(pastTime) {
			tx.Rollback()
			t.Errorf("task run_at = %v, want %v", savedTask.RunAt, pastTime)
		}

		tx.Commit()
	})

	t.Run("nested transactions not supported", func(t *testing.T) {
		ctx := context.Background()
		tx1, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin first transaction: %v", err)
		}

		// Try to start a second transaction while first is active
		tx2, err := db.BeginTxx(ctx, nil)
		if err != nil {
			tx1.Rollback()
			t.Fatalf("failed to begin second transaction: %v", err)
		}

		// Create and update task in first transaction
		task1 := &models.Task{
			ID:          "test-task-8",
			Type:        "test",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err = db.CreateTaskTx(ctx, tx1, task1)
		if err != nil {
			tx1.Rollback()
			tx2.Rollback()
			t.Fatalf("failed to create task in first transaction: %v", err)
		}

		// Try to update same task in second transaction - should fail or block
		task1.Status = models.TaskStatusRunning
		err = db.UpdateTaskTx(ctx, tx2, task1)
		if err == nil {
			tx1.Rollback()
			tx2.Rollback()
			t.Fatal("expected error or block when updating same task in parallel transaction")
		}

		tx1.Rollback()
		tx2.Rollback()
	})
}
