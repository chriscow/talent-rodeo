package scrapingdog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/search"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func TestProvider_StartSearchWorkers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Increase test timeout to match command timeout
	testTimeout := 25 * time.Second // Leave 5s buffer for cleanup
	searchCtx, searchCancel := context.WithTimeout(ctx, testTimeout)
	defer searchCancel()

	// Create a mock server that returns job listings and details
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add logging to debug mock server responses
		t.Logf("Mock server received request: %s", r.URL.String())

		if r.URL.Query().Get("job_id") != "" {
			// Return job details
			details := []jobDetail{
				{
					ID:                "1234",
					Position:          "Senior Go Developer",
					Status:            "active",
					Location:          "Remote",
					CompanyName:       "Test Company",
					CompanyLinkedInID: "test-job1",
					PostingTime:       "2024-02-06",
					BasePay:           "$150,000 - $200,000",
					Description:       "Looking for an experienced Go developer...",
					SeniorityLevel:    "Senior",
					EmploymentType:    "Full-time",
					JobFunction:       "Software Development",
					Industries:        "Technology",
				},
			}
			if err := json.NewEncoder(w).Encode(details); err != nil {
				t.Logf("Error encoding job details: %v", err)
			}
			t.Logf("Returned job details for ID: %s", r.URL.Query().Get("job_id"))
		} else {
			// Return job listings
			jobs := []jobListing{
				{
					Position:      "Senior Go Developer",
					Link:          "https://linkedin.com/jobs/1",
					ID:            "1234",
					LinkedInJobID: "test-job1",
					CompanyName:   "Test Company",
					Location:      "Remote",
					PostingDate:   "2024-02-06",
				},
			}
			if err := json.NewEncoder(w).Encode(jobs); err != nil {
				t.Logf("Error encoding job listings: %v", err)
			}
			t.Logf("Returned job listings")
		}
	}))
	defer server.Close()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()
	defer internal.DeleteTestData(t, fs, "test-user")
	defer internal.DeleteTestData(t, fs, "test-job-")

	// Create test users
	userIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		phone := fmt.Sprintf("+1555555%04d", i)
		user := internal.CreateTestUser(t, fs,
			fmt.Sprintf("Test User %d", i),
			fmt.Sprintf("test%d@example.com", i),
			phone,
			false,
		)
		userIDs[i] = user.ID

		// Initialize search state
		_, err = fs.Collection("users").Doc(user.ID).Update(ctx, []firestore.Update{
			{Path: "search", Value: map[string]any{
				"status":     "pending",
				"started_at": time.Now(),
			}},
		})
		if err != nil {
			t.Fatalf("failed to initialize search state for user %s: %v", user.ID, err)
		}
	}

	// Clean up test users when done using helper
	defer func() {
		internal.DeleteTestData(t, fs, "test-user")
	}()

	// Initialize store before creating provider
	store, err := NewJobStore(searchCtx, slog.Default())
	if err != nil {
		t.Fatalf("failed to create job store: %v", err)
	}

	// Create provider with store already initialized
	p := NewProvider("test-key",
		WithBaseURL(server.URL),
		WithFirestore(fs, slog.Default()),
		WithJobStore(store),
	)

	// Set a smaller search queue for testing
	p.searchQueue = make(chan QueuedSearch, 10)
	p.searchSemaphore = make(chan struct{}, 2)

	// Start workers
	p.StartSearchWorkers(searchCtx, 2)

	// Queue some searches
	criteria := search.Criteria{
		Keywords: []string{"golang"},
		Location: "Remote",
	}

	// Create a channel to track completion
	completionChan := make(chan string, len(userIDs))

	// Queue searches and track completion
	for _, userID := range userIDs {
		go func(uid string) {
			if err := p.QueueSearch(searchCtx, uid, criteria); err != nil {
				t.Logf("Error queueing search for user %s: %v", uid, err)
				return
			}
			completionChan <- uid
			t.Logf("Successfully queued search for user %s", uid)
		}(userID)
	}

	// Wait for all searches to be queued with shorter timeout
	queuedUsers := make(map[string]bool)
	queueTimeout := time.After(5 * time.Second)
	for i := 0; i < len(userIDs); i++ {
		select {
		case uid := <-completionChan:
			queuedUsers[uid] = true
			t.Logf("Received queue completion for user %s", uid)
		case <-queueTimeout:
			t.Fatalf("timed out waiting for searches to be queued after 5s, got %d/%d", len(queuedUsers), len(userIDs))
		}
	}

	// Wait for processing to complete - poll until all users have jobs or timeout
	deadline := time.Now().Add(testTimeout)
	success := false
	var lastError error

	for time.Now().Before(deadline) {
		processedCount := 0
		jobsCreated := 0
		errorCount := 0

		for _, userID := range userIDs {
			// Check search completion
			doc, err := fs.Collection("users").Doc(userID).Get(searchCtx)
			if err != nil {
				lastError = fmt.Errorf("failed to get user doc: %w", err)
				t.Logf("Error getting user doc for %s: %v", userID, err)
				errorCount++
				continue
			}

			var data map[string]any
			if err := doc.DataTo(&data); err != nil {
				lastError = fmt.Errorf("failed to parse user data: %w", err)
				t.Logf("Error parsing user data for %s: %v", userID, err)
				errorCount++
				continue
			}

			search, ok := data["search"].(map[string]any)
			if !ok {
				lastError = fmt.Errorf("invalid search data format")
				t.Logf("Invalid search data format for user %s", userID)
				errorCount++
				continue
			}

			// Check for errors in search status
			if errStr, ok := search["error"].(string); ok && errStr != "" {
				lastError = fmt.Errorf("search failed: %s", errStr)
				t.Logf("Search error for user %s: %s", userID, errStr)
				errorCount++
				continue
			}

			if search["completed_at"] != nil {
				processedCount++
				t.Logf("Search completed for user %s", userID)
			}

			// Check if jobs were created
			jobs, err := internal.GetUserJobs(searchCtx, fs, userID)
			if err != nil {
				t.Logf("Error getting jobs for user %s: %v", userID, err)
			} else if len(jobs) > 0 {
				jobsCreated++
				t.Logf("Jobs created for user %s: %d", userID, len(jobs))
			}
		}

		t.Logf("Progress: processed=%d/%d, jobs=%d/%d, errors=%d",
			processedCount, len(userIDs),
			jobsCreated, len(userIDs),
			errorCount)

		// Success if all users have completed searches and jobs
		if processedCount == len(userIDs) && jobsCreated == len(userIDs) {
			success = true
			break
		}

		// Shorter sleep interval for more frequent checks
		time.Sleep(200 * time.Millisecond)
	}

	if !success {
		if lastError != nil {
			t.Errorf("searches failed: %v", lastError)
		} else {
			t.Error("not all searches were processed successfully with jobs created")
		}
	}

	// Cancel context to stop workers
	searchCancel()
}

