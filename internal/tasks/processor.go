package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/database"
)

// TaskHandler processes a specific type of task
type TaskHandler interface {
	// ProcessTask handles the execution of a task
	ProcessTask(ctx context.Context, task *internal.Task) error
	// TaskType returns the type of task this handler processes
	TaskType() string
}

// Processor manages the execution of background tasks
type Processor struct {
	db           *database.DB
	handlers     map[string]TaskHandler
	logger       *slog.Logger
	wg           sync.WaitGroup
	mu           sync.RWMutex
	done         chan struct{}
	retryBackoff time.Duration // Duration to wait between retries
}

// NewProcessor creates a new task processor
func NewProcessor(db *database.DB, logger *slog.Logger) *Processor {
	return &Processor{
		db:           db,
		handlers:     make(map[string]TaskHandler),
		logger:       logger,
		done:         make(chan struct{}),
		retryBackoff: time.Minute, // Default to 1 minute
	}
}

// RegisterHandler adds a new task handler
func (p *Processor) RegisterHandler(handler TaskHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logger.Debug("registering task handler", "type", handler.TaskType())
	p.handlers[handler.TaskType()] = handler
}

// SetRetryBackoff sets the duration to wait between retries
func (p *Processor) SetRetryBackoff(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retryBackoff = d
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
	tasks, err := p.db.GetPendingTasks(ctx)
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
		oldStatus := task.Status
		task.Status = internal.TaskStatusRunning
		task.Attempts++ // Always increment attempts when processing starts
		p.logger.Debug("updating task status",
			"task_id", task.ID,
			"type", task.Type,
			"old_status", oldStatus,
			"new_status", task.Status,
			"attempts", task.Attempts,
			"max_attempts", task.MaxAttempts,
		)
		if err := p.db.UpdateTaskTx(ctx, tx, task); err != nil {
			tx.Rollback()
			p.logger.Error("failed to update task status",
				"error", err,
				"task_id", task.ID,
				"old_status", oldStatus,
				"new_status", task.Status,
			)
			continue
		}

		// Commit the initial transaction before spawning the goroutine
		if err := tx.Commit(); err != nil {
			p.logger.Error("failed to commit initial transaction",
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
					"old_status", t.Status,
				)

				t.Error = err.Error()
				oldStatus = t.Status
				if t.Attempts >= t.MaxAttempts {
					p.logger.Debug("task exceeded max attempts, marking as failed",
						"task_id", t.ID,
						"attempts", t.Attempts,
						"max_attempts", t.MaxAttempts,
						"old_status", oldStatus,
						"new_status", internal.TaskStatusFailed,
					)
					t.Status = internal.TaskStatusFailed
				} else {
					t.Status = internal.TaskStatusRetrying
					t.RunAt = time.Now().Add(p.retryBackoff * time.Duration(t.Attempts))
					t.Error = fmt.Sprintf("Call failed with status: %s, will retry", t.Status)
				}
			} else {
				oldStatus = t.Status
				t.Status = internal.TaskStatusComplete
				t.Error = ""
				p.logger.Debug("task completed successfully",
					"task_id", t.ID,
					"type", t.Type,
					"attempts", t.Attempts,
					"old_status", oldStatus,
					"new_status", t.Status,
				)
			}

			p.logger.Debug("updating task after processing",
				"task_id", t.ID,
				"old_status", oldStatus,
				"new_status", t.Status,
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
	}

	return nil
}

// generateTaskID creates a unique ID for a task
func generateTaskID() (int64, error) {
	return time.Now().UnixNano(), nil
}

// EnqueueTask adds a new task to be processed
func (p *Processor) EnqueueTask(ctx context.Context, task *internal.Task) error {
	p.logger.Debug("enqueueing task",
		"task_id", task.ID,
		"type", task.Type,
		"run_at", task.RunAt,
		"max_attempts", task.MaxAttempts,
		"payload_size", len(task.Payload),
	)

	_, exists := p.handlers[task.Type]
	if !exists {
		p.logger.Error("no handler registered for task type", "type", task.Type)
		return fmt.Errorf("no handler registered for task type: %s", task.Type)
	}

	if err := p.db.CreateTask(ctx, task); err != nil {
		p.logger.Error("failed to create task",
			"error", err,
			"task_id", task.ID,
			"type", task.Type,
		)
		return fmt.Errorf("failed to create task: %w", err)
	}

	p.logger.Debug("task enqueued successfully",
		"task_id", task.ID,
		"type", task.Type,
	)

	return nil
}

// CreateAndEnqueueTask is a helper method that creates a new task with the given parameters and enqueues it
func (p *Processor) CreateAndEnqueueTask(ctx context.Context, taskType string, payload []byte, runAt time.Time) (*internal.Task, error) {
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

	task := &internal.Task{
		ID:          id,
		Type:        taskType,
		Payload:     payload,
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3, // Default to 3 attempts
		RunAt:       runAt,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := p.EnqueueTask(ctx, task); err != nil {
		return nil, err
	}

	return task, nil
}
