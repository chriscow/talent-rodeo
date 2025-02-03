package voice

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/models"

	"github.com/google/uuid"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(":memory:", true)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create background_jobs table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS background_jobs (
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

		CREATE INDEX IF NOT EXISTS idx_background_jobs_status_run_at ON background_jobs (status, run_at)
		WHERE
			status IN ('pending', 'retrying');

		CREATE TRIGGER IF NOT EXISTS background_jobs_updated_at 
			AFTER UPDATE ON background_jobs
		BEGIN
			UPDATE background_jobs 
			SET updated_at = CURRENT_TIMESTAMP
			WHERE id = NEW.id;
		END;
	`)
	if err != nil {
		t.Fatalf("failed to create background_jobs table: %v", err)
	}

	return db
}

func TestCallScheduler_ScheduleCall(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.Default()
	scheduler := NewCallScheduler(db, logger)

	tests := []struct {
		name          string
		userID        int
		scheduledTime time.Time
		wantErr       bool
	}{
		{
			name:          "valid future time",
			userID:        1,
			scheduledTime: time.Now().Add(1 * time.Hour),
			wantErr:       false,
		},
		{
			name:          "past time",
			userID:        1,
			scheduledTime: time.Now().Add(-1 * time.Hour),
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := scheduler.ScheduleCall(context.Background(), tt.userID, tt.scheduledTime)
			if (err != nil) != tt.wantErr {
				t.Errorf("ScheduleCall() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				// Verify task was created
				tasks, err := db.GetPendingTasks(1)
				if err != nil {
					t.Fatalf("failed to get pending tasks: %v", err)
				}

				found := false
				for _, task := range tasks {
					if task.Type == "phone_call" {
						found = true
						break
					}
				}

				if !found {
					t.Error("task not found in database")
				}
			}
		})
	}
}

func TestCallScheduler_CancelCall(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.Default()
	scheduler := NewCallScheduler(db, logger)

	// Create a test task
	task := &models.Task{
		ID:        uuid.New().String(),
		Type:      "phone_call",
		Status:    models.TaskStatusPending,
		RunAt:     time.Now().Add(1 * time.Hour).UTC(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Payload:   []byte(`{"user_id": 1}`),
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

	// Create a logger that writes to stderr for better visibility in test output
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

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
		ID:        uuid.New().String(),
		Type:      "phone_call",
		Status:    models.TaskStatusPending,
		RunAt:     initialTime,
		CreatedAt: now,
		UpdatedAt: now,
		Payload:   []byte(`{"user_id": 1}`),
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
