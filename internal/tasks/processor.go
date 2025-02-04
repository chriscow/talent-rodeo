package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/models"
)

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusComplete  TaskStatus = "complete"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusRetrying  TaskStatus = "retrying"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// Task represents a background task that needs to be processed
type Task struct {
	ID          string
	Type        string
	Payload     []byte
	Status      TaskStatus
	Attempts    int
	MaxAttempts int
	RunAt       time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Error       string
}

// TaskHandler processes a specific type of task
type TaskHandler interface {
	// ProcessTask handles the execution of a task
	ProcessTask(ctx context.Context, task *models.Task) error
	// TaskType returns the type of task this handler processes
	TaskType() string
}

// Processor manages the execution of background tasks
type Processor struct {
	db       *database.DB
	handlers map[string]TaskHandler
	logger   *slog.Logger
	wg       sync.WaitGroup
	mu       sync.RWMutex
	done     chan struct{}
}

// NewProcessor creates a new task processor
func NewProcessor(db *database.DB, logger *slog.Logger) *Processor {
	return &Processor{
		db:       db,
		handlers: make(map[string]TaskHandler),
		logger:   logger,
		done:     make(chan struct{}),
	}
}

// RegisterHandler adds a new task handler
func (p *Processor) RegisterHandler(handler TaskHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logger.Debug("registering task handler", "type", handler.TaskType())
	p.handlers[handler.TaskType()] = handler
}

// Start begins processing tasks
func (p *Processor) Start(ctx context.Context) error {
	p.logger.Debug("starting task processor")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Debug("task processor context cancelled")
			return ctx.Err()
		case <-p.done:
			p.logger.Debug("task processor stopped")
			return nil
		case <-ticker.C:
			if err := p.processPendingTasks(ctx); err != nil {
				p.logger.Error("failed to process pending tasks", "error", err)
			}
		}
	}
}

// Stop gracefully shuts down the processor
func (p *Processor) Stop() {
	p.logger.Debug("stopping task processor")
	close(p.done)
	p.wg.Wait()
	p.logger.Debug("task processor stopped, all tasks completed")
}

