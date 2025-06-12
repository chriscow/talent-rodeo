package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"talent-rodeo/internal"
	"testing"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/mcp"
)

func init() {
	if err := godotenv.Overload("../../.env"); err != nil {
		log.Fatal("No .env file found, using system env")
	}
}

func TestMain(m *testing.M) {
	// Start emulator
	if err := internal.StartEmulator(); err != nil {
		log.Fatalf("Failed to start emulator: %v", err)
	}

	// Ensure emulator is stopped after tests
	defer internal.StopEmulator()

	// Run tests
	code := m.Run()

	os.Exit(code)
}

func TestListUsers(t *testing.T) {
	ctx := context.Background()

	// Test direct ListUsers function
	t.Run("ListUsers call", func(t *testing.T) {
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			t.Fatalf("Failed to create Firestore client: %v", err)
		}
		defer fs.Close()

		// CreateTestUser(t *testing.T, client *internal.FirestoreClient, name string, email string, phone string, isSuperUser bool) *internal.User
		internal.CreateTestUser(t, fs, "test", "test@example.com", "1234567890", true)

		users, err := internal.ListUsers(ctx, fs, 0, 0)
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}

		t.Logf("Direct call found %d users", len(users))
		for i, user := range users {
			t.Logf("User %d: ID=%s, Email=%s", i, user.ID, user.Email)
		}

		if len(users) == 0 {
			t.Error("Expected to find users, but got none")
		}
	})

	// Test through handler
	t.Run("Handler ListUsers call", func(t *testing.T) {
		params := map[string]interface{}{
			"limit":  10,
			"offset": 0,
		}

		/*
				type CallToolRequest struct {
			Request
			Params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments,omitempty"`
				Meta      *struct {
					// If specified, the caller is requesting out-of-band progress
					// notifications for this request (as represented by
					// notifications/progress). The value of this parameter is an
					// opaque token that will be attached to any subsequent
					// notifications. The receiver is not obligated to provide these
					// notifications.
					ProgressToken ProgressToken `json:"progressToken,omitempty"`
				} `json:"_meta,omitempty"`
			} `json:"params"`
				}
		*/

		request := mcp.CallToolRequest{
			Params: struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments,omitempty"`
				Meta      *struct {
					ProgressToken mcp.ProgressToken `json:"progressToken,omitempty"`
				} `json:"_meta,omitempty"`
			}{
				Arguments: params,
			},
		}

		result, err := listUsersHandler(ctx, request)
		if err != nil {
			t.Fatalf("Handler failed: %v", err)
		}

		if len(result.Content) == 0 {
			t.Fatal("No content in result")
		}

		var response map[string]interface{}
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		t.Logf("Raw response: %s", result.Content[0].(mcp.TextContent).Text)

		if response["status"] != "success" {
			t.Errorf("Expected status success, got %v", response["status"])
		}

		if users, ok := response["users"].([]interface{}); ok {
			t.Logf("Handler call found %d users", len(users))
			for i, user := range users {
				if u, ok := user.(map[string]interface{}); ok {
					t.Logf("User %d: ID=%v, Email=%v", i, u["id"], u["email"])
				}
			}
			if len(users) == 0 {
				t.Error("Expected to find users through handler, but got none")
			}
		} else {
			t.Error("No users array in response")
		}
	})
}

func TestListUserJobs(t *testing.T) {
	ctx := context.Background()

	// First, get a user ID to use for testing
	t.Run("Get test user ID", func(t *testing.T) {
		fs, err := createFirestoreClient(ctx)
		if err != nil {
			t.Fatalf("Failed to create Firestore client: %v", err)
		}
		defer fs.Close()

		users, err := internal.ListUsers(ctx, fs, 1, 0)
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}

		if len(users) == 0 {
			t.Skip("No users found, skipping test")
		}

		testUserID := users[0].ID
		t.Logf("Using test user ID: %s", testUserID)

		// Test direct GetUserJobs function
		jobs, err := internal.GetUserJobs(ctx, fs, testUserID)
		if err != nil {
			t.Fatalf("ListUserJobs failed: %v", err)
		}

		t.Logf("Direct call found %d jobs for user", len(jobs))
		for i, job := range jobs {
			if i < 5 { // Only log first 5 jobs to avoid excessive output
				t.Logf("Job %d: ID=%s, Status=%s", i, job.ID, job.Status)
			}
		}

		// Test through handler
		params := map[string]interface{}{
			"user_id": testUserID,
		}

		request := mcp.CallToolRequest{
			Params: struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments,omitempty"`
				Meta      *struct {
					ProgressToken mcp.ProgressToken `json:"progressToken,omitempty"`
				} `json:"_meta,omitempty"`
			}{
				Arguments: params,
			},
		}

		result, err := listUserJobsHandler(ctx, request)
		if err != nil {
			t.Fatalf("Handler failed: %v", err)
		}

		if len(result.Content) == 0 {
			t.Fatal("No content in result")
		}

		var response map[string]interface{}
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		t.Logf("Raw response: %s", result.Content[0].(mcp.TextContent).Text)

		if response["status"] != "success" {
			t.Errorf("Expected status success, got %v", response["status"])
		}

		if jobs, ok := response["jobs"].([]interface{}); ok {
			t.Logf("Handler call found %d jobs", len(jobs))
			for i, job := range jobs {
				if i < 5 && job != nil { // Only log first 5 jobs
					if j, ok := job.(map[string]interface{}); ok {
						t.Logf("Job %d: ID=%v, Status=%v, Company=%v", i, j["ID"], j["Status"], j["CompanyName"])
					}
				}
			}
		} else {
			t.Log("No jobs array in response or user has no jobs")
		}
	})
}
