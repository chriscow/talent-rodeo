package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"talent-rodeo/internal"

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

type SearchJobDetailsParams struct {
	Query string `json:"query" description:"The search query to find relevant job details"`
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

// NewSearchJobDetailsTool creates a new tool for searching job details using Pinecone
func NewSearchJobDetailsTool() mcp.Tool {
	schema, err := minds.GenerateSchema(SearchJobDetailsParams{})
	if err != nil {
		panic("Could not generate schema from SearchJobDetailsParams")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("pinecone_jobdetails_similarity_search",
		"Searches for job details using semantic search via Pinecone in the job-details namespace, returning job details with similarity scores.",
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

	var jobDetail internal.JobDetail
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

func searchJobDetailsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	logMsg("Processing search job details request...")
	data, err := getJSONData(request.Params.Arguments)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Invalid parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Received raw parameters: %s", string(data)))

	var params SearchJobDetailsParams
	if err := json.Unmarshal(data, &params); err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to parse parameters: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Parsed parameters - Query: %s", params.Query))

	if params.Query == "" {
		result["status"] = "error"
		logMsg("Search query is required")
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

	// Create Pinecone client
	logMsg("Creating Pinecone client...")
	pineconeClient, err := internal.NewPineconeClient()
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to create Pinecone client: %v", err))
		return createResult(result)
	}
	logMsg("Pinecone client created successfully")

	// Get index host from environment or config
	indexHost := os.Getenv("PINECONE_DENSE_INDEX_HOST")
	if indexHost == "" {
		result["status"] = "error"
		logMsg("PINECONE_DENSE_INDEX_HOST environment variable not set")
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Using Pinecone index host: %s", indexHost))

	// Search records in Pinecone
	logMsg(fmt.Sprintf("Searching records with query: %s", params.Query))
	searchResp, err := pineconeClient.SearchRecords(ctx, indexHost, internal.JobDetailNamespace, params.Query,
		internal.WithTopK(10)) // Default to top 10 results
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to search records: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Found %d hits in Pinecone", len(searchResp.Result.Hits)))

	// Extract job IDs and scores
	var jobIDs []string
	scoreMap := make(map[string]float32)

	for _, hit := range searchResp.Result.Hits {
		// Extract the job ID from hit fields
		if id, ok := hit.Fields["ID"].(string); ok && id != "" {
			jobIDs = append(jobIDs, id)
			scoreMap[id] = hit.Score
		} else if id, ok := hit.Id, true; ok && id != "" {
			// Use the hit ID directly if Fields["ID"] is not available
			jobIDs = append(jobIDs, id)
			scoreMap[id] = hit.Score
		}
	}

	if len(jobIDs) == 0 {
		result["status"] = "success"
		result["job_details"] = []interface{}{}
		logMsg("No matching job details found")
		return createResult(result)
	}

	// Fetch job details for the matching IDs
	jobDetails, err := internal.GetJobDetailsByIDs(ctx, fs, jobIDs)
	if err != nil {
		result["status"] = "error"
		logMsg(fmt.Sprintf("Failed to get job details: %v", err))
		return createResult(result)
	}
	logMsg(fmt.Sprintf("Retrieved %d job details from Firestore", len(jobDetails)))

	// Build search results with scores
	type SearchResult struct {
		JobDetail *internal.JobDetail `json:"job_detail"`
		Score     float32             `json:"score"`
	}

	var searchResults []SearchResult
	for _, job := range jobDetails {
		score, ok := scoreMap[job.ID]
		if !ok {
			score = 0
		}

		searchResults = append(searchResults, SearchResult{
			JobDetail: &job,
			Score:     score,
		})
	}

	// Sort results by score in descending order
	sort.Slice(searchResults, func(i, j int) bool {
		return searchResults[i].Score > searchResults[j].Score
	})

	result["status"] = "success"
	result["job_details"] = searchResults

	return createResult(result)
}
