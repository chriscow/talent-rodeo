package scrapingdog

import (
	"context"
	"fmt"
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
} // MockEventHandler implements the JobEventHandler interface for testing
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
	assert.Equal(t, queueSize*10, cap(wm.taskQueue))
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

func TestQueueTaskAndGetQueueDepth(t *testing.T) {
	mockSearchProvider := &MockSearchProvider{}
	wm := NewWorkerManager(mockSearchProvider, 3, 5)

	// Initial queue should be empty
	assert.Equal(t, 0, wm.GetQueueDepth())

	// Add a search task to the queue
	ctx := context.Background()
	searchTask := WorkerTask{
		Type:       TaskTypeSearch,
		SearchTask: &internal.SearchTask{Criteria: internal.SearchCriteria{Query: "golang"}},
	}
	err := wm.QueueTask(ctx, searchTask)
	assert.NoError(t, err)
	assert.Equal(t, 1, wm.GetQueueDepth())

	// Add another search task
	searchTask2 := WorkerTask{
		Type:       TaskTypeSearch,
		SearchTask: &internal.SearchTask{Criteria: internal.SearchCriteria{Query: "python"}},
	}
	err = wm.QueueTask(ctx, searchTask2)
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

	// Queue a search task
	searchTask := WorkerTask{
		Type:       TaskTypeSearch,
		SearchTask: &internal.SearchTask{Criteria: internal.SearchCriteria{Query: "golang"}},
	}
	err := wm.QueueTask(ctx, searchTask)
	assert.NoError(t, err)

	// Give workers time to process
	time.Sleep(50 * time.Millisecond)

	// Shutdown
	err = wm.Shutdown(ctx)
	assert.NoError(t, err)

	// Verify that the queue is empty after processing
	assert.Equal(t, 0, wm.GetQueueDepth())
}

func TestHandleSearchTask(t *testing.T) {
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

	// Process a search directly using the internal method
	ctx := context.Background()
	wm.handleSearchTask(ctx, internal.SearchTask{Criteria: criteria})

	// Verify the provider methods were called
	assert.True(t, mockSearchProvider.searchJobsCalled, "SearchJobs should have been called")

	// Verify the event handler methods were called
	assert.True(t, mockHandler.onSearchCompletedCalled, "OnSearchCompleted should have been called")

	// Verify that job detail tasks were enqueued (check queue depth)
	assert.Greater(t, wm.GetQueueDepth(), 0, "Job detail tasks should be enqueued")
}

func TestHandleSearchTaskWithErrors(t *testing.T) {
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
	wm.handleSearchTask(ctx, internal.SearchTask{Criteria: internal.SearchCriteria{Query: "golang"}})

	assert.True(t, mockSearchProvider.searchJobsCalled, "SearchJobs should have been called")
	assert.True(t, mockHandler.onSearchCompletedCalled, "OnSearchCompleted should have been called")
}

func TestQueueTaskWhenShuttingDown(t *testing.T) {
	mockSearchProvider := &MockSearchProvider{}
	wm := NewWorkerManager(mockSearchProvider, 3, 5)

	// Set shutdown flag
	wm.shutdownFlag = 1

	// Try to queue a search task
	ctx := context.Background()
	searchTask := WorkerTask{
		Type:       TaskTypeSearch,
		SearchTask: &internal.SearchTask{Criteria: internal.SearchCriteria{Query: "golang"}},
	}
	err := wm.QueueTask(ctx, searchTask)

	// Should return an error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutting down")
}

func TestQueueTaskWhenQueueIsFull(t *testing.T) {
	mockSearchProvider := &MockSearchProvider{}

	// Create a worker manager with a small queue (actual capacity will be 1*10 = 10)
	wm := NewWorkerManager(mockSearchProvider, 3, 1)

	ctx := context.Background()

	// Fill the queue completely (capacity is queueSize*10 = 10)
	for i := 0; i < 10; i++ {
		searchTask := WorkerTask{
			Type:       TaskTypeSearch,
			SearchTask: &internal.SearchTask{Criteria: internal.SearchCriteria{Query: fmt.Sprintf("job%d", i)}},
		}
		err := wm.QueueTask(ctx, searchTask)
		assert.NoError(t, err)
	}

	// Now the queue should be full, try to add another task with a timeout context
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	searchTask := WorkerTask{
		Type:       TaskTypeSearch,
		SearchTask: &internal.SearchTask{Criteria: internal.SearchCriteria{Query: "overflow"}},
	}
	err := wm.QueueTask(ctxWithTimeout, searchTask)

	// Should return an error due to timeout or queue full
	assert.Error(t, err)
}
