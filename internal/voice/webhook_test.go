package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"talent-rodeo/internal/database"
	"talent-rodeo/internal/models"
)

func setupWebhookTest(t *testing.T) (*WebhookHandler, *database.DB) {
	t.Helper()

	db := database.SetupTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	handler := NewWebhookHandler(db, logger)
	return handler, db
}

func createTestTask(t *testing.T, db *database.DB) *models.Task {
	t.Helper()

	msg := &models.Message{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		To:        "+1234567890",
		From:      "system",
		Body:      "Test message",
		CreatedAt: time.Now(),
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	task := &models.Task{
		ID:          time.Now().UnixNano(),
		Type:        "phone_call",
		Status:      models.TaskStatusRunning,
		MaxAttempts: 3,
		Attempts:    1,
		RunAt:       time.Now().Add(time.Hour),
		Payload:     payload,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()
	if err := db.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create test task: %v", err)
	}

	return task
}

func TestWebhookHandler_HandleTwilioCallback(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(context.Context, *database.DB, int64) error
		taskID     int64
		callStatus string
		wantStatus string
		wantCode   int
	}{
		{
			name:       "completed_call",
			taskID:     time.Now().UnixNano(),
			callStatus: "completed",
			wantStatus: models.TaskStatusComplete,
			wantCode:   http.StatusOK,
			setup: func(ctx context.Context, db *database.DB, taskID int64) error {
				msg := &models.Message{
					ID:        fmt.Sprintf("%d", taskID),
					To:        "+1234567890",
					From:      "system",
					Body:      "Test message",
					CreatedAt: time.Now().UTC(),
				}
				payload, err := json.Marshal(msg)
				if err != nil {
					return fmt.Errorf("failed to marshal message: %v", err)
				}

				task := &models.Task{
					ID:          taskID,
					Type:        "phone_call",
					Status:      models.TaskStatusPending,
					MaxAttempts: 3,
					RunAt:       time.Now().Add(time.Hour).UTC(),
					Payload:     payload,
					CreatedAt:   time.Now().UTC(),
					UpdatedAt:   time.Now().UTC(),
				}
				if err := db.CreateTask(ctx, task); err != nil {
					return fmt.Errorf("failed to create task: %w", err)
				}

				// Verify task was created
				created, err := db.GetTask(ctx, taskID)
				if err != nil {
					return fmt.Errorf("failed to verify task creation: %w", err)
				}
				if created == nil {
					return fmt.Errorf("task not found after creation")
				}

				return nil
			},
		},
		{
			name:       "failed_call",
			taskID:     time.Now().UnixNano(),
			callStatus: "failed",
			wantStatus: models.TaskStatusRetrying,
			wantCode:   http.StatusOK,
			setup: func(ctx context.Context, db *database.DB, taskID int64) error {
				msg := &models.Message{
					ID:        fmt.Sprintf("%d", taskID),
					To:        "+1234567890",
					From:      "system",
					Body:      "Test message",
					CreatedAt: time.Now().UTC(),
				}
				payload, err := json.Marshal(msg)
				if err != nil {
					return fmt.Errorf("failed to marshal message: %v", err)
				}

				task := &models.Task{
					ID:          taskID,
					Type:        "phone_call",
					Status:      models.TaskStatusPending,
					MaxAttempts: 3,
					RunAt:       time.Now().Add(time.Hour).UTC(),
					Payload:     payload,
					CreatedAt:   time.Now().UTC(),
					UpdatedAt:   time.Now().UTC(),
				}
				if err := db.CreateTask(ctx, task); err != nil {
					return fmt.Errorf("failed to create task: %w", err)
				}

				// Verify task was created
				created, err := db.GetTask(ctx, taskID)
				if err != nil {
					return fmt.Errorf("failed to verify task creation: %w", err)
				}
				if created == nil {
					return fmt.Errorf("task not found after creation")
				}

				return nil
			},
		},
		{
			name:       "nonexistent_task",
			taskID:     time.Now().UnixNano(),
			callStatus: "completed",
			wantCode:   http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := database.SetupTestDB(t)
			handler := NewWebhookHandler(db, slog.Default())

			ctx := context.Background()
			if tt.setup != nil {
				if err := tt.setup(ctx, db, tt.taskID); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			// Create test request with form data
			form := url.Values{}
			form.Set("CallSid", "test-call-sid")
			form.Set("CallStatus", tt.callStatus)
			form.Set("TaskId", fmt.Sprintf("%d", tt.taskID))

			req := httptest.NewRequest(
				http.MethodPost,
				"/webhook/twilio",
				strings.NewReader(form.Encode()),
			)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Create response recorder
			rr := httptest.NewRecorder()

			// Handle the request
			handler.HandleTwilioCallback(rr, req)

			// Check response status
			if got := rr.Code; got != tt.wantCode {
				t.Errorf("HandleTwilioCallback() status = %v, want %v", got, tt.wantCode)
			}

			// If we expect success, verify task state
			if tt.wantCode == http.StatusOK {
				task, err := db.GetTask(ctx, tt.taskID)
				if err != nil {
					t.Fatalf("failed to get task: %v", err)
				}

				if task.Status != tt.wantStatus {
					t.Errorf("task status = %v, want %v", task.Status, tt.wantStatus)
				}
			}
		})
	}
}

func TestWebhookHandler_HandleTwilioCallback_Concurrent(t *testing.T) {
	handler, db := setupWebhookTest(t)

	// Create a context with timeout to ensure cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Defer database cleanup until after all goroutines complete
	defer func() {
		// Give any in-progress transactions time to complete
		time.Sleep(100 * time.Millisecond)
		db.Close()
	}()

	task := createTestTask(t, db)

	// Create multiple concurrent requests
	concurrentRequests := 5
	var wg sync.WaitGroup
	wg.Add(concurrentRequests)
	errCh := make(chan error, concurrentRequests)

	// Create a semaphore to limit concurrent database access
	sem := make(chan struct{}, 2) // Allow 2 concurrent transactions

	for i := 0; i < concurrentRequests; i++ {
		go func(i int) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				errCh <- fmt.Errorf("request %d timed out", i)
				return
			case sem <- struct{}{}: // Acquire semaphore
				defer func() { <-sem }() // Release semaphore
			}

			form := url.Values{}
			form.Set("CallSid", fmt.Sprintf("test-call-sid-%d", i))
			form.Set("CallStatus", "completed")
			form.Set("TaskId", fmt.Sprintf("%d", task.ID))

			req := httptest.NewRequest(
				http.MethodPost,
				"/webhook/twilio",
				strings.NewReader(form.Encode()),
			).WithContext(ctx)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rr := httptest.NewRecorder()
			handler.HandleTwilioCallback(rr, req)

			if rr.Code != http.StatusOK {
				errCh <- fmt.Errorf("concurrent request %d failed with status %d: %s", i, rr.Code, rr.Body.String())
			}
		}(i)
	}

	// Wait for all requests to complete
	wg.Wait()
	close(errCh)

	// Check for any errors
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}
	if len(errors) > 0 {
		for _, err := range errors {
			t.Error(err)
		}
		t.Fatal("one or more concurrent requests failed")
	}

	// Verify final task state
	updatedTask, err := db.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get final task state: %v", err)
	}

	if updatedTask.Status != models.TaskStatusComplete {
		t.Errorf("final task status = %v, want %v", updatedTask.Status, models.TaskStatusComplete)
	}
}
