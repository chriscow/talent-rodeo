package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"talent-rodeo/internal"

	firebase "firebase.google.com/go"
	"github.com/chriscow/minds"
	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/api/option"
)

// createFirestoreClient creates a Firestore client using Firebase Admin SDK.
// DO NOT USE internal/firestore.go - this is the canonical way to create the client.
func createFirestoreClient(ctx context.Context) (*internal.FirestoreClient, error) {
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID == "" {
		return nil, fmt.Errorf("FIREBASE_PROJECT_ID environment variable is not set")
	}

	config := &firebase.Config{
		ProjectID: projectID,
	}

	opt := option.WithCredentialsFile(os.Getenv("FIREBASE_CREDENTIALS_FILE"))
	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to create Firebase app: %w", err)
	}

	fs, err := app.Firestore(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Firestore client: %w", err)
	}

	return &internal.FirestoreClient{Client: fs}, nil
}

// NewTestFirestoreConnectionTool creates a new tool for testing Firestore connection
func NewTestFirestoreConnectionTool() mcp.Tool {
	// No parameters needed for this tool
	type input struct {
		RandomString string `json:"random_string" description:"Dummy parameter for no-parameter tools"`
	}
	schema, err := minds.GenerateSchema(input{})
	if err != nil {
		panic("Could not generate schema from input")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("firestore_test_connection",
		"Tests Firestore connection and authentication. Verifies environment variables, credentials file, and attempts a test query. Returns detailed diagnostic information.",
		schemaJSON,
	)
}

func testFirestoreConnectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result := map[string]any{
		"status":  "checking",
		"details": make([]string, 0),
	}

	// Log function for both stderr and result details
	logMsg := func(msg string) {
		fmt.Fprintf(os.Stderr, "DEBUG: %s\n", msg)
		result["details"] = append(result["details"].([]string), msg)
	}

	// Check environment variables
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	credentialsPath := os.Getenv("FIREBASE_CREDENTIALS_FILE")

	if projectID == "" {
		result["status"] = "error"
		logMsg("FIREBASE_PROJECT_ID environment variable is not set")
	} else {
		logMsg(fmt.Sprintf("FIREBASE_PROJECT_ID is set to: %s", projectID))
	}

	if credentialsPath == "" {
		result["status"] = "error"
		logMsg("FIREBASE_CREDENTIALS_FILE environment variable is not set")
	} else {
		logMsg(fmt.Sprintf("FIREBASE_CREDENTIALS_FILE is set to: %s", credentialsPath))

		// Check if credentials file exists and try to read it
		if _, err := os.Stat(credentialsPath); os.IsNotExist(err) {
			result["status"] = "error"
			logMsg(fmt.Sprintf("Credentials file does not exist at path: %s", credentialsPath))
		} else {
			logMsg("Credentials file exists")

			// Try to read the credentials file
			credBytes, err := os.ReadFile(credentialsPath)
			if err != nil {
				result["status"] = "error"
				logMsg(fmt.Sprintf("Failed to read credentials file: %v", err))
			} else {
				logMsg(fmt.Sprintf("Successfully read %d bytes from credentials file", len(credBytes)))

				// Validate JSON format
				var credJSON map[string]any
				if err := json.Unmarshal(credBytes, &credJSON); err != nil {
					result["status"] = "error"
					logMsg(fmt.Sprintf("Failed to parse credentials JSON: %v", err))
				} else {
					logMsg("Successfully parsed credentials JSON")
				}
			}
		}
	}

	// Only proceed with Firestore connection if we have basic requirements
	if result["status"] != "error" {
		logMsg("Attempting to create Firestore client...")
		fs, err := createFirestoreClient(ctx)
		if err != nil {
			result["status"] = "error"
			logMsg(fmt.Sprintf("Failed to create Firestore client: %v", err))
			return createResult(result)
		}
		defer fs.Close()
		logMsg("Firestore client created successfully")

		// Try to list one user to verify read access
		logMsg("Attempting to verify read access by listing users collection...")

		// Try a simple query to test the connection
		usersRef := fs.Collection("users")
		if usersRef == nil {
			result["status"] = "error"
			logMsg("Failed to get users collection reference")
			return createResult(result)
		}

		logMsg("Got users collection reference, attempting to get documents...")
		docs, err := usersRef.Limit(1).Documents(ctx).GetAll()
		if err != nil {
			result["status"] = "error"
			logMsg(fmt.Sprintf("Failed to query users: %v", err))
		} else {
			result["status"] = "success"
			logMsg(fmt.Sprintf("Successfully queried users collection. Found %d document(s)", len(docs)))
		}
	}

	return createResult(result)
}
