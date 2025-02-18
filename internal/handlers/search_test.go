package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"talent-rodeo/internal/handlers/search"
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

	// Test search criteria
	criteria := search.Criteria{
		Keywords: []string{"Go Developer"},
		Location: "Remote",
	}

	// Expected job search results
	want := []search.JobListing{
		{
			Title:    "Senior Go Developer",
			Company:  "Tech Corp",
			Location: "Remote",
		},
		{
			Title:    "Go Engineer",
			Company:  "Innovate Ltd",
			Location: "Remote",
		},
	}

	// Set up mock job provider response
	criteriaJSON, err := json.Marshal(criteria)
	if err != nil {
		t.Fatalf("Failed to marshal criteria: %v", err)
	}
	provider.responses[string(criteriaJSON)] = want

	// Create handler with a background context since we don't need to control its lifecycle.
	// The handler is required even though we don't explicitly use it because it registers
	// the message handler with the GenericHandler during creation.
	_, err = NewSearchHandler(context.Background(), env.js, provider, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Send test message and verify response
	got, err := SendAndReceiveMessage[search.Criteria, []search.JobListing](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicSearchInput,
		TopicSearchResult,
		criteria,
		1*time.Second,
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Compare results
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
