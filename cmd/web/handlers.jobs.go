package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"talent-rodeo/internal/jobmatch/processor"
	"talent-rodeo/internal/jobmatch/types"
	"talent-rodeo/internal/response"

	"github.com/gorilla/mux"
)

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// JobHandler handles all job-related HTTP endpoints
type JobHandler struct {
	logger    *slog.Logger
	processor processor.JobProcessor
}

// respondWithError writes an error response in JSON format
func respondWithError(w http.ResponseWriter, status int, message string) {
	response.JSON(w, status, ErrorResponse{Error: message})
}

// NewJobHandler creates a new JobHandler instance
func NewJobHandler(logger *slog.Logger, proc processor.JobProcessor) *JobHandler {
	return &JobHandler{
		logger:    logger,
		processor: proc,
	}
}

// SearchJobsRequest represents the request body for job search
type SearchJobsRequest struct {
	Keywords   []string   `json:"keywords,omitempty"`
	Location   string     `json:"location,omitempty"`
	DatePosted *time.Time `json:"date_posted,omitempty"`
	JobType    []string   `json:"job_type,omitempty"`
	Experience []string   `json:"experience,omitempty"`
	Industry   []string   `json:"industry,omitempty"`
	Skills     []string   `json:"skills,omitempty"`
	PageSize   int        `json:"page_size"`
	PageNumber int        `json:"page_number"`
}

// SearchJobsResponse represents the response for job search
type SearchJobsResponse struct {
	Jobs       []types.Job `json:"jobs"`
	TotalCount int         `json:"total_count"`
	PageSize   int         `json:"page_size"`
	PageNumber int         `json:"page_number"`
}

// HandleSearchJobs handles the job search endpoint
// @Summary Search for jobs based on various criteria
// @Description Search for jobs with filtering and pagination
// @Tags jobs
// @Accept json
// @Produce json
// @Param request body SearchJobsRequest true "Search criteria"
// @Success 200 {object} SearchJobsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/jobs/search [post]
func (h *JobHandler) HandleSearchJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h.logger.Info("Handling job search request from", slog.String("remote_addr", r.RemoteAddr))

	var req SearchJobsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode search request", slog.String("error", err.Error()))
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if err := h.validateSearchRequest(&req); err != nil {
		h.logger.Error("Invalid search request", slog.String("error", err.Error()))
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Convert request to processor criteria
	criteria := types.SearchCriteria{
		Keywords:   req.Keywords,
		Location:   req.Location,
		JobType:    req.JobType,
		Experience: req.Experience,
		Industry:   req.Industry,
		Skills:     req.Skills,
		PageSize:   req.PageSize,
		PageNumber: req.PageNumber,
	}
	if req.DatePosted != nil {
		criteria.DatePosted = *req.DatePosted
	}

	h.logger.Info("Searching jobs with criteria", slog.Any("criteria", criteria))

	// Create process request
	processReq := processor.ProcessRequest{
		Type:     processor.ProcessTypeSearch,
		Params:   map[string]interface{}{"criteria": criteria},
		Priority: 1,
	}

	// Execute search
	resp, err := h.processor.Process(ctx, processReq)
	if err != nil {
		h.logger.Error("Failed to search jobs", slog.String("error", err.Error()))
		respondWithError(w, http.StatusInternalServerError, "Failed to search jobs")
		return
	}

	// Extract results
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		h.logger.Error("Invalid response format")
		respondWithError(w, http.StatusInternalServerError, "Invalid response format")
		return
	}

	jobs, ok := result["jobs"].([]types.Job)
	if !ok {
		h.logger.Error("Invalid jobs format in response")
		respondWithError(w, http.StatusInternalServerError, "Invalid jobs format in response")
		return
	}

	total, _ := result["total"].(int)
	h.logger.Info("Found jobs matching criteria", slog.Int("total_count", total))

	// Prepare response
	searchResp := SearchJobsResponse{
		Jobs:       jobs,
		TotalCount: total,
		PageSize:   req.PageSize,
		PageNumber: req.PageNumber,
	}

	response.JSON(w, http.StatusOK, searchResp)
}

// validateSearchRequest validates the search request parameters
func (h *JobHandler) validateSearchRequest(req *SearchJobsRequest) error {
	if req.PageSize < 1 || req.PageSize > 100 {
		return fmt.Errorf("page size must be between 1 and 100")
	}
	if req.PageNumber < 0 {
		return fmt.Errorf("page number must be non-negative")
	}
	return nil
}

// HandleGetJobDetails handles retrieving detailed information about a specific job
// @Summary Get detailed information about a job
// @Description Retrieve comprehensive details about a specific job posting
// @Tags jobs
// @Accept json
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} types.JobDetail
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/jobs/{id} [get]
func (h *JobHandler) HandleGetJobDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	jobID := vars["id"]

	h.logger.Info("Fetching details for job", slog.String("job_id", jobID))

	// Create process request
	processReq := processor.ProcessRequest{
		Type:     processor.ProcessTypeAnalyze,
		Params:   map[string]interface{}{"job_id": jobID},
		Priority: 1,
	}

	resp, err := h.processor.Process(ctx, processReq)
	if err != nil {
		h.logger.Error("Failed to get job details for", slog.String("job_id", jobID), slog.String("error", err.Error()))
		if strings.Contains(err.Error(), "job not found") {
			respondWithError(w, http.StatusNotFound, "Job not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Failed to get job details")
		return
	}

	jobDetail, ok := resp.Result.(*types.JobDetail)
	if !ok {
		h.logger.Error("Invalid job detail format in response")
		respondWithError(w, http.StatusInternalServerError, "Invalid job detail format")
		return
	}

	h.logger.Info("Successfully retrieved details for job", slog.String("job_id", jobID))
	response.JSON(w, http.StatusOK, jobDetail)
}

// HandleAnalyzeJob handles the job analysis endpoint
// @Summary Analyze a job posting
// @Description Perform detailed analysis of a job posting including requirements and skills
// @Tags jobs
// @Accept json
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} types.JobAnalysis
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/jobs/{id}/analyze [post]
func (h *JobHandler) HandleAnalyzeJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	jobID := vars["id"]

	h.logger.Info("Starting analysis for job", slog.String("job_id", jobID))

	// Create process request
	processReq := processor.ProcessRequest{
		Type:     processor.ProcessTypeAnalyze,
		Params:   map[string]interface{}{"job_id": jobID},
		Priority: 1,
	}

	resp, err := h.processor.Process(ctx, processReq)
	if err != nil {
		h.logger.Error("Failed to analyze job", slog.String("job_id", jobID), slog.String("error", err.Error()))
		if strings.Contains(err.Error(), "job not found") {
			respondWithError(w, http.StatusNotFound, "Job not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Failed to analyze job")
		return
	}

	// Since we don't have a JobAnalysis type yet, we'll return the raw analysis result
	h.logger.Info("Successfully completed analysis for job", slog.String("job_id", jobID))
	response.JSON(w, http.StatusOK, resp.Result)
}
