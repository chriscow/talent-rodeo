package voice

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/models"

	"github.com/google/uuid"
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

// ScheduleCall schedules a call for a specific time
func (s *CallScheduler) ScheduleCall(ctx context.Context, userID int, scheduledTime time.Time) error {
	s.logger.Debug("scheduling call",
		"user_id", userID,
		"scheduled_time", scheduledTime,
		"scheduled_time_utc", scheduledTime.UTC(),
		"current_time", time.Now(),
		"current_time_utc", time.Now().UTC(),
	)

	// Validate the scheduled time is in the future
	if scheduledTime.Before(time.Now()) {
		s.logger.Debug("rejecting past scheduled time",
			"scheduled_time", scheduledTime,
			"current_time", time.Now(),
		)
		return fmt.Errorf("scheduled time must be in the future")
	}

	// Create a task for the scheduled call
	task := &models.Task{
		ID:        uuid.New().String(),
		Type:      "phone_call",
		Status:    models.TaskStatusPending,
		RunAt:     scheduledTime.UTC(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Payload:   []byte(fmt.Sprintf(`{"user_id": %d}`, userID)),
	}

	s.logger.Debug("creating task",
		"task_id", task.ID,
		"type", task.Type,
		"status", task.Status,
		"run_at", task.RunAt,
		"created_at", task.CreatedAt,
		"updated_at", task.UpdatedAt,
	)

	// Save the task
	if err := s.db.CreateTask(task); err != nil {
		s.logger.Error("failed to create task",
			"error", err,
			"task_id", task.ID,
		)
		return fmt.Errorf("failed to create task: %w", err)
	}

	s.logger.Info("call scheduled",
		"user_id", userID,
		"scheduled_time", scheduledTime,
		"task_id", task.ID,
	)

	return nil
}

// CancelCall cancels a scheduled call
func (s *CallScheduler) CancelCall(ctx context.Context, taskID string) error {
	s.logger.Debug("cancelling call", "task_id", taskID)

	// Get the task
	task, err := s.db.GetTask(taskID)
	if err != nil {
		s.logger.Error("failed to get task",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		s.logger.Debug("task not found", "task_id", taskID)
		return fmt.Errorf("task not found: %s", taskID)
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
	task.Status = models.TaskStatusCancelled
	task.UpdatedAt = time.Now().UTC()

	s.logger.Debug("updating task",
		"task_id", task.ID,
		"new_status", task.Status,
		"updated_at", task.UpdatedAt,
	)

	if err := s.db.UpdateTask(task); err != nil {
		s.logger.Error("failed to update task",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to update task: %w", err)
	}

	s.logger.Info("call cancelled",
		"task_id", taskID,
	)

	return nil
}

// RescheduleCall reschedules a call to a new time
func (s *CallScheduler) RescheduleCall(ctx context.Context, taskID string, newTime time.Time) error {
	s.logger.Debug("rescheduling call",
		"task_id", taskID,
		"new_time", newTime,
		"new_time_utc", newTime.UTC(),
		"current_time", time.Now(),
		"current_time_utc", time.Now().UTC(),
	)

	// Get the task
	task, err := s.db.GetTask(taskID)
	if err != nil {
		s.logger.Error("failed to get task",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		s.logger.Debug("task not found", "task_id", taskID)
		return fmt.Errorf("task not found: %s", taskID)
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
	task.RunAt = newTime.UTC()
	task.UpdatedAt = time.Now().UTC()

	s.logger.Debug("updating task",
		"task_id", task.ID,
		"new_run_at", task.RunAt,
		"updated_at", task.UpdatedAt,
	)

	if err := s.db.UpdateTask(task); err != nil {
		s.logger.Error("failed to update task",
			"error", err,
			"task_id", taskID,
		)
		return fmt.Errorf("failed to update task: %w", err)
	}

	s.logger.Info("call rescheduled",
		"task_id", taskID,
		"new_time", newTime,
	)

	return nil
}
