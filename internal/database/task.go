package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"talent-rodeo/internal/models"
)

// CreateTask creates a new task in the database
func (db *DB) CreateTask(task *models.Task) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	query := `
		INSERT INTO tasks (
			id, type, payload, status, attempts, max_attempts, run_at, error, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)`

	_, err := db.ExecContext(ctx, query,
		task.ID,
		task.Type,
		task.Payload,
		task.Status,
		task.Attempts,
		task.MaxAttempts,
		task.RunAt,
		task.Error,
		task.CreatedAt,
		task.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	return nil
}

// UpdateTask updates an existing task in the database
func (db *DB) UpdateTask(task *models.Task) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	task.UpdatedAt = time.Now()

	query := `
		UPDATE tasks SET
			status = $1,
			attempts = $2,
			run_at = $3,
			error = $4,
			updated_at = $5
		WHERE id = $6`

	result, err := db.ExecContext(ctx, query,
		task.Status,
		task.Attempts,
		task.RunAt,
		task.Error,
		task.UpdatedAt,
		task.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("task not found: %s", task.ID)
	}

	return nil
}

// GetTask retrieves a task by ID
func (db *DB) GetTask(id string) (*models.Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var task models.Task
	query := `SELECT * FROM tasks WHERE id = $1`

	err := db.GetContext(ctx, &task, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	return &task, nil
}

// GetPendingTasks retrieves tasks that are ready to be processed
func (db *DB) GetPendingTasks(limit int) ([]models.Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var tasks []models.Task
	query := `
		SELECT * FROM tasks
		WHERE status IN ($1, $2)
		AND run_at <= $3
		ORDER BY run_at ASC
		LIMIT $4`

	err := db.SelectContext(ctx, &tasks, query,
		models.TaskStatusPending,
		models.TaskStatusRetrying,
		time.Now(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending tasks: %w", err)
	}

	return tasks, nil
}

// DeleteTask removes a task from the database
func (db *DB) DeleteTask(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	query := `DELETE FROM tasks WHERE id = $1`

	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}
