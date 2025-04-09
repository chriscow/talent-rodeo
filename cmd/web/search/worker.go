package search

import (
	"context"
	"encoding/json"
	"fmt"
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
	for i := 0; i < numWorkers; i++ {
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
		s.logger.Debug(internal.Cyan("search: worker: job already processed"),
			"user_id", job.UserID,
			"job_id", job.JobID)
		return nil
	}

	// 2. Load the job details from Firestore
	jobDetail, err := s.loadJobDetail(ctx, job.JobID)
	if err != nil {
		return fmt.Errorf("failed to load job details: %w", err)
	}

	// 3. Use LLM to summarize the job details
	summary, err := s.summarizeJobDetails(ctx, jobDetail)
	if err != nil {
		return fmt.Errorf("failed to summarize job details: %w", err)
	}

	// 4. Save the summary, company name and logo to the UserJob document
	if err := s.saveUserJobSummary(ctx, job.UserID, userJob, summary, jobDetail); err != nil {
		return fmt.Errorf("failed to save user job summary: %w", err)
	}

	user, found, err := internal.GetUser(ctx, s.fs, job.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if !found {
		return fmt.Errorf("user not found: %s", job.UserID)
	}

	// 5. Analyze the job match
	analysis, err := s.analyzeJobMatch(ctx, user, userJob, jobDetail)
	if err != nil {
		return fmt.Errorf("failed to analyze job match: %w", err)
	}

	if err := s.extractJobMatch(ctx, user.ID, userJob, analysis); err != nil {
		return fmt.Errorf("failed to extract job match: %w", err)
	}

	// 6. Save the job analysis to the UserJob document
	if err := internal.SaveUserJob(ctx, s.fs, job.UserID, userJob); err != nil {
		return fmt.Errorf("failed to save user job: %w", err)
	}

	// 7. Send a message to the user via Pusher
	if userJob.Analysis.FinalJudgment.Rating == "Definitely Yes" {
		if err := s.notifyUser(ctx, job.UserID, job.JobID); err != nil {
			return fmt.Errorf("failed to notify user: %w", err)
		}
	}

	return nil
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

// summarizeJobDetails uses LLM to create a job summary
func (s *Service) summarizeJobDetails(ctx context.Context, jobDetail *search.JobDetail) (*internal.UserJobSummary, error) {
	s.logger.Debug("search: worker: summarizing job details", "job_id", jobDetail.ID)

	// Create OpenAI provider with GPT-4o model
	llm, err := openai.NewProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to create openai provider: %w", err)
	}

	// Create system prompt from template
	const summaryFile = "prompts/job-summary.001.txt"
	systemPrompt, err := utils.CreateTemplate(summaryFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	// Execute template with job details
	jobJSON, err := json.MarshalIndent(jobDetail, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal job details: %w", err)
	}

	messages := []minds.Message{
		{
			Role:    minds.RoleSystem,
			Content: systemPrompt.RawContent,
		},
		{
			Role:    minds.RoleUser,
			Content: fmt.Sprintf("Please summarize this job listing:\n\n%s", string(jobJSON)),
		},
	}

	schema, err := minds.NewResponseSchema("job_summary", "job_summary.001.txt", internal.UserJobSummary{})
	if err != nil {
		return nil, fmt.Errorf("failed to create response schema: %w", err)
	}

	response, err := llm.GenerateContent(ctx, minds.NewRequest(messages, minds.WithResponseSchema(*schema)))
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	var summary internal.UserJobSummary
	if err := json.Unmarshal([]byte(response.String()), &summary); err != nil {
		return nil, fmt.Errorf("failed to unmarshal summary: %w", err)
	}

	return &summary, nil
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

// saveUserJobSummary saves the summary and company info to UserJob
func (s *Service) saveUserJobSummary(ctx context.Context, userID string, job *internal.UserJob, summary *internal.UserJobSummary, jobDetail *search.JobDetail) error {
	s.logger.Debug("search: worker: saving user job summary", "user_id", userID, "job_id", job.ID)

	// Update the job with summary and company information
	job.Summary = summary.Summary
	job.CompanyName = jobDetail.Company
	job.CompanyLogo = jobDetail.CompanyInfo.LogoURL
	job.Qualifications = summary.Qualifications

	// Set company logo if available in the CompanyInfo
	if jobDetail.CompanyInfo != nil && jobDetail.CompanyInfo.LogoURL != "" {
		job.CompanyLogo = jobDetail.CompanyInfo.LogoURL
	}

	// Save the updated job
	if err := internal.SaveUserJob(ctx, s.fs, userID, job); err != nil {
		return fmt.Errorf("failed to save user job: %w", err)
	}

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
