package processor

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"talent-rodeo/internal/jobmatch/types"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
)

// processor implements the JobProcessor interface.
// This is the Phase 1 implementation that will evolve into
// a distributed system in Phase 5.
type processor struct {
	mu        sync.RWMutex
	jobs      map[uuid.UUID]*ProcessStatus
	providers map[string]types.JobProvider
	// Phase 5 additions:
	// - messageQueue *watermill.Router
	// - deadLetterQueue *watermill.Topic
	// - metrics *prometheus.Registry
}

// NewProcessor creates a new job processor.
// In Phase 5, this will initialize NATS/Watermill and other dependencies.
func NewProcessor(providers ...types.JobProvider) JobProcessor {
	p := &processor{
		jobs:      make(map[uuid.UUID]*ProcessStatus),
		providers: make(map[string]types.JobProvider),
	}

	// Register providers
	for _, provider := range providers {
		p.providers[provider.Name()] = provider
	}

	return p
}

// Process handles a job processing request.
// In Phase 1, this is a synchronous operation. In Phase 5, this will
// queue the job for async processing.
func (p *processor) Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
	// Debug logging for request details
	log.Printf("Processing request: ID=%s, Type=%s, Provider=%s, Priority=%d",
		req.ID, req.Type, req.Provider, req.Priority)

	// Validate request
	if err := p.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Initialize status
	status := &ProcessStatus{
		State:       StateProcessing,
		Progress:    0.0,
		LastUpdated: time.Now(),
	}

	// Store status
	p.mu.Lock()
	p.jobs[req.ID] = status
	p.mu.Unlock()

	// Phase 5: This will be replaced with message queue publishing
	result, err := p.processSync(ctx, req)

	// Update final status
	p.mu.Lock()
	if err != nil {
		status.State = StateFailed
		status.Error = err
	} else {
		status.State = StateCompleted
		status.Progress = 100.0
	}
	status.LastUpdated = time.Now()
	p.mu.Unlock()

	return &ProcessResponse{
		RequestID: req.ID,
		Status:    *status,
		Result:    result,
		Error:     err,
		StartTime: status.LastUpdated,
		EndTime:   time.Now(),
	}, err
}

// Status retrieves the current status of a job.
func (p *processor) Status(ctx context.Context, id uuid.UUID) (*ProcessStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status, exists := p.jobs[id]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", id)
	}

	// Return a copy to prevent external modification
	statusCopy := *status
	return &statusCopy, nil
}

// Cancel attempts to cancel a running job.
// In Phase 1, this is basic. In Phase 5, this will handle distributed cancellation.
func (p *processor) Cancel(ctx context.Context, id uuid.UUID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	status, exists := p.jobs[id]
	if !exists {
		return fmt.Errorf("job not found: %s", id)
	}

	if status.State != StateProcessing {
		return fmt.Errorf("job is not processing: %s", id)
	}

	status.State = StateCancelled
	status.LastUpdated = time.Now()

	// Phase 5: This will publish a cancellation message to all nodes

	return nil
}

// validateRequest performs basic request validation.
func (p *processor) validateRequest(req ProcessRequest) error {
	if req.ID == uuid.Nil {
		return fmt.Errorf("request ID is required")
	}
	if req.Type == "" {
		return fmt.Errorf("process type is required")
	}
	if req.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	return nil
}