// TestWorkerInitialization tests the initialization of a single worker
func TestWorkerInitialization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create a mock server that returns job listings
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("job_id") != "" {
			// Return job details
			details := []jobDetail{{
				Position:    "Test Job",
				Status:      "active",
				Location:    "Remote",
				CompanyName: "Test Co",
			}}
			json.NewEncoder(w).Encode(details)
		} else {
			// Return job listings
			jobs := []jobListing{{
				Position:    "Test Job",
				Link:        "https://test.com",
				ID:          "test-1",
				CompanyName: "Test Co",
			}}
			json.NewEncoder(w).Encode(jobs)
		}
	}))
	defer server.Close()

	// Create test user
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+15555550000", false)
	defer internal.DeleteTestData(t, fs, "test-user")

	// Initialize search state
	_, err = fs.Collection("users").Doc(user.ID).Update(ctx, []firestore.Update{
		{Path: "search", Value: map[string]any{
			"status":     "pending",
			"started_at": time.Now(),
		}},
	})
	if err != nil {
		t.Fatalf("failed to initialize search state: %v", err)
	}

	p := NewProvider("test-key",
		WithBaseURL(server.URL),
		WithFirestore(fs, slog.Default()),
	)

	// Create a small queue for testing
	p.searchQueue = make(chan QueuedSearch, 1)
	p.searchSemaphore = make(chan struct{}, 1)

	// Start a single worker
	p.initializeWorker(ctx, 0)

	// Create test criteria
	criteria := search.Criteria{
		Keywords: []string{"test"},
		Location: "Remote",
	}

	// Queue a search
	err = p.QueueSearch(ctx, user.ID, criteria)
	if err != nil {
		t.Fatalf("failed to queue search: %v", err)
	}

	// Wait briefly to ensure the worker has time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop worker
	cancel()
}

