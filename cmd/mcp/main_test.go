package main

import (
	"context"
	"encoding/json"
	"log"
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

func TestListUsers(t *testing.T) {
	ctx := context.Background()

	// Test direct ListUsers function
	t.Run("Direct ListUsers call", func(t *testing.T) {
		fs, err := createFirestoreClient(ctx)
		if err != nil {
			t.Fatalf("Failed to create Firestore client: %v", err)
		}
		defer fs.Close()

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
