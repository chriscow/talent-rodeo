package processor

import (
	"context"
	"errors"
	"log"
	"os"
	"testing"

	"talent-rodeo/internal/jobmatch/types"

	"github.com/chriscow/minds"
	"github.com/joho/godotenv"
)

func init() {
	godotenv.Load("/Users/chris/dev/talent-rodeo/.env")
}

// mockLLMProvider implements the LLM provider interface for testing
type mockLLMProvider struct {
	normalizeResponse    string
	requirementsResponse string
	shouldError          bool
}

func (m *mockLLMProvider) GenerateContent(ctx context.Context, req minds.Request) (minds.Response, error) {
	if m.shouldError {
		return nil, errors.New("mock error")
	}

	// Return different responses based on the prompt
	if req.Messages[1].Content == normalizePrompt+"\n\nJob Description:\n"+testJobDescription {
		return &mockResponse{content: m.normalizeResponse}, nil
	}
	return &mockResponse{content: m.requirementsResponse}, nil
}

func (m *mockLLMProvider) ModelName() string {
	return "mock"
}

func (m *mockLLMProvider) Close() {
	return
}

type mockResponse struct {
	content string
}

func (m *mockResponse) String() string {
	return m.content
}

func (m *mockResponse) ToolCalls() []minds.ToolCall {
	return nil
}

const testJobDescription = `We are seeking a Senior Golang Developer to join our dynamic team! 
The ideal candidate will have 5+ years of experience with Go and strong knowledge of AWS.
Experience with Kubernetes is a plus. Must be comfortable working in an agile environment.`

func setupTestLogger(t *testing.T) *log.Logger {
	return log.New(os.Stdout, t.Name()+": ", log.LstdFlags|log.Lshortfile)
}

func TestLLMAnalyzer_ExtractKeywords(t *testing.T) {
	// Create mock responses
	normalizeResp := `{
		"core_description": "Senior Golang Developer role requiring strong backend development skills",
		"key_differentiators": ["cloud native", "microservices"],
		"work_environment": "Agile development environment"
	}`

	requirementsResp := `{
		"must_have": [
			{
				"skill": "Go",
				"weight": 0.9,
				"alternatives": ["Golang"]
			},
			{
				"skill": "AWS",
				"weight": 0.8,
				"alternatives": ["cloud platforms", "GCP", "Azure"]
			}
		],
		"nice_to_have": [
			{
				"skill": "Kubernetes",
				"weight": 0.6
			}
		]
	}`

	tests := []struct {
		name         string
		provider     *mockLLMProvider
		wantKeywords int
		wantErr      bool
	}{
		{
			name: "successful_analysis",
			provider: &mockLLMProvider{
				normalizeResponse:    normalizeResp,
				requirementsResponse: requirementsResp,
			},
			wantKeywords: 3, // 2 must-have + 1 nice-to-have
			wantErr:      false,
		},
		{
			name: "llm_error",
			provider: &mockLLMProvider{
				shouldError: true,
			},
			wantKeywords: 0,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := setupTestLogger(t)
			analyzer, err := NewLLMAnalyzer(context.Background(), logger)
			if err != nil {
				t.Fatalf("NewLLMAnalyzer() error = %v", err)
			}

			// Override the provider with our mock
			analyzer.provider = tt.provider

			got, err := analyzer.ExtractKeywords(context.Background(), testJobDescription)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractKeywords() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if len(got) != tt.wantKeywords {
				t.Errorf("ExtractKeywords() got %d keywords, want %d", len(got), tt.wantKeywords)
			}

			// Verify keyword properties
			for _, k := range got {
				if k.Text == "" {
					t.Error("keyword text is empty")
				}
				if k.Category == "" {
					t.Error("keyword category is empty")
				}
				if k.Weight <= 0 {
					t.Errorf("keyword weight = %f, want > 0", k.Weight)
				}
				if k.Confidence <= 0 {
					t.Errorf("keyword confidence = %f, want > 0", k.Confidence)
				}
			}
		})
	}
}

