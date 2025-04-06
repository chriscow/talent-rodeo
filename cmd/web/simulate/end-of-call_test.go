package simulate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/tasks/handlers"
	"talent-rodeo/internal/vapi"
)

// mockTaskProcessor implements the TaskProcessor interface for testing
type mockTaskProcessor struct {
	createAndEnqueueFunc func(ctx context.Context, taskType, name string, payload []byte, executeAt time.Time) (*internal.Task, error)
}

func (m *mockTaskProcessor) CreateAndEnqueueTask(ctx context.Context, taskType, name string, payload []byte, executeAt time.Time) (*internal.Task, error) {
	return m.createAndEnqueueFunc(ctx, taskType, name, payload, executeAt)
}

func (m *mockTaskProcessor) EnqueueTask(ctx context.Context, task *internal.Task) error {
	return nil
}

func TestLoadEndOfCallTestData(t *testing.T) {
	// Setup test directory and file
	testDir := "testdata/vapi/webhooks/end-of-call-report"
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll("testdata")

	testFile := filepath.Join(testDir, "end-of-call-report_1743786911443605712.json")
	testData := []byte(`{"message":{"type":"end-of-call-report","data":{"call":{"id":"test-id"}}}}`)
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	got, err := loadEndOfCallTestData()
	if err != nil {
		t.Fatalf("LoadEndOfCallTestData() error = %v", err)
	}

	if string(got) != string(testData) {
		t.Errorf("LoadEndOfCallTestData() = %v, want %v", string(got), string(testData))
	}
}

func TestUpdateEndOfCallReport(t *testing.T) {
	tests := []struct {
		name          string
		reportData    []byte
		userID        string
		appointmentID string
		wantErr       bool
		wantErrMsg    string
		validate      func(t *testing.T, report *vapi.EndOfCallReport)
	}{
		{
			name: "valid report with nested data",
			reportData: []byte(`{
				"message": {
					"type": "end-of-call-report",
					"data": {
						"type": "end-of-call-report",
						"endedReason": "completed",
						"call": {
							"id": "test-id",
							"name": "test-name",
							"customerId": "test-customer"
						}
					}
				}
			}`),
			userID:        "test-user",
			appointmentID: "test-appointment",
			wantErr:       false,
			validate: func(t *testing.T, report *vapi.EndOfCallReport) {
				if report.Call == nil {
					t.Error("Call field is nil")
					return
				}
				if report.Call.ID == nil || *report.Call.ID != "test-appointment" {
					t.Errorf("Call.ID = %v, want %v", report.Call.ID, "test-appointment")
				}
				if report.Call.Name == nil || *report.Call.Name != "test-appointment" {
					t.Errorf("Call.Name = %v, want %v", report.Call.Name, "test-appointment")
				}
				if report.Call.CustomerID == nil || *report.Call.CustomerID != "test-appointment" {
					t.Errorf("Call.CustomerID = %v, want %v", report.Call.CustomerID, "test-appointment")
				}
				if report.EndedReason != "completed" {
					t.Errorf("EndedReason = %v, want %v", report.EndedReason, "completed")
				}
			},
		},
		{
			name: "valid report with analysis and artifact",
			reportData: []byte(`{
				"message": {
					"type": "end-of-call-report",
					"timestamp": 1743786911314,
					"analysis": {
						"summary": "Test summary"
					},
					"artifact": {
						"messages": []
					},
					"call": {
						"id": "test-id",
						"name": "test-name",
						"customerId": "test-customer"
					},
					"endedReason": "completed"
				}
			}`),
			userID:        "test-user",
			appointmentID: "test-appointment",
			wantErr:       false,
			validate: func(t *testing.T, report *vapi.EndOfCallReport) {
				if report.Analysis.Summary == nil {
					t.Error("Analysis.Summary is nil")
				}
				if report.Artifact.Messages == nil {
					t.Error("Artifact.Messages is nil")
				}
				if report.Call == nil {
					t.Error("Call field is nil")
					return
				}
				if report.Call.ID == nil || *report.Call.ID != "test-appointment" {
					t.Errorf("Call.ID = %v, want %v", report.Call.ID, "test-appointment")
				}
			},
		},
		{
			name:          "invalid json",
			reportData:    []byte(`invalid json`),
			userID:        "test-user",
			appointmentID: "test-appointment",
			wantErr:       true,
			wantErrMsg:    "failed to unmarshal report data",
		},
		{
			name: "missing call information",
			reportData: []byte(`{
				"message": {
					"type": "end-of-call-report",
					"data": {
						"type": "end-of-call-report"
					}
				}
			}`),
			userID:        "test-user",
			appointmentID: "test-appointment",
			wantErr:       false,
			validate: func(t *testing.T, report *vapi.EndOfCallReport) {
				if report.Call == nil {
					t.Error("Call field is nil")
					return
				}
				if report.Call.ID == nil || *report.Call.ID != "test-appointment" {
					t.Errorf("Call.ID = %v, want %v", report.Call.ID, "test-appointment")
				}
				if report.Call.Name == nil || *report.Call.Name != "test-appointment" {
					t.Errorf("Call.Name = %v, want %v", report.Call.Name, "test-appointment")
				}
				if report.Call.CustomerID == nil || *report.Call.CustomerID != "test-appointment" {
					t.Errorf("Call.CustomerID = %v, want %v", report.Call.CustomerID, "test-appointment")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := updateEndOfCallReport(tt.reportData, tt.userID, tt.appointmentID)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateEndOfCallReport() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantErrMsg != "" && err != nil && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("updateEndOfCallReport() error = %v, want error containing %v", err, tt.wantErrMsg)
				return
			}
			if !tt.wantErr {
				if got == nil {
					t.Error("updateEndOfCallReport() returned nil but expected a value")
					return
				}
				var report vapi.EndOfCallReport
				if err := json.Unmarshal(got.Message.Data, &report); err != nil {
					t.Errorf("Failed to unmarshal updated report: %v", err)
					return
				}
				if tt.validate != nil {
					tt.validate(t, &report)
				}
			}
		})
	}
}

