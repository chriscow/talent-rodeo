package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/database"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	return database.SetupTestDB(t)
}

func TestCallScheduler_ScheduleCall(t *testing.T) {
	tests := []struct {
		name    string
		msg     *internal.Message
		runAt   time.Time
		wantErr bool
	}{
		{
			name: "valid_future_time",
			msg: &internal.Message{
				ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
				To:        "+1234567890",
				From:      "system",
				Body:      "Test message",
				CreatedAt: time.Now(),
			},
			runAt:   time.Now().Add(24 * time.Hour).UTC(),
			wantErr: false,
		},
		{
			name: "past_time",
			msg: &internal.Message{
				ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
				To:        "+1234567890",
				From:      "system",
				Body:      "Test message",
				CreatedAt: time.Now(),
			},
			runAt:   time.Now().Add(-24 * time.Hour).UTC(),
			wantErr: true,
		},
		{
			name: "valid_message",
			msg: &internal.Message{
				ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
				To:        "+1234567890",
				From:      "system",
				Body:      "Test message",
				CreatedAt: time.Now(),
			},
			runAt:   time.Now().Add(time.Hour).UTC(),
			wantErr: false,
		},
		{
			name: "missing_phone",
			msg: &internal.Message{
				ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
				From:      "system",
				Body:      "Test message",
				CreatedAt: time.Now(),
			},
			runAt:   time.Now().Add(time.Hour).UTC(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := database.SetupTestDB(t)
			scheduler := NewCallScheduler(db, slog.Default())

			taskID, err := scheduler.ScheduleCall(context.Background(), tt.msg, tt.runAt)
			if (err != nil) != tt.wantErr {
				t.Errorf("ScheduleCall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				task, err := db.GetTask(context.Background(), taskID)
				if err != nil {
					t.Fatalf("failed to get task: %v", err)
				}

				// Compare times in UTC to avoid timezone issues
				if !task.RunAt.Equal(tt.runAt) {
					t.Errorf("task run_at = %v, want %v", task.RunAt, tt.runAt)
				}

				// Verify task type and status
				if task.Type != "phone_call" {
					t.Errorf("task type = %v, want phone_call", task.Type)
				}
				if task.Status != internal.TaskStatusPending {
					t.Errorf("task status = %v, want %v", task.Status, internal.TaskStatusPending)
				}
			}
		})
	}
}

func TestCallScheduler_CancelCall(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(context.Context, *database.DB, int64) error
		taskID  int64
		wantErr bool
	}{
		{
			name:    "cancel_nonexistent_task",
			taskID:  time.Now().UnixNano(),
			wantErr: true,
		},
		{
			name:    "cancel_existing_task",
			taskID:  time.Now().UnixNano(),
			wantErr: false,
			setup: func(ctx context.Context, db *database.DB, taskID int64) error {
				msg := &internal.Message{
					ID:        fmt.Sprintf("%d", taskID),
					To:        "+1234567890",
					From:      "system",
					Body:      "Test message",
					CreatedAt: time.Now().UTC(),
				}
				payload, err := json.Marshal(msg)
				if err != nil {
					return fmt.Errorf("failed to marshal message: %v", err)
				}

				task := &internal.Task{
					ID:          taskID,
					Type:        "phone_call",
					Status:      internal.TaskStatusPending,
					MaxAttempts: 3,
					RunAt:       time.Now().Add(time.Hour).UTC(),
					Payload:     payload,
					CreatedAt:   time.Now().UTC(),
					UpdatedAt:   time.Now().UTC(),
				}
				if err := db.CreateTask(ctx, task); err != nil {
					return fmt.Errorf("failed to create task: %w", err)
				}

				// Verify task was created
				created, err := db.GetTask(ctx, taskID)
				if err != nil {
					return fmt.Errorf("failed to verify task creation: %w", err)
				}
				if created == nil {
					return fmt.Errorf("task not found after creation")
				}

				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := database.SetupTestDB(t)
			scheduler := NewCallScheduler(db, slog.Default())

			ctx := context.Background()
			if tt.setup != nil {
				if err := tt.setup(ctx, db, tt.taskID); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			err := scheduler.CancelCall(ctx, tt.taskID)
			if (err != nil) != tt.wantErr {
				t.Errorf("CancelCall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				task, err := db.GetTask(ctx, tt.taskID)
				if err != nil {
					t.Fatalf("failed to get task: %v", err)
				}

				if task.Status != internal.TaskStatusCancelled {
					t.Errorf("task status = %v, want %v", task.Status, internal.TaskStatusCancelled)
				}
			}
		})
	}
}

func TestCallScheduler_RescheduleCall(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(context.Context, *database.DB, int64) error
		taskID  int64
		newTime time.Time
		wantErr bool
	}{
		{
			name:    "reschedule_nonexistent_task",
			taskID:  time.Now().UnixNano(),
			newTime: time.Now().Add(time.Hour).UTC(),
			wantErr: true,
		},
		{
			name:    "reschedule_to_past_time",
			taskID:  time.Now().UnixNano(),
			newTime: time.Now().Add(-time.Hour).UTC(),
			wantErr: true,
			setup: func(ctx context.Context, db *database.DB, taskID int64) error {
				msg := &internal.Message{
					ID:        fmt.Sprintf("%d", taskID),
					To:        "+1234567890",
					From:      "system",
					Body:      "Test message",
					CreatedAt: time.Now().UTC(),
				}
				payload, err := json.Marshal(msg)
				if err != nil {
					return fmt.Errorf("failed to marshal message: %v", err)
				}

				task := &internal.Task{
					ID:          taskID,
					Type:        "phone_call",
					Status:      internal.TaskStatusPending,
					MaxAttempts: 3,
					RunAt:       time.Now().Add(2 * time.Hour).UTC(),
					Payload:     payload,
					CreatedAt:   time.Now().UTC(),
					UpdatedAt:   time.Now().UTC(),
				}
				if err := db.CreateTask(ctx, task); err != nil {
					return fmt.Errorf("failed to create task: %w", err)
				}

				// Verify task was created
				created, err := db.GetTask(ctx, taskID)
				if err != nil {
					return fmt.Errorf("failed to verify task creation: %w", err)
				}
				if created == nil {
					return fmt.Errorf("task not found after creation")
				}

				return nil
			},
		},
		{
			name:    "reschedule_successfully",
			taskID:  time.Now().UnixNano(),
			newTime: time.Now().Add(2 * time.Hour).UTC(),
			wantErr: false,
			setup: func(ctx context.Context, db *database.DB, taskID int64) error {
				msg := &internal.Message{
					ID:        fmt.Sprintf("%d", taskID),
					To:        "+1234567890",
					From:      "system",
					Body:      "Test message",
					CreatedAt: time.Now().UTC(),
				}
				payload, err := json.Marshal(msg)
				if err != nil {
					return fmt.Errorf("failed to marshal message: %v", err)
				}

				task := &internal.Task{
					ID:          taskID,
					Type:        "phone_call",
					Status:      internal.TaskStatusPending,
					MaxAttempts: 3,
					RunAt:       time.Now().Add(time.Hour).UTC(),
					Payload:     payload,
					CreatedAt:   time.Now().UTC(),
					UpdatedAt:   time.Now().UTC(),
				}
				if err := db.CreateTask(ctx, task); err != nil {
					return fmt.Errorf("failed to create task: %w", err)
				}

				// Verify task was created
				created, err := db.GetTask(ctx, taskID)
				if err != nil {
					return fmt.Errorf("failed to verify task creation: %w", err)
				}
				if created == nil {
					return fmt.Errorf("task not found after creation")
				}

				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := database.SetupTestDB(t)
			scheduler := NewCallScheduler(db, slog.Default())

			ctx := context.Background()
			if tt.setup != nil {
				if err := tt.setup(ctx, db, tt.taskID); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			err := scheduler.RescheduleCall(ctx, tt.taskID, tt.newTime)
			if (err != nil) != tt.wantErr {
				t.Errorf("RescheduleCall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				task, err := db.GetTask(ctx, tt.taskID)
				if err != nil {
					t.Fatalf("failed to get task: %v", err)
				}

				if !task.RunAt.Equal(tt.newTime) {
					t.Errorf("task run_at = %v, want %v", task.RunAt, tt.newTime)
				}
			}
		})
	}
}
