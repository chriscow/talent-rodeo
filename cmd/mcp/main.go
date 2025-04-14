package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"talent-rodeo/internal"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/chriscow/minds"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/api/option"
)

// createFirestoreClient creates a Firestore client using Firebase Admin SDK.
// DO NOT USE internal/firestore.go - this is the canonical way to create the client.
func createFirestoreClient(ctx context.Context) (*firestore.Client, error) {
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID == "" {
		return nil, fmt.Errorf("FIREBASE_PROJECT_ID environment variable is not set")
	}

	config := &firebase.Config{
		ProjectID: projectID,
	}

	opt := option.WithCredentialsFile(os.Getenv("FIREBASE_CREDENTIALS"))
	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to create Firebase app: %w", err)
	}

	fs, err := app.Firestore(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Firestore client: %w", err)
	}

	return fs, nil
}

type UserInfo struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Phone *string `json:"phone,omitempty"`
	Email string  `json:"email"`
}

type ListUsersParams struct {
	Limit  int `json:"limit" description:"Maximum number of users to return"`
	Offset int `json:"offset" description:"Number of users to skip"`
}

type DeleteUserParams struct {
	ID     string `json:"id" description:"ID of the user to delete"`
	Email  string `json:"email" description:"Email of the user to delete"`
	Prefix string `json:"prefix" description:"Prefix to match user IDs for deletion"`
}

// NewListUsersTool creates a new tool for listing users
func NewListUsersTool() mcp.Tool {
	schema, err := minds.GenerateSchema(ListUsersParams{})
	if err != nil {
		panic("Could not generate schema from ListUsersParams")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("firestore_list_users",
		"Lists Firestore users with pagination. Returns array of users with their IDs, names, emails, and phone numbers. Use limit/offset for pagination.",
		schemaJSON,
	)
}

// NewDeleteUserTool creates a new tool for deleting users
func NewDeleteUserTool() mcp.Tool {
	schema, err := minds.GenerateSchema(DeleteUserParams{})
	if err != nil {
		panic("Could not generate schema from DeleteUserParams")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("firestore_delete_user",
		"Deletes one or more users from Firestore. Can delete by ID, email, or prefix (to delete multiple users with IDs starting with prefix).",
		schemaJSON,
	)
}

// NewListGeoIDsTool creates a new tool for listing GeoIDs
func NewListGeoIDsTool() mcp.Tool {
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

	return mcp.NewToolWithRawSchema("firestore_list_geoids",
		"Lists all geographic location IDs (GeoIDs) from Firestore. Returns array of location identifiers used for user regions/locations.",
		schemaJSON,
	)
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

func listUsersHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	data, err := getJSONData(request.Params.Arguments)
	if err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	var params struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	if err := json.Unmarshal(data, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters format: %w", err)
	}

	if params.Limit == 0 {
		params.Limit = 100
	}

	fs, err := createFirestoreClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer fs.Close()

	users, err := internal.ListUsers(ctx, fs, params.Limit, params.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	userInfos := make([]UserInfo, len(users))
	for i, user := range users {
		userInfos[i] = UserInfo{
			ID:    user.ID,
			Name:  user.Name,
			Phone: user.Phone,
			Email: user.Email,
		}
	}

	result, err := json.Marshal(userInfos)
	if err != nil {
		return nil, fmt.Errorf("error marshaling result: %w", err)
	}

	content := mcp.NewTextContent(string(result))
	return &mcp.CallToolResult{
		Content: []mcp.Content{content},
	}, nil
}

func deleteUserHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	data, err := getJSONData(request.Params.Arguments)
	if err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	var params struct {
		ID     string `json:"id"`
		Email  string `json:"email"`
		Prefix string `json:"prefix"`
	}
	if err := json.Unmarshal(data, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters format: %w", err)
	}

	if params.ID == "" && params.Email == "" && params.Prefix == "" {
		return nil, fmt.Errorf("must specify either id, email, or prefix")
	}

	fs, err := createFirestoreClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer fs.Close()

	var result interface{}

	if params.Prefix != "" {
		users, err := internal.ListUsers(ctx, fs, 0, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to list users: %w", err)
		}

		deleted := 0
		for _, user := range users {
			if len(user.ID) >= len(params.Prefix) && user.ID[:len(params.Prefix)] == params.Prefix {
				if err := internal.DeleteUser(ctx, fs, user.ID); err != nil {
					return nil, fmt.Errorf("failed to delete user %s: %w", user.ID, err)
				}
				deleted++
			}
		}
		result = map[string]int{"deleted": deleted}
	} else {
		if params.Email != "" {
			user, found, err := internal.GetUserByEmail(ctx, fs, params.Email)
			if err != nil {
				return nil, fmt.Errorf("failed to get user by email: %w", err)
			}
			if !found {
				return nil, fmt.Errorf("user with email %s not found", params.Email)
			}
			params.ID = user.ID
		}

		if err := internal.DeleteUser(ctx, fs, params.ID); err != nil {
			return nil, fmt.Errorf("failed to delete user: %w", err)
		}
		result = map[string]string{"deleted": params.ID}
	}

	jsonResult, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("error marshaling result: %w", err)
	}

	content := mcp.NewTextContent(string(jsonResult))
	return &mcp.CallToolResult{
		Content: []mcp.Content{content},
	}, nil
}

func listGeoIDsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Setup logging and result
	result := map[string]interface{}{
		"status":  "checking",
		"details": make([]string, 0),
	}

	// Log function for both stderr and result details
	logMsg := func(msg string) {
		fmt.Fprintf(os.Stderr, "DEBUG: %s\n", msg)
		result["details"] = append(result["details"].([]string), msg)
	}

	logMsg("Creating Firestore client...")
	fs, err := createFirestoreClient(ctx)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to create Firestore client: %v", err))
		return createResult(result)
	}
	defer fs.Close()
	logMsg("Firestore client created successfully")

	logMsg("Attempting to list GeoIDs...")
	geoIDs, err := internal.ListGeoIDs(fs)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to list GeoIDs: %v", err))
		return createResult(result)
	}

	logMsg(fmt.Sprintf("Successfully retrieved %d GeoIDs", len(geoIDs)))
	result["status"] = "success"
	result["geoIDs"] = geoIDs

	return createResult(result)
}

func testFirestoreConnectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result := map[string]interface{}{
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
	credentialsPath := os.Getenv("FIREBASE_CREDENTIALS")

	if projectID == "" {
		result["status"] = "error"
		logMsg("FIREBASE_PROJECT_ID environment variable is not set")
	} else {
		logMsg(fmt.Sprintf("FIREBASE_PROJECT_ID is set to: %s", projectID))
	}

	if credentialsPath == "" {
		result["status"] = "error"
		logMsg("FIREBASE_CREDENTIALS environment variable is not set")
	} else {
		logMsg(fmt.Sprintf("FIREBASE_CREDENTIALS is set to: %s", credentialsPath))

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
				var credJSON map[string]interface{}
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

// Helper function to create the final result
func createResult(result map[string]interface{}) (*mcp.CallToolResult, error) {
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("error marshaling result: %w", err)
	}

	content := mcp.NewTextContent(string(jsonResult))
	return &mcp.CallToolResult{
		Content: []mcp.Content{content},
	}, nil
}

// getJSONData extracts JSON data from an argument value
func getJSONData(args interface{}) ([]byte, error) {
	if args == nil {
		return nil, fmt.Errorf("nil argument")
	}

	// If args is a map, extract the first value
	if argsMap, ok := args.(map[string]interface{}); ok {
		if len(argsMap) == 0 {
			return nil, fmt.Errorf("empty arguments map")
		}
		// Get the first value from the map
		for _, v := range argsMap {
			args = v
			break
		}
	}

	// Handle string input (already JSON encoded)
	if str, ok := args.(string); ok {
		return []byte(str), nil
	}

	// Handle []byte input (already JSON encoded)
	if bytes, ok := args.([]byte); ok {
		return bytes, nil
	}

	// For any other type, marshal to JSON
	return json.Marshal(args)
}

func main() {
	// Create MCP server
	s := server.NewMCPServer("talent-rodeo-firestore", "1.0.0",
		server.WithLogging(),
		server.WithToolCapabilities(true),
		server.WithInstructions(`
Provides access to Firestore data in the "talent-rodeo-dev" project, including users and GeoIDs.

Available operations:
- List users with pagination
- Delete users by ID, email, or prefix
- List all GeoIDs
- Test Firestore connection

The data is read directly from Firestore and reflects the current state of the database.`),
	)

	// Register tools
	s.AddTool(NewListUsersTool(), listUsersHandler)
	s.AddTool(NewDeleteUserTool(), deleteUserHandler)
	s.AddTool(NewListGeoIDsTool(), listGeoIDsHandler)
	s.AddTool(NewTestFirestoreConnectionTool(), testFirestoreConnectionHandler)

	// Start server
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// func main() {
// 	s := server.NewMCPServer("sequential-thinking", "1.0.0",
// 		server.WithLogging(),
// 		server.WithToolCapabilities(true),
// 		server.WithInstructions(`Sequential thinking server for dynamic and reflective problem-solving.
// This server helps analyze problems through a flexible thinking process that can adapt and evolve.
// Each thought can build on, question, or revise previous insights as understanding deepens.`),
// 	)

// 	tm := sequentialthinking.NewThoughtManager()
// 	tool := sequentialthinking.NewSequentialThinkingTool()
// 	s.AddTool(tool, tm.ProcessThoughtHandler)

// 	if err := server.ServeStdio(s); err != nil {
// 		panic(err)
// 	}
// }