// TestWorkerGracefulShutdown tests that workers stop gracefully when context is cancelled
func TestWorkerGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create a mock server that returns job listings
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("job_id") != "" {
			// Return job details
			details := []jobDetail{{
				Position:    "Test Job",
				Status:      "active",
				Location:    "Remote",
				CompanyName: "Test Co",
			}}
			json.NewEncoder(w).Encode(details)
		} else {
			// Return job listings
			jobs := []jobListing{{
				Position:    "Test Job",
				Link:        "https://test.com",
				ID:          "test-1",
				CompanyName: "Test Co",
			}}
			json.NewEncoder(w).Encode(jobs)
		}
	}))
	defer server.Close()

	// Create test users
	users := make([]*internal.User, 2)
	for i := range users {
		users[i] = internal.CreateTestUser(t, fs,
			fmt.Sprintf("Test User %d", i),
			fmt.Sprintf("test%d@example.com", i),
			fmt.Sprintf("+1555555%04d", i),
			false,
		)

		// Initialize search state
		_, err = fs.Collection("users").Doc(users[i].ID).Update(ctx, []firestore.Update{
			{Path: "search", Value: map[string]any{
				"status":     "pending",
				"started_at": time.Now(),
			}},
		})
		if err != nil {
			t.Fatalf("failed to initialize search state for user %d: %v", i, err)
		}
	}
	defer internal.DeleteTestData(t, fs, "test-user")

	p := NewProvider("test-key",
		WithBaseURL(server.URL),
		WithFirestore(fs, slog.Default()),
	)

	// Create channels for testing
	p.searchQueue = make(chan QueuedSearch, 2)
	p.searchSemaphore = make(chan struct{}, 1)

	// Start workers
	numWorkers := 2
	p.StartSearchWorkers(ctx, numWorkers)

	// Queue some searches
	for i, user := range users {
		criteria := search.Criteria{
			Keywords: []string{"test"},
			Location: "Remote",
		}
		if err := p.QueueSearch(ctx, user.ID, criteria); err != nil {
			t.Fatalf("failed to queue search %d: %v", i, err)
		}
	}

	// Wait briefly to ensure workers have started processing
	time.Sleep(100 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait briefly to allow workers to stop
	time.Sleep(100 * time.Millisecond)

	// Create a new context for the final test
	newCtx := context.Background()

	// Queue another search - should fail due to context cancellation
	criteria := search.Criteria{
		Keywords: []string{"test"},
		Location: "Remote",
	}

	// Create a new test user for the final test
	finalUser := internal.CreateTestUser(t, fs, "Final Test User", "final@example.com", "+15555559999", false)
	defer internal.DeleteTestData(t, fs, "test-user")

	err = p.QueueSearch(newCtx, finalUser.ID, criteria)
	if err == nil {
		t.Error("expected error when queueing search after shutdown, got nil")
	}
}

// TestSearchQueueProcessing tests the processing of queued searches
func TestSearchQueueProcessing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create a mock server that returns job listings
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("job_id") != "" {
			// Return job details
			details := []jobDetail{{
				Position:    "Test Job",
				Status:      "active",
				Location:    "Remote",
				CompanyName: "Test Co",
			}}
			json.NewEncoder(w).Encode(details)
		} else {
			// Return job listings
			jobs := []jobListing{{
				Position:      "Test Job",
				Link:          "https://test.com",
				ID:            "test-1",
				LinkedInJobID: "test-linkedin-1",
				CompanyName:   "Test Co",
				Location:      "Remote",
				PostingDate:   "2024-03-14",
			}}
			json.NewEncoder(w).Encode(jobs)
		}
	}))
	defer server.Close()

	p := NewProvider("test-key",
		WithBaseURL(server.URL),
		WithFirestore(fs, slog.Default()),
	)

	// Create test user
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+15555550000", false)
	defer internal.DeleteTestData(t, fs, "test-user")
	defer internal.DeleteTestData(t, fs, "test-linkedin-")

	// Initialize search state
	_, err = fs.Collection("users").Doc(user.ID).Update(ctx, []firestore.Update{
		{Path: "search", Value: map[string]any{
			"status":     "pending",
			"started_at": time.Now(),
		}},
	})
	if err != nil {
		t.Fatalf("failed to initialize search state: %v", err)
	}

	// Process a queued search
	qs := QueuedSearch{
		UserID: user.ID,
		Criteria: search.Criteria{
			Keywords: []string{"test"},
			Location: "Remote",
		},
	}

	if err := p.processQueuedSearch(ctx, qs); err != nil {
		t.Fatalf("failed to process queued search: %v", err)
	}

	// Verify search completion status
	doc, err := fs.Collection("users").Doc(user.ID).Get(ctx)
	if err != nil {
		t.Fatalf("failed to get user document: %v", err)
	}

	var data map[string]any
	if err := doc.DataTo(&data); err != nil {
		t.Fatalf("failed to unmarshal user data: %v", err)
	}

	search, ok := data["search"].(map[string]any)
	if !ok {
		t.Fatal("search field not found or wrong type")
	}

	if search["completed_at"] == nil {
		t.Error("completed_at timestamp not set")
	}

	// Verify jobs were created
	jobs, err := internal.GetUserJobs(ctx, fs, user.ID)
	if err != nil {
		t.Fatalf("failed to get user jobs: %v", err)
	}

	if len(jobs) == 0 {
		t.Error("no jobs were created")
	}
}

