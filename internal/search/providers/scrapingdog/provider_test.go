package scrapingdog

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/search"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock HTTP Server for API
type MockScrapingDogServer struct {
	server            *httptest.Server
	jobListingsJSON   string
	jobDetailsJSON    string
	requestedJobID    string
	searchesPerformed int
	detailsRetrieved  int
	logger            *slog.Logger
}

func NewMockScrapingDogServer() *MockScrapingDogServer {
	logger := slog.Default().With("component", "mock_server")

	m := &MockScrapingDogServer{
		jobListingsJSON: `[
			{
				"id": "test-id-1",
				"job_id": "job-1",
				"job_position": "Go Developer",
				"job_link": "https://example.com/jobs/1",
				"company_name": "Test Company",
				"company_profile": "https://example.com/company/1",
				"job_location": "Remote",
				"job_posting_date": "1d ago",
				"company_logo_url": "https://example.com/logo.png"
			},
			{
				"id": "test-id-2",
				"job_id": "job-2",
				"job_position": "Senior Go Developer",
				"job_link": "https://example.com/jobs/2",
				"company_name": "Another Company",
				"company_profile": "https://example.com/company/2",
				"job_location": "New York",
				"job_posting_date": "2d ago",
				"company_logo_url": "https://example.com/logo2.png"
			}
		]`,
		jobDetailsJSON: `[{
			"id": "test-job-1",
			"job_position": "Go Developer",
			"jobs_status": "active",
			"job_location": "Remote",
			"company_name": "Test Company",
			"company_linkedin_id": "123456",
			"job_posting_time": "1d ago",
			"base_pay": "$80K-$120K",
			"job_description": "This is a test job description",
			"Seniority_level": "Mid-Senior level",
			"Employment_type": "Full-time",
			"Job_function": "Engineering",
			"Industries": "Software",
			"recruiter_details": [
				{
					"recruiter_photo": "https://example.com/photo.jpg",
					"recruiter_name": "John Recruiter",
					"recruiter_title": "Talent Acquisition",
					"recruiter_profile_url": "https://example.com/recruiter/1"
				}
			],
			"similar_jobs": [
				{
					"job_position": "Similar Job",
					"job_company": "Similar Company",
					"job_location": "Remote",
					"job_posting_time": "2d ago",
					"job_link": "https://example.com/similar/1"
				}
			],
			"people_also_viewed": [
				{
					"job_position": "Viewed Job",
					"job_company": "Viewed Company",
					"job_location": "Remote",
					"job_posting_time": "3d ago",
					"job_link": "https://example.com/viewed/1"
				}
			]
		}]`,
		logger: logger,
	}

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log incoming request
		fmt.Printf("Mock server received request: %s %s\n", r.Method, r.URL.String())

		// Basic auth check
		apiKey := r.URL.Query().Get("api_key")
		if apiKey == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Log the incoming request for debugging
		m.logger.Info("Mock server received request",
			"path", r.URL.Path,
			"query", r.URL.RawQuery)

		// Check if this is a job details request by exact path match
		if r.URL.Path == "/linkedinjobs" {
			jobID := r.URL.Query().Get("job_id")
			m.requestedJobID = jobID
			m.detailsRetrieved++

			// Ensure Content-Type is set before writing response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(m.jobDetailsJSON))
			m.logger.Info("Mock server returned job details", "jobID", jobID)
			return
		}

		// Default to job listings
		m.searchesPerformed++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(m.jobListingsJSON))
		m.logger.Info("Mock server returned job listings")
	}))

	return m
}

func (m *MockScrapingDogServer) Close() {
	m.server.Close()
}

func (m *MockScrapingDogServer) URL() string {
	return m.server.URL
}

// Tests require a real Firestore client
func setupTestProvider(_ *testing.T) (*Provider, *MockScrapingDogServer, func()) {
	// Create mock server
	mockServer := NewMockScrapingDogServer()

	// Create HTTP client
	httpClient := &http.Client{Timeout: 5 * time.Second}

	// Create provider with mock server URL - ensure it has the correct full URL
	provider := NewProvider(
		"test-api-key",
		WithBaseURL(mockServer.URL()), // This passes the full mock server URL
		WithHTTPClient(httpClient),
		WithProviderLogger(slog.Default()),
	)

	cleanup := func() {
		mockServer.Close()
	}

	return provider, mockServer, cleanup
}

