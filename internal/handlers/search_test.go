package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"talent-rodeo/internal/services/search"
	"testing"
	"time"
)

// mockJobProvider implements search.JobProvider for testing
type mockJobProvider struct {
	responses map[string][]search.JobListing // key is JSON string of search.Criteria
}

func (m *mockJobProvider) SearchJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	key, err := json.Marshal(criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal criteria: %w", err)
	}
	jobs, ok := m.responses[string(key)]
	if !ok {
		return nil, nil
	}
	return jobs, nil
}

func (m *mockJobProvider) GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {
	return nil, nil
}

func (m *mockJobProvider) Name() string {
	return "mock"
}

func (m *mockJobProvider) Validate(ctx context.Context) error {
	return nil
}

func TestSearchHandler_HandleMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	provider := &mockJobProvider{
		responses: make(map[string][]search.JobListing),
	}

	// Test search input
	input := search.SearchInput{
		CandidateID:     "123",
		MustHaveSkills:  []string{"Go", "Docker"},
		PreferredSkills: []string{"Kubernetes"},
		Page:            1,
	}

	// Expected job search results
	want := []search.Result{
		{
			Job: search.JobListing{
				Title:       "Senior Go Developer",
				Description: "We need a Go developer with Docker experience",
				Company:     "Tech Corp",
				Location:    "Remote",
			},
			Score:     20, // 10 points each for Go and Docker
			Keywords:  []string{"Go", "Docker"},
			CreatedAt: time.Now().UTC(),
		},
		{
			Job: search.JobListing{
				Title:       "Go Engineer",
				Description: "Experience with Go and Kubernetes required",
				Company:     "Innovate Ltd",
				Location:    "Remote",
			},
			Score:     15, // 10 for Go + 5 for Kubernetes
			Keywords:  []string{"Go", "Kubernetes"},
			CreatedAt: time.Now().UTC(),
		},
	}

	// Set up mock job provider response
	criteria := search.Criteria{
		Keywords:   []string{"Go", "Docker"},
		Skills:     []string{"Go", "Docker", "Kubernetes"},
		PageNumber: 1,
	}
	criteriaJSON, err := json.Marshal(criteria)
	if err != nil {
		t.Fatalf("Failed to marshal criteria: %v", err)
	}
	provider.responses[string(criteriaJSON)] = []search.JobListing{want[0].Job, want[1].Job}

	// Create handler
	_, err = NewSearchHandler(context.Background(), env.js, provider, env.logger, env.db)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Send test message and verify response
	got, err := SendAndReceiveMessage[search.SearchInput, []search.Result](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		search.TopicSearchInput,
		search.TopicSearchResult,
		input,
		5*time.Second,
		"search-test",
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Compare results, ignoring CreatedAt field
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d", len(got), len(want))
	}

	for i := range got {
		got[i].CreatedAt = want[i].CreatedAt // Ignore timestamp differences
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("result[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSearchHandler_constructSearchCriteria(t *testing.T) {
	tests := []struct {
		name         string
		input        search.SearchInput
		wantCriteria search.Criteria
	}{
		{
			name: "must have skills only",
			input: search.SearchInput{
				CandidateID:     "123",
				MustHaveSkills:  []string{"Go", "Docker"},
				PreferredSkills: []string{},
				Page:            1,
			},
			wantCriteria: search.Criteria{
				Keywords:   []string{"Go", "Docker"},
				Skills:     []string{"Go", "Docker"},
				PageNumber: 1,
			},
		},
		{
			name: "must have and preferred skills",
			input: search.SearchInput{
				CandidateID:     "123",
				MustHaveSkills:  []string{"Go"},
				PreferredSkills: []string{"Docker", "Kubernetes"},
				Page:            2,
			},
			wantCriteria: search.Criteria{
				Keywords:   []string{"Go"},
				Skills:     []string{"Go", "Docker", "Kubernetes"},
				PageNumber: 2,
			},
		},
		{
			name: "no must have skills, use preferred as keywords",
			input: search.SearchInput{
				CandidateID:     "123",
				MustHaveSkills:  []string{},
				PreferredSkills: []string{"Python", "JavaScript"},
				Page:            1,
			},
			wantCriteria: search.Criteria{
				Keywords:   []string{"Python", "JavaScript"},
				Skills:     []string{"Python", "JavaScript"},
				PageNumber: 1,
			},
		},
		{
			name: "no skills at all",
			input: search.SearchInput{
				CandidateID:     "123",
				MustHaveSkills:  []string{},
				PreferredSkills: []string{},
				Page:            1,
			},
			wantCriteria: search.Criteria{
				Keywords:   []string{},
				Skills:     []string{},
				PageNumber: 1,
			},
		},
		{
			name: "duplicate skills between must have and preferred",
			input: search.SearchInput{
				CandidateID:     "123",
				MustHaveSkills:  []string{"Go", "Docker"},
				PreferredSkills: []string{"Docker", "Kubernetes"},
				Page:            1,
			},
			wantCriteria: search.Criteria{
				Keywords:   []string{"Go", "Docker"},
				Skills:     []string{"Go", "Docker", "Docker", "Kubernetes"},
				PageNumber: 1,
			},
		},
		{
			name: "large number of skills",
			input: search.SearchInput{
				CandidateID:     "123",
				MustHaveSkills:  make([]string, 50),  // 50 empty strings
				PreferredSkills: make([]string, 100), // 100 empty strings
				Page:            1,
			},
			wantCriteria: search.Criteria{
				Keywords:   make([]string, 50),
				Skills:     make([]string, 150),
				PageNumber: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &SearchHandler{
				logger:   nil, // Not needed for this test
				provider: &mockJobProvider{},
				generic:  nil, // Not needed for this test
				db:       nil, // Not needed for this test
			}

			got := h.constructSearchCriteria(tt.input)
			if !reflect.DeepEqual(got, tt.wantCriteria) {
				t.Errorf("constructSearchCriteria() = %v, want %v", got, tt.wantCriteria)
			}
		})
	}
}

func TestSearchHandler_scoreJobs(t *testing.T) {
	tests := []struct {
		name         string
		job          search.JobListing
		input        search.SearchInput
		wantScore    float64
		wantKeywords []string
	}{
		{
			name: "all must have skills present",
			job: search.JobListing{
				Title:       "Senior Go Developer",
				Description: "We need a Go developer with Docker experience",
			},
			input: search.SearchInput{
				MustHaveSkills:  []string{"Go", "Docker"},
				PreferredSkills: []string{"Kubernetes"},
			},
			wantScore:    20, // 10 points each for Go and Docker
			wantKeywords: []string{"Go", "Docker"},
		},
		{
			name: "some must have and preferred skills",
			job: search.JobListing{
				Title:       "Software Engineer",
				Description: "Experience with Go and Kubernetes required",
			},
			input: search.SearchInput{
				MustHaveSkills:  []string{"Go", "Docker"},
				PreferredSkills: []string{"Kubernetes"},
			},
			wantScore:    15, // 10 for Go + 5 for Kubernetes
			wantKeywords: []string{"Go", "Kubernetes"},
		},
		{
			name: "case insensitive matching",
			job: search.JobListing{
				Title:       "Developer",
				Description: "Experience with golang and DOCKER required",
			},
			input: search.SearchInput{
				MustHaveSkills:  []string{"Go", "Docker"},
				PreferredSkills: []string{},
			},
			wantScore:    20, // 10 points each for golang and DOCKER
			wantKeywords: []string{"Go", "Docker"},
		},
		{
			name: "no skills match",
			job: search.JobListing{
				Title:       "Python Developer",
				Description: "Looking for Python and Django experience",
			},
			input: search.SearchInput{
				MustHaveSkills:  []string{"Go", "Docker"},
				PreferredSkills: []string{"Kubernetes"},
			},
			wantScore:    0,
			wantKeywords: nil,
		},
		{
			name: "skills in title should not count",
			job: search.JobListing{
				Title:       "Senior Go Developer with Docker",
				Description: "Python required",
			},
			input: search.SearchInput{
				MustHaveSkills:  []string{"Go", "Docker"},
				PreferredSkills: []string{},
			},
			wantScore:    0, // Title matches don't count
			wantKeywords: nil,
		},
		{
			name: "duplicate skill matches",
			job: search.JobListing{
				Title:       "Developer",
				Description: "Go developer needed. Must know Go and have Go experience. Docker required.",
			},
			input: search.SearchInput{
				MustHaveSkills:  []string{"Go", "Docker"},
				PreferredSkills: []string{},
			},
			wantScore:    20, // Should only count each skill once
			wantKeywords: []string{"Go", "Docker"},
		},
		{
			name: "empty job description",
			job: search.JobListing{
				Title:       "Developer",
				Description: "",
			},
			input: search.SearchInput{
				MustHaveSkills:  []string{"Go", "Docker"},
				PreferredSkills: []string{},
			},
			wantScore:    0,
			wantKeywords: nil,
		},
		{
			name: "very long job description",
			job: search.JobListing{
				Title:       "Developer",
				Description: strings.Repeat("Lorem ipsum ", 1000) + "Go" + strings.Repeat(" dolor sit amet", 1000) + "Docker",
			},
			input: search.SearchInput{
				MustHaveSkills:  []string{"Go", "Docker"},
				PreferredSkills: []string{},
			},
			wantScore:    20, // Should still find matches in long text
			wantKeywords: []string{"Go", "Docker"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &SearchHandler{
				logger:   nil,
				provider: &mockJobProvider{},
				generic:  nil,
				db:       nil,
			}

			results := h.scoreJobs([]search.JobListing{tt.job}, tt.input)
			if len(results) != 1 {
				t.Fatalf("scoreJobs() returned %d results, want 1", len(results))
			}

			if results[0].Score != tt.wantScore {
				t.Errorf("scoreJobs() score = %v, want %v", results[0].Score, tt.wantScore)
			}

			if !reflect.DeepEqual(results[0].Keywords, tt.wantKeywords) {
				t.Errorf("scoreJobs() keywords = %v, want %v", results[0].Keywords, tt.wantKeywords)
			}
		})
	}
}