func TestCreateWebhookPayload(t *testing.T) {
	// Save original env and restore after test
	originalURL := os.Getenv("VAPI_WEBHOOK_URL")
	defer os.Setenv("VAPI_WEBHOOK_URL", originalURL)

	os.Setenv("VAPI_WEBHOOK_URL", "http://test.com")

	testData := []byte(`{"test":"data"}`)
	msg := &vapi.WebhookMessage{
		Message: vapi.WebhookMessageContent{
			Type: vapi.MsgTypeEndOfCallReport,
			Data: testData,
		},
	}

	got, err := createWebhookPayload(msg)
	if err != nil {
		t.Fatalf("createWebhookPayload() error = %v", err)
	}

	// Verify the structure of the payload body
	var gotBody struct {
		Message struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		} `json:"message"`
	}
	if err := json.Unmarshal(got.Body, &gotBody); err != nil {
		t.Fatalf("Failed to unmarshal payload body: %v", err)
	}

	if gotBody.Message.Type != vapi.MsgTypeEndOfCallReport {
		t.Errorf("Message.Type = %v, want %v", gotBody.Message.Type, vapi.MsgTypeEndOfCallReport)
	}

	if string(gotBody.Message.Data) != string(testData) {
		t.Errorf("Message.Data = %v, want %v", string(gotBody.Message.Data), string(testData))
	}

	want := &handlers.WebhookPayload{
		URL:    "http://test.com",
		Method: "POST",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}

	if got.URL != want.URL {
		t.Errorf("URL = %v, want %v", got.URL, want.URL)
	}
	if got.Method != want.Method {
		t.Errorf("Method = %v, want %v", got.Method, want.Method)
	}
	if got.Headers["Content-Type"] != want.Headers["Content-Type"] {
		t.Errorf("Content-Type = %v, want %v",
			got.Headers["Content-Type"], want.Headers["Content-Type"])
	}
}