// TestProcessJobListings tests the job listing processing function
func TestProcessJobListings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create a mock server that returns job listings
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jobs := []jobListing{{
			Position:    "Test Job",
			Link:        "https://test.com",
			ID:          "test-1",
			CompanyName: "Test Co",
		}}
		json.NewEncoder(w).Encode(jobs)
	}))
	defer server.Close()

	p := NewProvider("test-key",
		WithBaseURL(server.URL),
		WithFirestore(fs, slog.Default()),
	)

	// Test cases
	tests := []struct {
		name     string
		criteria search.Criteria
		wantErr  bool
	}{
		{
			name: "valid search criteria",
			criteria: search.Criteria{
				Keywords: []string{"test"},
				Location: "Remote",
			},
			wantErr: false,
		},
		{
			name: "empty keywords",
			criteria: search.Criteria{
				Keywords: []string{},
				Location: "Remote",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs, err := p.processJobListings(ctx, tt.criteria)
			if (err != nil) != tt.wantErr {
				t.Errorf("processJobListings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(jobs) == 0 {
				t.Error("processJobListings() returned no jobs")
			}
		})
	}

	// Test context cancellation
	t.Run("context cancellation", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		_, err := p.processJobListings(cancelCtx, search.Criteria{
			Keywords: []string{"test"},
			Location: "Remote",
		})
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("processJobListings() error = %v, want error containing 'context canceled'", err)
		}
	})
}

// TestProcessJobDetailsAndSave tests the job details processing and saving function
func TestProcessJobDetailsAndSave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create a mock server that returns job details
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		details := []jobDetail{{
			Position:          "Test Job",
			Status:            "active",
			Location:          "Remote",
			CompanyName:       "Test Co",
			CompanyLinkedInID: "test-company-1", // Add required fields
			PostingTime:       "2024-03-14",
		}}
		json.NewEncoder(w).Encode(details)
	}))
	defer server.Close()

	// Initialize JobStore
	store, err := NewJobStore(ctx, slog.Default())
	if err != nil {
		t.Fatalf("failed to create job store: %v", err)
	}

	p := NewProvider("test-key",
		WithBaseURL(server.URL),
		WithFirestore(fs, slog.Default()),
		WithJobStore(store),
	)

	// Create test user
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+15555550000", false)
	defer internal.DeleteTestData(t, fs, "test-user")

	// Test cases
	tests := []struct {
		name       string
		listing    search.JobListing
		searchHash string
		wantErr    bool
	}{
		{
			name: "valid job details",
			listing: search.JobListing{
				ProviderJobID: "test-job-1",
				Title:         "Test Job",
				Company:       "Test Co",
				Location:      "Remote",
				URL:           "https://example.com/job/test-job-1",
				Provider:      providerName,
			},
			searchHash: "test-search-hash",
			wantErr:    false,
		},
		{
			name: "empty job ID",
			listing: search.JobListing{
				ProviderJobID: "",
				Title:         "Test Job",
				Company:       "Test Co",
				Location:      "Remote",
				URL:           "https://example.com/job/test-job-1",
				Provider:      providerName,
			},
			searchHash: "test-search-hash",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.processJobDetailsAndSave(ctx, tt.listing, tt.searchHash, user.ID)
			if (err != nil) != tt.wantErr {
				t.Errorf("processJobDetailsAndSave() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify job was saved
				jobs, err := internal.GetUserJobs(ctx, fs, user.ID)
				if err != nil {
					t.Fatalf("failed to get user jobs: %v", err)
				}
				if len(jobs) == 0 {
					t.Error("no jobs were saved")
				}
				for _, job := range jobs {
					if job.SearchHash != tt.searchHash {
						t.Errorf("job.SearchHash = %v, want %v", job.SearchHash, tt.searchHash)
					}
				}
			}
		})
	}

	// Test context cancellation
	t.Run("context cancellation", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		listing := search.JobListing{
			ProviderJobID: "test-job-1",
			Title:         "Test Job",
			URL:           "https://example.com/job/test-job-1",
		}
		err := p.processJobDetailsAndSave(cancelCtx, listing, "test-hash", user.ID)
		if err != context.Canceled {
			t.Errorf("processJobDetailsAndSave() error = %v, want %v", err, context.Canceled)
		}
	})
}

