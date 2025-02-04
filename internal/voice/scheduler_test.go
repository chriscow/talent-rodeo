package voice

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/models"

	"github.com/google/uuid"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	return database.SetupTestDB(t)
}

func TestCallScheduler_ScheduleCall(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.Default()
	scheduler := NewCallScheduler(db, logger)

	t.Run("valid future time", func(t *testing.T) {
		futureTime := time.Now().Add(24 * time.Hour)
		msg := &models.Message{
			ID:        "test-msg-1",
			To:        "+1234567890",
			From:      "+0987654321",
			Body:      "Test message",
			CreatedAt: time.Now(),
		}

		taskID, err := scheduler.ScheduleCall(context.Background(), msg, futureTime)
		if err != nil {
			t.Errorf("ScheduleCall() error = %v, wantErr false", err)
			return
		}

		// Verify task was created
		task, err := db.GetTask(taskID)
		if err != nil {
			t.Errorf("failed to get task: %v", err)
			return
		}

		if task == nil {
			t.Error("no tasks found in database")
			return
		}

		if task.Status != models.TaskStatusPending {
			t.Errorf("task status = %v, want %v", task.Status, models.TaskStatusPending)
		}

		if task.RunAt.Unix() != futureTime.Unix() {
			t.Errorf("task run_at = %v, want %v", task.RunAt, futureTime)
		}

		if task.MaxAttempts != 3 { // Verify max attempts is set correctly
			t.Errorf("task max_attempts = %v, want 3", task.MaxAttempts)
		}
	})

	t.Run("past time", func(t *testing.T) {
		pastTime := time.Now().Add(-24 * time.Hour)
		msg := &models.Message{
			ID:        "test-msg-2",
			To:        "+1234567890",
			From:      "+0987654321",
			Body:      "Test message",
			CreatedAt: time.Now(),
		}

		_, err := scheduler.ScheduleCall(context.Background(), msg, pastTime)
		if err == nil {
			t.Error("ScheduleCall() error = nil, wantErr true")
		}
	})
}

func TestCallScheduler_CancelCall(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.Default()
	scheduler := NewCallScheduler(db, logger)

	// Create a test task
	task := &models.Task{
		ID:          uuid.New().String(),
		Type:        "phone_call",
		Status:      models.TaskStatusPending,
		MaxAttempts: 3, // Set valid max_attempts
		RunAt:       time.Now().Add(1 * time.Hour),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Payload:     []byte(`{"user_id": 1}`),
	}

	if err := db.CreateTask(task); err != nil {
		t.Fatalf("failed to create test task: %v", err)
	}

	tests := []struct {
		name    string
		taskID  string
		wantErr bool
	}{
		{
			name:    "valid task",
			taskID:  task.ID,
			wantErr: false,
		},
		{
			name:    "nonexistent task",
			taskID:  "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := scheduler.CancelCall(context.Background(), tt.taskID)
			if (err != nil) != tt.wantErr {
				t.Errorf("CancelCall() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				// Verify task was cancelled
				updatedTask, err := db.GetTask(tt.taskID)
				if err != nil {
					t.Fatalf("failed to get task: %v", err)
				}

				if updatedTask.Status != models.TaskStatusCancelled {
					t.Errorf("task status = %v, want %v", updatedTask.Status, models.TaskStatusCancelled)
				}
			}
		})
	}
}

func TestCallScheduler_RescheduleCall(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.Default()
	scheduler := NewCallScheduler(db, logger)

	// Use a fixed reference time for testing
	now := time.Now().UTC()
	initialTime := now.Add(1 * time.Hour).Truncate(time.Second)
	newTime := now.Add(2 * time.Hour).Truncate(time.Second)

	logger.Debug("test setup",
		"now", now,
		"initial_time", initialTime,
		"new_time", newTime,
	)

	// Create a test task
	task := &models.Task{
		ID:          uuid.New().String(),
		Type:        "phone_call",
		Status:      models.TaskStatusPending,
		MaxAttempts: 3, // Set valid max_attempts
		RunAt:       initialTime,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Payload:     []byte(`{"user_id": 1}`),
	}

	logger.Debug("creating test task",
		"task_id", task.ID,
		"run_at", task.RunAt,
	)

	if err := db.CreateTask(task); err != nil {
		t.Fatalf("failed to create test task: %v", err)
	}

	// Verify task was created correctly
	createdTask, err := db.GetTask(task.ID)
	if err != nil {
		t.Fatalf("failed to get created task: %v", err)
	}
	logger.Debug("created task state",
		"task_id", createdTask.ID,
		"run_at", createdTask.RunAt,
	)

	tests := []struct {
		name    string
		taskID  string
		newTime time.Time
		wantErr bool
	}{
		{
			name:    "valid future time",
			taskID:  task.ID,
			newTime: newTime,
			wantErr: false,
		},
		{
			name:    "past time",
			taskID:  task.ID,
			newTime: now.Add(-1 * time.Hour),
			wantErr: true,
		},
		{
			name:    "nonexistent task",
			taskID:  "nonexistent",
			newTime: now.Add(1 * time.Hour),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger.Debug("running test case",
				"test", tt.name,
				"task_id", tt.taskID,
				"new_time", tt.newTime,
			)

			err := scheduler.RescheduleCall(context.Background(), tt.taskID, tt.newTime)
			if (err != nil) != tt.wantErr {
				t.Errorf("RescheduleCall() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				// Verify task was rescheduled
				updatedTask, err := db.GetTask(tt.taskID)
				if err != nil {
					t.Fatalf("failed to get task: %v", err)
				}

				logger.Debug("comparing times",
					"got_run_at", updatedTask.RunAt,
					"want_run_at", tt.newTime,
					"got_run_at_unix", updatedTask.RunAt.Unix(),
					"want_run_at_unix", tt.newTime.Unix(),
				)

				// Compare times truncated to seconds to avoid subsecond differences
				gotTime := updatedTask.RunAt.Truncate(time.Second)
				wantTime := tt.newTime.Truncate(time.Second)
				if !gotTime.Equal(wantTime) {
					t.Errorf("task RunAt = %v, want %v", gotTime, wantTime)
				}
			}
		})
	}
}
