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

type ListSearchCriteriaParams struct {
	Limit  int  `json:"limit" description:"Maximum number of search criteria to return"`
	Offset int  `json:"offset" description:"Number of search criteria to skip"`
	Count  bool `json:"count" description:"If true, only return the count of search criteria"`
}

type GetSearchCriteriaParams struct {
	ID string `json:"id" description:"ID of the search criteria to retrieve"`
}

type GetSearchCriteriaNeedsSearchParams struct {
	RandomString string `json:"random_string" description:"Dummy parameter for no-parameter tools"`
}

// NewListSearchCriteriaTool creates a new tool for listing search criteria
func NewListSearchCriteriaTool() mcp.Tool {
	schema, err := minds.GenerateSchema(ListSearchCriteriaParams{})
	if err != nil {
		panic("Could not generate schema from ListSearchCriteriaParams")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("firestore_list_search_criteria",
		"Lists or counts search criteria from Firestore. Returns array of search criteria with pagination support, or just the count if count parameter is true.",
		schemaJSON,
	)
}

// NewGetSearchCriteriaTool creates a new tool for getting a specific search criteria
func NewGetSearchCriteriaTool() mcp.Tool {
	schema, err := minds.GenerateSchema(GetSearchCriteriaParams{})
	if err != nil {
		panic("Could not generate schema from GetSearchCriteriaParams")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("firestore_get_search_criteria",
		"Gets details for a specific search criteria from Firestore by its ID.",
		schemaJSON,
	)
}

// NewGetSearchCriteriaNeedsSearchTool creates a new tool for getting search criteria that need searching
func NewGetSearchCriteriaNeedsSearchTool() mcp.Tool {
	schema, err := minds.GenerateSchema(GetSearchCriteriaNeedsSearchParams{})
	if err != nil {
		panic("Could not generate schema from GetSearchCriteriaNeedsSearchParams")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("firestore_get_search_criteria_needs_search",
		"Gets all search criteria from Firestore that either have never been searched (LastSearchedAt is nil) or haven't been searched in the last 24 hours.",
		schemaJSON,
	)
}

func listSearchCriteriaHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	logMsg("Processing list search criteria request...")
	data, err := getJSONData(request.Params.Arguments)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Invalid parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Received raw parameters: %s", string(data)))

	var params ListSearchCriteriaParams
	if err := json.Unmarshal(data, &params); err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to parse parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Parsed parameters - Limit: %d, Offset: %d, Count: %v", params.Limit, params.Offset, params.Count))

	logMsg("Creating Firestore client...")
	fs, err := createFirestoreClient(ctx)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to create Firestore client: %v", err))
		return createResult(result)
	}
	defer fs.Close()
	logMsg("Firestore client created successfully")

	if params.Count {
		// Get all criteria to count them
		criteria, err := internal.ListSearchCriteria(fs)
		if err != nil {
			result["status"] = "error"
			logMsg(fmt.Sprintf("Failed to get search criteria count: %v", err))
			return createResult(result)
		}

		count := len(criteria)
		logMsg(fmt.Sprintf("Successfully retrieved search criteria count: %d", count))
		result["status"] = "success"
		result["count"] = count
	} else {
		// Get all criteria first (since we don't have a paginated version in internal package)
		allCriteria, err := internal.ListSearchCriteria(fs)
		if err != nil {
			result["status"] = "error"
			logMsg(fmt.Sprintf("Failed to list search criteria: %v", err))
			return createResult(result)
		}

		// Apply pagination manually
		start := params.Offset
		end := min(start+params.Limit, len(allCriteria))
		if start > len(allCriteria) {
			start = len(allCriteria)
		}

		paginatedCriteria := allCriteria[start:end]
		logMsg(fmt.Sprintf("Successfully retrieved %d search criteria", len(paginatedCriteria)))

		result["status"] = "success"
		result["search_criteria"] = paginatedCriteria
	}

	return createResult(result)
}

func getSearchCriteriaHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	logMsg("Processing get search criteria request...")
	data, err := getJSONData(request.Params.Arguments)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Invalid parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Received raw parameters: %s", string(data)))

	var params GetSearchCriteriaParams
	if err := json.Unmarshal(data, &params); err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to parse parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Parsed parameters - ID: %s", params.ID))

	if params.ID == "" {
		result["status"] = "error"
		logMsg("Search criteria ID is required")
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

	// Get the search criteria
	criteria, exists, err := internal.GetSearchCriteria(fs, params.ID)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to get search criteria: %v", err))
		return createResult(result)
	}

	if !exists {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Search criteria with ID %s not found", params.ID))
		return createResult(result)
	}

	logMsg("Successfully retrieved search criteria")
	result["status"] = "success"
	result["search_criteria"] = criteria

	return createResult(result)
}

func getSearchCriteriaNeedsSearchHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	logMsg("Processing get search criteria needs search request...")

	logMsg("Creating Firestore client...")
	fs, err := createFirestoreClient(ctx)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to create Firestore client: %v", err))
		return createResult(result)
	}
	defer fs.Close()
	logMsg("Firestore client created successfully")

	// Get the search criteria that need searching
	criteria, err := internal.GetSearchCriteriaNeedsSearch(fs)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to get search criteria that need searching: %v", err))
		return createResult(result)
	}

	logMsg(fmt.Sprintf("Successfully retrieved %d search criteria that need searching", len(criteria)))
	result["status"] = "success"
	result["search_criteria"] = criteria

	return createResult(result)
}