// processSync handles synchronous processing for Phase 1.
// We will add a processAsync function with async message handling in Phase 5.
func (p *processor) processSync(ctx context.Context, req ProcessRequest) (interface{}, error) {
	// Debug logging for processing stages
	log.Printf("Starting sync processing for request ID=%s", req.ID)

	// Check for deadline
	if req.Deadline != nil && time.Now().After(*req.Deadline) {
		return nil, fmt.Errorf("deadline exceeded")
	}

	// Process based on type
	switch req.Type {
	case ProcessTypeSearch:
		return p.handleSearch(ctx, req)
	case ProcessTypeAnalyze:
		return p.handleAnalyze(ctx, req)
	case ProcessTypeMatch:
		return p.handleMatch(ctx, req)
	case ProcessTypeValidate:
		return p.handleValidate(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported process type: %s", req.Type)
	}
}

// Handler functions - These will be moved to separate files in Phase 5
// for better organization and scalability

func (p *processor) handleSearch(ctx context.Context, req ProcessRequest) (interface{}, error) {
	log.Printf("Handling search request: ID=%s, Provider=%s", req.ID, req.Provider)

	// Get the provider
	provider, exists := p.providers[req.Provider]
	if !exists {
		return nil, fmt.Errorf("provider not found: %s", req.Provider)
	}

	// Extract search criteria from request params
	criteria, err := extractSearchCriteria(req.Params)
	if err != nil {
		return nil, fmt.Errorf("invalid search criteria: %w", err)
	}

	// Perform the search
	log.Printf("Executing search with criteria: %+v", criteria)
	jobs, err := provider.SearchJobs(ctx, criteria)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Build response
	result := map[string]interface{}{
		"jobs":  jobs,
		"total": len(jobs),
		"page":  criteria.PageNumber,
	}

	log.Printf("Search completed successfully: found %d jobs", len(jobs))
	return result, nil
}

// extractSearchCriteria converts request parameters to SearchCriteria
func extractSearchCriteria(params map[string]interface{}) (types.SearchCriteria, error) {
	criteria := types.SearchCriteria{
		PageSize:   10, // Default page size
		PageNumber: 1,  // Default page number
	}

	// Extract keywords
	if keywords, ok := params["keywords"].([]string); ok {
		criteria.Keywords = keywords
	} else if keyword, ok := params["keywords"].(string); ok {
		criteria.Keywords = []string{keyword}
	}

	// Extract location
	if location, ok := params["location"].(string); ok {
		criteria.Location = location
	}

	// Extract job types
	if jobTypes, ok := params["job_types"].([]string); ok {
		criteria.JobType = jobTypes
	}

	// Extract experience levels
	if exp, ok := params["experience"].([]string); ok {
		criteria.Experience = exp
	}

	// Extract industry
	if industries, ok := params["industries"].([]string); ok {
		criteria.Industry = industries
	}

	// Extract skills
	if skills, ok := params["skills"].([]string); ok {
		criteria.Skills = skills
	}

	// Extract pagination
	if pageSize, ok := params["page_size"].(int); ok && pageSize > 0 {
		criteria.PageSize = pageSize
	}
	if pageNum, ok := params["page"].(int); ok && pageNum > 0 {
		criteria.PageNumber = pageNum
	}

	// Validate required fields
	if len(criteria.Keywords) == 0 {
		return criteria, fmt.Errorf("at least one keyword is required")
	}

	return criteria, nil
}

func (p *processor) handleAnalyze(ctx context.Context, req ProcessRequest) (interface{}, error) {
	log.Printf("Handling analyze request: ID=%s", req.ID)

	// Extract job description from params
	jobDesc, ok := req.Params["job_description"].(string)
	if !ok || jobDesc == "" {
		return nil, fmt.Errorf("job_description is required and must be a non-empty string")
	}

	// Create LLM analyzer with debug logging
	logger := log.New(os.Stdout, fmt.Sprintf("[ANALYZE-%s] ", req.ID), log.LstdFlags|log.Lmicroseconds)
	analyzer, err := NewLLMAnalyzer(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create analyzer: %w", err)
	}

	// Extract keywords
	logger.Printf("Extracting keywords from job description")
	keywords, err := analyzer.ExtractKeywords(ctx, jobDesc)
	if err != nil {
		return nil, fmt.Errorf("failed to extract keywords: %w", err)
	}
	logger.Printf("Extracted %d keywords", len(keywords))

	// Parse requirements
	logger.Printf("Parsing requirements from job description")
	requirements, err := analyzer.ParseRequirements(ctx, jobDesc)
	if err != nil {
		return nil, fmt.Errorf("failed to parse requirements: %w", err)
	}
	logger.Printf("Parsed %d requirements", len(requirements))

	// Create a job struct for validation with safe type assertions
	job := &types.JobDetail{
		FullDescription: jobDesc,
	}

	// Safely extract optional fields
	if title, ok := req.Params["title"].(string); ok {
		job.Title = title
	}
	if company, ok := req.Params["company"].(string); ok {
		job.Company = company
	}
	if location, ok := req.Params["location"].(string); ok {
		job.Location = location
	}
	if url, ok := req.Params["url"].(string); ok {
		job.URL = url
	}

	// Validate job data
	logger.Printf("Validating job data")
	validation, err := analyzer.ValidateJobData(ctx, job)
	if err != nil {
		logger.Printf("Job validation failed: %v", err)
		// Create a validation result with failure info
		validation = &ValidationResult{
			Valid:  false,
			Score:  0.0,
			Errors: []string{err.Error()},
		}
	} else if validation == nil {
		// Handle nil validation result
		validation = &ValidationResult{
			Valid:  false,
			Score:  0.0,
			Errors: []string{"validation result is nil"},
		}
	}
	logger.Printf("Job validation completed with score: %f", validation.Score)

	// Return combined analysis results
	return map[string]interface{}{
		"keywords":     keywords,
		"requirements": requirements,
		"validation":   validation,
	}, nil
}

func (p *processor) handleMatch(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
	// Extract job and candidate data from request params
	var job types.Job
	if err := mapstructure.Decode(req.Params["job"], &job); err != nil {
		return nil, fmt.Errorf("failed to decode job data: %w", err)
	}

	var candidate types.Candidate
	if err := mapstructure.Decode(req.Params["candidate"], &candidate); err != nil {
		return nil, fmt.Errorf("failed to decode candidate data: %w", err)
	}

	// Create matcher with debug logging
	logger := log.New(os.Stdout, fmt.Sprintf("[MATCH-%s] ", req.ID), log.LstdFlags|log.Lmicroseconds)
	matcher, err := NewMatcher(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create matcher: %w", err)
	}

	// Compute match
	details, err := matcher.Match(ctx, &job, &candidate)
	if err != nil {
		return nil, fmt.Errorf("failed to compute match: %w", err)
	}

	// Return match results in ProcessResponse
	return &ProcessResponse{
		RequestID: req.ID,
		Status: ProcessStatus{
			State:       StateCompleted,
			Progress:    100.0,
			LastUpdated: time.Now(),
		},
		Result: map[string]interface{}{
			"match_score":    details.Score.Overall,
			"skills_score":   details.Score.Skills,
			"location_score": details.Score.Location,
			"matched_skills": details.MatchedSkills,
			"missing_skills": details.MissingSkills,
			"analysis":       details.Analysis,
			"confidence":     details.Score.Confidence,
		},
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}, nil
}

func (p *processor) handleValidate(ctx context.Context, req ProcessRequest) (interface{}, error) {
	log.Printf("Handling validate request: ID=%s", req.ID)
	// Phase 1: Basic implementation
	// Phase 5: Will include comprehensive validation and the following enhancements:
	// - Schema validation
	// - Data quality checks
	// - Cross-reference validation
	// - Historical data comparison
	// - Enhanced error reporting

	return nil, fmt.Errorf("validate: not implemented")
}