func TestLLMAnalyzer_ParseRequirements(t *testing.T) {
	requirementsResp := `{
		"must_have": [
			{
				"skill": "Go (5+ years)",
				"weight": 0.9,
				"alternatives": ["Golang"]
			},
			{
				"skill": "AWS",
				"weight": 0.8,
				"alternatives": ["cloud platforms"]
			}
		],
		"nice_to_have": [
			{
				"skill": "Kubernetes",
				"weight": 0.6
			}
		]
	}`

	tests := []struct {
		name     string
		provider *mockLLMProvider
		wantReqs int
		wantErr  bool
	}{
		{
			name: "successful_analysis",
			provider: &mockLLMProvider{
				requirementsResponse: requirementsResp,
			},
			wantReqs: 3, // 2 must-have + 1 nice-to-have
			wantErr:  false,
		},
		{
			name: "llm_error",
			provider: &mockLLMProvider{
				shouldError: true,
			},
			wantReqs: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := setupTestLogger(t)
			analyzer, err := NewLLMAnalyzer(context.Background(), logger)
			if err != nil {
				t.Fatalf("NewLLMAnalyzer() error = %v", err)
			}

			// Override the provider with our mock
			analyzer.provider = tt.provider

			got, err := analyzer.ParseRequirements(context.Background(), testJobDescription)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRequirements() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if len(got) != tt.wantReqs {
				t.Errorf("ParseRequirements() got %d requirements, want %d", len(got), tt.wantReqs)
			}

			// Verify requirement properties
			for _, r := range got {
				if r.Text == "" {
					t.Error("requirement text is empty")
				}
				if r.Type == "" {
					t.Error("requirement type is empty")
				}

				// For LLM analyzer, verify years needed for Go requirement
				if r.Text == "Go (5+ years)" && r.YearsNeeded != 5 {
					t.Errorf("got years needed = %d, want 5", r.YearsNeeded)
				}
			}
		})
	}
}

func TestLLMAnalyzer_ValidateJobData(t *testing.T) {
	logger := setupTestLogger(t)
	analyzer, err := NewLLMAnalyzer(context.Background(), logger)
	if err != nil {
		t.Fatalf("NewLLMAnalyzer() error = %v", err)
	}

	// Test with valid job
	validJob := &types.JobDetail{
		Job: types.Job{
			ID:             "123",
			Title:          "Software Engineer",
			Description:    "Valid job description",
			Company:        "Test Company",
			Location:       "Remote",
			URL:            "https://example.com/job/123",
			EmploymentType: "full-time",
			WorkplaceType:  "hybrid",
			Industry:       "Technology",
		},
		FullDescription: "Detailed job description with requirements",
		Requirements:    []string{"Go", "AWS"},
		Skills:          []string{"Go", "AWS", "Docker"},
	}

	got, err := analyzer.ValidateJobData(context.Background(), validJob)
	if err != nil {
		t.Errorf("ValidateJobData() error = %v", err)
	}
	if !got.Valid {
		t.Error("ValidateJobData() got invalid result, want valid")
	}
	if got.Score < 0.5 {
		t.Errorf("ValidateJobData() got score = %f, want >= 0.5", got.Score)
	}

	// Test with invalid job
	invalidJob := &types.JobDetail{
		Job: types.Job{
			// Missing all required fields
		},
	}

	got, err = analyzer.ValidateJobData(context.Background(), invalidJob)
	if err == nil {
		t.Error("ValidateJobData() expected error for invalid job")
	}
	if got != nil {
		t.Error("ValidateJobData() got result for invalid job, want nil")
	}
}

func TestLLMAnalyzer_ExtractKeywords_EmptyText(t *testing.T) {
	analyzer, err := NewLLMAnalyzer(context.Background(), setupTestLogger(t))
	if err != nil {
		t.Fatalf("NewLLMAnalyzer() error = %v", err)
	}

	_, err = analyzer.ExtractKeywords(context.Background(), "")
	if err == nil {
		t.Error("ExtractKeywords() expected error for empty text")
	}
}

func TestLLMAnalyzer_ProviderError(t *testing.T) {
	logger := setupTestLogger(t)
	provider := &mockLLMProvider{shouldError: true}

	analyzer := &LLMAnalyzer{
		provider: provider,
		logger:   logger,
	}

	_, err := analyzer.ExtractKeywords(context.Background(), testJobDescription)
	if err == nil {
		t.Error("ExtractKeywords() expected error when provider fails")
	}
}

func TestValidateJobData_InvalidURL(t *testing.T) {
	analyzer, err := NewLLMAnalyzer(context.Background(), setupTestLogger(t))
	if err != nil {
		t.Fatalf("NewLLMAnalyzer() error = %v", err)
	}

	job := &types.JobDetail{
		Job: types.Job{
			ID:          "123",
			Title:       "Software Engineer",
			Description: "Looking for a talented engineer",
			Company:     "Tech Corp",
			Location:    "Remote",
			URL:         "invalid-url",
		},
	}

	_, err = analyzer.ValidateJobData(context.Background(), job)
	if err == nil {
		t.Error("ValidateJobData() expected error for invalid URL")
	}
}

// Helper function to check if a string slice contains a specific string
func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
