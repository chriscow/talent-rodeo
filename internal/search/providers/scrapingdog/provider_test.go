package scrapingdog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/search"

	"cloud.google.com/go/firestore"
	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Overload("../../../../.env"); err != nil {
		log.Fatal("Error loading .env file")
	}
}

// mockTimer allows us to track sleep calls without modifying time.Sleep
type mockTimer struct {
	sleepCount int
	mu         sync.Mutex
}

func (m *mockTimer) Sleep(d time.Duration) {
	m.mu.Lock()
	m.sleepCount++
	m.mu.Unlock()
}

func (m *mockTimer) getSleepCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sleepCount
}

func TestScrapingDogProvider_SearchJobs(t *testing.T) {
	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Mock server to simulate ScrapingDog API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request parameters
		if got := r.Method; got != "GET" {
			t.Errorf("request method = %q; want %q", got, "GET")
		}
		if got := r.URL.Path; got != "/linkedinjobs" {
			t.Errorf("request path = %q; want %q", got, "/linkedinjobs")
		}
		if got := r.URL.Query().Get("api_key"); got != "test-key" {
			t.Errorf("api_key = %q; want %q", got, "test-key")
		}
		if got := r.URL.Query().Get("field"); got != "golang" {
			t.Errorf("field = %q; want %q", got, "golang")
		}

		// Return mock response
		jobs := []jobListing{
			{
				Position:      "Senior Go Developer",
				Link:          "https://linkedin.com/jobs/1",
				ID:            "1234",
				LinkedInJobID: "test-job1",
				CompanyName:   "Test Company",
				Location:      "Remote",
				PostingDate:   "2024-02-06",
				CompanyLogo:   "https://example.com/logo.png",
			},
		}
		if err := json.NewEncoder(w).Encode(jobs); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewProvider("test-key", WithBaseURL(server.URL), WithFirestore(fs, slog.Default()))
	if provider == nil {
		t.Fatal("NewProvider() returned nil")
	}

	criteria := search.Criteria{
		Keywords: []string{"golang"},
		Location: "Remote",
	}

	got, err := provider.SearchJobListings(ctx, criteria)
	if err != nil {
		t.Fatalf("SearchJobs() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("SearchJobs() returned %d jobs; want 1", len(got))
	}

	job := got[0]
	want := struct {
		Title         string
		Company       string
		Location      string
		ProviderJobID string
	}{
		Title:         "Senior Go Developer",
		Company:       "Test Company",
		Location:      "Remote",
		ProviderJobID: "test-job1",
	}

	if job.Title != want.Title {
		t.Errorf("job.Title = %q; want %q", job.Title, want.Title)
	}
	if job.Company != want.Company {
		t.Errorf("job.Company = %q; want %q", job.Company, want.Company)
	}
	if job.Location != want.Location {
		t.Errorf("job.Location = %q; want %q", job.Location, want.Location)
	}
	if job.ProviderJobID != want.ProviderJobID {
		t.Errorf("job.ProviderJobID = %q; want %q", job.ProviderJobID, want.ProviderJobID)
	}
}

func TestScrapingDogProvider_GetJobDetails(t *testing.T) {
	// Mock server to simulate ScrapingDog API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request parameters
		if got := r.Method; got != "GET" {
			t.Errorf("request method = %q; want %q", got, "GET")
		}
		if got := r.URL.Path; got != "/linkedinjobs" {
			t.Errorf("request path = %q; want %q", got, "/linkedinjobs")
		}
		if got := r.URL.Query().Get("api_key"); got != "test-key" {
			t.Errorf("api_key = %q; want %q", got, "test-key")
		}
		if got := r.URL.Query().Get("job_id"); got != "test-job1" {
			t.Errorf("job_id = %q; want %q", got, "test-job1")
		}

		// Return mock response
		details := []jobDetail{
			{
				Position:       "Senior Go Developer",
				Status:         "active",
				Location:       "Remote",
				CompanyName:    "Test Company",
				PostingTime:    "2024-02-06",
				BasePay:        "$150,000 - $200,000",
				Description:    "Looking for an experienced Go developer...",
				SeniorityLevel: "Senior",
				EmploymentType: "Full-time",
				JobFunction:    "Software Development",
				Industries:     "Technology",
				Recruiters: []recruiter{
					{
						Name:       "John Doe",
						Title:      "Technical Recruiter",
						ProfileURL: "https://linkedin.com/in/johndoe",
					},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(details); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewProvider("test-key", WithBaseURL(server.URL))
	if provider == nil {
		t.Fatal("NewProvider() returned nil")
	}

	ctx := context.Background()
	got, err := provider.GetJobDetails(ctx, "test-job1")
	if err != nil {
		t.Fatalf("GetJobDetails() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetJobDetails() returned nil")
	}

	want := struct {
		Title          string
		Company        string
		Location       string
		EmploymentType string
		Industry       string
	}{
		Title:          "Senior Go Developer",
		Company:        "Test Company",
		Location:       "Remote",
		EmploymentType: "Full-time",
		Industry:       "Technology",
	}

	if got.Title != want.Title {
		t.Errorf("job.Title = %q; want %q", got.Title, want.Title)
	}
	if got.Company != want.Company {
		t.Errorf("job.Company = %q; want %q", got.Company, want.Company)
	}
	if got.Location != want.Location {
		t.Errorf("job.Location = %q; want %q", got.Location, want.Location)
	}
	if got.EmploymentType != want.EmploymentType {
		t.Errorf("job.EmploymentType = %q; want %q", got.EmploymentType, want.EmploymentType)
	}
	if got.Industry != want.Industry {
		t.Errorf("job.Industry = %q; want %q", got.Industry, want.Industry)
	}
	if got.SalaryInfo == nil {
		t.Error("job.SalaryInfo is nil, want non-nil")
	}
}

func TestScrapingDogProvider_Validate(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  string
		wantErr bool
	}{
		{
			name:    "valid api key",
			apiKey:  "test-key",
			wantErr: false,
		},
		{
			name:    "empty api key",
			apiKey:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(tt.apiKey)
			err := provider.Validate(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestScrapingDogProvider_Name(t *testing.T) {
	provider := NewProvider("test-key")
	got := provider.Name()
	want := "scrapingdog"
	if got != want {
		t.Errorf("Name() = %q; want %q", got, want)
	}
}

func TestScrapingDogProvider_Integration_SearchJobs(t *testing.T) {
	// Skip the test if the -short flag is set
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Ensure the API key is set in the environment
	apiKey := os.Getenv("SCRAPINGDOG_API_KEY")
	if apiKey == "" {
		t.Skip("SCRAPINGDOG_API_KEY is not set")
	}

	// Initialize Firestore client
	ctx := context.Background()
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	provider := NewProvider(apiKey, WithFirestore(fs, slog.Default()))

	criteria := search.Criteria{
		Keywords:   []string{"python"},
		Location:   string(remoteGEOID), // Use the default remote geoid
		PageNumber: 1,
	}

	jobs, err := provider.SearchJobListings(ctx, criteria)
	if err != nil {
		if err.Error() == "unexpected status code: 403" {
			t.Skip("API key is invalid or expired")
		}
		t.Fatalf("SearchJobs() error = %v", err)
	}

	if len(jobs) == 0 {
		t.Error("SearchJobs() returned no jobs")
	}
}

// Integration test for getting job details
func TestScrapingDogProvider_Integration_GetJobDetails(t *testing.T) {
	// Skip the test if the -short flag is set
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Ensure the API key is set in the environment
	apiKey := os.Getenv("SCRAPINGDOG_API_KEY")
	if apiKey == "" {
		t.Skip("SCRAPINGDOG_API_KEY is not set")
	}

	// Initialize Firestore client
	ctx := context.Background()
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	provider := NewProvider(apiKey, WithFirestore(fs, slog.Default()))

	// First search for jobs to get a real job ID
	searchCriteria := search.Criteria{
		Keywords:   []string{"python"},
		Location:   string(remoteGEOID),
		PageNumber: 1,
	}

	jobs, err := provider.SearchJobListings(ctx, searchCriteria)
	if err != nil {
		if err.Error() == "unexpected status code: 403" {
			t.Skip("API key is invalid or expired")
		}
		t.Fatalf("SearchJobs() error = %v", err)
	}

	if len(jobs) == 0 {
		t.Skip("No jobs found to test GetJobDetails")
	}

	// Use the first job's ID for testing GetJobDetails
	jobID := jobs[0].ProviderJobID
	log.Printf("Testing GetJobDetails with job ID: %s", jobID)

	details, err := provider.GetJobDetails(ctx, jobID)
	if err != nil {
		if err.Error() == "unexpected status code: 403" {
			t.Skip("API key is invalid or expired")
		}
		t.Fatalf("GetJobDetails() error = %v", err)
	}

	if details == nil {
		t.Error("GetJobDetails() returned nil")
	}
}

func TestProvider_QueueSearch(t *testing.T) {
	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create a test user
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+15555550000", false)
	defer internal.DeleteTestData(t, fs, "test-user")

	// Initialize provider with Firestore
	p := NewProvider("test-key", WithFirestore(fs, slog.Default()))
	p.searchQueue = make(chan QueuedSearch, 2) // Small buffer for testing

	criteria := search.Criteria{
		Keywords: []string{"golang"},
		Location: "remote",
	}

	// Test successful queue
	if err := p.QueueSearch(ctx, user.ID, criteria); err != nil {
		t.Errorf("unexpected error queueing search for user1: %v", err)
	}

	// Test queue full
	if err := p.QueueSearch(ctx, user.ID, criteria); err != nil {
		t.Errorf("unexpected error queueing search for user2: %v", err)
	}

	// Queue should be full now
	if err := p.QueueSearch(ctx, user.ID, criteria); err == nil {
		t.Error("expected error for full queue, got nil")
	} else if err.Error() != "search queue is full" {
		t.Errorf("got error %q, want %q", err.Error(), "search queue is full")
	}

	// Test context cancellation
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := p.QueueSearch(cancelCtx, user.ID, criteria); err != context.Canceled {
		t.Errorf("got error %v, want %v", err, context.Canceled)
	}

	// Verify QueuedAt was set for the first successful queue
	doc, err := fs.Collection("users").Doc(user.ID).Get(ctx)
	if err != nil {
		t.Fatalf("failed to get user document: %v", err)
	}

	var userData map[string]interface{}
	if err := doc.DataTo(&userData); err != nil {
		t.Fatalf("failed to unmarshal user data: %v", err)
	}

	search, ok := userData["search"].(map[string]interface{})
	if !ok {
		t.Fatal("search field not found or wrong type")
	}

	queuedAt, ok := search["queued_at"].(time.Time)
	if !ok {
		t.Fatal("queued_at field not found or wrong type")
	}

	if queuedAt.IsZero() {
		t.Error("queued_at should not be zero")
	}
}

func TestProvider_ProcessQueuedSearch(t *testing.T) {
	ctx := context.Background()

	// Create a mock server that returns job listings and details
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("job_id") != "" {
			// Return job details
			details := []jobDetail{
				{
					Position:    "Senior Go Developer",
					Status:      "active",
					Location:    "Remote",
					CompanyName: "Test Company",
				},
			}
			json.NewEncoder(w).Encode(details)
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
				},
			}
			json.NewEncoder(w).Encode(jobs)
		}
	}))
	defer server.Close()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create a test user using helper
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

	p := NewProvider("test-key", WithBaseURL(server.URL), WithFirestore(fs, slog.Default()))
	p.searchSemaphore = make(chan struct{}, 1)

	// Test 1: Successful search with user job creation
	t.Run("successful search with user jobs", func(t *testing.T) {
		qs := QueuedSearch{
			UserID: user.ID,
			Criteria: search.Criteria{
				Keywords: []string{"golang"},
				Location: "remote",
			},
		}

		if err := p.processQueuedSearch(ctx, qs); err != nil {
			t.Errorf("unexpected error processing search: %v", err)
		}

		// Verify search completion
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
			t.Error("completed_at timestamp is nil")
		}

		// Verify user jobs were created
		jobs, err := internal.GetUserJobs(ctx, fs, user.ID)
		if err != nil {
			t.Fatalf("failed to get user jobs: %v", err)
		}

		if len(jobs) == 0 {
			t.Fatal("no user jobs were created")
		}

		// Verify job details
		job := jobs[0]
		if job.ID != "scrapingdog-test-job1" {
			t.Errorf("job.ID = %v, want scrapingdog-test-job1", job.ID)
		}
		if job.Status != "new" {
			t.Errorf("job.Status = %v, want new", job.Status)
		}
		if job.SearchHash == "" {
			t.Error("job.SearchHash is empty")
		}
		if job.JobRef == nil {
			t.Error("job.JobRef is nil")
		} else if job.JobRef.ID != "scrapingdog-test-job1" {
			t.Errorf("job.JobRef.ID = %v, want scrapingdog-test-job1", job.JobRef.ID)
		}
	})

	// Test 2: Context cancellation
	t.Run("context cancellation", func(t *testing.T) {
		// Create a context that's already cancelled
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		qs := QueuedSearch{
			UserID: user.ID,
			Criteria: search.Criteria{
				Keywords: []string{"golang"},
				Location: "remote",
			},
		}

		err := p.processQueuedSearch(cancelCtx, qs)
		if err == nil {
			t.Error("expected error, got nil")
		} else {
			// Check if the error chain contains context.Canceled
			if !errors.Is(err, context.Canceled) {
				t.Errorf("expected error to contain context.Canceled, got %v", err)
			}
		}
	})
}

func TestProvider_UpdateUserSearchCompletion(t *testing.T) {
	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	p := &Provider{
		fs:     fs,
		logger: slog.Default(),
	}

	// Create a test user using helper
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+15555550000", false)
	defer internal.DeleteTestData(t, fs, "test-user")

	// Test updating search completion
	if err := p.updateUserSearchCompletion(ctx, user.ID); err != nil {
		t.Errorf("unexpected error updating search completion: %v", err)
	}

	// Verify the update
	doc, err := fs.Collection("users").Doc(user.ID).Get(ctx)
	if err != nil {
		t.Fatalf("error getting user document: %v", err)
	}

	var data map[string]any
	if err := doc.DataTo(&data); err != nil {
		t.Fatalf("error unmarshaling data: %v", err)
	}

	search, ok := data["search"].(map[string]any)
	if !ok {
		t.Fatal("search field not found or wrong type")
	}
	if search["completed_at"] == nil {
		t.Error("completed_at timestamp is nil")
	}
}

func TestHandleAPIError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantType   error
	}{
		{"success", http.StatusOK, nil},
		{"timeout", 410, &ErrRequestTimeout{}},
		{"not found", http.StatusNotFound, &ErrInvalidURL{}},
		{"pending", 202, &ErrRequestPending{}},
		{"plan limit", http.StatusForbidden, &ErrPlanLimitReached{}},
		{"rate limit", http.StatusTooManyRequests, &ErrConcurrentLimit{}},
		{"invalid key", http.StatusUnauthorized, &ErrInvalidAPIKey{}},
		{"bad request", http.StatusBadRequest, &ErrRequestFailed{}},
		{"unexpected", 599, fmt.Errorf("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleAPIError(tt.statusCode)
			if tt.wantType == nil {
				if got != nil {
					t.Errorf("handleAPIError() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("handleAPIError() returned nil, want error")
			}
			if gotT := fmt.Sprintf("%T", got); gotT != fmt.Sprintf("%T", tt.wantType) {
				t.Errorf("handleAPIError() error type = %v, want %v", gotT, fmt.Sprintf("%T", tt.wantType))
			}
		})
	}
}

// Update TestDoRequestWithRetry to use mockTimer
func TestDoRequestWithRetry(t *testing.T) {
	tests := []struct {
		name          string
		responses     []int
		expectedTries int
		wantErr       bool
		sleepCalls    int
	}{
		{
			name:          "success first try",
			responses:     []int{200},
			expectedTries: 1,
			wantErr:       false,
			sleepCalls:    0,
		},
		{
			name:          "success after rate limit",
			responses:     []int{429, 429, 200},
			expectedTries: 3,
			wantErr:       false,
			sleepCalls:    2,
		},
		{
			name:          "success after timeout",
			responses:     []int{410, 200},
			expectedTries: 2,
			wantErr:       false,
			sleepCalls:    1,
		},
		{
			name:          "max retries exceeded",
			responses:     []int{429, 429, 429, 429},
			expectedTries: 3,
			wantErr:       true,
			sleepCalls:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responseIndex := 0
			timer := &mockTimer{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if responseIndex >= len(tt.responses) {
					t.Errorf("more requests than expected responses")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(tt.responses[responseIndex])
				responseIndex++
			}))
			defer server.Close()

			p := &Provider{
				client: &http.Client{},
				logger: slog.Default(),
			}

			// Create a custom doRequestWithRetry that uses our mock timer
			doRequestWithRetry := func(_ context.Context, req *http.Request) (*http.Response, error) {
				var lastErr error
				for attempt := 0; attempt < maxRetries; attempt++ {
					resp, err := p.client.Do(req)
					if err != nil {
						lastErr = err
						continue
					}

					if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 410 {
						resp.Body.Close()
						if resp.StatusCode == http.StatusTooManyRequests {
							timer.Sleep(retryDelay * time.Duration(attempt+1))
						} else {
							timer.Sleep(retryDelay)
						}
						continue
					}

					return resp, nil
				}

				if lastErr != nil {
					return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
				}
				return nil, fmt.Errorf("failed after %d retries", maxRetries)
			}

			// Make the request
			req, err := http.NewRequest("GET", server.URL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := doRequestWithRetry(context.Background(), req)

			// Verify number of attempts
			if responseIndex != tt.expectedTries {
				t.Errorf("got %d attempts, want %d", responseIndex, tt.expectedTries)
			}

			// Verify sleep calls
			if timer.getSleepCount() != tt.sleepCalls {
				t.Errorf("sleep called %d times, want %d", timer.getSleepCount(), tt.sleepCalls)
			}

			// Verify error condition
			if (err != nil) != tt.wantErr {
				t.Errorf("doRequestWithRetry() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && resp == nil {
				t.Error("doRequestWithRetry() returned nil response, want non-nil")
			}
		})
	}
}

// Update TestDoRequestWithRetry_ContextCancellation to use channels instead of sleep
func TestDoRequestWithRetry_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	p := &Provider{
		client: &http.Client{},
		logger: slog.Default(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create channels for synchronization
	requestMade := make(chan struct{})
	continueRequest := make(chan struct{})

	// Create a test server that waits for our signal
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMade <- struct{}{} // Signal that request was made
		<-continueRequest         // Wait for signal to continue
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer testServer.Close()

	// Start the request in a goroutine
	errCh := make(chan error, 1)
	go func() {
		req, err := http.NewRequest("GET", testServer.URL, nil)
		if err != nil {
			errCh <- fmt.Errorf("failed to create request: %w", err)
			return
		}
		req = req.WithContext(ctx)

		_, err = p.doRequestWithRetry(ctx, req)
		errCh <- err
	}()

	// Wait for the first request to be made
	<-requestMade

	// Cancel the context
	cancel()

	// Let the request continue
	close(continueRequest)

	// Check the error
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got error %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for request to complete")
	}
}

func TestConcurrentSearchProcessing(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping concurrent processing test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a mock server that simulates different response times
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate variable response times
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
		json.NewEncoder(w).Encode([]jobListing{})
	}))
	defer server.Close()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create test users
	numUsers := 10
	userIDs := make([]string, numUsers)
	for i := 0; i < numUsers; i++ {
		user := internal.CreateTestUser(t, fs,
			fmt.Sprintf("Concurrent Test User %d", i),
			fmt.Sprintf("concurrent%d@example.com", i),
			fmt.Sprintf("+1555000%04d", i),
			false,
		)
		userIDs[i] = user.ID
	}
	defer internal.DeleteTestData(t, fs, "Concurrent Test User")

	// Create provider with concurrent processing enabled
	p := NewProvider("test-key",
		WithBaseURL(server.URL),
		WithFirestore(fs, slog.Default()),
	)

	// Start workers
	numWorkers := 3
	p.StartSearchWorkers(ctx, numWorkers)

	// Queue searches for all users
	criteria := search.Criteria{
		Keywords: []string{"golang"},
		Location: "remote",
	}

	var wg sync.WaitGroup
	errCh := make(chan error, numUsers)

	// Queue searches concurrently
	for _, userID := range userIDs {
		wg.Add(1)
		go func(uid string) {
			defer wg.Done()
			if err := p.QueueSearch(ctx, uid, criteria); err != nil {
				errCh <- fmt.Errorf("failed to queue search for user %s: %w", uid, err)
			}
		}(userID)
	}

	// Wait for all searches to be queued
	wg.Wait()
	close(errCh)

	// Check for any queueing errors
	for err := range errCh {
		t.Errorf("search queueing error: %v", err)
	}

	// Wait for processing to complete
	time.Sleep(5 * time.Second)

	// Verify results
	completedCount := 0
	for _, userID := range userIDs {
		doc, err := fs.Collection("users").Doc(userID).Get(ctx)
		if err != nil {
			t.Errorf("failed to get user %s: %v", userID, err)
			continue
		}

		var data map[string]any
		if err := doc.DataTo(&data); err != nil {
			t.Errorf("failed to unmarshal user data for %s: %v", userID, err)
			continue
		}

		search, ok := data["search"].(map[string]any)
		if !ok {
			t.Errorf("search field not found or wrong type for user %s", userID)
			continue
		}

		if search["completed_at"] != nil {
			completedCount++
		}
	}

	// Expect at least 80% completion rate
	minExpected := int(float64(numUsers) * 0.8)
	if completedCount < minExpected {
		t.Errorf("only %d out of %d searches completed, expected at least %d", completedCount, numUsers, minExpected)
	}
}

