package search

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func setupTestService(t *testing.T) (*Service, *firestore.Client) {
	t.Helper()

	// Setup Firestore client
	ctx := context.Background()
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}

	// Create a test service with logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	service := &Service{
		fs:                fs,
		logger:            logger,
		jobQueue:          make(chan QueuedJob, 5),
		jobSemaphore:      make(chan struct{}, 2),
		maxConcurrentJobs: 2,
	}

	return service, fs
}

func createTestJobDetail(t *testing.T, fs *firestore.Client) *search.JobDetail {
	t.Helper()

	// Generate a unique job ID for testing
	jobID := fmt.Sprintf("test-job-%d", time.Now().UnixNano())
	now := time.Now()

	// Create a job detail
	jobDetail := &search.JobDetail{
		ID:              jobID,
		Title:           "Software Engineer",
		Company:         "Test Company",
		Location:        "Remote",
		PostedDate:      now.Add(-24 * time.Hour),
		FullDescription: "This is a Software Engineer position at Test Company. The role involves developing and maintaining software applications.",
		Requirements:    []string{"5+ years of experience", "Strong knowledge of Go"},
		Qualifications:  []string{"Bachelor's degree in Computer Science or related field"},
		Skills:          []string{"Go", "Docker", "Kubernetes"},
		EmploymentType:  "Full-time",
		WorkplaceType:   "Remote",
		Industry:        "Technology",
		CompanyInfo: &search.Company{
			Name:        "Test Company",
			Description: "A great place to work",
			Industry:    "Technology",
			Size:        "50-100 employees",
			LogoURL:     "https://example.com/logo.png",
		},
		ExtractedData: &search.JobSummary{
			Summary: "A Software Engineer position at Test Company focused on Go development.",
			Qualifications: []string{
				"Bachelor's degree in Computer Science",
				"Experience with microservices architecture",
			},
			Requirements: []string{
				"5+ years of experience",
				"Go expertise",
			},
		},
		Status:         "active",
		SeniorityLevel: "Mid-Senior level",
		JobFunction:    "Engineering",
		CreatedAt:      now,
	}

	// Save the job detail
	ctx := context.Background()
	_, err := fs.Collection("job_details").Doc(jobDetail.ID).Set(ctx, jobDetail)
	if err != nil {
		t.Fatalf("Failed to create test job detail: %v", err)
	}

	return jobDetail
}

func TestLoadJobDetail(t *testing.T) {
	service, fs := setupTestService(t)
	defer fs.Close()
	defer internal.DeleteTestData(t, fs, "test-")

	// Create test job detail
	expected := createTestJobDetail(t, fs)

	// Test loading the job detail
	ctx := context.Background()
	actual, err := service.loadJobDetail(ctx, expected.ID)
	if err != nil {
		t.Fatalf("loadJobDetail() error = %v", err)
	}

	// Verify job details
	if actual.ID != expected.ID {
		t.Errorf("loadJobDetail() got ID = %v, want %v", actual.ID, expected.ID)
	}
	if actual.Title != expected.Title {
		t.Errorf("loadJobDetail() got Title = %v, want %v", actual.Title, expected.Title)
	}
	if actual.Company != expected.Company {
		t.Errorf("loadJobDetail() got Company = %v, want %v", actual.Company, expected.Company)
	}
}

func TestLoadUserJob(t *testing.T) {
	_, fs := setupTestService(t)
	defer fs.Close()
	defer internal.DeleteTestData(t, fs, "test-")

	// Create test user and job detail
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+1234567890", false)
	jobDetail := createTestJobDetail(t, fs)

	// First test - when job doesn't exist
	ctx := context.Background()
	_, found, err := internal.GetUserJob(ctx, fs, user.ID, jobDetail.ID)
	if found {
		t.Fatalf("loadUserJob() expected job to not be found, got found")
	}
	if err != nil {
		t.Fatalf("loadUserJob() error = %v", err)
	}

	// Create a user job to test loading
	userJob := &internal.UserJob{
		ID:     jobDetail.ID,
		JobRef: fs.Collection("job_details").Doc(jobDetail.ID),
		Status: "new",
	}

	// Save the user job
	if err := internal.SaveUserJob(ctx, fs, user.ID, userJob); err != nil {
		t.Fatalf("Failed to save user job: %v", err)
	}

	// Test loading existing job
	loadedJob, found, err := internal.GetUserJob(ctx, fs, user.ID, jobDetail.ID)
	if err != nil {
		t.Fatalf("loadUserJob() error = %v", err)
	}
	if !found {
		t.Fatalf("loadUserJob() expected job to be found, got not found")
	}

	// Verify loaded job
	if loadedJob.ID != userJob.ID {
		t.Errorf("loadUserJob() got ID = %v, want %v", loadedJob.ID, userJob.ID)
	}
	if loadedJob.Status != userJob.Status {
		t.Errorf("loadUserJob() got Status = %v, want %v", loadedJob.Status, userJob.Status)
	}
}

