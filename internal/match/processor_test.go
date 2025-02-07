package match

import (
	"context"
	"testing"
	"time"

	"talent-rodeo/internal/search"

	"github.com/google/uuid"
)

// mockProvider implements the JobProvider interface for testing
type mockProvider struct {
	name string
}

func (m *mockProvider) SearchJobs(ctx context.Context, criteria search.Criteria) ([]search.Job, error) {
	return []search.Job{
		{
			ID:            "job1",
			Title:         "Software Engineer",
			Company:       "Test Company",
			Location:      "Remote",
			PostedDate:    time.Now(),
			Description:   "Test job description",
			URL:           "https://example.com/jobs/1",
			Provider:      m.name,
			ProviderJobID: "test-job-1",
		},
	}, nil
}

func (m *mockProvider) GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {
	return &search.JobDetail{
		Job: search.Job{
			ID:            jobID,
			Title:         "Software Engineer",
			Company:       "Test Company",
			Location:      "Remote",
			PostedDate:    time.Now(),
			Description:   "Test job description",
			URL:           "https://example.com/jobs/1",
			Provider:      m.name,
			ProviderJobID: jobID,
		},
		FullDescription: "Full test job description",
		Requirements:    []string{"Go", "SQL"},
		Qualifications:  []string{"Bachelor's degree"},
		Benefits:        []string{"Health insurance"},
		Skills:          []string{"Go", "PostgreSQL"},
		EmploymentType:  "Full-time",
		WorkplaceType:   "Remote",
		Industry:        "Technology",
	}, nil
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Validate(ctx context.Context) error {
	return nil
}

func TestNewProcessor(t *testing.T) {
	provider := &mockProvider{name: "test-provider"}
	got := NewProcessor(provider)
	if got == nil {
		t.Fatal("NewProcessor() returned nil")
	}
}

func TestProcess(t *testing.T) {
	tests := []struct {
		name    string
		req     ProcessRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: ProcessRequest{
				ID:       uuid.New(),
				Type:     ProcessTypeSearch,
				Provider: "test-provider",
				Priority: PriorityNormal,
				Params: map[string]interface{}{
					"keywords": []string{"golang"},
					"location": "Remote",
				},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			req: ProcessRequest{
				Type:     ProcessTypeSearch,
				Provider: "test-provider",
			},
			wantErr: true,
		},
		{
			name: "missing type",
			req: ProcessRequest{
				ID:       uuid.New(),
				Provider: "test-provider",
			},
			wantErr: true,
		},
		{
			name: "missing provider",
			req: ProcessRequest{
				ID:   uuid.New(),
				Type: ProcessTypeSearch,
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			req: ProcessRequest{
				ID:       uuid.New(),
				Type:     "invalid",
				Provider: "test-provider",
			},
			wantErr: true,
		},
		{
			name: "past deadline",
			req: ProcessRequest{
				ID:       uuid.New(),
				Type:     ProcessTypeSearch,
				Provider: "test-provider",
				Deadline: &time.Time{}, // Zero time is in the past
			},
			wantErr: true,
		},
		{
			name: "provider not found",
			req: ProcessRequest{
				ID:       uuid.New(),
				Type:     ProcessTypeSearch,
				Provider: "non-existent-provider",
				Params: map[string]interface{}{
					"keywords": []string{"golang"},
				},
			},
			wantErr: true,
		},
		{
			name: "missing keywords",
			req: ProcessRequest{
				ID:       uuid.New(),
				Type:     ProcessTypeSearch,
				Provider: "test-provider",
				Params:   map[string]interface{}{},
			},
			wantErr: true,
		},
	}

	provider := &mockProvider{name: "test-provider"}
	p := NewProcessor(provider)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Process(context.Background(), tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got == nil {
				t.Fatal("Process() returned nil response")
			}

			if got.RequestID != tt.req.ID {
				t.Errorf("Process() RequestID = %v, want %v", got.RequestID, tt.req.ID)
			}
			if got.StartTime.IsZero() {
				t.Error("Process() StartTime is zero")
			}
			if got.EndTime.IsZero() {
				t.Error("Process() EndTime is zero")
			}
			if !got.EndTime.After(got.StartTime) {
				t.Error("Process() EndTime is not after StartTime")
			}

			if tt.req.Type == ProcessTypeSearch {
				result, ok := got.Result.(map[string]interface{})
				if !ok {
					t.Fatal("Process() result is not a map")
				}

				jobs, ok := result["jobs"].([]search.Job)
				if !ok {
					t.Fatal("Process() jobs is not a []types.Job")
				}
				if len(jobs) == 0 {
					t.Error("Process() jobs is empty")
				}

				total, ok := result["total"].(int)
				if !ok {
					t.Fatal("Process() total is not an int")
				}
				if want := 1; total != want {
					t.Errorf("Process() total = %v, want %v", total, want)
				}
			}
		})
	}
}