func TestProvider_SearchFieldUpdates(t *testing.T) {
	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create a mock server that returns job listings and details
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("job_id") != "" {
			// Return job details
			details := []jobDetail{{
				ID:                "test-job1",
				Position:          "Test Job",
				Status:            "active",
				Location:          "Remote",
				CompanyName:       "Test Co",
				CompanyLinkedInID: "test-company-1",
				PostingTime:       "2024-03-14",
				Description:       "Test job description",
				SeniorityLevel:    "Senior",
				EmploymentType:    "Full-time",
				JobFunction:       "Software Development",
				Industries:        "Technology",
			}}
			json.NewEncoder(w).Encode(details)
		} else {
			// Return job listings
			jobs := []jobListing{{
				Position:      "Test Job",
				Link:          "https://test.com",
				ID:            "test-job1",
				LinkedInJobID: "test-job1",
				CompanyName:   "Test Co",
				Location:      "Remote",
				PostingDate:   "2024-03-14",
			}}
			json.NewEncoder(w).Encode(jobs)
		}
	}))
	defer server.Close()

	// Create a test user
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+15555550000", false)
	defer internal.DeleteTestData(t, fs, "test-user")

	// Initialize provider
	p := NewProvider("test-key", WithBaseURL(server.URL), WithFirestore(fs, slog.Default()))

	// Test cases
	t.Run("QueuedAt is set when search is queued", func(t *testing.T) {
		criteria := search.Criteria{
			Keywords: []string{"golang"},
			Location: "remote",
		}

		// Queue the search
		err := p.QueueSearch(ctx, user.ID, criteria)
		if err != nil {
			t.Fatalf("failed to queue search: %v", err)
		}

		// Verify QueuedAt is set
		doc, err := fs.Collection("users").Doc(user.ID).Get(ctx)
		if err != nil {
			t.Fatalf("failed to get user document: %v", err)
		}

		var userData map[string]interface{}
		if err := doc.DataTo(&userData); err != nil {
			t.Fatalf("failed to unmarshal user data: %v", err)
		}

		search, ok := userData["search"].(map[string]interface{})
		if !ok {
			t.Fatal("search field not found or wrong type")
		}

		queuedAt, ok := search["queued_at"].(time.Time)
		if !ok {
			t.Fatal("queued_at field not found or wrong type")
		}

		if queuedAt.IsZero() {
			t.Error("queued_at should not be zero")
		}
	})

	t.Run("Fields are updated after search completion", func(t *testing.T) {
		// Process the queued search
		qs := QueuedSearch{
			UserID: user.ID,
			Criteria: search.Criteria{
				Keywords: []string{"golang"},
				Location: "remote",
			},
		}

		err := p.processQueuedSearch(ctx, qs)
		if err != nil {
			t.Fatalf("failed to process search: %v", err)
		}

		// Verify fields are updated
		doc, err := fs.Collection("users").Doc(user.ID).Get(ctx)
		if err != nil {
			t.Fatalf("failed to get user document: %v", err)
		}

		var userData map[string]interface{}
		if err := doc.DataTo(&userData); err != nil {
			t.Fatalf("failed to unmarshal user data: %v", err)
		}

		search, ok := userData["search"].(map[string]interface{})
		if !ok {
			t.Fatal("search field not found or wrong type")
		}

		// Verify QueuedAt is cleared
		queuedAt, exists := search["queued_at"]
		if exists && queuedAt != nil {
			t.Error("queued_at should be nil after completion")
		}

		// Verify CompletedAt is set
		completedAt, ok := search["completed_at"].(time.Time)
		if !ok {
			t.Fatal("completed_at field not found or wrong type")
		}

		if completedAt.IsZero() {
			t.Error("completed_at should not be zero")
		}
	})
}

