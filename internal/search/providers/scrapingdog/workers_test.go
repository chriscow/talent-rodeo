package scrapingdog

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"talent-rodeo/internal/search"

	"github.com/stretchr/testify/assert"
)

// MockSearchProvider implements the SearchProvider interface for testing
type MockSearchProvider struct {
	searchJobsCalled    bool
	getJobDetailsCalled bool
	mockJobs            []search.JobListing
	mockJobDetail       search.JobDetail
	searchJobsError     error
	getJobDetailsError  error
	searchJobsFunc      func(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error)
	getJobDetailsFunc   func(ctx context.Context, jobID string) (search.JobDetail, error)
}

func (m *MockSearchProvider) SearchJobListings(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	m.searchJobsCalled = true
	if m.searchJobsFunc != nil {
		return m.searchJobsFunc(ctx, criteria)
	}
	return m.mockJobs, m.searchJobsError
}

func (m *MockSearchProvider) GetJobDetails(ctx context.Context, jobID string) (search.JobDetail, error) {
	m.getJobDetailsCalled = true
	if m.getJobDetailsFunc != nil {
		return m.getJobDetailsFunc(ctx, jobID)
	}
	return m.mockJobDetail, m.getJobDetailsError
}

// MockJobStore implements the JobStore interface for testing
type MockJobStore struct {
	getCachedJobsCalled  bool
	saveJobsCalled       bool
	getJobDetailsCalled  bool
	saveJobDetailsCalled bool
	mockCachedJobs       []search.JobListing
	mockJobDetails       *search.JobDetail
	getCachedJobsError   error
	saveJobsError        error
	getJobDetailsError   error
	saveJobDetailsError  error
}

func (m *MockJobStore) GetCachedJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
	m.getCachedJobsCalled = true
	return m.mockCachedJobs, m.getCachedJobsError
}

func (m *MockJobStore) SaveJobs(ctx context.Context, criteria search.Criteria, jobs []search.JobListing) error {
	m.saveJobsCalled = true
	return m.saveJobsError
}

func (m *MockJobStore) GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {
	m.getJobDetailsCalled = true
	return m.mockJobDetails, m.getJobDetailsError
}

func (m *MockJobStore) SaveJobDetails(ctx context.Context, jobDetail search.JobDetail) error {
	m.saveJobDetailsCalled = true
	return m.saveJobDetailsError
}

// MockEventHandler implements the JobEventHandler interface for testing
type MockEventHandler struct {
	onNewJobCalled          bool
	onSearchCompletedCalled bool
	onNewJobError           error
	onSearchCompletedError  error
	jobsReceived            []string
	mutex                   sync.Mutex
}

func (m *MockEventHandler) OnNewJob(userID string, criteria search.Criteria, jobID string, details search.JobDetail) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onNewJobCalled = true
	m.jobsReceived = append(m.jobsReceived, jobID)
	return m.onNewJobError
}

func (m *MockEventHandler) OnSearchCompleted(userID string, criteria search.Criteria, totalJobs int, errors []error) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onSearchCompletedCalled = true
	return m.onSearchCompletedError
}

func TestNewWorkerManager(t *testing.T) {
	mockSearchProvider := &MockSearchProvider{}
	maxConcurrent := 3
	queueSize := 5

	wm := NewWorkerManager(mockSearchProvider, maxConcurrent, queueSize)

	assert.NotNil(t, wm)
	assert.Equal(t, mockSearchProvider, wm.provider)
	assert.Equal(t, maxConcurrent, cap(wm.semaphore))
	assert.Equal(t, queueSize, cap(wm.searchQueue))
}

func TestWorkerManagerWithOptions(t *testing.T) {
	mockSearchProvider := &MockSearchProvider{}
	mockStore := &MockJobStore{}
	mockHandler := &MockEventHandler{}
	mockLogger := slog.Default()

	wm := NewWorkerManager(
		mockSearchProvider, 3, 5,
		WithWorkerLogger(mockLogger),
		WithJobStore(mockStore),
		WithEventHandler(mockHandler),
	)

	assert.NotNil(t, wm)
	assert.Equal(t, mockSearchProvider, wm.provider)
	assert.Equal(t, mockStore, wm.store)
	assert.Equal(t, mockHandler, wm.eventHandler)
	assert.Equal(t, mockLogger, wm.logger)
}

func TestQueueSearchAndGetQueueDepth(t *testing.T) {
	mockSearchProvider := &MockSearchProvider{}
	wm := NewWorkerManager(mockSearchProvider, 3, 5)

	// Initial queue should be empty
	assert.Equal(t, 0, wm.GetQueueDepth())

	// Add a search to the queue
	ctx := context.Background()
	err := wm.QueueSearch(ctx, "user123", search.Criteria{Query: "golang"})
	assert.NoError(t, err)
	assert.Equal(t, 1, wm.GetQueueDepth())

	// Add another search
	err = wm.QueueSearch(ctx, "user456", search.Criteria{Query: "python"})
	assert.NoError(t, err)
	assert.Equal(t, 2, wm.GetQueueDepth())
}

