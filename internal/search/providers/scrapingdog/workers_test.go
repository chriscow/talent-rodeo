package scrapingdog

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"talent-rodeo/internal"

	"github.com/stretchr/testify/assert"
)

// MockSearchProvider implements the SearchProvider interface for testing
type MockSearchProvider struct {
	searchJobsCalled    bool
	getJobDetailsCalled bool
	mockJobs            []internal.JobListing
	mockJobDetail       *internal.JobDetail
	searchJobsError     error
	getJobDetailsError  error
	searchJobsFunc      func(ctx context.Context, criteria internal.SearchCriteria, page int) ([]internal.JobListing, error)
	getJobDetailsFunc   func(ctx context.Context, jobID string) (*internal.JobDetail, error)
}

func (m *MockSearchProvider) SearchJobListings(ctx context.Context, criteria internal.SearchCriteria, page int) ([]internal.JobListing, error) {
	m.searchJobsCalled = true
	if m.searchJobsFunc != nil {
		return m.searchJobsFunc(ctx, criteria, page)
	}
	return m.mockJobs, m.searchJobsError
}

func (m *MockSearchProvider) GetJobDetails(ctx context.Context, jobID string) (*internal.JobDetail, error) {
	m.getJobDetailsCalled = true
	if m.getJobDetailsFunc != nil {
		return m.getJobDetailsFunc(ctx, jobID)
	}
	return m.mockJobDetail, m.getJobDetailsError
}

// MockEventHandler implements the JobEventHandler interface for testing
type MockEventHandler struct {
	onJobListingCalled      bool
	onNewJobCalled          bool
	onSearchCompletedCalled bool
	onSearchStartedCalled   bool
	jobsReceived            []string
	mutex                   sync.Mutex
}

func (m *MockEventHandler) OnSearchStarted(task internal.SearchTask) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onSearchStartedCalled = true
}

func (m *MockEventHandler) OnJobListing(search internal.SearchTask, listing internal.JobListing) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onJobListingCalled = true
	m.jobsReceived = append(m.jobsReceived, listing.ID)
}

func (m *MockEventHandler) OnJobDetail(search internal.SearchTask, details internal.JobDetail) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onNewJobCalled = true
	m.jobsReceived = append(m.jobsReceived, details.ID)
}

func (m *MockEventHandler) OnPageComplete(search internal.SearchTask, page int, jobsFound int, errors []error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onSearchCompletedCalled = true
	if len(errors) > 0 {
		m.jobsReceived = append(m.jobsReceived, "error")
	} else {
		m.jobsReceived = append(m.jobsReceived, "page_complete")
	}
}

func (m *MockEventHandler) OnSearchCompleted(search internal.SearchTask, totalJobs int, errors []error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onSearchCompletedCalled = true
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
	mockHandler := &MockEventHandler{}
	mockLogger := slog.Default()

	wm := NewWorkerManager(
		mockSearchProvider, 3, 5,
		withWorkerLogger(mockLogger),
		withEventHandler(mockHandler),
	)

	assert.NotNil(t, wm)
	assert.Equal(t, mockSearchProvider, wm.provider)
	assert.Equal(t, mockHandler, wm.eventHandler)
}

func TestQueueSearchAndGetQueueDepth(t *testing.T) {
	mockSearchProvider := &MockSearchProvider{}
	wm := NewWorkerManager(mockSearchProvider, 3, 5)

	// Initial queue should be empty
	assert.Equal(t, 0, wm.GetQueueDepth())

	// Add a search to the queue
	ctx := context.Background()
	err := wm.QueueSearch(ctx, internal.SearchTask{Criteria: internal.SearchCriteria{Query: "golang"}})
	assert.NoError(t, err)
	assert.Equal(t, 1, wm.GetQueueDepth())

	// Add another search
	err = wm.QueueSearch(ctx, internal.SearchTask{Criteria: internal.SearchCriteria{Query: "python"}})
	assert.NoError(t, err)
	assert.Equal(t, 2, wm.GetQueueDepth())
}

func TestWorkerManagerStartAndShutdown(t *testing.T) {
	mockSearchProvider := &MockSearchProvider{
		mockJobs: []internal.JobListing{
			{ID: "job1", Title: "Job 1"},
			{ID: "job2", Title: "Job 2"},
		},
		mockJobDetail: &internal.JobDetail{ID: "job1", Title: "Job 1 Detail"},
	}

	mockHandler := &MockEventHandler{}

	wm := NewWorkerManager(
		mockSearchProvider, 3, 5,
		withEventHandler(mockHandler),
	)

	assert.NotNil(t, wm.eventHandler)

	// Start workers
	ctx := context.Background()
	wm.Start(ctx, 2)

	// Queue a search
	err := wm.QueueSearch(ctx, internal.SearchTask{Criteria: internal.SearchCriteria{Query: "golang"}})
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
	mockJobs := []internal.JobListing{
		{ID: "job1", Title: "Job 1", ProviderJobID: "prov-job1"},
		{ID: "job2", Title: "Job 2", ProviderJobID: "prov-job2"},
	}

	mockJobDetail := internal.JobDetail{ID: "job1", Title: "Job 1 Detail"}

	mockSearchProvider := &MockSearchProvider{
		mockJobs:      mockJobs,
		mockJobDetail: &mockJobDetail,
	}

	// Set up a custom searchJobsFunc that only returns jobs on the first call
	var searchCalled bool
	mockSearchProvider.searchJobsFunc = func(ctx context.Context, criteria internal.SearchCriteria, page int) ([]internal.JobListing, error) {
		if searchCalled {
			return []internal.JobListing{}, nil
		}
		searchCalled = true
		return mockJobs, nil
	}

	// Set up getJobDetailsFunc to track calls for each job
	jobDetailsCalled := make(map[string]bool)
	mockSearchProvider.getJobDetailsFunc = func(ctx context.Context, jobID string) (*internal.JobDetail, error) {
		jobDetailsCalled[jobID] = true
		return &internal.JobDetail{ID: jobID, Title: "Details for " + jobID}, nil
	}

	// Create a mock event handler to track callbacks
	mockHandler := &MockEventHandler{}

	// Create a worker manager with our mocks
	wm := NewWorkerManager(
		mockSearchProvider, 3, 5,
		withEventHandler(mockHandler),
	)

	// Create test criteria
	criteria := internal.SearchCriteria{
		Query: "golang",
		GeoID: "remote",
	}

	// Process a search
	ctx := context.Background()
	err := wm.processQueuedSearch(ctx, 1, internal.SearchTask{Criteria: criteria})

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
		withEventHandler(mockHandler),
	)

	ctx := context.Background()
	err := wm.processQueuedSearch(ctx, 1, internal.SearchTask{Criteria: internal.SearchCriteria{Query: "golang"}})

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
	err := wm.QueueSearch(ctx, internal.SearchTask{Criteria: internal.SearchCriteria{Query: "golang"}})

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
	err := wm.QueueSearch(ctx, internal.SearchTask{Criteria: internal.SearchCriteria{Query: "golang"}})
	assert.NoError(t, err)

	// Try to add another search to the full queue with a timeout context
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	err = wm.QueueSearch(ctxWithTimeout, internal.SearchTask{Criteria: internal.SearchCriteria{Query: "python"}})

	// Should return an error
	assert.Error(t, err)
}
