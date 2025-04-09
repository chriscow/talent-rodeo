package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/funcs"

	"cloud.google.com/go/firestore"
)

// NATSMessagePayload represents the data needed to publish a NATS message
type NATSMessagePayload struct {
	Subject  string            `json:"subject"`  // The subject to publish to
	Data     json.RawMessage   `json:"data"`     // The data to publish
	Metadata map[string]string `json:"metadata"` // Optional metadata for the message
}

// TaskHandler processes a specific type of task
type TaskHandler interface {
	// ProcessTask handles the execution of a task
	ProcessTask(ctx context.Context, task *internal.Task) error
	// TaskType returns the type of task this handler processes
	TaskType() string
}

// Processor manages the execution of background tasks
type Processor struct {
	storage      TaskStorage
	handlers     map[string]TaskHandler
	logger       *slog.Logger
	wg           sync.WaitGroup
	mu           sync.RWMutex
	done         chan struct{}
	retryBackoff time.Duration // Duration to wait between retries
}

// NewProcessor creates a new task processor
func NewProcessor(storage TaskStorage, logger *slog.Logger) *Processor {
	return &Processor{
		storage:      storage,
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
	p.logger.Debug("tasks: registering task handler", "type", handler.TaskType())
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

	// Get pending tasks from storage
	tasks, err := p.storage.GetPendingTasks(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending tasks: %w", err)
	}

	if len(tasks) > 0 {
		p.logger.Info("processing tasks",
			"count", len(tasks),
			"task_ids", fmt.Sprintf("%v", tasks),
		)
	}

	for _, task := range tasks {
		handler, exists := p.handlers[task.Type]
		if !exists {
			p.logger.Error("no handler for task type",
				"type", task.Type,
				"task_id", task.ID,
				"run_at", task.RunAt,
				"status", task.Status,
			)
			continue
		}

		// Process the task within a transaction
		err := p.storage.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			// Get the latest task state to ensure we have the current attempts count
			latestTask, err := p.storage.GetTask(ctx, task.ID)
			if err != nil {
				return fmt.Errorf("failed to get latest task state: %w", err)
			}
			if latestTask == nil {
				return fmt.Errorf("task not found: %s", task.ID)
			}

			p.logger.Debug("starting task processing",
				"task_id", latestTask.ID,
				"current_status", latestTask.Status,
				"current_attempts", latestTask.Attempts,
				"max_attempts", latestTask.MaxAttempts,
				"run_at", latestTask.RunAt,
			)

			// Update task status to running within transaction
			latestTask.Status = internal.TaskStatusRunning
			latestTask.Attempts++ // Always increment attempts when processing starts

			if err := p.storage.UpdateTask(ctx, latestTask); err != nil {
				return fmt.Errorf("failed to update task status: %w", err)
			}

			// Update our task reference to match the latest state
			task = latestTask
			return nil
		})

		if err != nil {
			p.logger.Error("failed to update task status",
				"error", err,
				"task_id", task.ID,
			)
			continue
		}

		// Process the task
		p.wg.Add(1)
		taskID := task.ID             // Capture task ID for the goroutine
		taskAttempts := task.Attempts // Capture current attempts count
		go func() {
			defer p.wg.Done()

			// Process the task and update its status within a transaction
			err := p.storage.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
				// Get the latest task state
				t, err := p.storage.GetTask(ctx, taskID)
				if err != nil {
					return fmt.Errorf("failed to get task: %w", err)
				}
				if t == nil {
					return fmt.Errorf("task not found: %s", taskID)
				}

				err = handler.ProcessTask(ctx, t)
				if err != nil {
					p.logger.Error("task processing failed",
						"error", err,
						"task_id", t.ID,
						"type", t.Type,
						"current_attempts", t.Attempts,
						"captured_attempts", taskAttempts,
						"max_attempts", t.MaxAttempts,
						"old_status", t.Status,
					)

					t.Error = err.Error()
					oldStatus := t.Status
					if taskAttempts >= t.MaxAttempts {
						p.logger.Warn(internal.BGYellow("task exceeded max attempts, marking as failed"),
							"task_id", t.ID,
							"current_attempts", t.Attempts,
							"captured_attempts", taskAttempts,
							"max_attempts", t.MaxAttempts,
							"old_status", oldStatus,
							"new_status", internal.TaskStatusFailed,
						)
						t.Status = internal.TaskStatusFailed
					} else {
						p.logger.Info("task will retry",
							"task_id", t.ID,
							"current_attempts", t.Attempts,
							"captured_attempts", taskAttempts,
							"max_attempts", t.MaxAttempts,
							"old_status", oldStatus,
							"new_status", internal.TaskStatusRetrying,
							"retry_backoff", p.retryBackoff*time.Duration(taskAttempts),
						)
						t.Status = internal.TaskStatusRetrying
						t.RunAt = time.Now().Add(p.retryBackoff * time.Duration(taskAttempts))
						t.Error = fmt.Sprintf("Call failed with status: %s, will retry", t.Status)
					}
				} else {
					oldStatus := t.Status
					t.Status = internal.TaskStatusComplete
					t.Error = ""
					p.logger.Info("task completed successfully",
						"task_id", t.ID,
						"type", t.Type,
						"current_attempts", t.Attempts,
						"captured_attempts", taskAttempts,
						"old_status", oldStatus,
						"new_status", t.Status,
					)
				}

				return p.storage.UpdateTask(ctx, t)
			})

			if err != nil {
				p.logger.Error("failed to process task",
					"error", err,
					"task_id", taskID,
				)
			}
		}()
	}

	return nil
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

	if err := p.storage.CreateTask(ctx, task); err != nil {
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
func (p *Processor) CreateAndEnqueueTask(ctx context.Context, taskType string, name string, payload []byte, runAt time.Time) (*internal.Task, error) {
	_, exists := p.handlers[taskType]
	if !exists {
		p.logger.Error("no handler registered for task type", "type", taskType)
		return nil, fmt.Errorf("no handler registered for task type: %s", taskType)
	}

	id := funcs.Slugify(fmt.Sprintf("%s-%d", name, time.Now().UnixNano()))

	task := &internal.Task{
		ID:          id,
		Name:        name,
		Type:        taskType,
		Payload:     payload,
		Status:      internal.TaskStatusPending,
		MaxAttempts: 1, // Default to 3 attempts
		RunAt:       runAt,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := p.EnqueueTask(ctx, task); err != nil {
		return nil, err
	}

	return task, nil
}

// ScheduleMessage schedules a message to be published at the specified time
func (h *Processor) ScheduleMessage(ctx context.Context, subject string, data any, metadata map[string]string, runAt time.Time) error {
	h.logger.Debug("scheduling message",
		"subject", subject,
		"run_at", runAt,
	)

	// Validate parameters
	if subject == "" {
		return fmt.Errorf("subject is required")
	}
	if data == nil {
		return fmt.Errorf("data is required")
	}
	if runAt.IsZero() {
		return fmt.Errorf("run time is required")
	}

	// Don't schedule messages in the past
	if runAt.Before(time.Now().UTC()) {
		return fmt.Errorf("cannot schedule messages in the past")
	}

	// Marshal the message data
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Create task payload
	payload := NATSMessagePayload{
		Subject:  subject,
		Data:     dataBytes,
		Metadata: metadata,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create unique task ID
	taskID := fmt.Sprintf("nats-message-scheduler-%d", time.Now().UnixNano())

	// Create the task
	task := &internal.Task{
		ID:          taskID,
		Name:        fmt.Sprintf("NATS Message: %s", subject),
		Type:        "nats_message_scheduler",
		Payload:     payloadBytes,
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3, // Allow up to 3 retry attempts
		RunAt:       runAt.UTC(),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	h.logger.Debug("creating scheduler task",
		"task_id", task.ID,
		"name", task.Name,
		"subject", subject,
		"run_at", task.RunAt,
		"max_attempts", task.MaxAttempts,
	)

	// Enqueue the task
	if err := h.EnqueueTask(ctx, task); err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	h.logger.Info("message scheduled",
		"task_id", task.ID,
		"name", task.Name,
		"subject", subject,
		"run_at", runAt.UTC(),
	)

	return nil
}
