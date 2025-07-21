package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"talent-rodeo/internal"
)

// HeldResponseRevisionHandler handles background tasks for processing held response revisions.
type HeldResponseRevisionHandler struct {
	revisionProcessor *internal.RevisionProcessor
	logger            *slog.Logger
}

// NewHeldResponseRevisionHandler creates a new held response revision handler.
func NewHeldResponseRevisionHandler(revisionProcessor *internal.RevisionProcessor, logger *slog.Logger) *HeldResponseRevisionHandler {
	return &HeldResponseRevisionHandler{
		revisionProcessor: revisionProcessor,
		logger:            logger,
	}
}

// HandleTask processes held response revision tasks.
func (h *HeldResponseRevisionHandler) HandleTask(ctx context.Context, task *internal.Task) error {
	if task.Type != "process_held_response_revisions" {
		return fmt.Errorf("unsupported task type: %s", task.Type)
	}

	if h.logger != nil {
		h.logger.Info("processing held response revisions task", "task_id", task.ID)
	}

	if err := h.revisionProcessor.ProcessRevisionRequests(ctx); err != nil {
		return fmt.Errorf("failed to process revision requests: %w", err)
	}

	if h.logger != nil {
		h.logger.Info("completed held response revisions task", "task_id", task.ID)
	}

	return nil
}

// CanHandle returns true if this handler can process the given task type.
func (h *HeldResponseRevisionHandler) CanHandle(taskType string) bool {
	return taskType == "process_held_response_revisions"
}