func TestStatus(t *testing.T) {
	provider := &mockProvider{name: "test-provider"}
	p := NewProcessor(provider)
	ctx := context.Background()

	// Test non-existent job
	nonExistentID := uuid.New()
	got, err := p.Status(ctx, nonExistentID)
	if err == nil {
		t.Error("Status() error = nil, want error for non-existent job")
	}
	if got != nil {
		t.Error("Status() got non-nil status for non-existent job")
	}

	// Create a job and test its status
	req := ProcessRequest{
		ID:       uuid.New(),
		Type:     ProcessTypeSearch,
		Provider: "test-provider",
		Params: map[string]interface{}{
			"keywords": []string{"golang"},
		},
	}

	resp, err := p.Process(ctx, req)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if resp == nil {
		t.Fatal("Process() returned nil response")
	}
	if got := resp.Status.State; got != StateCompleted {
		t.Errorf("Process() state = %v, want %v", got, StateCompleted)
	}

	got, err = p.Status(ctx, req.ID)
	if err != nil {
		t.Errorf("Status() error = %v", err)
	}
	if got == nil {
		t.Fatal("Status() returned nil status")
	}
	if got.State != StateCompleted {
		t.Errorf("Status() state = %v, want %v", got.State, StateCompleted)
	}
}

func TestCancel(t *testing.T) {
	provider := &mockProvider{name: "test-provider"}
	p := NewProcessor(provider)
	ctx := context.Background()

	// Test cancelling non-existent job
	nonExistentID := uuid.New()
	if err := p.Cancel(ctx, nonExistentID); err == nil {
		t.Error("Cancel() error = nil, want error for non-existent job")
	}

	// Create a job and test cancellation
	req := ProcessRequest{
		ID:       uuid.New(),
		Type:     ProcessTypeSearch,
		Provider: "test-provider",
		Params: map[string]interface{}{
			"keywords": []string{"golang"},
		},
	}

	// Process will complete immediately in our test implementation
	_, err := p.Process(ctx, req)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Try to cancel completed job
	if err := p.Cancel(ctx, req.ID); err == nil {
		t.Error("Cancel() error = nil, want error for completed job")
	}
}

func TestConcurrentProcessing(t *testing.T) {
	provider := &mockProvider{name: "test-provider"}
	p := NewProcessor(provider)
	ctx := context.Background()
	numJobs := 10

	// Create channels for results
	type result struct {
		resp *ProcessResponse
		err  error
	}
	results := make(chan result, numJobs)

	// Start concurrent jobs
	for i := 0; i < numJobs; i++ {
		go func() {
			req := ProcessRequest{
				ID:       uuid.New(),
				Type:     ProcessTypeSearch,
				Provider: "test-provider",
				Params: map[string]interface{}{
					"keywords": []string{"golang"},
				},
			}
			resp, err := p.Process(ctx, req)
			results <- result{resp, err}
		}()
	}

	// Collect results
	for i := 0; i < numJobs; i++ {
		r := <-results
		if r.err != nil {
			t.Errorf("Process() error = %v", r.err)
		}
		if r.resp == nil {
			t.Error("Process() returned nil response")
		}
	}
}

