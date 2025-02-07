package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"talent-rodeo/internal/handlers/search"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
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

func TestSearchHandler_ProcessNextMessage(t *testing.T) {
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
	expectedJobs := []search.JobListing{
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
	provider.responses[string(criteriaJSON)] = expectedJobs

	// Create handler
	handler, err := NewSearchHandler(context.Background(), env.js, provider, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a consumer for the result topic
	consumer, err := getConsumer(ctx, env.js, rodeoStream, "test-consumer", TopicSearchResult)
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}

	// Start the handler in a goroutine
	go handler.Start(ctx)
	time.Sleep(100 * time.Millisecond) // Allow handler to initialize

	// Marshal criteria to JSON
	criteriaJSON, err = json.Marshal(criteria)
	if err != nil {
		t.Fatalf("Failed to marshal criteria: %v", err)
	}

	// Publish search criteria message
	_, err = env.js.Publish(context.Background(), TopicSearchInput, criteriaJSON)
	if err != nil {
		t.Fatalf("Failed to publish search criteria: %v", err)
	}

	// Fetch the search result message
	msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
	if err != nil {
		t.Fatalf("Failed to fetch message: %v", err)
	}

	msg := <-msgs.Messages()
	if msg == nil {
		t.Fatal("No message received")
	}

	// Unmarshal the search result
	var gotJobs []search.JobListing
	if err := json.Unmarshal(msg.Data(), &gotJobs); err != nil {
		t.Fatalf("Failed to unmarshal search result: %v", err)
	}

	// Compare results
	if !reflect.DeepEqual(gotJobs, expectedJobs) {
		t.Errorf("got %v, want %v", gotJobs, expectedJobs)
	}

	// Acknowledge the message
	msg.Ack()

	// Stop the handler
	cancel()
	time.Sleep(100 * time.Millisecond)
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
	expectedJobs := []search.JobListing{
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
	provider.responses[string(criteriaJSON)] = expectedJobs

	// Create handler
	handler, err := NewSearchHandler(context.Background(), env.js, provider, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Start the handler in a goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go handler.Start(ctx)
	time.Sleep(100 * time.Millisecond) // Allow handler to initialize

	// Marshal criteria to JSON
	criteriaJSON, err = json.Marshal(criteria)
	if err != nil {
		t.Fatalf("Failed to marshal criteria: %v", err)
	}

	// Publish search criteria message
	_, err = env.js.Publish(context.Background(), TopicSearchInput, criteriaJSON)
	if err != nil {
		t.Fatalf("Failed to publish search criteria: %v", err)
	}

	// Create a consumer for the result topic
	consumer, err := getConsumer(ctx, env.js, rodeoStream, "test-consumer-2", TopicSearchResult)
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}

	// Fetch the search result message
	msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
	if err != nil {
		t.Fatalf("Failed to fetch message: %v", err)
	}

	msg := <-msgs.Messages()
	if msg == nil {
		t.Fatal("No message received")
	}

	// Unmarshal the search result
	var gotJobs []search.JobListing
	if err := json.Unmarshal(msg.Data(), &gotJobs); err != nil {
		t.Fatalf("Failed to unmarshal search result: %v", err)
	}

	// Compare results
	if !reflect.DeepEqual(gotJobs, expectedJobs) {
		t.Errorf("got %v, want %v", gotJobs, expectedJobs)
	}

	// Acknowledge the message
	msg.Ack()
}
