package internal

import (
	"encoding/json"
	"time"
)

// Task status constants
const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusComplete  = "complete"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"
	TaskStatusRetrying  = "retrying"
)

// TaskError represents an error that occurred during task processing.
// It includes information about whether the error should trigger a retry.
type TaskError struct {
	Err         error // The underlying error
	ShouldRetry bool  // Whether the task should be retried
}

// Error implements the error interface.
func (e *TaskError) Error() string {
	return e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *TaskError) Unwrap() error {
	return e.Err
}

// Task represents a background job to be processed
type Task struct {
	ID          int64           `db:"id" json:"id"`
	Type        string          `db:"type" json:"type"`
	Payload     json.RawMessage `db:"payload" json:"payload"`
	Status      string          `db:"status" json:"status"`
	Attempts    int             `db:"attempts" json:"attempts"`
	MaxAttempts int             `db:"max_attempts" json:"max_attempts"`
	RunAt       time.Time       `db:"run_at" json:"run_at"`
	Error       string          `db:"error" json:"error"`
	CreatedAt   time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time       `db:"updated_at" json:"updated_at"`
}

// NewTask creates a new task with default values
func NewTask(taskType string, payload any) (*Task, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Task{
		Type:        taskType,
		Payload:     payloadBytes,
		Status:      TaskStatusPending,
		Attempts:    0,
		MaxAttempts: 3,
		RunAt:       time.Now(),
	}, nil
}

// GetPayload unmarshals the task payload into the provided interface
func (t *Task) GetPayload(v any) error {
	return json.Unmarshal(t.Payload, v)
}

// SetPayload marshals the provided interface into the task payload
func (t *Task) SetPayload(v any) error {
	payloadBytes, err := json.Marshal(v)
	if err != nil {
		return err
	}
	t.Payload = payloadBytes
	return nil
}

// IsComplete returns true if the task is in a terminal state
func (t *Task) IsComplete() bool {
	return t.Status == TaskStatusComplete ||
		t.Status == TaskStatusFailed ||
		t.Status == TaskStatusCancelled
}

// CanRetry returns true if the task can be retried
func (t *Task) CanRetry() bool {
	return t.Status == TaskStatusFailed &&
		t.Attempts < t.MaxAttempts
}

// ShouldRun returns true if the task is ready to run
func (t *Task) ShouldRun() bool {
	return (t.Status == TaskStatusPending || t.Status == TaskStatusRetrying) &&
		time.Now().After(t.RunAt)
}