func TestProcessorStateTransitions(t *testing.T) {
	provider := &mockProvider{name: "test-provider"}
	p := NewProcessor(provider)
	ctx := context.Background()
	req := ProcessRequest{
		ID:       uuid.New(),
		Type:     ProcessTypeSearch,
		Provider: "test-provider",
		Params: map[string]interface{}{
			"keywords": []string{"golang"},
		},
	}

	// Start processing
	got, err := p.Process(ctx, req)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Verify final state
	want := StateCompleted
	if got.Status.State != want {
		t.Errorf("Process() state = %v, want %v", got.Status.State, want)
	}

	wantProgress := float64(100)
	if got.Status.Progress != wantProgress {
		t.Errorf("Process() progress = %v, want %v", got.Status.Progress, wantProgress)
	}

	if got.Status.LastUpdated.IsZero() {
		t.Error("Process() LastUpdated is zero")
	}
}

func TestAnalyze(t *testing.T) {
	provider := &mockProvider{name: "test-provider"}
	p := NewProcessor(provider)
	ctx := context.Background()

	// Test cases
	tests := []struct {
		name    string
		req     ProcessRequest
		wantErr bool
	}{
		{
			name: "valid_analysis_request",
			req: ProcessRequest{
				ID:       uuid.New(),
				Type:     ProcessTypeAnalyze,
				Provider: "test-provider",
				Params: map[string]interface{}{
					"job_description": "Senior Golang Developer with 5+ years experience in AWS and Kubernetes",
				},
			},
			wantErr: false,
		},
		{
			name: "missing_job_description",
			req: ProcessRequest{
				ID:       uuid.New(),
				Type:     ProcessTypeAnalyze,
				Provider: "test-provider",
				Params:   map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name: "empty_job_description",
			req: ProcessRequest{
				ID:       uuid.New(),
				Type:     ProcessTypeAnalyze,
				Provider: "test-provider",
				Params: map[string]interface{}{
					"job_description": "",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Process(ctx, tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got == nil {
				t.Fatal("Process() returned nil response")
			}

			if got.RequestID != tt.req.ID {
				t.Errorf("Process() RequestID = %v, want %v", got.RequestID, tt.req.ID)
			}

			if got.Status.State != StateCompleted {
				t.Errorf("Process() state = %v, want %v", got.Status.State, StateCompleted)
			}

			if got.StartTime.IsZero() {
				t.Error("Process() StartTime is zero")
			}
			if got.EndTime.IsZero() {
				t.Error("Process() EndTime is zero")
			}
			if !got.EndTime.After(got.StartTime) {
				t.Error("Process() EndTime is not after StartTime")
			}

			// Verify analysis result structure
			result, ok := got.Result.(map[string]interface{})
			if !ok {
				t.Fatal("Process() result is not a map")
			}

			requiredFields := []string{"keywords", "requirements", "validation"}
			for _, field := range requiredFields {
				if result[field] == nil {
					t.Errorf("Process() missing required field: %s", field)
				}
			}
		})
	}
}

func TestMatch(t *testing.T) {
	t.Skip("Job matching functionality not yet implemented - pending Phase 1 implementation")
	// Test implementation to be added when the feature is implemented
}

func TestValidate(t *testing.T) {
	t.Skip("Job validation functionality not yet implemented - pending Phase 1 implementation")
	// Test implementation to be added when the feature is implemented
}

// Future Phase 5 tests to add:
// - Test message queue integration
// - Test distributed cancellation
// - Test retry mechanisms
// - Test dead letter queue
// - Test metrics collection
// - Test circuit breaker behavior
// - Test resource allocation
// - Test node coordination