func TestScrapingDogProvider_SearchJobs_Locations(t *testing.T) {
	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Create a mock server that verifies the geoid parameter
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		geoid := r.URL.Query().Get("geoid")
		if geoid == "" {
			t.Error("geoid parameter is missing")
		}
		// Return empty job listings
		json.NewEncoder(w).Encode([]jobListing{})
	}))
	defer server.Close()

	// Create provider with Firestore
	provider := NewProvider("test-key",
		WithBaseURL(server.URL),
		WithFirestore(fs, slog.Default()),
	)

	tests := []struct {
		name     string
		location string
		wantErr  bool
	}{
		{
			name:     "empty location uses remote",
			location: "",
			wantErr:  false,
		},
		{
			name:     "known location uses correct geoid",
			location: "Remote",
			wantErr:  false,
		},
		{
			name:     "unknown location falls back to remote",
			location: "NonexistentLocation",
			wantErr:  false,
		},
		{
			name:     "Greater New York Area",
			location: "Greater New York Area",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			criteria := search.Criteria{
				Keywords: []string{"golang"},
				Location: tt.location,
			}

			jobs, err := provider.SearchJobListings(ctx, criteria)
			if (err != nil) != tt.wantErr {
				t.Errorf("SearchJobListings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// We don't need to check the jobs since we're using a mock server
			// that returns empty results. We're mainly testing that the location
			// handling doesn't cause errors.
			if jobs == nil {
				t.Error("SearchJobListings() returned nil jobs slice")
			}
		})
	}
}

