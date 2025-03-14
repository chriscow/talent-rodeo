package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/services/search"
	"talent-rodeo/internal/services/search/providers/scrapingdog"
	"talent-rodeo/internal/tasks/handlers"

	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// testTaskProcessor implements TaskProcessor for testing
type testTaskProcessor struct {
	enqueuedTasks []*internal.Task
}

func (p *testTaskProcessor) EnqueueTask(ctx context.Context, task *internal.Task) error {
	p.enqueuedTasks = append(p.enqueuedTasks, task)
	return nil
}

// mockPublisher adapts JetStream to the Publisher interface
type mockPublisher struct {
	js         jetstream.JetStream
	stream     jetstream.Stream
	streamName string
}

func (p *mockPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	_, err := p.js.Publish(ctx, subject, data)
	return err
}

// searchTestEnv provides a test environment with all dependencies
type searchTestEnv struct {
	db         *database.DB
	js         jetstream.JetStream
	nc         *nats.Conn
	logger     *slog.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	cleanups   []func()
	stream     jetstream.Stream
	streamName string
}

func newSearchTestEnv(t *testing.T) *searchTestEnv {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create NATS server with JetStream enabled
	opts := test.DefaultTestOptions
	opts.JetStream = true
	srv := test.RunServer(&opts)
	t.Cleanup(func() {
		logger.Debug("cleaning up test NATS server")
		srv.Shutdown()
	})

	// Wait for server to be ready
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server failed to start")
	}

	// Connect to NATS
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}

	// Create JetStream
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("Failed to create JetStream: %v", err)
	}

	// Create unique stream name for this test
	streamName := fmt.Sprintf("TEST_STREAM_%d", time.Now().UnixNano())

	// Create stream
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      streamName,
		Subjects:  []string{fmt.Sprintf("%s.>", streamName)},
		Retention: jetstream.InterestPolicy,
	})
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	db := database.SetupTestDB(t)

	env := &searchTestEnv{
		db:         db,
		js:         js,
		nc:         nc,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		cleanups:   make([]func(), 0),
		stream:     stream,
		streamName: streamName,
	}

	// Add cleanup functions
	env.cleanups = append(env.cleanups, func() {
		logger.Debug("cleaning up test environment", "component", "test_env")
		if err := js.DeleteStream(ctx, streamName); err != nil {
			logger.Error("error deleting stream", "error", err)
		}
		cancel()
		nc.Close()
		if err := db.Close(); err != nil {
			logger.Error("error closing database", "error", err)
		}
	})

	return env
}

func (env *searchTestEnv) cleanup() {
	for _, cleanup := range env.cleanups {
		cleanup()
	}
}

// mockScrapingDogProvider simulates the behavior of the ScrapingDog provider
type mockScrapingDogProvider struct{}

func (m *mockScrapingDogProvider) SearchJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	// Simulate 10 job listings per page
	listings := make([]search.JobListing, 10)
	for i := range listings {
		listings[i] = search.JobListing{
			ID:            fmt.Sprintf("job-%d", i+1),
			Title:         fmt.Sprintf("Senior Go Developer %d", i+1),
			Company:       fmt.Sprintf("Tech Company %d", i+1),
			Location:      "Remote",
			Description:   fmt.Sprintf("We are looking for a Go developer with %d+ years of experience. Skills: Go, Python, Docker, Kubernetes", i+3),
			URL:           fmt.Sprintf("https://example.com/jobs/%d", i+1),
			Provider:      "scrapingdog",
			ProviderJobID: fmt.Sprintf("job-%d", i+1),
			PostedDate:    time.Now().Add(-time.Duration(i) * 24 * time.Hour),
		}
	}
	return listings, nil
}

func (m *mockScrapingDogProvider) GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {
	return &search.JobDetail{
		ID:            jobID,
		Provider:      "scrapingdog",
		ProviderJobID: jobID,
		Title:         "Senior Go Developer",
		Company:       "Tech Company",
		Location:      "Remote",
		PostedDate:    time.Now(),
		URL:           "https://example.com/jobs/1",
		CompanyInfo: &search.Company{
			Name:     "Tech Company",
			Industry: "Technology",
		},
	}, nil
}

