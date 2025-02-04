package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/models"
)

func TestCallScheduler_CancelCallTx(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(context.Context, *database.DB, int64) error
		taskID  int64
		wantErr bool
	}{
		{
			name:   "cancel existing task",
			taskID: time.Now().UnixNano(),
			setup: func(ctx context.Context, db *database.DB, taskID int64) error {
				msg := &models.Message{
					ID:        fmt.Sprintf("%d", taskID),
					To:        "+1234567890",
					From:      "system",
					Body:      "Test message",
					CreatedAt: time.Now(),
				}
				payload, err := json.Marshal(msg)
				if err != nil {
					return fmt.Errorf("failed to marshal message: %v", err)
				}

				task := &models.Task{
					ID:          taskID,
					Type:        "phone_call",
					Status:      models.TaskStatusPending,
					MaxAttempts: 3,
					RunAt:       time.Now().Add(time.Hour),
					Payload:     payload,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				return db.CreateTask(ctx, task)
			},
			wantErr: false,
		},
		{
			name:    "cancel nonexistent task",
			taskID:  time.Now().UnixNano(),
			wantErr: true,
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

			tx, err := db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatalf("failed to begin transaction: %v", err)
			}
			defer tx.Rollback()

			err = scheduler.CancelCallTx(ctx, tx, tt.taskID)
			if (err != nil) != tt.wantErr {
				t.Errorf("CancelCallTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				task, err := db.GetTaskTx(ctx, tx, tt.taskID)
				if err != nil {
					t.Fatalf("failed to get task: %v", err)
				}

				if task.Status != models.TaskStatusCancelled {
					t.Errorf("task status = %v, want %v", task.Status, models.TaskStatusCancelled)
				}
			}

			if err := tx.Commit(); err != nil {
				t.Fatalf("failed to commit transaction: %v", err)
			}
		})
	}
}

func TestCallScheduler_RescheduleCallTx(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(context.Context, *database.DB, int64) error
		taskID    int64
		newRunAt  time.Time
		wantErr   bool
		wantRunAt time.Time
	}{
		{
			name:      "reschedule existing task",
			taskID:    time.Now().UnixNano(),
			newRunAt:  time.Now().Add(2 * time.Hour),
			wantRunAt: time.Now().Add(2 * time.Hour),
			setup: func(ctx context.Context, db *database.DB, taskID int64) error {
				msg := &models.Message{
					ID:        fmt.Sprintf("%d", taskID),
					To:        "+1234567890",
					From:      "system",
					Body:      "Test message",
					CreatedAt: time.Now(),
				}
				payload, err := json.Marshal(msg)
				if err != nil {
					return fmt.Errorf("failed to marshal message: %v", err)
				}

				task := &models.Task{
					ID:          taskID,
					Type:        "phone_call",
					Status:      models.TaskStatusPending,
					MaxAttempts: 3,
					RunAt:       time.Now().Add(time.Hour),
					Payload:     payload,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				return db.CreateTask(ctx, task)
			},
			wantErr: false,
		},
		{
			name:      "reschedule nonexistent task",
			taskID:    time.Now().UnixNano(),
			newRunAt:  time.Now().Add(2 * time.Hour),
			wantRunAt: time.Now().Add(2 * time.Hour),
			wantErr:   true,
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

			tx, err := db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatalf("failed to begin transaction: %v", err)
			}
			defer tx.Rollback()

			err = scheduler.RescheduleCallTx(ctx, tx, tt.taskID, tt.newRunAt)
			if (err != nil) != tt.wantErr {
				t.Errorf("RescheduleCallTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				task, err := db.GetTaskTx(ctx, tx, tt.taskID)
				if err != nil {
					t.Fatalf("failed to get task: %v", err)
				}

				if !task.RunAt.Equal(tt.wantRunAt) {
					t.Errorf("task run_at = %v, want %v", task.RunAt, tt.wantRunAt)
				}
			}

			if err := tx.Commit(); err != nil {
				t.Fatalf("failed to commit transaction: %v", err)
			}
		})
	}
}
