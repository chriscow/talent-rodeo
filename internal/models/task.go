package models

import "time"

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusComplete  TaskStatus = "complete"
	TaskStatusRetrying  TaskStatus = "retrying"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// Task represents a background job that needs to be processed
type Task struct {
	ID          string     `db:"id"`
	Type        string     `db:"type"`
	Payload     []byte     `db:"payload"`
	Status      TaskStatus `db:"status"`
	Attempts    int        `db:"attempts"`
	MaxAttempts int        `db:"max_attempts"`
	RunAt       time.Time  `db:"run_at"`
	Error       string     `db:"error"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}
