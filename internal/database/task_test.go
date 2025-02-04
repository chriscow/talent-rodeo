package database

import (
	"context"
	"testing"
	"time"

	"talent-rodeo/internal/models"
)

func TestTaskTransactions(t *testing.T) {
	db := SetupTestDB(t)
	defer db.Close()

	t.Run("basic CRUD operations", func(t *testing.T) {
		// Create task
		task := &models.Task{
			ID:          "test-task-1",
			Type:        "test",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now().Add(1 * time.Hour),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Test Create
		err := db.CreateTask(task)
		if err != nil {
			t.Fatalf("failed to create task: %v", err)
		}

		// Test Read
		savedTask, err := db.GetTask(task.ID)
		if err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		if savedTask.ID != task.ID {
			t.Errorf("task ID = %v, want %v", savedTask.ID, task.ID)
		}

		// Test Update
		task.Status = models.TaskStatusRunning
		err = db.UpdateTask(task)
		if err != nil {
			t.Fatalf("failed to update task: %v", err)
		}

		// Verify Update
		updatedTask, err := db.GetTask(task.ID)
		if err != nil {
			t.Fatalf("failed to get updated task: %v", err)
		}
		if updatedTask.Status != models.TaskStatusRunning {
			t.Errorf("task status = %v, want %v", updatedTask.Status, models.TaskStatusRunning)
		}
	})

	t.Run("basic transaction", func(t *testing.T) {
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
			RunAt:       time.Now().Add(1 * time.Hour),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Create in transaction
		err = db.CreateTaskTx(ctx, tx, task)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to create task in transaction: %v", err)
		}

		// Commit and verify
		err = tx.Commit()
		if err != nil {
			t.Fatalf("failed to commit transaction: %v", err)
		}

		// Verify task was created
		savedTask, err := db.GetTask(task.ID)
		if err != nil {
			t.Fatalf("failed to get task after commit: %v", err)
		}
		if savedTask.ID != task.ID {
			t.Errorf("task ID = %v, want %v", savedTask.ID, task.ID)
		}
	})

	t.Run("rollback", func(t *testing.T) {
		ctx := context.Background()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		task := &models.Task{
			ID:          "test-task-3",
			Type:        "test",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now().Add(1 * time.Hour),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Create in transaction
		err = db.CreateTaskTx(ctx, tx, task)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to create task in transaction: %v", err)
		}

		// Rollback
		err = tx.Rollback()
		if err != nil {
			t.Fatalf("failed to rollback transaction: %v", err)
		}

		// Verify task was not created
		_, err = db.GetTask(task.ID)
		if err == nil {
			t.Error("expected error getting rolled back task, got nil")
		}
	})

	t.Run("validation", func(t *testing.T) {
		task := &models.Task{
			ID:          "", // Invalid empty ID
			Type:        "test",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err := db.CreateTask(task)
		if err == nil {
			t.Fatal("expected error for empty task ID, got nil")
		}
	})
}