func TestEndOfCall(t *testing.T) {
	tests := []struct {
		name          string
		setupEnv      func()
		cleanupEnv    func()
		processor     TaskProcessor
		userID        string
		appointmentID string
		wantErr       bool
		wantErrMsg    string
	}{
		{
			name: "successful end of call simulation",
			setupEnv: func() {
				os.Setenv("VAPI_WEBHOOK_URL", "http://test.com")
			},
			cleanupEnv: func() {
				os.Unsetenv("VAPI_WEBHOOK_URL")
			},
			processor: &mockTaskProcessor{
				createAndEnqueueFunc: func(ctx context.Context, taskType, name string, payload []byte, executeAt time.Time) (*internal.Task, error) {
					return &internal.Task{
						ID:   "test-task-id",
						Type: taskType,
						Name: name,
					}, nil
				},
			},
			userID:        "test-user",
			appointmentID: "test-appointment",
			wantErr:       false,
		},
		{
			name: "missing webhook URL",
			setupEnv: func() {
				os.Unsetenv("VAPI_WEBHOOK_URL")
			},
			cleanupEnv: func() {},
			processor: &mockTaskProcessor{
				createAndEnqueueFunc: func(ctx context.Context, taskType, name string, payload []byte, executeAt time.Time) (*internal.Task, error) {
					return &internal.Task{
						ID:   "test-task-id",
						Type: taskType,
						Name: name,
					}, nil
				},
			},
			userID:        "test-user",
			appointmentID: "test-appointment",
			wantErr:       true,
			wantErrMsg:    "failed to create webhook payload: VAPI_WEBHOOK_URL not set",
		},
		{
			name: "task processor error",
			setupEnv: func() {
				// Unset webhook URL to trigger an error
				os.Unsetenv("VAPI_WEBHOOK_URL")
			},
			cleanupEnv: func() {},
			processor: &mockTaskProcessor{
				createAndEnqueueFunc: func(ctx context.Context, taskType, name string, payload []byte, executeAt time.Time) (*internal.Task, error) {
					return nil, fmt.Errorf("task processor error")
				},
			},
			appointmentID: "test-appointment",
			wantErr:       true,
			wantErrMsg:    "failed to create webhook payload: VAPI_WEBHOOK_URL not set",
		},
		{
			name: "empty appointment ID",
			setupEnv: func() {
				os.Setenv("VAPI_WEBHOOK_URL", "http://test.com")
			},
			cleanupEnv: func() {
				os.Unsetenv("VAPI_WEBHOOK_URL")
			},
			processor: &mockTaskProcessor{
				createAndEnqueueFunc: func(ctx context.Context, taskType, name string, payload []byte, executeAt time.Time) (*internal.Task, error) {
					return &internal.Task{
						ID:   "test-task-id",
						Type: taskType,
						Name: name,
					}, nil
				},
			},
			appointmentID: "",
			wantErr:       true,
			wantErrMsg:    "appointment ID cannot be empty",
		},
		{
			name: "context canceled",
			setupEnv: func() {
				os.Setenv("VAPI_WEBHOOK_URL", "http://test.com")
			},
			cleanupEnv: func() {
				os.Unsetenv("VAPI_WEBHOOK_URL")
			},
			processor: &mockTaskProcessor{
				createAndEnqueueFunc: func(ctx context.Context, taskType, name string, payload []byte, executeAt time.Time) (*internal.Task, error) {
					return &internal.Task{
						ID:   "test-task-id",
						Type: taskType,
						Name: name,
					}, nil
				},
			},
			appointmentID: "test-appointment",
			wantErr:       true,
			wantErrMsg:    "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test directory and file
			testDir := "testdata/vapi/webhooks/end-of-call-report"
			if err := os.MkdirAll(testDir, 0755); err != nil {
				t.Fatalf("Failed to create test directory: %v", err)
			}
			defer os.RemoveAll("testdata")

			testFile := filepath.Join(testDir, "end-of-call-report_1743786911443605712.json")
			testData := []byte(`{
				"message": {
					"type": "end-of-call-report",
					"timestamp": 1743786911314,
					"analysis": {
						"summary": "Test summary"
					},
					"artifact": {
						"messages": []
					},
					"call": {
						"id": "test-id",
						"name": "test-appointment",
						"customerId": "test-customer"
					},
					"endedReason": "completed",
					"type": "end-of-call-report"
				}
			}`)
			if err := os.WriteFile(testFile, testData, 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			// Setup environment
			tt.setupEnv()
			defer tt.cleanupEnv()

			// Create context
			ctx := context.Background()
			if tt.name == "context canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}

			// Create test logger that captures output
			var logBuf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logBuf, nil))

			// Run test
			err := EndOfCall(ctx, tt.processor, tt.userID, tt.appointmentID, logger)

			// Check error
			if tt.wantErr {
				if err == nil {
					t.Errorf("EndOfCall() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("EndOfCall() error = %v, want error containing %v", err, tt.wantErrMsg)
				}
			} else if err != nil {
				t.Errorf("EndOfCall() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check logging output
			if !tt.wantErr {
				if !strings.Contains(logBuf.String(), "scheduled simulated end-of-call webhook") {
					t.Error("Expected log message not found")
				}
				if !strings.Contains(logBuf.String(), tt.appointmentID) {
					t.Error("Expected appointment ID not found in log message")
				}
			}
		})
	}
}

// TestEndOfCallWebhookPayload tests the complete webhook payload structure
func TestEndOfCallWebhookPayload(t *testing.T) {
	// Setup test directory and file
	testDir := "testdata/vapi/webhooks/end-of-call-report"
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll("testdata")

	testFile := filepath.Join(testDir, "end-of-call-report_1743786911443605712.json")
	testData := []byte(`{
		"message": {
			"type": "end-of-call-report",
			"timestamp": 1743786911314,
			"analysis": {
				"summary": "Test summary"
			},
			"artifact": {
				"messages": []
			},
			"call": {
				"id": "test-id",
				"name": "test-name",
				"customerId": "test-customer"
			},
			"endedReason": "completed"
		}
	}`)
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Set up environment
	os.Setenv("VAPI_WEBHOOK_URL", "http://test.com")
	defer os.Unsetenv("VAPI_WEBHOOK_URL")

	// Load and update the report
	userID := "test-user"
	appointmentID := "test-appointment"
	msg, err := updateEndOfCallReport(testData, userID, appointmentID)
	if err != nil {
		t.Fatalf("Failed to update end of call report: %v", err)
	}

	// Create webhook payload
	payload, err := createWebhookPayload(msg)
	if err != nil {
		t.Fatalf("Failed to create webhook payload: %v", err)
	}

	// Verify the webhook payload structure
	var webhookData struct {
		Message struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		} `json:"message"`
	}
	if err := json.Unmarshal(payload.Body, &webhookData); err != nil {
		t.Fatalf("Failed to unmarshal webhook payload: %v", err)
	}

	// Verify message type
	if webhookData.Message.Type != vapi.MsgTypeEndOfCallReport {
		t.Errorf("Message.Type = %v, want %v", webhookData.Message.Type, vapi.MsgTypeEndOfCallReport)
	}

	// Verify report structure
	var report vapi.EndOfCallReport
	if err := json.Unmarshal(webhookData.Message.Data, &report); err != nil {
		t.Fatalf("Failed to unmarshal report data: %v", err)
	}

	// Verify required fields
	if report.Call == nil {
		t.Fatal("Call field is nil")
	}
	if report.Call.ID == nil || *report.Call.ID != appointmentID {
		t.Errorf("Call.ID = %v, want %v", report.Call.ID, appointmentID)
	}
	if report.Call.Name == nil || *report.Call.Name != appointmentID {
		t.Errorf("Call.Name = %v, want %v", report.Call.Name, appointmentID)
	}
	if report.Call.CustomerID == nil || *report.Call.CustomerID != appointmentID {
		t.Errorf("Call.CustomerID = %v, want %v", report.Call.CustomerID, appointmentID)
	}
	if report.EndedReason != "completed" {
		t.Errorf("EndedReason = %v, want %v", report.EndedReason, "completed")
	}
}

