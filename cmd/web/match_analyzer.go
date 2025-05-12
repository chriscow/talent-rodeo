package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"talent-rodeo/internal"
	"talent-rodeo/internal/utils"
	"time"

	"github.com/chriscow/minds/tools"
)

const (
	maxConcurrentJobs = 5
)

type MatchAnalyzer struct {
	fs           *internal.FirestoreClient
	logger       *slog.Logger
	done         chan struct{}
	wg           sync.WaitGroup
	ticker       *time.Ticker
	queue        chan QueuedUserJob
	jobSemaphore chan struct{} // Semaphore for job processing
	model        string        // LLM model to use for analysis
}

type QueuedUserJob struct {
	UserID string
	JobID  string
}

func NewMatchAnalyzer(fs *internal.FirestoreClient, logger *slog.Logger) *MatchAnalyzer {
	logger = logger.With("service", "MatchAnalyzer")
	m := &MatchAnalyzer{
		fs:           fs,
		logger:       logger,
		queue:        make(chan QueuedUserJob, 100),
		done:         make(chan struct{}),
		jobSemaphore: make(chan struct{}, maxConcurrentJobs),
		model:        tools.GPT4oMini, // Default to production model
	}

	return m
}

// SetModel allows overriding the default model, primarily for testing
func (m *MatchAnalyzer) SetModel(model string) {
	m.model = model
}

func (m *MatchAnalyzer) Analyze(ctx context.Context, userID string, jobID string) error {
	m.logger.Info("queueing job for user processing", "userID", userID, "jobID", jobID)

	m.queue <- QueuedUserJob{
		UserID: userID,
		JobID:  jobID,
	}

	return nil
}

// StartJobWorkers initializes the job processing workers
func (m *MatchAnalyzer) Start(ctx context.Context) error {
	m.logger.Info("starting job processing workers")

	for i := range maxConcurrentJobs {
		workerID := i
		m.initWorker(ctx, workerID)
	}

	// First recover any unprocessed jobs
	if err := m.recoverUnprocessedJobs(ctx); err != nil {
		m.logger.Error("failed to recover unprocessed jobs", "error", err)
		return err
	}

	return nil
}

// Close gracefully shuts down the search service
func (m *MatchAnalyzer) Close(_ context.Context) error {
	m.logger.Info("stopping job processor")
	if m.ticker != nil {
		m.ticker.Stop()
	}

	// Signal processing to stop
	close(m.done)

	// Wait for any in-progress processing to complete
	m.wg.Wait()
	m.logger.Info("job processor stopped")

	return nil
}

// initWorker starts a single worker goroutine for processing jobs
func (m *MatchAnalyzer) initWorker(ctx context.Context, workerID int) {
	logger := m.logger.With("worker_id", workerID)
	logger.Debug("starting worker")

	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.Debug("stopping due to context cancellation")
				return
			case item := <-m.queue:
				logger = logger.With("job_id", item.JobID)
				logger.Debug("received job for processing")

				if err := m.processQueuedJob(ctx, workerID, item.UserID, item.JobID); err != nil {
					if err == context.Canceled {
						logger.Debug("job processing cancelled",
							"error", err)
					} else {
						logger.Error("failed to process queued job",
							"error", err)
					}
				} else {
					logger.Debug("completed job processing")
				}
			}
		}
	}()
}

// recoverUnprocessedJobs finds and requeues any jobs that were in an unprocessed state
func (m *MatchAnalyzer) recoverUnprocessedJobs(ctx context.Context) error {
	// ctx := context.Background()
	// s.logger.Info("recovering unprocessed jobs")

	// // Get all users
	// allUsers, err := internal.ListUsers(ctx, s.fs, 0, 0)
	// if err != nil {
	// 	return fmt.Errorf("failed to list users: %w", err)
	// }

	// recoveredCount := 0
	// for _, user := range allUsers {
	// 	// Get all jobs for this user
	// 	jobs, err := internal.GetUserJobs(ctx, s.fs, user.ID)
	// 	if err != nil {
	// 		s.logger.Error("failed to get jobs for user",
	// 			"error", err,
	// 			"user_id", user.ID)
	// 		continue
	// 	}

	// 	// Look for unprocessed jobs
	// 	for _, job := range jobs {
	// 		if job.Status == internal.JobStatusUnprocessed {
	// 			s.logger.Debug("requeueing unprocessed job",
	// 				"user_id", user.ID,
	// 				"job_id", job.ID)

	// 			// Requeue the job
	// 			s.jobQueue <- QueuedJob{
	// 				UserID: user.ID,
	// 				JobID:  job.ID,
	// 			}
	// 			recoveredCount++
	// 		}
	// 	}
	// }

	// if recoveredCount > 0 {
	// 	s.logger.Info("recovered unprocessed jobs", "count", recoveredCount)
	// }

	return nil
}