func TestJobDetailHasSummary(t *testing.T) {
	service, fs := setupTestService(t)
	defer fs.Close()
	defer internal.DeleteTestData(t, fs, "test-")

	// Create test user
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+1234567890", false)

	// Create a job detail with summary and qualifications
	jobDetail := createTestJobDetail(t, fs)
	jobDetail.ExtractedData = &search.JobSummary{
		Summary: "This is a Software Engineer position at Test Company. The role involves developing and maintaining software applications, with a focus on Go programming. Requirements include 5+ years of experience and strong knowledge of Go.",
		Qualifications: []string{
			"Bachelor's degree in Computer Science or related field",
			"Experience with microservices architecture",
		},
	}

	// Save the job detail
	ctx := context.Background()
	_, err := fs.Collection("job_details").Doc(jobDetail.ID).Set(ctx, jobDetail)
	if err != nil {
		t.Fatalf("Failed to save job detail: %v", err)
	}

	// Create a user job
	userJob := &internal.UserJob{
		ID:     jobDetail.ID,
		JobRef: fs.Collection("job_details").Doc(jobDetail.ID),
		Status: "new",
	}

	// Save the user job
	if err := internal.SaveUserJob(ctx, fs, user.ID, userJob); err != nil {
		t.Fatalf("Failed to save user job: %v", err)
	}

	// Load the job detail
	loadedDetail, err := service.loadJobDetail(ctx, jobDetail.ID)
	if err != nil {
		t.Fatalf("loadJobDetail() error = %v", err)
	}

	// Verify job detail fields
	if loadedDetail.ExtractedData.Summary != jobDetail.ExtractedData.Summary {
		t.Errorf("JobDetail has incorrect Summary = %v, want %v", loadedDetail.ExtractedData.Summary, jobDetail.ExtractedData.Summary)
	}
	if !reflect.DeepEqual(loadedDetail.ExtractedData.Qualifications, jobDetail.ExtractedData.Qualifications) {
		t.Errorf("JobDetail has incorrect Qualifications = %v, want %v", loadedDetail.ExtractedData.Qualifications, jobDetail.ExtractedData.Qualifications)
	}
}

func TestNotifyUser(t *testing.T) {
	// Create a mock pusher
	mockPusher := &mockPusher{
		triggers: make(map[string][]map[string]interface{}),
	}

	service, fs := setupTestService(t)
	defer fs.Close()
	defer internal.DeleteTestData(t, fs, "test-")

	// Set the mock pusher
	service.pusher = mockPusher

	// Create test user
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+1234567890", false)

	// Test notifying user
	ctx := context.Background()
	err := service.notifyUser(ctx, user.ID, "test-job-id")
	if err != nil {
		t.Fatalf("notifyUser() error = %v", err)
	}

	// Verify notification was sent
	channel := "user-" + user.ID
	if events, ok := mockPusher.triggers[channel]; !ok {
		t.Errorf("notifyUser() no event sent to channel %s", channel)
	} else if len(events) == 0 {
		t.Errorf("notifyUser() no events in channel %s", channel)
	}
}

func TestProcessQueuedJob(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	// Create a mock pusher
	mockPusher := &mockPusher{
		triggers: make(map[string][]map[string]interface{}),
	}

	service, fs := setupTestService(t)
	defer fs.Close()
	defer internal.DeleteTestData(t, fs, "test-")

	// Set the mock pusher
	service.pusher = mockPusher

	// Create test user and job detail
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+1234567890", false)
	jobDetail := createTestJobDetail(t, fs)

	// Create a user job first since loadUserJob won't create it
	userJob := &internal.UserJob{
		ID:     jobDetail.ID,
		JobRef: fs.Collection("job_details").Doc(jobDetail.ID),
		Status: "new",
	}

	ctx := context.Background()
	if err := internal.SaveUserJob(ctx, fs, user.ID, userJob); err != nil {
		t.Fatalf("Failed to save user job: %v", err)
	}

	// Test processing a queued job
	err := service.processQueuedJob(ctx, QueuedJob{
		UserID: user.ID,
		JobID:  jobDetail.ID,
	})
	if err != nil {
		t.Fatalf("processQueuedJob() error = %v", err)
	}

	// Verify job was processed
	jobs, err := internal.GetUserJobs(ctx, fs, user.ID)
	if err != nil {
		t.Fatalf("Failed to get user jobs: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("Expected 1 job, got %d", len(jobs))
	}

	// Verify job status was updated
	if jobs[0].Status != internal.JobStatusNew {
		t.Errorf("Job status was not updated to 'new', got %s", jobs[0].Status)
	}

	// Verify the job detail now has analysis
	if jobs[0].Analysis == nil {
		t.Errorf("Job was not updated with analysis")
	}

	// Load job detail to check for summary
	loadedDetail, err := service.loadJobDetail(ctx, jobDetail.ID)
	if err != nil {
		t.Fatalf("Failed to load job detail: %v", err)
	}

	// The loaded job detail should have a summary from the scrapingdog provider
	if loadedDetail.ExtractedData == nil || loadedDetail.ExtractedData.Summary == "" {
		t.Errorf("Job detail was not updated with extracted data and summary")
	}

	// Verify notification was sent
	channel := "user-" + user.ID
	if events, ok := mockPusher.triggers[channel]; !ok {
		t.Errorf("processQueuedJob() no event sent to channel %s", channel)
	} else if len(events) == 0 {
		t.Errorf("processQueuedJob() no events in channel %s", channel)
	}
}

// mockPusher is a mock implementation of the PusherInterface for testing
type mockPusher struct {
	triggers map[string][]map[string]interface{}
}

func (m *mockPusher) Trigger(channel string, eventName string, data interface{}) error {
	if m.triggers == nil {
		m.triggers = make(map[string][]map[string]interface{})
	}

	event := map[string]interface{}{
		"event": eventName,
		"data":  data,
	}

	m.triggers[channel] = append(m.triggers[channel], event)
	return nil
}