func TestWorkerManagerStartAndShutdown(t *testing.T) {
	mockSearchProvider := &MockSearchProvider{
		mockJobs: []search.JobListing{
			{ID: "job1", Title: "Job 1"},
			{ID: "job2", Title: "Job 2"},
		},
		mockJobDetail: search.JobDetail{ID: "job1", Title: "Job 1 Detail"},
	}

	mockHandler := &MockEventHandler{}

	wm := NewWorkerManager(
		mockSearchProvider, 3, 5,
		WithEventHandler(mockHandler),
	)

	assert.NotNil(t, wm.eventHandler)

	// Start workers
	ctx := context.Background()
	wm.Start(ctx, 2)

	// Queue a search
	err := wm.QueueSearch(ctx, "user123", search.Criteria{Query: "golang"})
	assert.NoError(t, err)

	// Give workers time to process
	time.Sleep(50 * time.Millisecond)

	// Shutdown
	err = wm.Shutdown(ctx)
	assert.NoError(t, err)

	// Verify that the queue is empty after processing
	assert.Equal(t, 0, wm.GetQueueDepth())
}

func TestProcessQueuedSearch(t *testing.T) {
	// Create mock jobs
	mockJobs := []search.JobListing{
		{ID: "job1", Title: "Job 1", ProviderJobID: "prov-job1"},
		{ID: "job2", Title: "Job 2", ProviderJobID: "prov-job2"},
	}

	mockJobDetail := search.JobDetail{ID: "job1", Title: "Job 1 Detail"}

	mockSearchProvider := &MockSearchProvider{
		mockJobs:      mockJobs,
		mockJobDetail: mockJobDetail,
	}

	// Set up a custom searchJobsFunc that only returns jobs on the first call
	var searchCalled bool
	mockSearchProvider.searchJobsFunc = func(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {
		if searchCalled {
			return []search.JobListing{}, nil
		}
		searchCalled = true
		return mockJobs, nil
	}

	// Set up getJobDetailsFunc to track calls for each job
	jobDetailsCalled := make(map[string]bool)
	mockSearchProvider.getJobDetailsFunc = func(ctx context.Context, jobID string) (search.JobDetail, error) {
		jobDetailsCalled[jobID] = true
		return search.JobDetail{ID: jobID, Title: "Details for " + jobID}, nil
	}

	// Create a mock event handler to track callbacks
	mockHandler := &MockEventHandler{}

	// Create a worker manager with our mocks
	wm := NewWorkerManager(
		mockSearchProvider, 3, 5,
		WithEventHandler(mockHandler),
	)

	// Create test criteria
	criteria := search.Criteria{
		Query:    "golang",
		Location: "remote",
	}

	// Process a search
	ctx := context.Background()
	err := wm.processQueuedSearch(ctx, QueuedSearch{
		UserID:   "user123",
		Criteria: criteria,
	})

	// Check that the process completed without error
	assert.NoError(t, err)

	// Verify the provider methods were called
	assert.True(t, mockSearchProvider.searchJobsCalled, "SearchJobs should have been called")
	assert.True(t, mockSearchProvider.getJobDetailsCalled, "GetJobDetails should have been called")

	// Verify the event handler methods were called
	assert.True(t, mockHandler.onNewJobCalled, "OnNewJob should have been called")
	assert.True(t, mockHandler.onSearchCompletedCalled, "OnSearchCompleted should have been called")

	// Verify jobs were received
	assert.Equal(t, 2, len(mockHandler.jobsReceived), "Should have received 2 jobs")
}

func TestProcessQueuedSearchWithErrors(t *testing.T) {
	// Test with search error
	mockSearchProvider := &MockSearchProvider{
		searchJobsError: assert.AnError,
	}

	mockHandler := &MockEventHandler{}

	wm := NewWorkerManager(
		mockSearchProvider, 3, 5,
		WithEventHandler(mockHandler),
	)

	ctx := context.Background()
	err := wm.processQueuedSearch(ctx, QueuedSearch{
		UserID:   "user123",
		Criteria: search.Criteria{Query: "golang"},
	})

	assert.NoError(t, err, "Process should complete even with search errors")
	assert.True(t, mockSearchProvider.searchJobsCalled, "SearchJobs should have been called")
	assert.True(t, mockHandler.onSearchCompletedCalled, "OnSearchCompleted should have been called")
}

func TestQueueSearchWhenShuttingDown(t *testing.T) {
	mockSearchProvider := &MockSearchProvider{}
	wm := NewWorkerManager(mockSearchProvider, 3, 5)

	// Set shutdown flag
	wm.shutdownFlag = 1

	// Try to queue a search
	ctx := context.Background()
	err := wm.QueueSearch(ctx, "user123", search.Criteria{Query: "golang"})

	// Should return an error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutting down")
}

func TestQueueSearchWhenQueueIsFull(t *testing.T) {
	mockSearchProvider := &MockSearchProvider{}

	// Create a worker manager with a small queue
	wm := NewWorkerManager(mockSearchProvider, 3, 1)

	// Fill the queue
	ctx := context.Background()
	err := wm.QueueSearch(ctx, "user123", search.Criteria{Query: "golang"})
	assert.NoError(t, err)

	// Try to add another search to the full queue with a timeout context
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	err = wm.QueueSearch(ctxWithTimeout, "user456", search.Criteria{Query: "python"})

	// Should return an error
	assert.Error(t, err)
}
