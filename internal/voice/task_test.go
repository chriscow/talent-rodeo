package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/database"
)

func TestSimpleTaskCreateAndRead(t *testing.T) {
	// Setup logger with debug level
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Setup test DB
	db := database.SetupTestDB(t)
	ctx := context.Background()

	// Create minimal task
	taskID := time.Now().UnixNano()
	logger.Debug("generated task ID",
		"task_id", taskID,
		"type", fmt.Sprintf("%T", taskID),
	)

	// Create minimal message
	msg := &internal.Message{
		ID:        fmt.Sprintf("%d", taskID),
		To:        "+1234567890",
		From:      "system",
		Body:      "Test message",
		CreatedAt: time.Now().UTC(),
	}

	// Marshal message to payload
	payload, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}
	logger.Debug("created payload",
		"payload_size", len(payload),
		"payload_type", fmt.Sprintf("%T", payload),
	)

	// Create task
	task := &internal.Task{
		ID:          taskID,
		Type:        "phone_call",
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		RunAt:       time.Now().Add(time.Hour).UTC(),
		Payload:     payload,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// Log task details before creation
	logger.Debug("creating task",
		"task_id", task.ID,
		"task_id_type", fmt.Sprintf("%T", task.ID),
		"type", task.Type,
		"status", task.Status,
		"run_at", task.RunAt,
	)

	// Create task in DB
	if err := db.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	logger.Debug("task created successfully")

	// Immediately try to read it back
	logger.Debug("attempting to read task",
		"task_id", taskID,
		"task_id_type", fmt.Sprintf("%T", taskID),
	)
	readTask, err := db.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("failed to read task: %v", err)
	}
	if readTask == nil {
		t.Fatal("task not found after creation")
	}

	// Log retrieved task details
	logger.Debug("task retrieved successfully",
		"task_id", readTask.ID,
		"task_id_type", fmt.Sprintf("%T", readTask.ID),
		"type", readTask.Type,
		"status", readTask.Status,
		"run_at", readTask.RunAt,
	)

	// Verify fields match
	if readTask.ID != task.ID {
		t.Errorf("task ID mismatch: got %v (%T), want %v (%T)",
			readTask.ID, readTask.ID,
			task.ID, task.ID)
	}
	if readTask.Type != task.Type {
		t.Errorf("task type mismatch: got %v, want %v", readTask.Type, task.Type)
	}
	if readTask.Status != task.Status {
		t.Errorf("task status mismatch: got %v, want %v", readTask.Status, task.Status)
	}
	if !readTask.RunAt.Truncate(time.Millisecond).Equal(task.RunAt.Truncate(time.Millisecond)) {
		t.Errorf("task run_at mismatch: got %v, want %v", readTask.RunAt.Truncate(time.Microsecond), task.RunAt.Truncate(time.Microsecond))
	}
}
