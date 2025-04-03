package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"
)

func TestWebhookHandler_ProcessTask(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	tests := []struct {
		name       string
		payload    WebhookPayload
		setupMock  func() *httptest.Server
		wantErr    bool
		wantRetry  bool
		errMessage string
	}{
		{
			name: "successful_request",
			payload: WebhookPayload{
				URL:    "", // Will be set by test
				Name:   "Test Successful Webhook",
				Method: http.MethodPost,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: []byte(`{"test":"data"}`),
			},
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			},
			wantErr: false,
		},
		{
			name: "retry_on_429",
			payload: WebhookPayload{
				URL:    "", // Will be set by test
				Name:   "Test Rate Limited Webhook",
				Method: http.MethodPost,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: []byte(`{"test":"data"}`),
			},
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusTooManyRequests)
				}))
			},
			wantErr:    true,
			wantRetry:  true,
			errMessage: "received retryable status code 429",
		},
		{
			name: "permanent_failure_on_400",
			payload: WebhookPayload{
				URL:    "", // Will be set by test
				Name:   "Test Bad Request Webhook",
				Method: http.MethodPost,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: []byte(`{"test":"data"}`),
			},
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
				}))
			},
			wantErr:    true,
			wantRetry:  false,
			errMessage: "webhook call failed with status 400",
		},
		{
			name: "respects_custom_headers",
			payload: WebhookPayload{
				URL:    "", // Will be set by test
				Name:   "Test Custom Headers Webhook",
				Method: http.MethodPost,
				Headers: map[string]string{
					"X-Custom-Header": "test-value",
					"Authorization":   "Bearer test-token",
				},
				Body: []byte(`{"test":"data"}`),
			},
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("X-Custom-Header") != "test-value" {
						t.Error("custom header not set correctly")
					}
					if r.Header.Get("Authorization") != "Bearer test-token" {
						t.Error("authorization header not set correctly")
					}
					w.WriteHeader(http.StatusOK)
				}))
			},
			wantErr: false,
		},
		{
			name: "handles_large_response",
			payload: WebhookPayload{
				URL:    "", // Will be set by test
				Name:   "Test Large Response Webhook",
				Method: http.MethodGet,
			},
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Write more than MaxResponseSize
					w.Write(make([]byte, MaxResponseSize+1))
				}))
			},
			wantErr: false, // We don't error on large responses, just truncate them
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Starting test case: %s", tt.name)

			// Setup mock server
			server := tt.setupMock()
			defer server.Close()

			// Update payload URL to point to mock server
			tt.payload.URL = server.URL
			t.Logf("Using mock server URL: %s", server.URL)

			// Create handler
			handler := NewWebhookScheduler(logger, nil)

			// Create task
			payloadBytes, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			task := &internal.Task{
				ID:          fmt.Sprintf("test-task-%d", time.Now().UnixNano()),
				Name:        tt.payload.Name, // Use the webhook name as the task name
				Type:        TaskTypeWebhookCall,
				Payload:     payloadBytes,
				Status:      internal.TaskStatusPending,
				MaxAttempts: 3,
				RunAt:       time.Now(),
			}

			t.Logf("Created task: id=%s, name=%s, type=%s, status=%s",
				task.ID, task.Name, task.Type, task.Status)

			// Process task
			err = handler.ProcessTask(context.Background(), task)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessTask() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMessage != "" && err.Error() != tt.errMessage {
				t.Errorf("ProcessTask() error message = %v, want %v", err.Error(), tt.errMessage)
			}

			// For retryable errors, verify task is marked for retry
			if tt.wantRetry {
				if task.Status != internal.TaskStatusRetrying {
					t.Errorf("Task status = %v, want %v", task.Status, internal.TaskStatusRetrying)
				}
				t.Logf("Task marked for retry: attempts=%d, next_run=%v",
					task.Attempts, task.RunAt)
			}

			t.Logf("Test case completed: status=%s, error=%v", task.Status, err)
		})
	}
}