// TestJobIDFormat tests that job IDs are correctly formatted
func TestJobIDFormat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create a mock server that returns job details
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		details := []jobDetail{{
			Position:          "Test Job",
			Status:            "active",
			Location:          "Remote",
			CompanyName:       "Test Co",
			CompanyLinkedInID: "test-company-1",
			PostingTime:       "2024-03-14",
		}}
		json.NewEncoder(w).Encode(details)
	}))
	defer server.Close()

	// Initialize JobStore
	store, err := NewJobStore(ctx, slog.Default())
	if err != nil {
		t.Fatalf("failed to create job store: %v", err)
	}

	p := NewProvider("test-key",
		WithBaseURL(server.URL),
		WithFirestore(fs, slog.Default()),
		WithJobStore(store),
	)

	tests := []struct {
		name          string
		linkedInJobID string
		expectedJobID string
	}{
		{
			name:          "simple job id",
			linkedInJobID: "123456",
			expectedJobID: "scrapingdog-123456",
		},
		{
			name:          "complex job id",
			linkedInJobID: "job-posting-789",
			expectedJobID: "scrapingdog-job-posting-789",
		},
		{
			name:          "job id with special characters",
			linkedInJobID: "job/posting#789",
			expectedJobID: "scrapingdog-jobposting789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details, err := p.GetJobDetails(ctx, tt.linkedInJobID)
			if err != nil {
				t.Fatalf("GetJobDetails() error = %v", err)
			}

			if details.ID != tt.expectedJobID {
				t.Errorf("Job ID format incorrect, got %q, want %q", details.ID, tt.expectedJobID)
			}

			if details.ProviderJobID != tt.linkedInJobID {
				t.Errorf("ProviderJobID not preserved, got %q, want %q", details.ProviderJobID, tt.linkedInJobID)
			}

			if details.Provider != providerName {
				t.Errorf("Provider name incorrect, got %q, want %q", details.Provider, providerName)
			}
		})
	}
}

// TestURLCapture tests that the URL from a job listing is correctly captured in job details
func TestURLCapture(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create a mock server that returns job details
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		details := []jobDetail{{
			Position:          "Test Job",
			Status:            "active",
			Location:          "Remote",
			CompanyName:       "Test Co",
			CompanyLinkedInID: "test-company-1",
			PostingTime:       "2024-03-14",
		}}
		json.NewEncoder(w).Encode(details)
	}))
	defer server.Close()

	// Initialize JobStore
	store, err := NewJobStore(ctx, slog.Default())
	if err != nil {
		t.Fatalf("failed to create job store: %v", err)
	}

	p := NewProvider("test-key",
		WithBaseURL(server.URL),
		WithFirestore(fs, slog.Default()),
		WithJobStore(store),
	)

	// Define test job with URL
	jobID := "test-job-with-url"
	expectedURL := "https://example.com/jobs/test-job-with-url"

	// Process the job and save details
	err = p.processJobDetails(ctx, jobID, expectedURL)
	if err != nil {
		t.Fatalf("processJobDetails() error = %v", err)
	}

	// Retrieve the job details from store to verify URL was set
	fullJobID := funcs.Slugify(fmt.Sprintf("%s %s", providerName, jobID))
	details, err := p.store.GetCachedJobDetails(ctx, fullJobID)
	if err != nil {
		t.Fatalf("GetCachedJobDetails() error = %v", err)
	}

	if details == nil {
		t.Fatal("job details not found in cache")
	}

	// Verify URL was set correctly
	if details.URL != expectedURL {
		t.Errorf("Job URL was not captured correctly. Got %q, want %q", details.URL, expectedURL)
	}
}
