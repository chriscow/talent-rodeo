package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search"
	"talent-rodeo/internal/utils"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/openai"
)

// QueuedJob represents a job to be processed
type QueuedJob struct {
	UserID string
	JobID  string
}

// StartJobWorkers initializes the job processing workers
func (s *Service) StartJobWorkers(ctx context.Context, numWorkers int) {
	s.logger.Info("starting job processing workers", "num_workers", numWorkers)
	for i := range numWorkers {
		workerID := i
		s.initializeJobWorker(ctx, workerID)
	}
}

// initializeJobWorker starts a single worker goroutine for processing jobs
func (s *Service) initializeJobWorker(ctx context.Context, workerID int) {
	s.logger.Debug("search: worker: starting job processing worker", "worker_id", workerID)
	go func() {
		for {
			select {
			case <-ctx.Done():
				s.logger.Debug("search: worker: job worker stopping due to context cancellation", "worker_id", workerID)
				return
			case job := <-s.jobQueue:
				s.logger.Debug("search: worker: worker received job for processing",
					"worker_id", workerID,
					"user_id", job.UserID,
					"job_id", job.JobID)

				if err := s.processQueuedJob(ctx, job); err != nil {
					if err == context.Canceled {
						s.logger.Debug("search: worker: job processing cancelled",
							"worker_id", workerID,
							"user_id", job.UserID,
							"error", err)
					} else {
						s.logger.Error("search: worker: failed to process queued job",
							"worker_id", workerID,
							"user_id", job.UserID,
							"error", err)
					}
				} else {
					s.logger.Debug("search: worker: worker completed job processing",
						"worker_id", workerID,
						"user_id", job.UserID)
				}
			}
		}
	}()
}

// processQueuedJob handles the processing of a single job
func (s *Service) processQueuedJob(ctx context.Context, job QueuedJob) error {
	// Acquire semaphore with context
	select {
	case s.jobSemaphore <- struct{}{}:
		defer func() { <-s.jobSemaphore }()
	case <-ctx.Done():
		return ctx.Err()
	}

	// 1. Load the user job (this was created in the internal/search/providers/scrapingdog/store.go file)
	userJob, found, err := internal.GetUserJob(ctx, s.fs, job.UserID, job.JobID)
	if err != nil {
		return fmt.Errorf("failed to load user job: %w", err)
	}
	if !found {
		return fmt.Errorf("user job not found: %s", job.JobID)
	}

	// We need to check if the job has already been processed
	if userJob.Analysis != nil {
		s.logger.Warn(internal.Yellow("search: worker: job already processed"),
			"user_id", job.UserID,
			"job_id", job.JobID)

		userJob.Status = internal.JobStatusNew
		if err := internal.CreateUserJob(ctx, s.fs, job.UserID, userJob); err != nil {
			return fmt.Errorf("failed to save user job: %w", err)
		}
		return nil
	}

	// 2. Load the job details from Firestore
	jobDetail, err := s.loadJobDetail(ctx, job.JobID)
	if err != nil {
		return fmt.Errorf("failed to load job details: %w", err)
	}

	user, found, err := internal.GetUser(ctx, s.fs, job.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if !found {
		return fmt.Errorf("user not found: %s", job.UserID)
	}

	// 3. Analyze the job match
	analysis, err := s.analyzeJobMatch(ctx, user, userJob, jobDetail)
	if err != nil {
		return fmt.Errorf("failed to analyze job match: %w", err)
	}

	if err := s.extractJobMatch(ctx, user.ID, userJob, analysis); err != nil {
		return fmt.Errorf("failed to extract job match: %w", err)
	}

	userJob.Status = internal.JobStatusNew

	// 5. Check if job is a match and update progress
	rating := strings.ToLower(userJob.Analysis.FinalJudgment.Rating)
	isMatch := strings.Contains(rating, "yes")

	// Set the IsRecommended field based on the rating
	userJob.IsRecommended = isMatch

	// 4. Save the job analysis to the UserJob document
	if err := internal.CreateUserJob(ctx, s.fs, job.UserID, userJob); err != nil {
		return fmt.Errorf("failed to save user job: %w", err)
	}

	// Record the analysis results in our progress tracker
	s.recordJobAnalyzed(job.UserID, isMatch)

	// Send a notification only if it's a good match
	if isMatch {
		if err := s.notifyUser(ctx, job.UserID, job.JobID); err != nil {
			return fmt.Errorf("failed to notify user: %w", err)
		}
	}

	return nil
}

// getAnalysisProgress returns the total number of jobs and how many have been analyzed
func (s *Service) getAnalysisProgress(ctx context.Context, userID string) (int, int, error) {
	// Get all jobs for this user
	jobs, err := internal.GetUserJobs(ctx, s.fs, userID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get user jobs: %w", err)
	}

	totalJobs := 0
	analyzed := 0

	for _, job := range jobs {
		// Count only unprocessed jobs (jobs needing analysis) plus already analyzed ones
		if job.Status == internal.JobStatusUnprocessed || job.Analysis != nil {
			totalJobs++

			// Count jobs that have already been analyzed
			if job.Analysis != nil {
				analyzed++
			}
		}
	}

	return totalJobs, analyzed, nil
}

// countMatchedJobs counts how many jobs were matched favorably for a user
func (s *Service) countMatchedJobs(ctx context.Context, userID string) (int, error) {
	// Get all jobs for this user
	jobs, err := internal.GetUserJobs(ctx, s.fs, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get user jobs: %w", err)
	}

	matchedCount := 0

	for _, job := range jobs {
		if job.Analysis != nil && strings.Contains(strings.ToLower(job.Analysis.FinalJudgment.Rating), "yes") {
			matchedCount++
		}
	}

	return matchedCount, nil
}

// countPendingJobsForUser checks if there are any queued or in-progress jobs for a user
func (s *Service) countPendingJobsForUser(ctx context.Context, userID string) (int, error) {
	// Count queued jobs (still in the job queue)
	count := 0

	// Check if there are any unprocessed jobs in Firestore
	jobs, err := internal.GetUserJobs(ctx, s.fs, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get user jobs: %w", err)
	}

	for _, job := range jobs {
		if job.Status == internal.JobStatusUnprocessed {
			count++
		}
	}

	s.logger.Debug("countPendingJobsForUser", "user_id", userID, "pending_jobs", count)
	return count, nil
}

// loadJobDetail loads the job details from Firestore
func (s *Service) loadJobDetail(ctx context.Context, jobID string) (*search.JobDetail, error) {
	s.logger.Debug("search: worker: loading job details", "job_id", jobID)

	doc, err := s.fs.Collection("job_details").Doc(jobID).Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get job details from firestore: %w", err)
	}

	var jobDetail search.JobDetail
	if err := doc.DataTo(&jobDetail); err != nil {
		return nil, fmt.Errorf("failed to parse job details: %w", err)
	}

	return &jobDetail, nil
}

