package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"talent-rodeo/internal"

	"github.com/chriscow/minds"
	"github.com/mark3labs/mcp-go/mcp"
)

// type UserInfo struct {
// 	ID    string  `json:"id"`
// 	Name  string  `json:"name"`
// 	Phone *string `json:"phone,omitempty"`
// 	Email string  `json:"email"`
// }

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

func listUsersHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	logMsg("Processing list users request...")
	data, err := getJSONData(request.Params.Arguments)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Invalid parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Received raw parameters: %s", string(data)))

	var params struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	if err := json.Unmarshal(data, &params); err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to parse parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Parsed parameters - Limit: %d, Offset: %d", params.Limit, params.Offset))

	if params.Limit == 0 {
		params.Limit = 100
		logMsg(fmt.Sprintf("No limit specified, defaulting to %d", params.Limit))
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

	logMsg("Fetching users from Firestore...")
	users, err := internal.ListUsers(ctx, fs, params.Limit, params.Offset)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to list users: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Successfully retrieved %d users", len(users)))

	result["status"] = "success"
	result["users"] = users

	return createResult(result)
}

func deleteUserHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	logMsg("Processing delete user request...")
	data, err := getJSONData(request.Params.Arguments)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Invalid parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Received raw parameters: %s", string(data)))

	var params struct {
		ID     string `json:"id"`
		Email  string `json:"email"`
		Prefix string `json:"prefix"`
	}
	if err := json.Unmarshal(data, &params); err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to parse parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Parsed parameters - ID: %s, Email: %s, Prefix: %s", params.ID, params.Email, params.Prefix))

	if params.ID == "" && params.Email == "" && params.Prefix == "" {
		result["status"] = "error"
		logMsg("Error: must specify either id, email, or prefix")
		return createResult(result)
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

	var deleteResult interface{}

	if params.Prefix != "" {
		logMsg(fmt.Sprintf("Deleting users with prefix: %s", params.Prefix))
		logMsg("Fetching all users to filter by prefix...")
		users, err := internal.ListUsers(ctx, fs, 0, 0)
		if err != nil {
			result["status"] = "error"
			logMsg(fmt.Sprintf("Failed to list users: %v", err))
			return createResult(result)
		}
		logMsg(fmt.Sprintf("Found %d total users to check against prefix", len(users)))

		deleted := 0
		for _, user := range users {
			if len(user.ID) >= len(params.Prefix) && user.ID[:len(params.Prefix)] == params.Prefix {
				logMsg(fmt.Sprintf("Deleting user with ID: %s", user.ID))
				if err := internal.DeleteUser(ctx, fs, user.ID); err != nil {
					result["status"] = "error"
					logMsg(fmt.Sprintf("Failed to delete user %s: %v", user.ID, err))
					return createResult(result)
				}
				deleted++
				logMsg(fmt.Sprintf("Successfully deleted user: %s", user.ID))
			}
		}
		logMsg(fmt.Sprintf("Completed prefix deletion. Total users deleted: %d", deleted))
		deleteResult = map[string]int{"deleted": deleted}
	} else {
		if params.Email != "" {
			logMsg(fmt.Sprintf("Looking up user by email: %s", params.Email))
			user, found, err := internal.GetUserByEmail(ctx, fs, params.Email)
			if err != nil {
				result["status"] = "error"
				logMsg(fmt.Sprintf("Failed to get user by email: %v", err))
				return createResult(result)
			}
			if !found {
				result["status"] = "error"
				logMsg(fmt.Sprintf("User with email %s not found", params.Email))
				return createResult(result)
			}
			params.ID = user.ID
			logMsg(fmt.Sprintf("Found user ID: %s for email: %s", params.ID, params.Email))
		}

		logMsg(fmt.Sprintf("Deleting user with ID: %s", params.ID))
		if err := internal.DeleteUser(ctx, fs, params.ID); err != nil {
			result["status"] = "error"
			logMsg(fmt.Sprintf("Failed to delete user: %v", err))
			return createResult(result)
		}
		logMsg(fmt.Sprintf("Successfully deleted user with ID: %s", params.ID))
		deleteResult = map[string]string{"deleted": params.ID}
	}

	result["status"] = "success"
	result["result"] = deleteResult

	return createResult(result)
}
