package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"talent-rodeo/internal"

	"github.com/jmoiron/sqlx"
)

var (
	taskMutex sync.Mutex
)

// Task represents a background task in the system
type Task struct {
	ID          int64          `db:"id"`
	Type        string         `db:"type"`
	Payload     []byte         `db:"payload"`
	Status      string         `db:"status"`
	Attempts    int            `db:"attempts"`
	MaxAttempts int            `db:"max_attempts"`
	RunAt       time.Time      `db:"run_at"`
	Error       sql.NullString `db:"error"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
}

// CreateTask creates a new task
func (db *DB) CreateTask(ctx context.Context, task *internal.Task) error {
	if task.Type == "" {
		return fmt.Errorf("task type is required")
	}
	if task.RunAt.IsZero() {
		return fmt.Errorf("run_at is required")
	}
	if task.Payload == nil {
		return fmt.Errorf("payload is required")
	}

	db.logger.Debug("creating task",
		"task_id", task.ID,
		"task_id_type", fmt.Sprintf("%T", task.ID),
		"type", task.Type,
		"status", task.Status,
		"run_at", task.RunAt,
		"payload_size", len(task.Payload),
	)

	query := `
		INSERT INTO tasks (
			id, type, payload, status, attempts, max_attempts, run_at, error, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`

	// Don't generate a new ID if one is already set
	if task.ID == 0 {
		task.ID = generateTaskID(task)
	}
	task.CreatedAt = time.Now()
	task.UpdatedAt = task.CreatedAt

	db.logger.Debug("executing insert query",
		"task_id", task.ID,
		"query", query,
	)

	result, err := db.ExecContext(ctx, query,
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
		db.logger.Error("failed to create task",
			"error", err,
			"task_id", task.ID,
		)
		return fmt.Errorf("failed to create task: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		db.logger.Error("failed to get rows affected",
			"error", err,
			"task_id", task.ID,
		)
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows != 1 {
		db.logger.Error("unexpected number of rows affected",
			"rows", rows,
			"task_id", task.ID,
		)
		return fmt.Errorf("expected 1 row to be affected, got %d", rows)
	}

	db.logger.Debug("task created successfully",
		"task_id", task.ID,
		"rows_affected", rows,
	)

	return nil
}

// GetTask retrieves a task by ID
func (db *DB) GetTask(ctx context.Context, id int64) (*internal.Task, error) {
	db.logger.Debug("getting task",
		"task_id", id,
		"task_id_type", fmt.Sprintf("%T", id),
	)

	task := &internal.Task{}
	query := `SELECT * FROM tasks WHERE id = $1`

	db.logger.Debug("executing select query",
		"task_id", id,
		"query", query,
	)

	err := db.GetContext(ctx, task, query, id)
	if err != nil {
		db.logger.Error("failed to get task",
			"error", err,
			"task_id", id,
		)
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	db.logger.Debug("task retrieved successfully",
		"task_id", task.ID,
		"type", task.Type,
		"status", task.Status,
	)

	return task, nil
}

// UpdateTask updates an existing task
func (db *DB) UpdateTask(ctx context.Context, task *internal.Task) error {
	task.UpdatedAt = time.Now()
	query := `
		UPDATE tasks SET
			type = $1,
			payload = $2,
			status = $3,
			attempts = $4,
			max_attempts = $5,
			run_at = $6,
			error = $7,
			updated_at = $8
		WHERE id = $9
	`
	result, err := db.ExecContext(ctx, query,
		task.Type,
		task.Payload,
		task.Status,
		task.Attempts,
		task.MaxAttempts,
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
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task not found: %d", task.ID)
	}

	return nil
}

// DeleteTask deletes a task by ID
func (db *DB) DeleteTask(ctx context.Context, id int64) error {
	query := `DELETE FROM tasks WHERE id = $1`
	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task not found: %d", id)
	}

	return nil
}

// GetPendingTasks retrieves all pending tasks that are due to run
func (db *DB) GetPendingTasks(ctx context.Context) ([]*internal.Task, error) {
	var tasks []*internal.Task
	query := `
		SELECT * FROM tasks
		WHERE status IN ('pending', 'retrying')
		AND run_at <= $1
		ORDER BY run_at ASC
	`
	err := db.SelectContext(ctx, &tasks, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to get pending tasks: %w", err)
	}
	return tasks, nil
}

func generateTaskID(task *internal.Task) int64 {
	return time.Now().UnixNano()
}

// CreateTaskTx creates a new task within a transaction
func (db *DB) CreateTaskTx(ctx context.Context, tx *sqlx.Tx, task *internal.Task) error {
	taskMutex.Lock()
	defer taskMutex.Unlock()

	query := `
		INSERT INTO tasks (
			id,
			type,
			payload,
			status,
			attempts,
			max_attempts,
			run_at,
			error,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := tx.ExecContext(ctx, query,
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
		return fmt.Errorf("failed to create task in transaction: %w", err)
	}

	return nil
}

// UpdateTaskTx updates an existing task within an existing transaction
func (db *DB) UpdateTaskTx(ctx context.Context, tx *sqlx.Tx, task *internal.Task) error {
	taskMutex.Lock()
	defer taskMutex.Unlock()

	task.UpdatedAt = time.Now()

	query := `
		UPDATE tasks SET
			status = $1,
			attempts = $2,
			run_at = $3,
			error = $4,
			updated_at = $5
		WHERE id = $6`

	result, err := tx.ExecContext(ctx, query,
		task.Status,
		task.Attempts,
		task.RunAt,
		task.Error,
		task.UpdatedAt,
		task.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update task in transaction: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("task not found: %d", task.ID)
	}

	return nil
}

// GetTaskTx gets a task by ID within a transaction
func (db *DB) GetTaskTx(ctx context.Context, tx *sqlx.Tx, id int64) (*internal.Task, error) {
	taskMutex.Lock()
	defer taskMutex.Unlock()

	var task internal.Task
	err := tx.GetContext(ctx, &task, `
		SELECT *
		FROM tasks
		WHERE id = $1
	`, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task in transaction: %w", err)
	}

	return &task, nil
}

// DeleteTaskTx removes a task from the database within an existing transaction
func (db *DB) DeleteTaskTx(ctx context.Context, tx *sqlx.Tx, id int64) error {
	taskMutex.Lock()
	defer taskMutex.Unlock()

	query := `DELETE FROM tasks WHERE id = $1`

	result, err := tx.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete task in transaction: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("task not found: %d", id)
	}

	return nil
}

// GetTasksWithFilter retrieves tasks with optional status filter
func (db *DB) GetTasksWithFilter(ctx context.Context, statuses []string, limit int, offset int) ([]*Task, error) {
	query := `
		SELECT id, type, payload, status, attempts, max_attempts, run_at, error, created_at, updated_at
		FROM tasks
		WHERE 1=1
	`
	args := []any{}

	if len(statuses) > 0 {
		placeholders := make([]string, len(statuses))
		for i, status := range statuses {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args = append(args, status)
		}
		query += fmt.Sprintf(" AND status IN (%s)", strings.Join(placeholders, ","))
	}

	query += fmt.Sprintf(" ORDER BY run_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := db.dbtx.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var task Task
		if err := rows.StructScan(&task); err != nil {
			return nil, fmt.Errorf("error scanning task: %w", err)
		}
		tasks = append(tasks, &task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tasks: %w", err)
	}

	return tasks, nil
}

// GetTasksCount returns total count of tasks matching the filter
func (db *DB) GetTasksCount(ctx context.Context, statuses []string) (int, error) {
	query := "SELECT COUNT(*) FROM tasks WHERE 1=1"
	args := []any{}

	if len(statuses) > 0 {
		placeholders := make([]string, len(statuses))
		for i, status := range statuses {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args = append(args, status)
		}
		query += fmt.Sprintf(" AND status IN (%s)", strings.Join(placeholders, ","))
	}

	var count int
	if err := db.dbtx.GetContext(ctx, &count, query, args...); err != nil {
		return 0, fmt.Errorf("error counting tasks: %w", err)
	}

	return count, nil
}