// Integration test with real Firestore client
func TestProviderWithRealFirestore(t *testing.T) {
	// Skip if running in CI environment without credentials
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := godotenv.Load("../../../../.env"); err != nil {
		t.Fatalf("Failed to load .env file: %v", err)
	}

	// Create mock server
	mockServer := NewMockScrapingDogServer()
	defer mockServer.Close()

	// Get Firestore client
	ctx := context.Background()
	fs, err := getFirestoreClient(t, ctx)
	if err != nil {
		t.Skipf("Skipping test due to Firestore client error: %v", err)
		return
	}
	defer fs.Close()

	// Create HTTP client
	httpClient := &http.Client{Timeout: 5 * time.Second}

	// Create event handler mock
	mockHandler := &MockEventHandler{}

	// Create provider with real Firestore
	provider := NewProvider(
		"test-api-key",
		WithBaseURL(mockServer.URL()),
		WithHTTPClient(httpClient),
		WithFirestore(fs),
		WithJobEventHandler(mockHandler),
		WithProviderLogger(slog.Default()),
	)

	// Search for jobs
	criteria := search.Criteria{
		Query:    "golang",
		Location: "remote",
	}

	listings, err := provider.SearchJobListings(ctx, criteria)
	require.NoError(t, err)
	assert.Len(t, listings, 2, "Should have found 2 jobs")

	// Get job details using the raw provider job ID
	jobID := listings[0].ProviderJobID
	details, err := provider.GetJobDetails(ctx, jobID)
	require.NoError(t, err)

	// The job detail ID should be scrapingdog-{raw_id}
	expectedDetailID := CreateProviderJobID(jobID)
	assert.Equal(t, expectedDetailID, details.ID)
	assert.Equal(t, jobID, details.ProviderJobID)
	assert.Equal(t, ProviderName, details.Provider)
}

// Test job search
func TestProviderSearchJobs(t *testing.T) {
	provider, mockServer, cleanup := setupTestProvider(t)
	defer cleanup()

	// Search for jobs
	ctx := context.Background()
	criteria := search.Criteria{
		Query:    "golang",
		Location: "remote",
	}

	jobs, err := provider.SearchJobListings(ctx, criteria)
	require.NoError(t, err)
	assert.Len(t, jobs, 2, "Should have found 2 jobs")
	assert.Equal(t, "Go Developer", jobs[0].Title)
	assert.Equal(t, "Test Company", jobs[0].Company)
	assert.Equal(t, "Remote", jobs[0].Location)
	assert.Equal(t, ProviderName, jobs[0].Provider)

	// Verify the search was performed
	assert.Equal(t, 1, mockServer.searchesPerformed)
}

func TestProviderGetJobDetails(t *testing.T) {
	// Use a custom logger for the test to capture output
	testLogger := slog.Default().With("test", "TestProviderGetJobDetails")

	// Setup the test provider with the custom logger
	provider, mockServer, cleanup := setupTestProvider(t)
	defer cleanup()

	// Override the provider's logger for this test
	provider.logger = testLogger

	// Get job details
	ctx := context.Background()
	jobID := "test-job-1"

	// Log the mock server URL to verify it's correctly set up
	t.Logf("Mock server URL: %s", mockServer.URL())

	// Execute the test
	details, err := provider.GetJobDetails(ctx, jobID)
	require.NoError(t, err)

	// Verify details
	assert.Equal(t, ProviderName, details.Provider)
	assert.Equal(t, "Go Developer", details.Title)
	assert.Equal(t, "Test Company", details.Company)
	assert.Equal(t, "Remote", details.Location)
	assert.Equal(t, "This is a test job description", details.FullDescription)

	// Verify salary info was parsed
	require.NotNil(t, details.SalaryInfo)
	assert.Equal(t, 80000.0, details.SalaryInfo.Min)
	assert.Equal(t, 120000.0, details.SalaryInfo.Max)
	assert.Equal(t, "USD", details.SalaryInfo.Currency)

	// Verify the job detail request was made
	assert.Equal(t, 1, mockServer.detailsRetrieved)
	assert.Equal(t, jobID, mockServer.requestedJobID)
}

func TestQueueSearch(t *testing.T) {
	provider, _, cleanup := setupTestProvider(t)
	defer cleanup()

	// Start workers
	ctx := context.Background()
	provider.StartWorkers(ctx, 1)
	defer provider.StopWorkers(ctx)

	// Queue a search
	err := provider.QueueSearch(ctx, "test-user", search.Criteria{
		Query:    "golang",
		Location: "remote",
	})

	require.NoError(t, err)
}

// Helper function to get a real Firestore client
func getFirestoreClient(t *testing.T, ctx context.Context) (*internal.FirestoreClient, error) {
	t.Helper()

	// Use the shared function from internal package
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		// Check if credentials are available
		t.Logf("Failed to initialize Firestore client: %v", err)
		return nil, err
	}

	return fs, nil
}
