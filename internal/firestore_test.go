package internal

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Overload("../.env"); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
}

func TestNewFirestoreClient(t *testing.T) {
	// Save original env vars to restore later
	originalProjectID := os.Getenv("FIREBASE_PROJECT_ID")
	originalCredentials := os.Getenv("FIREBASE_CREDENTIALS")
	originalGoogleCreds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	defer func() {
		os.Setenv("FIREBASE_PROJECT_ID", originalProjectID)
		os.Setenv("FIREBASE_CREDENTIALS", originalCredentials)
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", originalGoogleCreds)
	}()

	// Clear all env vars initially
	os.Unsetenv("FIREBASE_PROJECT_ID")
	os.Unsetenv("FIREBASE_CREDENTIALS")
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")

	tests := []struct {
		name        string
		projectID   string
		credentials string
		wantErr     bool
		errContains string
	}{
		{
			name:        "missing project ID",
			projectID:   "",
			credentials: "",
			wantErr:     true,
			errContains: "FIREBASE_PROJECT_ID must be set",
		},
		{
			name:        "project ID with invalid credentials",
			projectID:   "test-project",
			credentials: "invalid json",
			wantErr:     true,
			errContains: "invalid character 'i' looking for beginning of value",
		},
		{
			name:        "project ID without explicit credentials",
			projectID:   "test-project",
			credentials: "",
			wantErr:     true,
			errContains: "could not find default credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset environment for each test
			os.Unsetenv("FIREBASE_PROJECT_ID")
			os.Unsetenv("FIREBASE_CREDENTIALS")
			os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")

			// Set test-specific values
			if tt.projectID != "" {
				os.Setenv("FIREBASE_PROJECT_ID", tt.projectID)
			}
			if tt.credentials != "" {
				os.Setenv("FIREBASE_CREDENTIALS", tt.credentials)
			}

			// Create context
			ctx := context.Background()

			// Call the function
			client, err := NewFirestoreClient(ctx)

			// Check error conditions
			if tt.wantErr {
				if err == nil {
					t.Error("NewFirestoreClient() error = nil, want error")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("NewFirestoreClient() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			// Check successful case
			if err != nil {
				t.Errorf("NewFirestoreClient() unexpected error = %v", err)
				return
			}
			if client == nil {
				t.Error("NewFirestoreClient() client is nil, want non-nil")
				return
			}
			defer client.Close()
		})
	}
}

// TestNewFirestoreClient_Integration tests the actual connection to Firestore
// This test is skipped by default and only runs when integration tests are enabled
func TestNewFirestoreClient_Integration(t *testing.T) {
	// Check if we have valid credentials
	if os.Getenv("FIREBASE_PROJECT_ID") == "" {
		t.Fatalf("FIREBASE_PROJECT_ID not set")
	}

	creds := os.Getenv("FIREBASE_CREDENTIALS")
	if creds == "" {
		t.Fatalf("FIREBASE_CREDENTIALS not set")
	}

	// take the credentials json and try to parse it
	var credentials map[string]interface{}
	err := json.Unmarshal([]byte(creds), &credentials)
	if err != nil {
		t.Fatalf("FIREBASE_CREDENTIALS is not valid json: %v", err)
	}

	ctx := context.Background()
	client, err := NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("NewFirestoreClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewFirestoreClient() client is nil")
	}
	defer client.Close()

	// Try a simple operation to verify the connection
	_, err = client.Collection("test").Doc("test").Get(ctx)
	if err == nil {
		t.Error("Expected error for non-existent document, got nil")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}
