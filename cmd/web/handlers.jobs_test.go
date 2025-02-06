package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal/jobmatch/processor"
	"talent-rodeo/internal/jobmatch/types"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// mockJobProcessor implements processor.JobProcessor for testing
type mockJobProcessor struct {
	searchResult  map[string]interface{}
	detailResult  *types.JobDetail
	analyzeResult interface{}
	err           error
}

func (m *mockJobProcessor) Process(ctx context.Context, req processor.ProcessRequest) (*processor.ProcessResponse, error) {
	if m.err != nil {
		return nil, m.err
	}

	var result interface{}
	switch req.Type {
	case processor.ProcessTypeSearch:
		result = m.searchResult
	case processor.ProcessTypeAnalyze:
		if jobID, ok := req.Params["job_id"].(string); ok {
			if jobID == "not-found" {
				return nil, fmt.Errorf("job not found: %s", jobID)
			}
		}
		if m.analyzeResult != nil {
			result = m.analyzeResult
		} else {
			result = m.detailResult
		}
	default:
		return nil, fmt.Errorf("unsupported process type: %s", req.Type)
	}

	return &processor.ProcessResponse{
		Result: result,
		Status: processor.ProcessStatus{
			State:    processor.StateCompleted,
			Progress: 100,
		},
	}, nil
}

func (m *mockJobProcessor) Cancel(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return fmt.Errorf("job not found: %s", id)
	}
	return nil
}

func (m *mockJobProcessor) Status(ctx context.Context, id uuid.UUID) (*processor.ProcessStatus, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	return &processor.ProcessStatus{
		State:    processor.StateCompleted,
		Progress: 100,
	}, nil
}

func TestHandleSearchJobs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name           string
		request        SearchJobsRequest
		mockResult     map[string]interface{}
		mockError      error
		expectedStatus int
		expectedJobs   int
	}{
		{
			name: "successful search",
			request: SearchJobsRequest{
				Keywords:   []string{"golang"},
				Location:   "remote",
				PageSize:   10,
				PageNumber: 1,
			},
			mockResult: map[string]interface{}{
				"jobs": []types.Job{
					{
						ID:          "1",
						Title:       "Golang Developer",
						Company:     "TechCo",
						Location:    "Remote",
						PostedDate:  time.Now(),
						Description: "Looking for a Golang developer",
					},
				},
				"total": 1,
			},
			expectedStatus: http.StatusOK,
			expectedJobs:   1,
		},
		{
			name: "invalid page size",
			request: SearchJobsRequest{
				PageSize:   0,
				PageNumber: 1,
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "processor error",
			request: SearchJobsRequest{
				PageSize:   10,
				PageNumber: 1,
			},
			mockError:      fmt.Errorf("processor error"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockJobProcessor{
				searchResult: tt.mockResult,
				err:          tt.mockError,
			}

			handler := NewJobHandler(logger, mock)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/api/v1/jobs/search", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			handler.HandleSearchJobs(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var resp SearchJobsResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if len(resp.Jobs) != tt.expectedJobs {
					t.Errorf("expected %d jobs, got %d", tt.expectedJobs, len(resp.Jobs))
				}
			}
		})
	}
}

func TestHandleGetJobDetails(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name           string
		jobID          string
		mockResult     *types.JobDetail
		mockError      error
		expectedStatus int
	}{
		{
			name:  "successful detail fetch",
			jobID: "123",
			mockResult: &types.JobDetail{
				Job: types.Job{
					ID:      "123",
					Title:   "Senior Developer",
					Company: "TechCo",
				},
				FullDescription: "Full job description",
				Requirements:    []string{"Go", "5 years experience"},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "job not found",
			jobID:          "not-found",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "processor error",
			jobID:          "123",
			mockError:      fmt.Errorf("processor error"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockJobProcessor{
				detailResult: tt.mockResult,
				err:          tt.mockError,
			}

			handler := NewJobHandler(logger, mock)

			req := httptest.NewRequest("GET", "/api/v1/jobs/"+tt.jobID, nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.jobID})
			rec := httptest.NewRecorder()

			handler.HandleGetJobDetails(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var resp types.JobDetail
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if resp.ID != tt.mockResult.ID {
					t.Errorf("expected job ID %s, got %s", tt.mockResult.ID, resp.ID)
				}
			}
		})
	}
}

func TestHandleAnalyzeJob(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name           string
		jobID          string
		mockResult     interface{}
		mockError      error
		expectedStatus int
	}{
		{
			name:  "successful analysis",
			jobID: "123",
			mockResult: map[string]interface{}{
				"skills":        []string{"Go", "Docker"},
				"requirements":  []string{"5 years experience"},
				"culture_fit":   0.85,
				"tech_match":    0.9,
				"overall_score": 0.87,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "job not found",
			jobID:          "not-found",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "processor error",
			jobID:          "123",
			mockError:      fmt.Errorf("processor error"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockJobProcessor{
				analyzeResult: tt.mockResult,
				err:           tt.mockError,
			}

			handler := NewJobHandler(logger, mock)

			req := httptest.NewRequest("POST", "/api/v1/jobs/"+tt.jobID+"/analyze", nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.jobID})
			rec := httptest.NewRecorder()

			handler.HandleAnalyzeJob(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var resp map[string]interface{}
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if _, ok := resp["skills"]; !ok {
					t.Error("expected skills in analysis result")
				}
			}
		})
	}
}
