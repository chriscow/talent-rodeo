package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chriscow/minds"
	"github.com/mark3labs/mcp-go/mcp"
	"os"
	"talent-rodeo/internal"
)

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

func listGeoIDsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Setup logging and result
	result := map[string]any{
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
	geoIDs, err := internal.ListGeoIDs(ctx, fs)
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