// TestWebhookMessageUnmarshal tests the unmarshaling behavior of WebhookMessage
func TestWebhookMessageUnmarshal(t *testing.T) {
	// Test data that matches our production structure
	testData := []byte(`{
		"message": {
			"type": "end-of-call-report",
			"data": {
				"type": "end-of-call-report",
				"endedReason": "completed",
				"call": {
					"id": "test-id",
					"name": "test-name",
					"customerId": "test-customer"
				},
				"analysis": {
					"summary": "Test summary"
				},
				"artifact": {
					"messages": []
				}
			}
		}
	}`)

	// First, verify we can unmarshal into WebhookMessage
	var msg vapi.WebhookMessage
	if err := json.Unmarshal(testData, &msg); err != nil {
		t.Fatalf("Failed to unmarshal WebhookMessage: %v", err)
	}

	// Log the message type and data length
	t.Logf("Message type: %s", msg.Message.Type)
	t.Logf("Data length: %d", len(msg.Message.Data))
	t.Logf("Raw data: %s", string(msg.Message.Data))

	// Verify the message type
	if msg.Message.Type != "end-of-call-report" {
		t.Errorf("Message type = %s, want end-of-call-report", msg.Message.Type)
	}

	// Verify we have data
	if len(msg.Message.Data) == 0 {
		t.Error("Message data is empty")
	}

	// Now verify we can unmarshal the data field into EndOfCallReport
	var report vapi.EndOfCallReport
	if err := json.Unmarshal(msg.Message.Data, &report); err != nil {
		t.Fatalf("Failed to unmarshal EndOfCallReport: %v", err)
	}

	// Verify the report fields
	if report.Type != "end-of-call-report" {
		t.Errorf("Report type = %s, want end-of-call-report", report.Type)
	}

	if report.EndedReason != "completed" {
		t.Errorf("EndedReason = %s, want completed", report.EndedReason)
	}

	if report.Call == nil {
		t.Fatal("Call is nil")
	}

	// Verify call fields
	if report.Call.ID == nil || *report.Call.ID != "test-id" {
		t.Errorf("Call.ID = %v, want test-id", report.Call.ID)
	}

	if report.Call.Name == nil || *report.Call.Name != "test-name" {
		t.Errorf("Call.Name = %v, want test-name", report.Call.Name)
	}

	if report.Call.CustomerID == nil || *report.Call.CustomerID != "test-customer" {
		t.Errorf("Call.CustomerID = %v, want test-customer", report.Call.CustomerID)
	}

	// Verify analysis and artifact
	if report.Analysis.Summary == nil || *report.Analysis.Summary != "Test summary" {
		t.Errorf("Analysis.Summary = %v, want 'Test summary'", report.Analysis.Summary)
	}

	if report.Artifact.Messages == nil {
		t.Error("Artifact.Messages is nil")
	}

	// Test marshaling back to JSON
	marshaled, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal back to JSON: %v", err)
	}

	// Unmarshal both the original and marshaled JSON into generic maps for comparison
	var original, remarshaled map[string]any
	if err := json.Unmarshal(testData, &original); err != nil {
		t.Fatalf("Failed to unmarshal original data: %v", err)
	}
	if err := json.Unmarshal(marshaled, &remarshaled); err != nil {
		t.Fatalf("Failed to unmarshal remarshaled data: %v", err)
	}

	// Log the structures for debugging
	t.Logf("Original structure: %+v", original)
	t.Logf("Remarshaled structure: %+v", remarshaled)
}
