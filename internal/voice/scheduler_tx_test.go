package voice

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/models"
)

func TestSchedulerTransactions(t *testing.T) {
	db := database.SetupTestDB(t)
	defer db.Close()

	logger := slog.Default()
	scheduler := NewCallScheduler(db, logger)

	t.Run("basic transaction flow", func(t *testing.T) {
		// Create a test task
		task := &models.Task{
			ID:          "test-task-1",
			Type:        "phone_call",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now().Add(1 * time.Hour),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Payload:     []byte(`{"user_id": 1}`),
		}

		// Create task outside transaction
		err := db.CreateTask(task)
		if err != nil {
			t.Fatalf("failed to create test task: %v", err)
		}

		// Start a transaction
		ctx := context.Background()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		// Cancel call within transaction
		err = scheduler.CancelCallTx(ctx, tx, task.ID)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to cancel call in transaction: %v", err)
		}

		// Commit and verify cancellation
		err = tx.Commit()
		if err != nil {
			t.Fatalf("failed to commit transaction: %v", err)
		}

		// Verify final state
		savedTask, err := db.GetTask(task.ID)
		if err != nil {
			t.Fatalf("failed to get task after commit: %v", err)
		}

		if savedTask.Status != models.TaskStatusCancelled {
			t.Errorf("task status = %v, want %v", savedTask.Status, models.TaskStatusCancelled)
		}
	})

	t.Run("rollback", func(t *testing.T) {
		// Create a test task
		task := &models.Task{
			ID:          "test-task-2",
			Type:        "phone_call",
			Status:      models.TaskStatusPending,
			MaxAttempts: 3,
			RunAt:       time.Now().Add(1 * time.Hour),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Payload:     []byte(`{"user_id": 1}`),
		}

		// Create task outside transaction
		err := db.CreateTask(task)
		if err != nil {
			t.Fatalf("failed to create test task: %v", err)
		}

		// Start a transaction
		ctx := context.Background()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		// Cancel call within transaction
		err = scheduler.CancelCallTx(ctx, tx, task.ID)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to cancel call in transaction: %v", err)
		}

		// Rollback and verify task is not cancelled
		err = tx.Rollback()
		if err != nil {
			t.Fatalf("failed to rollback transaction: %v", err)
		}

		savedTask, err := db.GetTask(task.ID)
		if err != nil {
			t.Fatalf("failed to get task after rollback: %v", err)
		}

		if savedTask.Status == models.TaskStatusCancelled {
			t.Error("task was cancelled despite transaction rollback")
		}
	})

	t.Run("error cases", func(t *testing.T) {
		ctx := context.Background()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		// Try to cancel nonexistent task
		err = scheduler.CancelCallTx(ctx, tx, "nonexistent")
		if err == nil {
			tx.Rollback()
			t.Fatal("expected error cancelling nonexistent call, got nil")
		}

		tx.Rollback()
	})
}
