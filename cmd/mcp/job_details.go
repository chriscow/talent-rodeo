package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search"

	firestorepb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"github.com/chriscow/minds"
	"github.com/mark3labs/mcp-go/mcp"
)

type ListJobDetailsParams struct {
	Limit  int  `json:"limit" description:"Maximum number of job details to return"`
	Offset int  `json:"offset" description:"Number of job details to skip"`
	Count  bool `json:"count" description:"If true, only return the count of job details"`
}

type GetJobDetailParams struct {
	ID string `json:"id" description:"ID of the job detail to retrieve"`
}

// NewListJobDetailsTool creates a new tool for listing job details
func NewListJobDetailsTool() mcp.Tool {
	schema, err := minds.GenerateSchema(ListJobDetailsParams{})
	if err != nil {
		panic("Could not generate schema from ListJobDetailsParams")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("firestore_list_job_details",
		"Lists or counts job details from Firestore. Returns array of job details with pagination support, or just the count if count parameter is true.",
		schemaJSON,
	)
}

// NewGetJobDetailTool creates a new tool for getting a specific job detail
func NewGetJobDetailTool() mcp.Tool {
	schema, err := minds.GenerateSchema(GetJobDetailParams{})
	if err != nil {
		panic("Could not generate schema from GetJobDetailParams")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("firestore_get_job_detail",
		"Gets details for a specific job from Firestore by its ID.",
		schemaJSON,
	)
}

func listJobDetailsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	logMsg("Processing list job details request...")
	data, err := getJSONData(request.Params.Arguments)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Invalid parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Received raw parameters: %s", string(data)))

	var params ListJobDetailsParams
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
		// Use aggregation query for efficient counting
		query := fs.Collection("job_details")
		aggregationQuery := query.NewAggregationQuery().WithCount("count")
		results, err := aggregationQuery.Get(ctx)
		if err != nil {
			result["status"] = "error"
			logMsg(fmt.Sprintf("Failed to get job details count: %v", err))
			return createResult(result)
		}

		// Extract count from results
		countResult, ok := results["count"]
		if !ok {
			result["status"] = "error"
			logMsg("Count not found in aggregation results")
			return createResult(result)
		}

		// Convert to integer value
		countValue := countResult.(*firestorepb.Value)
		count := countValue.GetIntegerValue()

		logMsg(fmt.Sprintf("Successfully retrieved job details count: %d", count))
		result["status"] = "success"
		result["count"] = count
	} else {
		// Get paginated job details
		logMsg("Fetching job details from Firestore...")
		details, err := internal.ListJobDetails(ctx, fs, params.Limit, params.Offset)
		if err != nil {
			result["status"] = "error"
			logMsg(fmt.Sprintf("Failed to list job details: %v", err))
			return createResult(result)
		}
		logMsg(fmt.Sprintf("Successfully retrieved %d job details", len(details)))

		result["status"] = "success"
		result["job_details"] = details
	}

	return createResult(result)
}

func getJobDetailHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	logMsg("Processing get job detail request...")
	data, err := getJSONData(request.Params.Arguments)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Invalid parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Received raw parameters: %s", string(data)))

	var params GetJobDetailParams
	if err := json.Unmarshal(data, &params); err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to parse parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Parsed parameters - ID: %s", params.ID))

	if params.ID == "" {
		result["status"] = "error"
		logMsg("Job ID is required")
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

	// Get the job detail document
	doc, err := fs.Collection("job_details").Doc(params.ID).Get(ctx)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to get job detail: %v", err))
		return createResult(result)
	}

	var jobDetail search.JobDetail
	if err := doc.DataTo(&jobDetail); err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to parse job detail data: %v", err))
		return createResult(result)
	}

	logMsg("Successfully retrieved job detail")
	result["status"] = "success"
	result["job_detail"] = jobDetail

	return createResult(result)
}