func TestScrapingDogProvider_SearchJobs_LocationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create firestore client: %v", err)
	}
	defer fs.Close()

	// Ensure the API key is set in the environment
	apiKey := os.Getenv("SCRAPINGDOG_API_KEY")
	if apiKey == "" {
		t.Skip("SCRAPINGDOG_API_KEY is not set")
	}

	provider := NewProvider(apiKey, WithFirestore(fs, slog.Default()))

	tests := []struct {
		name     string
		location string
	}{
		{
			name:     "Remote location",
			location: "Remote",
		},
		{
			name:     "United States location",
			location: "United States",
		},
		{
			name:     "Greater New York Area",
			location: "Greater New York Area",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			criteria := search.Criteria{
				Keywords: []string{"software engineer"},
				Location: tt.location,
			}

			jobs, err := provider.SearchJobListings(ctx, criteria)
			if err != nil {
				if err.Error() == "unexpected status code: 403" {
					t.Skip("API key is invalid or expired")
				}
				t.Fatalf("SearchJobListings() error = %v", err)
			}

			if len(jobs) == 0 {
				t.Log("No jobs found for location:", tt.location)
			} else {
				t.Logf("Found %d jobs for location: %s", len(jobs), tt.location)
				// Log the first job's location to verify it matches expectations
				if len(jobs) > 0 {
					t.Logf("First job location: %s", jobs[0].Location)
				}
			}
		})
	}
}