// processQueuedJob handles the processing of a single job
func (m *MatchAnalyzer) processQueuedJob(ctx context.Context, workerID int, userID, jobID string) error {
	// Acquire semaphore with context
	select {
	case m.jobSemaphore <- struct{}{}:
		defer func() { <-m.jobSemaphore }()
	case <-ctx.Done():
		return ctx.Err()
	}

	// userJob := &internal.UserJob{
	// 	ID:     jobID,
	// 	JobRef: m.fs.Collection(internal.JobDetailsCollection).Doc(jobID),
	// 	Status: internal.JobStatusUnprocessed,
	// }

	// // 1. Save the user job
	// if err := internal.CreateUserJob(ctx, m.fs, userID, userJob); err != nil {
	// 	return fmt.Errorf("failed to save user job: %w", err)
	// }

	// // We need to check if the job has already been processed
	// if userJob.Analysis != nil {
	// 	m.logger.Warn(internal.Yellow("job already processed"),
	// 		"user_id", userID,
	// 		"job_id", jobID)

	// 	userJob.Status = internal.JobStatusNew
	// 	if err := internal.CreateUserJob(ctx, m.fs, userID, userJob); err != nil {
	// 		return fmt.Errorf("failed to save user job: %w", err)
	// 	}
	// 	return nil
	// }

	// // 2. Load the job details from Firestore
	// jobDetail, err := m.loadJobDetail(ctx, jobID)
	// if err != nil {
	// 	return fmt.Errorf("failed to load job details: %w", err)
	// }

	// user, found, err := internal.GetUser(ctx, m.fs, userID)
	// if err != nil {
	// 	return fmt.Errorf("failed to get user: %w", err)
	// }
	// if !found {
	// 	return fmt.Errorf("user not found: %s", userID)
	// }

	// // 3. Analyze the job match
	// analysis, err := m.analyzeJobMatch(ctx, user, userJob, *jobDetail)
	// if err != nil {
	// 	return fmt.Errorf("failed to analyze job match: %w", err)
	// }

	// if err := m.extractJobMatch(ctx, user.ID, userJob, analysis); err != nil {
	// 	return fmt.Errorf("failed to extract job match: %w", err)
	// }

	// userJob.Status = internal.JobStatusNew

	// // 5. Check if job is a match and update progress
	// rating := strings.ToLower(userJob.Analysis.FinalJudgment.Rating)
	// isMatch := strings.Contains(rating, "yes")

	// // Set the IsRecommended field based on the rating
	// userJob.IsRecommended = isMatch

	// // 4. Save the job analysis to the UserJob document
	// if err := internal.CreateUserJob(ctx, m.fs, userID, userJob); err != nil {
	// 	return fmt.Errorf("failed to save user job: %w", err)
	// }

	// // Send a notification only if it's a good match
	// if isMatch {
	// 	// if err := m.notifyUser(ctx, job.UserID, job.JobID); err != nil {
	// 	// 	return fmt.Errorf("failed to notify user: %w", err)
	// 	// }
	// }

	return nil
}

// // getAnalysisProgress returns the total number of jobs and how many have been analyzed
// func (m *MatchAnalyzer) getAnalysisProgress(ctx context.Context, userID string) (int, int, error) {
// 	// Get all jobs for this user
// 	jobs, err := internal.GetUserJobs(ctx, m.fs, userID)
// 	if err != nil {
// 		return 0, 0, fmt.Errorf("failed to get user jobs: %w", err)
// 	}

// 	totalJobs := 0
// 	analyzed := 0