func (m *mockScrapingDogProvider) Name() string {
	return "scrapingdog"
}

func (m *mockScrapingDogProvider) Validate(ctx context.Context) error {
	return nil
}

func TestSearchIntegration(t *testing.T) {
	// Create test environment with longer timeout for real API
	timeout := 30 * time.Second
	if !testing.Short() {
		timeout = 2 * time.Minute // Give plenty of time for API calls
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	env := newSearchTestEnv(t)
	// Don't defer cleanup, we'll do it manually after all messages are processed

	var provider search.JobProvider
	if testing.Short() {
		provider = &mockScrapingDogProvider{}
	} else {
		// Skip if no API key
		apiKey := os.Getenv("SCRAPINGDOG_API_KEY")
		if apiKey == "" {
			t.Skip("SCRAPINGDOG_API_KEY is not set")
		}
		provider = scrapingdog.NewProvider(apiKey)
	}

	// Create search handler with our test stream
	searchHandler, err := NewSearchHandler(ctx, env.js, provider, env.logger, env.db)
	if err != nil {
		t.Fatalf("Failed to create search handler: %v", err)
	}

	// Re-register the handler with our test stream
	if err := searchHandler.Register(ctx, env.streamName, search.TopicSearchInput, "RODEO-SEARCH-QUEUE", maxAckPending); err != nil {
		t.Fatalf("Failed to register search handler with test stream: %v", err)
	}

	env.logger.Debug("configured search handler",
		"stream_name", env.streamName,
		"input_subject", search.TopicSearchInput,
		"result_subject", search.TopicSearchResult)

	// Create test user and candidate with skills
	user, candidateID, skills, err := env.db.CreateTestUserWithSkills(t)
	if err != nil {
		t.Fatalf("Failed to create test user and skills: %v", err)
	}
	_ = user // Silence linter, we need the user for the test setup

	env.logger.Debug("created test candidate with skills",
		"candidate_id", candidateID,
		"skills", skills)

	// Update candidate tier to 'free' for testing
	_, err = env.db.Exec(`
		UPDATE candidates 
		SET tier = 'free'::tier_type
		WHERE id = $1`,
		candidateID)
	if err != nil {
		t.Fatalf("Failed to update candidate tier: %v", err)
	}

	// Store skills in the database
	skillNames := make([]string, len(skills))
	for i, skill := range skills {
		skillNames[i] = skill.Name
	}

	env.logger.Debug("using candidate skills",
		"candidate_id", candidateID,
		"skill_names", skillNames)

	// Create task processor and search scheduler
	taskProc := &testTaskProcessor{enqueuedTasks: make([]*internal.Task, 0)}
	publisher := &mockPublisher{js: env.js, stream: env.stream, streamName: env.streamName}
	scheduler := handlers.NewSearchScheduler(publisher, env.logger, env.db, taskProc)

	// Create scheduler task
	payload := handlers.SearchSchedulerPayload{
		CandidateID: candidateID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	task := &internal.Task{
		Type:        scheduler.TaskType(),
		Payload:     payloadBytes,
		Status:      internal.TaskStatusPending,
		MaxAttempts: 3,
		CreatedAt:   time.Now().UTC(),
		RunAt:       time.Now().UTC(),
	}

	// Process the scheduler task
	if err := scheduler.ProcessTask(ctx, task); err != nil {
		t.Fatalf("Failed to process scheduler task: %v", err)
	}

	// For free tier, we expect 3 search tasks plus 1 scheduler task
	expectedTasks := 4
	if got := len(taskProc.enqueuedTasks); got != expectedTasks {
		t.Errorf("Got %d enqueued tasks, want %d", got, expectedTasks)
	}

	// Create consumer to verify search results
	consumerSubject := fmt.Sprintf("%s.%s", env.streamName, search.TopicSearchResult)
	env.logger.Debug("creating consumer",
		"stream_name", env.streamName,
		"filter_subject", consumerSubject)

	consumer, err := env.js.CreateOrUpdateConsumer(ctx, env.streamName, jetstream.ConsumerConfig{
		Name:          "search-integration-test",
		FilterSubject: consumerSubject,
		DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}

	// Track number of successful messages
	successfulMessages := 0
	expectedMessages := 3 // We expect 3 pages of results

	// Create a channel to signal when all messages are received
	allMessagesReceived := make(chan struct{})

	// Start a goroutine to collect messages
	go func() {
		defer close(allMessagesReceived)
		for successfulMessages < expectedMessages {
			// Wait for and verify the search result with a shorter timeout
			msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(10*time.Second))
			if err != nil {
				t.Errorf("Failed to fetch search result: %v", err)
				return
			}

			msg := <-msgs.Messages()
			if msg == nil {
				t.Error("No search result received")
				return
			}

			var results []search.Result
			if err := json.Unmarshal(msg.Data(), &results); err != nil {
				t.Errorf("Failed to unmarshal search results: %v", err)
				return
			}

			// Verify the results
			if len(results) == 0 {
				t.Error("Expected at least one search result")
			}

			// Verify each result has required fields
			for i, result := range results {
				if result.Job.Title == "" {
					t.Errorf("Result %d missing job title", i)
				}
				if result.Job.Company == "" {
					t.Errorf("Result %d missing company name", i)
				}
				if result.Job.Description == "" {
					t.Errorf("Result %d missing job description", i)
				}
				if result.Job.Location == "" {
					t.Errorf("Result %d missing location", i)
				}
				if result.Job.URL == "" {
					t.Errorf("Result %d missing URL", i)
				}
				if result.Job.Provider != "scrapingdog" {
					t.Errorf("Result %d has wrong provider: got %s, want scrapingdog", i, result.Job.Provider)
				}
			}

			// Acknowledge the message
			if err := msg.Ack(); err != nil {
				t.Errorf("Failed to acknowledge message: %v", err)
				return
			}

			successfulMessages++
		}
	}()

	// Publish search messages
	for _, task := range taskProc.enqueuedTasks {
		if task.Type != "nats_message_scheduler" {
			continue
		}

		var msgPayload handlers.NATSMessagePayload
		if err := json.Unmarshal(task.Payload, &msgPayload); err != nil {
			t.Fatalf("Failed to unmarshal message payload: %v", err)
		}

		// Update the search input to include skills
		var searchInput search.SearchInput
		if err := json.Unmarshal(msgPayload.Data, &searchInput); err != nil {
			t.Fatalf("Failed to unmarshal search input: %v", err)
		}

		// Add the candidate's skills as must-have and preferred skills
		searchInput.MustHaveSkills = []string{skillNames[0]} // Use first skill as must-have
		searchInput.PreferredSkills = skillNames[1:]         // Use remaining skills as preferred

		env.logger.Debug("updating search input with skills",
			"must_have_skills", searchInput.MustHaveSkills,
			"preferred_skills", searchInput.PreferredSkills)

		// Re-marshal the updated search input
		updatedData, err := json.Marshal(searchInput)
		if err != nil {
			t.Fatalf("Failed to marshal updated search input: %v", err)
		}
		msgPayload.Data = updatedData

		// Publish the search message
		if err := publisher.Publish(ctx, msgPayload.Subject, msgPayload.Data); err != nil {
			t.Fatalf("Failed to publish message: %v", err)
		}
	}

	// Wait for all messages to be received or context to be cancelled
	select {
	case <-allMessagesReceived:
		// All messages received successfully
	case <-ctx.Done():
		t.Fatalf("Context cancelled before receiving all messages: got %d, want %d", successfulMessages, expectedMessages)
	}

	// Verify database state was updated
	var candidate struct {
		LastSearchAt *time.Time `db:"last_search_at"`
		LastPage     *int       `db:"last_page"`
	}
	err = env.db.GetContext(ctx, &candidate,
		`SELECT last_search_at, last_page 
		FROM candidates 
		WHERE id = $1`,
		candidateID)
	if err != nil {
		t.Fatalf("Failed to get candidate: %v", err)
	}

	if candidate.LastSearchAt == nil {
		t.Error("last_search_at was not updated")
	}
	if candidate.LastPage == nil || *candidate.LastPage != 3 {
		t.Errorf("last_page not updated correctly: got %v, want 3",
			func() interface{} {
				if candidate.LastPage == nil {
					return nil
				}
				return *candidate.LastPage
			}())
	}

	// Only cleanup after all verifications are complete
	env.cleanup()
}
