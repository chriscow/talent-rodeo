package search

import (
	"context"
	"log/slog"
	"os"
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

	// Create a test job detail
	jobDetail := &search.JobDetail{
		ID:            "test-job-id",
		Provider:      "test-provider",
		ProviderJobID: "provider-test-job-id",
		Title:         "Software Engineer",
		Company:       "Test Company",
		Location:      "Remote",
		PostedDate:    time.Now(),
		URL:           "https://example.com/job",
		FullDescription: "This is a test job description for a Software Engineer position. " +
			"Responsibilities include developing and maintaining software applications, " +
			"collaborating with cross-functional teams, and implementing new features.",
		Requirements: []string{
			"5+ years of experience in software development",
			"Strong knowledge of Go programming language",
			"Experience with cloud platforms like AWS or GCP",
		},
		Qualifications: []string{
			"Bachelor's degree in Computer Science or related field",
			"Experience with microservices architecture",
		},
		Skills:         []string{"Go", "Docker", "Kubernetes", "SQL"},
		EmploymentType: "Full-time",
		WorkplaceType:  "Remote",
		CompanyInfo: &search.Company{
			Name:    "Test Company",
			LogoURL: "https://example.com/logo.png",
		},
	}

	// Save to Firestore
	_, err := fs.Collection("job_details").Doc(jobDetail.ID).Set(context.Background(), jobDetail)
	if err != nil {
		t.Fatalf("Failed to create test job detail: %v", err)
	}

	return jobDetail
}

func createTestUser(t *testing.T, fs *firestore.Client) *internal.User {
	t.Helper()

	// Create a test user
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+1234567890", false)
	return user
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
	user := createTestUser(t, fs)
	jobDetail := createTestJobDetail(t, fs)

	// First test - when job doesn't exist
	ctx := context.Background()
	_, found, err := internal.GetUserJob(ctx, fs, user.ID, jobDetail.ID)
	if err == nil {
		t.Errorf("loadUserJob() expected error, got nil")
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

func TestSummarizeJobDetails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	service, fs := setupTestService(t)
	defer fs.Close()
	defer internal.DeleteTestData(t, fs, "test-")

	// Create test job detail
	jobDetail := createTestJobDetail(t, fs)

	// Test summarizing job details
	ctx := context.Background()
	summary, err := service.summarizeJobDetails(ctx, jobDetail)
	if err != nil {
		t.Fatalf("summarizeJobDetails() error = %v", err)
	}

	// Verify summary is not empty
	if summary.Summary == "" {
		t.Error("summarizeJobDetails() got empty summary")
	}
}

func TestSaveUserJobSummary(t *testing.T) {
	service, fs := setupTestService(t)
	defer fs.Close()
	defer internal.DeleteTestData(t, fs, "test-")

	// Create test user and job detail
	user := createTestUser(t, fs)
	jobDetail := createTestJobDetail(t, fs)

	// Create a user job
	userJob := &internal.UserJob{
		ID:     jobDetail.ID,
		JobRef: fs.Collection("job_details").Doc(jobDetail.ID),
		Status: "new",
	}

	// Test summary
	summary := &internal.UserJobSummary{
		Summary: "This is a Software Engineer position at Test Company. The role involves developing and maintaining software applications, with a focus on Go programming. Requirements include 5+ years of experience and strong knowledge of Go.",
		Qualifications: []string{
			"Bachelor's degree in Computer Science or related field",
			"Experience with microservices architecture",
		},
	}

	// Test saving the job summary
	ctx := context.Background()
	err := service.saveUserJobSummary(ctx, user.ID, userJob, summary, jobDetail)
	if err != nil {
		t.Fatalf("saveUserJobSummary() error = %v", err)
	}

	// Load the job to verify
	jobs, err := internal.GetUserJobs(ctx, fs, user.ID)
	if err != nil {
		t.Fatalf("Failed to get user jobs: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("Expected 1 job, got %d", len(jobs))
	}

	savedJob := jobs[0]

	// Verify job fields
	if savedJob.Summary != summary.Summary {
		t.Errorf("saveUserJobSummary() got Summary = %v, want %v", savedJob.Summary, summary)
	}
	if savedJob.CompanyName != jobDetail.Company {
		t.Errorf("saveUserJobSummary() got CompanyName = %v, want %v", savedJob.CompanyName, jobDetail.Company)
	}
	if jobDetail.CompanyInfo != nil && savedJob.CompanyLogo != jobDetail.CompanyInfo.LogoURL {
		t.Errorf("saveUserJobSummary() got CompanyLogo = %v, want %v", savedJob.CompanyLogo, jobDetail.CompanyInfo.LogoURL)
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
	user := createTestUser(t, fs)

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
	user := createTestUser(t, fs)
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

	// Verify the job has a summary now
	if jobs[0].Summary == "" {
		t.Errorf("Job was not updated with summary")
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
