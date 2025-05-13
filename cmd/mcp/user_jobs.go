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

type ListUserJobsParams struct {
	UserID string `json:"user_id" description:"ID of the user whose jobs to list"`
}

// NewListUserJobsTool creates a new tool for listing user jobs
func NewListUserJobsTool() mcp.Tool {
	schema, err := minds.GenerateSchema(ListUserJobsParams{})
	if err != nil {
		panic("Could not generate schema from ListUserJobsParams")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("firestore_list_user_jobs",
		"Lists all jobs for a specific user by their user ID. Returns array of job objects with details like status, company name, and timestamps.",
		schemaJSON,
	)
}

func listUserJobsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	logMsg("Processing list user jobs request...")
	data, err := getJSONData(request.Params.Arguments)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Invalid parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Received raw parameters: %s", string(data)))

	var params struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(data, &params); err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to parse parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Parsed parameter - UserID: %s", params.UserID))

	if params.UserID == "" {
		result["status"] = "error"
		logMsg("Error: user_id is required")
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

	logMsg(fmt.Sprintf("Fetching jobs for user ID: %s", params.UserID))
	jobs, err := internal.ListUserJobs(ctx, fs, params.UserID)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to get user jobs: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Successfully retrieved %d jobs", len(jobs)))

	result["status"] = "success"
	result["jobs"] = jobs

	return createResult(result)
}