func (p *Processor) processPendingTasks(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Get pending tasks from database
	tasks, err := p.db.GetPendingTasks(10) // Process up to 10 tasks at a time
	if err != nil {
		return fmt.Errorf("failed to get pending tasks: %w", err)
	}

	p.logger.Debug("fetched pending tasks",
		"count", len(tasks),
	)

	for _, task := range tasks {
		handler, exists := p.handlers[task.Type]
		if !exists {
			p.logger.Error("no handler for task type", "type", task.Type, "task_id", task.ID)
			continue
		}

		// Start transaction for task processing
		tx, err := p.db.BeginTxx(ctx, nil)
		if err != nil {
			p.logger.Error("failed to begin transaction",
				"error", err,
				"task_id", task.ID,
			)
			continue
		}

		// Update task status to running within transaction
		task.Status = models.TaskStatusRunning
		task.Attempts++
		p.logger.Debug("updating task status to running",
			"task_id", task.ID,
			"type", task.Type,
			"attempts", task.Attempts,
			"max_attempts", task.MaxAttempts,
		)
		if err := p.db.UpdateTaskTx(ctx, tx, &task); err != nil {
			tx.Rollback()
			p.logger.Error("failed to update task status",
				"error", err,
				"task_id", task.ID,
			)
			continue
		}

		// Process the task
		p.wg.Add(1)
		taskID := task.ID // Capture task ID for the goroutine
		go func() {
			defer p.wg.Done()

			// Start a new transaction for task completion
			completionTx, err := p.db.BeginTxx(ctx, nil)
			if err != nil {
				p.logger.Error("failed to begin completion transaction",
					"error", err,
					"task_id", taskID,
				)
				return
			}

			// Get the latest task state within transaction
			t, err := p.db.GetTaskTx(ctx, completionTx, taskID)
			if err != nil {
				completionTx.Rollback()
				p.logger.Error("failed to get task",
					"error", err,
					"task_id", taskID,
				)
				return
			}
			if t == nil {
				completionTx.Rollback()
				p.logger.Error("task not found",
					"task_id", taskID,
				)
				return
			}

			p.logger.Debug("processing task",
				"task_id", t.ID,
				"type", t.Type,
				"status", t.Status,
				"attempts", t.Attempts,
				"max_attempts", t.MaxAttempts,
			)

			err = handler.ProcessTask(ctx, t)
			if err != nil {
				p.logger.Error("task processing failed",
					"error", err,
					"task_id", t.ID,
					"type", t.Type,
					"attempts", t.Attempts,
					"max_attempts", t.MaxAttempts,
				)

				t.Error = err.Error()
				if t.Attempts >= t.MaxAttempts {
					p.logger.Debug("task exceeded max attempts, marking as failed",
						"task_id", t.ID,
						"attempts", t.Attempts,
						"max_attempts", t.MaxAttempts,
					)
					t.Status = models.TaskStatusFailed
				} else {
					nextRunAt := time.Now().Add(time.Duration(t.Attempts) * time.Minute)
					p.logger.Debug("task will be retried",
						"task_id", t.ID,
						"attempts", t.Attempts,
						"max_attempts", t.MaxAttempts,
						"next_run_at", nextRunAt,
					)
					t.Status = models.TaskStatusRetrying
					t.RunAt = nextRunAt
				}
			} else {
				p.logger.Debug("task completed successfully",
					"task_id", t.ID,
					"type", t.Type,
					"attempts", t.Attempts,
				)
				t.Status = models.TaskStatusComplete
				t.Error = ""
			}

			p.logger.Debug("updating task after processing",
				"task_id", t.ID,
				"status", t.Status,
				"attempts", t.Attempts,
				"error", t.Error != "",
			)
			if err := p.db.UpdateTaskTx(ctx, completionTx, t); err != nil {
				completionTx.Rollback()
				p.logger.Error("failed to update task after processing",
					"error", err,
					"task_id", t.ID,
				)
				return
			}

			if err := completionTx.Commit(); err != nil {
				p.logger.Error("failed to commit completion transaction",
					"error", err,
					"task_id", t.ID,
				)
			}
		}()

		// Commit the initial transaction after spawning the goroutine
		if err := tx.Commit(); err != nil {
			p.logger.Error("failed to commit initial transaction",
				"error", err,
				"task_id", task.ID,
			)
		}
	}

	return nil
}

// generateTaskID creates a unique ID for a task
func generateTaskID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// EnqueueTask adds a new task to be processed
func (p *Processor) EnqueueTask(ctx context.Context, taskType string, payload []byte, runAt time.Time) (*models.Task, error) {
	_, exists := p.handlers[taskType]
	if !exists {
		p.logger.Error("no handler registered for task type", "type", taskType)
		return nil, fmt.Errorf("no handler registered for task type: %s", taskType)
	}

	id, err := generateTaskID()
	if err != nil {
		p.logger.Error("failed to generate task ID", "error", err)
		return nil, fmt.Errorf("failed to generate task ID: %w", err)
	}

	task := &models.Task{
		ID:          id,
		Type:        taskType,
		Payload:     payload,
		Status:      models.TaskStatusPending,
		MaxAttempts: 3, // Default to 3 attempts
		RunAt:       runAt,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	p.logger.Debug("enqueueing task",
		"task_id", task.ID,
		"type", task.Type,
		"run_at", task.RunAt,
		"max_attempts", task.MaxAttempts,
		"payload_size", len(task.Payload),
	)

	if err := p.db.CreateTask(task); err != nil {
		p.logger.Error("failed to create task",
			"error", err,
			"task_id", task.ID,
			"type", task.Type,
		)
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	p.logger.Debug("task enqueued successfully",
		"task_id", task.ID,
		"type", task.Type,
	)

	return task, nil
}
