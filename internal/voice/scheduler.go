package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/database"

	"github.com/jmoiron/sqlx"
)

// CallScheduler manages scheduling and initiating phone calls
type CallScheduler struct {
	db     *database.DB
	logger *slog.Logger
}

// NewCallScheduler creates a new call scheduler
func NewCallScheduler(db *database.DB, logger *slog.Logger) *CallScheduler {
	return &CallScheduler{
		db:     db,
		logger: logger,
	}
}

// ScheduleCall schedules a call to be made at the specified time
func (s *CallScheduler) ScheduleCall(ctx context.Context, msg *internal.Message, runAt time.Time) (int64, error) {
	// Validate message
	if msg == nil {
		return 0, fmt.Errorf("message is required")
	}
	if msg.To == "" {
		return 0, fmt.Errorf("phone number is required")
	}

	// Validate run time
	if runAt.Before(time.Now()) {
		return 0, fmt.Errorf("cannot schedule call in the past")
	}

	// Serialize message to JSON
	payload, err := json.Marshal(msg)
	if err != nil {
		return 0, fmt.Errorf("failed to serialize message: %w", err)
	}

	taskID := time.Now().UnixNano()
	task := &internal.Task{
		ID:          taskID,
		Type:        "phone_call",
		Payload:     payload,
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       runAt.UTC().Truncate(time.Microsecond), // Truncate to microsecond precision
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}

	if err := s.db.CreateTask(ctx, task); err != nil {
		return 0, fmt.Errorf("failed to create task: %w", err)
	}

	return task.ID, nil
}

// CancelCall cancels a call
func (s *CallScheduler) CancelCall(ctx context.Context, taskID int64) error {
	s.logger.Debug("starting call cancellation",
		"task_id", taskID,
	)

	// Start transaction
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.Error("failed to begin transaction",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Cancel call within transaction
	if err := s.CancelCallTx(ctx, tx, taskID); err != nil {
		tx.Rollback()
		return err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		s.logger.Error("failed to commit transaction",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// CancelCallTx cancels a call within an existing transaction
func (s *CallScheduler) CancelCallTx(ctx context.Context, tx *sqlx.Tx, taskID int64) error {
	s.logger.Debug("cancelling call in transaction",
		"task_id", taskID,
	)

	// Get the task within transaction
	task, err := s.db.GetTaskTx(ctx, tx, taskID)
	if err != nil {
		s.logger.Error("failed to get task",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		s.logger.Debug("task not found", "task_id", taskID)
		return fmt.Errorf("task not found: %d", taskID)
	}

	s.logger.Debug("found task",
		"task_id", task.ID,
		"type", task.Type,
		"status", task.Status,
		"run_at", task.RunAt,
	)

	// Verify it's a phone call task
	if task.Type != "phone_call" {
		s.logger.Debug("invalid task type",
			"task_id", taskID,
			"type", task.Type,
		)
		return fmt.Errorf("task is not a phone call")
	}

	// Update task status to cancelled
	task.Status = internal.TaskStatusCancelled
	task.UpdatedAt = time.Now().UTC()

	s.logger.Debug("updating task in transaction",
		"task_id", task.ID,
		"new_status", task.Status,
		"updated_at", task.UpdatedAt,
	)

	if err := s.db.UpdateTaskTx(ctx, tx, task); err != nil {
		s.logger.Error("failed to update task",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to update task: %w", err)
	}

	s.logger.Info("call cancelled in transaction",
		"task_id", taskID,
	)

	return nil
}

// RescheduleCall reschedules a call to a new time
func (s *CallScheduler) RescheduleCall(ctx context.Context, taskID int64, newTime time.Time) error {
	s.logger.Debug("starting call rescheduling",
		"task_id", taskID,
		"new_time", newTime,
	)

	// Start transaction
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.Error("failed to begin transaction",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Reschedule call within transaction
	if err := s.RescheduleCallTx(ctx, tx, taskID, newTime); err != nil {
		tx.Rollback()
		return err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		s.logger.Error("failed to commit transaction",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RescheduleCallTx reschedules a call to a new time within an existing transaction
func (s *CallScheduler) RescheduleCallTx(ctx context.Context, tx *sqlx.Tx, taskID int64, newTime time.Time) error {
	s.logger.Debug("rescheduling call in transaction",
		"task_id", taskID,
		"new_time", newTime,
		"new_time_utc", newTime.UTC(),
		"current_time", time.Now(),
		"current_time_utc", time.Now().UTC(),
	)

	// Get the task within transaction
	task, err := s.db.GetTaskTx(ctx, tx, taskID)
	if err != nil {
		s.logger.Error("failed to get task",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		s.logger.Debug("task not found", "task_id", taskID)
		return fmt.Errorf("task not found: %d", taskID)
	}

	s.logger.Debug("found task",
		"task_id", task.ID,
		"type", task.Type,
		"status", task.Status,
		"current_run_at", task.RunAt,
	)

	// Verify it's a phone call task
	if task.Type != "phone_call" {
		s.logger.Debug("invalid task type",
			"task_id", taskID,
			"type", task.Type,
		)
		return fmt.Errorf("task is not a phone call")
	}

	// Validate the new time is in the future
	if newTime.Before(time.Now()) {
		s.logger.Debug("rejecting past time",
			"new_time", newTime,
			"current_time", time.Now(),
		)
		return fmt.Errorf("scheduled time must be in the future")
	}

	// Update task run time
	task.RunAt = newTime.UTC().Truncate(time.Microsecond)
	task.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)

	s.logger.Debug("updating task in transaction",
		"task_id", task.ID,
		"new_run_at", task.RunAt,
		"updated_at", task.UpdatedAt,
	)

	if err := s.db.UpdateTaskTx(ctx, tx, task); err != nil {
		s.logger.Error("failed to update task",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to update task: %w", err)
	}

	s.logger.Info("call rescheduled in transaction",
		"task_id", taskID,
		"new_time", newTime,
	)

	return nil
}
