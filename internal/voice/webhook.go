package voice

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/database"
)

// WebhookHandler processes Twilio webhook callbacks
type WebhookHandler struct {
	db     *database.DB
	logger *slog.Logger
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(db *database.DB, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{
		db:     db,
		logger: logger,
	}
}

// HandleTwilioCallback processes callbacks from Twilio about call status
func (h *WebhookHandler) HandleTwilioCallback(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		h.logger.Error("failed to parse form data", "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Extract relevant fields
	callSID := r.FormValue("CallSid")
	callStatus := r.FormValue("CallStatus")
	taskIDStr := r.FormValue("TaskId")

	// Parse task ID
	taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
	if err != nil {
		h.logger.Error("invalid task ID",
			"error", err,
			"task_id", taskIDStr,
		)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	h.logger.Debug("received Twilio callback",
		"call_sid", callSID,
		"status", callStatus,
		"task_id", taskID,
	)

	// Start transaction for updating task
	ctx := r.Context()
	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		h.logger.Error("failed to begin transaction",
			"error", err,
			"task_id", taskID,
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Get current task state
	task, err := h.db.GetTaskTx(ctx, tx, taskID)
	if err != nil {
		tx.Rollback()
		h.logger.Error("failed to get task",
			"error", err,
			"task_id", taskID,
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if task == nil {
		tx.Rollback()
		h.logger.Error("task not found",
			"task_id", taskID,
		)
		http.Error(w, "Task Not Found", http.StatusNotFound)
		return
	}

	// Update task based on call status
	switch callStatus {
	case "completed":
		task.Status = internal.TaskStatusComplete
	case "failed", "busy", "no-answer", "canceled":
		if task.Attempts >= task.MaxAttempts {
			task.Status = internal.TaskStatusFailed
			task.Error = fmt.Sprintf("Call failed with status: %s after %d attempts", callStatus, task.Attempts)
		} else {
			task.Status = internal.TaskStatusRetrying
			task.RunAt = time.Now().Add(time.Duration(task.Attempts) * time.Minute)
			task.Error = fmt.Sprintf("Call failed with status: %s, will retry", callStatus)
		}
	case "in-progress":
		task.Status = internal.TaskStatusRunning
	default:
		h.logger.Debug("ignoring intermediate call status",
			"status", callStatus,
			"task_id", taskID,
		)
		tx.Rollback()
		w.WriteHeader(http.StatusOK)
		return
	}

	// Update task in database
	task.UpdatedAt = time.Now()
	if err := h.db.UpdateTaskTx(ctx, tx, task); err != nil {
		tx.Rollback()
		h.logger.Error("failed to update task",
			"error", err,
			"task_id", taskID,
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		h.logger.Error("failed to commit transaction",
			"error", err,
			"task_id", taskID,
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("call status processed successfully",
		"task_id", taskID,
		"call_sid", callSID,
		"status", callStatus,
		"task_status", task.Status,
	)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"taskId": taskID,
	})
}
