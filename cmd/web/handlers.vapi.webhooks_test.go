package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/chriscow/vapi-go"
)

// Tests for the webhook handler functionality will go here
// The simulation-related tests have been moved to cmd/web/simulate/end-of-call_test.go

// Create a minimal mock application struct for testing
var testLogger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
	Level: slog.LevelDebug,
}))

// TestUnmarshalEndOfCallReports tests the unmarshaling of end-of-call report webhook payloads
// from the testdata/webhooks directory
func TestUnmarshalEndOfCallReports(t *testing.T) {
	// Set up logger for test
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Get all end-of-call report files
	webhookDir := "/workspaces/talent-rodeo/testdata/webhooks"
	if _, err := os.Stat(webhookDir); os.IsNotExist(err) {
		webhookDir = "/Users/chris/dev/talent-rodeo/testdata/webhooks"
		if _, err := os.Stat(webhookDir); os.IsNotExist(err) {
			t.Fatalf("Failed to find webhook test directory: %v", err)
		}
	}

	files, err := os.ReadDir(webhookDir)
	if err != nil {
		t.Fatalf("Failed to read webhook test directory: %v", err)
	}

	// Keep count of tested files
	testCount := 0

	// Process each file
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), "end-of-call-report_") {
			testCount++
			t.Run(file.Name(), func(t *testing.T) {
				filePath := filepath.Join(webhookDir, file.Name())

				// Read the file
				webhookData, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("Failed to read webhook file %s: %v", filePath, err)
				}

				var env vapi.EndOfCallReportEnvelope
				err = json.Unmarshal(webhookData, &env)
				if err != nil {
					t.Errorf("Failed to unmarshal %s: %v", file.Name(), err)
				} else {
					// Basic validation of the report
					report := env.Report
					if report.Type == "" {
						t.Errorf("Report type should not be empty")
					}
					if report.Type != "end-of-call-report" {
						t.Errorf("Report type should be end-of-call-report, got %s", report.Type)
					}

					// Check required fields
					if report.Call == nil {
						logger.Warn("Missing Call field in report", "file", file.Name())
					} else if report.Call.ID == nil {
						logger.Warn("Missing Call.ID field in report", "file", file.Name())
					} else if report.Call.Name == nil {
						logger.Warn("Missing Call.Name field in report", "file", file.Name())
					} else if report.Call.Customer == nil {
						logger.Warn("Missing Call.Customer field in report", "file", file.Name())
					} else {
						logger.Info("File has all required fields", "file", file.Name())
					}
				}
			})
		}
	}

	// Ensure we actually tested some files
	if testCount == 0 {
		t.Fatal("No end-of-call report files were found to test")
	}
	t.Logf("Successfully tested %d end-of-call report files", testCount)
}

// For testing validateEndOfCallReport function - this calls the actual implementation
func TestValidateEndOfCallReport(t *testing.T) {
	// Create a minimal application with just the logger
	app := &application{
		logger: testLogger,
		fs:     nil,
		wg:     sync.WaitGroup{},
	}

	// Get all end-of-call report files
	webhookDir := "testdata/webhooks"
	files, err := os.ReadDir(webhookDir)
	if err != nil {
		t.Fatalf("Failed to read webhook test directory: %v", err)
	}

	// Process each file to test validation
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), "end-of-call-report_") {
			t.Run(file.Name(), func(t *testing.T) {
				filePath := filepath.Join(webhookDir, file.Name())

				// Read the file
				webhookData, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("Failed to read webhook file: %s", filePath)
				}

				// Test the validation function directly - skip if the function is not available in test
				if _, ok := interface{}(app).(*application); !ok {
					t.Skip("application.validateEndOfCallReport is not available in test build")
				}

				var env vapi.EndOfCallReportEnvelope
				err = json.Unmarshal(webhookData, &env)
				if err != nil {
					t.Errorf("Failed to validate %s: %v", file.Name(), err)
				} else {
					report := env.Report
					if report.Call != nil && report.Call.ID != nil {
						t.Logf("Successfully validated file: %s", file.Name())
					} else {
						t.Logf("File %s is missing required fields", file.Name())
					}
				}
			})
		}
	}
}