func (s *Service) analyzeJobMatch(ctx context.Context, user *internal.User, job *internal.UserJob, jobDetail *search.JobDetail) (string, error) {
	s.logger.Debug("search: worker: analyzing job match", "user_id", user.ID, "job_id", job.ID)

	// Create OpenAI provider with GPT-4o model
	llm, err := openai.NewProvider(openai.WithModel("gpt-4o"))
	if err != nil {
		return "", fmt.Errorf("failed to create openai provider: %w", err)
	}

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

	statement := user.Search.Statement

	prompt, err := tmpl.Execute(map[string]any{
		"Statement": statement,
		"JobDetail": jobJSON,
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	messages := []minds.Message{
		{
			Role:    minds.RoleUser,
			Content: prompt,
		},
	}

	response, err := llm.GenerateContent(ctx, minds.NewRequest(messages))
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	return response.String(), nil
}

func (s *Service) extractJobMatch(ctx context.Context, userID string, job *internal.UserJob, analysis string) error {
	s.logger.Debug("search: worker: extracting job match to JSON", "user_id", userID)

	llm, err := openai.NewProvider(openai.WithModel("gpt-4o-mini"))
	if err != nil {
		return fmt.Errorf("failed to create openai provider: %w", err)
	}

	messages := []minds.Message{
		{
			Role:    minds.RoleUser,
			Content: `Given the following job analysis, extract the job match in JSON format: ` + analysis,
		},
	}

	schema, err := minds.NewResponseSchema("job_analysis", "job_analysis.001.txt", internal.JobAnalysis{})
	if err != nil {
		return fmt.Errorf("failed to create response schema: %w", err)
	}

	response, err := llm.GenerateContent(ctx, minds.NewRequest(messages, minds.WithResponseSchema(*schema)))
	if err != nil {
		return fmt.Errorf("failed to generate content: %w", err)
	}

	var jobAnalysis internal.JobAnalysis
	if err := json.Unmarshal([]byte(response.String()), &jobAnalysis); err != nil {
		return fmt.Errorf("failed to unmarshal job analysis: %w", err)
	}

	job.Analysis = &jobAnalysis

	return nil
}

// notifyUser sends a Pusher notification about the new job
func (s *Service) notifyUser(_ context.Context, userID, jobID string) error {
	s.logger.Debug("search: worker: notifying user about new job", "user_id", userID, "job_id", jobID)

	// Check if pusher is available
	if s.pusher == nil {
		s.logger.Warn("pusher not available, skipping notification")
		return nil
	}

	// Send notification via Pusher
	return s.notifyUserNewJob(userID, nil)
}
