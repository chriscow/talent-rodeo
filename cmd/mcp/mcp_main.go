package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Create MCP server
	s := server.NewMCPServer("talent-rodeo-mcp", "1.0.0",
		server.WithLogging(),
		server.WithToolCapabilities(true),
		server.WithInstructions(`
Provides access to Firestore data in the project.

Available operations:
- List users with pagination
- Delete users by ID, email, or prefix
- List all GeoIDs
- List all jobs for a specific user
- List or count job details
- Get details for a specific job by ID
- Search job details using semantic search with Pinecone
- List or count search criteria
- Get details for specific search criteria by ID
- Get search criteria that need searching (not searched in 24h)
- Test Firestore connection
- List appointments for a user (with optional type filtering)
- Get chat messages for a specific appointment (with optional checklist details)

The data is read directly from Firestore and reflects the current state of the database.`),
	)

	// Register tools
	s.AddTool(NewListUsersTool(), listUsersHandler)
	s.AddTool(NewDeleteUserTool(), deleteUserHandler)
	s.AddTool(NewListGeoIDsTool(), listGeoIDsHandler)
	s.AddTool(NewListUserJobsTool(), listUserJobsHandler)
	s.AddTool(NewListJobDetailsTool(), listJobDetailsHandler)
	s.AddTool(NewGetJobDetailTool(), getJobDetailHandler)
	s.AddTool(NewSearchJobDetailsTool(), searchJobDetailsHandler)
	s.AddTool(NewListSearchCriteriaTool(), listSearchCriteriaHandler)
	s.AddTool(NewGetSearchCriteriaTool(), getSearchCriteriaHandler)
	s.AddTool(NewGetSearchCriteriaNeedsSearchTool(), getSearchCriteriaNeedsSearchHandler)
	s.AddTool(NewTestFirestoreConnectionTool(), testFirestoreConnectionHandler)
	// Appointment tools
	s.AddTool(NewListAppointmentsTool(), listAppointmentsHandler)
	s.AddTool(NewGetAppointmentChatTool(), getAppointmentChatHandler)

	// Start server
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// Helper function to create the final result
func createResult(result map[string]any) (*mcp.CallToolResult, error) {
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
	// Setup logging
	fmt.Fprintf(os.Stderr, "DEBUG: Processing JSON data extraction...\n")

	if args == nil {
		fmt.Fprintf(os.Stderr, "DEBUG: Received nil argument\n")
		return nil, fmt.Errorf("nil argument")
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Argument type: %T\n", args)

	// Handle string input (already JSON encoded)
	if str, ok := args.(string); ok {
		fmt.Fprintf(os.Stderr, "DEBUG: Processing string input of length: %d\n", len(str))
		return []byte(str), nil
	}

	// Handle []byte input (already JSON encoded)
	if bytes, ok := args.([]byte); ok {
		fmt.Fprintf(os.Stderr, "DEBUG: Processing []byte input of length: %d\n", len(bytes))
		return bytes, nil
	}

	// For any other type, marshal to JSON
	fmt.Fprintf(os.Stderr, "DEBUG: Marshaling input to JSON\n")
	result, err := json.Marshal(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG: Failed to marshal to JSON: %v\n", err)
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "DEBUG: Successfully marshaled to JSON of length: %d\n", len(result))
	return result, nil
}