// 	for _, job := range jobs {
// 		// Count only unprocessed jobs (jobs needing analysis) plus already analyzed ones
// 		if job.Status == internal.JobStatusUnprocessed || job.Analysis != nil {
// 			totalJobs++

// 			// Count jobs that have already been analyzed
// 			if job.Analysis != nil {
// 				analyzed++
// 			}
// 		}
// 	}

// 	return totalJobs, analyzed, nil
// }

// // countMatchedJobs counts how many jobs were matched favorably for a user
// func (m *MatchAnalyzer) countMatchedJobs(ctx context.Context, userID string) (int, error) {
// 	// Get all jobs for this user
// 	jobs, err := internal.GetUserJobs(ctx, m.fs, userID)
// 	if err != nil {
// 		return 0, fmt.Errorf("failed to get user jobs: %w", err)
// 	}

// 	matchedCount := 0

// 	for _, job := range jobs {
// 		if job.Analysis != nil && strings.Contains(strings.ToLower(job.Analysis.FinalJudgment.Rating), "yes") {
// 			matchedCount++
// 		}
// 	}

// 	return matchedCount, nil
// }

// // countPendingJobsForUser checks if there are any queued or in-progress jobs for a user
// func (m *MatchAnalyzer) countPendingJobsForUser(ctx context.Context, userID string) (int, error) {
// 	// Count queued jobs (still in the job queue)
// 	count := 0

// 	// Check if there are any unprocessed jobs in Firestore
// 	jobs, err := internal.GetUserJobs(ctx, m.fs, userID)
// 	if err != nil {
// 		return 0, fmt.Errorf("failed to get user jobs: %w", err)
// 	}

// 	for _, job := range jobs {
// 		if job.Status == internal.JobStatusUnprocessed {
// 			count++
// 		}
// 	}

// 	m.logger.Debug("countPendingJobsForUser", "user_id", userID, "pending_jobs", count)
// 	return count, nil
// }

// loadJobDetail loads the job details from Firestore
func (m *MatchAnalyzer) loadJobDetail(ctx context.Context, jobID string) (*internal.JobDetail, error) {
	m.logger.Debug("loading job details", "job_id", jobID)

	doc, err := m.fs.Collection("job_details").Doc(jobID).Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get job details from firestore: %w", err)
	}

	var jobDetail internal.JobDetail
	if err := doc.DataTo(&jobDetail); err != nil {
		return nil, fmt.Errorf("failed to parse job details: %w", err)
	}

	return &jobDetail, nil
}

func (m *MatchAnalyzer) analyzeJobMatch(ctx context.Context, user *internal.User, job *internal.UserJob, jobDetail internal.JobDetail) (string, error) {
	m.logger.Debug("analyzing job match", "user_id", user.ID, "job_id", job.ID)

	// Create system prompt from template
	const matchFile = "prompts/job-match-analysis.001.txt"
	tmpl, err := utils.CreateTemplate(matchFile)
	if err != nil {
		return "", fmt.Errorf("failed to create template: %w", err)
	}

	// Execute template with job details
	jobJSON, err := json.MarshalIndent(jobDetail, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal job details: %w", err)
	}

	// statement := user.Search.Statement
	statement := ""
	panic("analyzeJobMatch: not migrated to new system yet")

	prompt, err := tmpl.Execute(map[string]any{
		"Statement": statement,
		"JobDetail": jobJSON,
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	// Use configured model
	response, err := tools.Ask(ctx, prompt, tools.WithModel(m.model))
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	return response, nil
}

func (m *MatchAnalyzer) extractJobMatch(ctx context.Context, userID string, job *internal.UserJob, analysis string) error {
	m.logger.Debug("extracting job match to JSON", "user_id", userID)

	// prompt := `Given the following job analysis, extract the job match in JSON format: ` + analysis

	// // Use configured model
	// jobAnalysis, err := tools.StructuredAsk[internal.JobAnalysis](ctx, "job_analysis", prompt, tools.WithModel(m.model))
	// if err != nil {
	// 	return fmt.Errorf("failed to generate content: %w", err)
	// }

	// job.Analysis = &jobAnalysis

	return nil
}
