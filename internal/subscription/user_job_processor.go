// Package internal provides shared functionality for the Talent Rodeo application.
package subscription

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"talent-rodeo/assets"
	"talent-rodeo/internal"

	"sync/atomic"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds"
	"github.com/chriscow/minds/tools"
	"github.com/getsentry/sentry-go"
	"github.com/lmittmann/tint"
	"github.com/pusher/pusher-http-go/v5"
	"github.com/sashabaranov/go-openai"
)

const (
	userJobProcessorConcurrency = 5 // Number of concurrent workers
)

type UserJobProcessor struct {
	fs        *internal.FirestoreClient
	logger    *slog.Logger
	psh       *pusher.Client
	queue     chan userJobEvaluationTask
	stopCh    chan struct{}
	stoppedCh chan struct{}
	workersWg sync.WaitGroup
	stopOnce  sync.Once

	// Progress tracking
	userID        string
	onDashboard   bool
	totalJobs     int
	processedJobs atomic.Int32
}

type userJobEvaluationTask struct {
	User          internal.User
	Job           internal.JobDetail
	Messages      minds.Messages
	Prompt        string
	AnalysisModel string
}

func NewUserJobProcessor(fs *internal.FirestoreClient, psh *pusher.Client, logger *slog.Logger) *UserJobProcessor {
	if fs == nil {
		panic("firestore client cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}
	logger = logger.With("pkg", "UserJobProcessor")
	return &UserJobProcessor{
		fs:        fs,
		logger:    logger,
		psh:       psh,
		queue:     make(chan userJobEvaluationTask, 100), // Buffered channel
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

func (p *UserJobProcessor) Start() {
	p.logger.Info("starting user job processor")
	for i := 0; i < userJobProcessorConcurrency; i++ {
		p.workersWg.Add(1)
		go p.worker()
	}
}

func (p *UserJobProcessor) Stop() {
	p.stopOnce.Do(func() {
		p.logger.Info("stopping user job processor")
		close(p.stopCh)
		p.workersWg.Wait()
		close(p.stoppedCh)
		p.logger.Info("user job processor stopped")
	})
}

// Wait blocks until all jobs in the queue have been processed and workers have stopped.
func (p *UserJobProcessor) Wait() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	done := make(chan struct{})

	// Start a goroutine to log queue length every 15 seconds
	go func() {
		for {
			select {
			case <-ticker.C:
				jobsLeft := len(p.queue)
				p.logger.Debug("jobs left in user job queue", "remaining", jobsLeft)
			case <-done:
				return
			}
		}
	}()

	p.workersWg.Wait()
	close(done)
	p.logger.Info("user job processor finished processing all jobs")
}

func (p *UserJobProcessor) worker() {
	defer p.workersWg.Done()
	for {
		select {
		case task, ok := <-p.queue:
			if !ok {
				return
			}
			p.evaluateJob(context.Background(), task)
		case <-p.stopCh:
			return
		}
	}
}

func (p *UserJobProcessor) evaluateJob(ctx context.Context, task userJobEvaluationTask) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("panic in evaluateJob", "error", r, "stack", string(debug.Stack()))
			internal.SentryReportError(ctx, fmt.Errorf("panic in evaluateJob: %v", r), sentry.LevelFatal, map[string]string{
				"jobID":  task.Job.ID,
				"userID": p.userID,
			})
		}
	}()

	p.logger.Debug("evaluating job for user", "jobID", task.Job.ID, "userID", p.userID)

	resp := openai.ChatCompletionResponse{}
	analysis, err := tools.Ask(ctx, task.Prompt, tools.WithMessages(task.Messages),
		tools.WithModel(task.AnalysisModel), tools.WithMaxTokens(8192),
		tools.WantCompletionResponse(&resp))
	if err != nil {
		p.logger.Error("failed to analyze job description", "error", err)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"jobID":  task.Job.ID,
			"userID": p.userID,
		})
		return
	}

	matchAnalysis, err := tools.StructuredAsk[internal.JobMatchAnalysis](ctx, "Please extract this analysis into structured JSON data:\n"+analysis, tools.WithModel(tools.GPT41Mini))
	if err != nil {
		p.logger.Error("failed to extract job match analysis", "error", err)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"jobID":  task.Job.ID,
			"userID": p.userID,
		})
	}

	matchAnalysis.RawAnalysis = analysis
	matchAnalysis.Model = resp.Model
	matchAnalysis.PromptTokens = resp.Usage.PromptTokens
	matchAnalysis.CompletionTokens = resp.Usage.CompletionTokens
	matchAnalysis.ProcessingComplete = true

	// If the score is 3, 4 or 5 set the status as JobStatusNew otherwise set it to JobStatusArchived
	status := internal.JobStatusNew
	if matchAnalysis.Rating.NumericRating < 3 {
		status = internal.JobStatusArchived
	}

	userJob := internal.UserJob{
		ID:               task.Job.ID,
		UserID:           p.userID,
		CreatedAt:        time.Now(),
		Status:           status,
		JobMatchAnalysis: matchAnalysis,
		JobRef:           p.fs.Collection(internal.JobDetailsCollection).Doc(task.Job.ID),
	}

	if _, err := internal.CreateUserJob(ctx, p.fs, userJob.UserID, userJob.ID); err != nil {
		p.logger.Error("failed to create user job", "error", err)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"jobID":  task.Job.ID,
			"userID": p.userID,
		})
		return // Return after logging the error
	}

	// Update the job with the analysis data separately
	err = p.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := internal.UpdateUserJobStatusTx(ctx, p.fs, tx, userJob.UserID, userJob.ID, status); err != nil {
			return fmt.Errorf("failed to update user job status: %w", err)
		}
		if err := internal.UpdateUserJobAnalysisTx(ctx, p.fs, tx, userJob.UserID, userJob.ID, matchAnalysis); err != nil {
			return fmt.Errorf("failed to update user job: %w", err)
		}
		if err := internal.UpdateUserJobRatingTx(ctx, p.fs, tx, userJob.UserID, userJob.ID, matchAnalysis.Rating.NumericRating); err != nil {
			return fmt.Errorf("failed to update user job rating: %w", err)
		}
		return nil
	})
	if err != nil {
		p.logger.Error("failed to update user job", "error", err)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"jobID":  task.Job.ID,
			"userID": p.userID,
		})
	}

	onDashboard, err := p.checkUserOnDashboard()
	if err != nil {
		p.logger.Error("failed to check user on dashboard", "error", err)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"jobID":  task.Job.ID,
			"userID": p.userID,
		})
	}
	p.logger.Info("successfully evaluated and saved job for user", "jobID", userJob.ID, "userID", userJob.UserID, "rating", matchAnalysis.Rating.NumericRating, "onDashboard", onDashboard)

	p.logger.Info(internal.HiBGCyan("User is on dashboard"), "onDashboard", onDashboard)

	// Update and notify progress
	processed := p.processedJobs.Add(1)
	// if p.onDashboard {
	progress := int(float32(processed) / float32(p.totalJobs) * 100)
	internal.SendPusherProgress(p.psh, p.userID, "subscription-progress", "Analyzing new jobs for you...", progress, 100)
	// }

	// Check for completion
	if processed == int32(p.totalJobs) {
		p.logger.Info("finished processing all jobs for user", "userID", p.userID)
		// if p.onDashboard {
		internal.SendPusherRedirect(p.psh, p.userID, "/dashboard")
		// }
	}
}

func (p *UserJobProcessor) checkUserOnDashboard() (bool, error) {
	onDashboard, err := internal.IsPusherUserPresent(p.psh, p.userID)
	if err != nil {
		p.logger.Warn("failed to check user presence", "error", err)
		return false, err
	}

	p.onDashboard = onDashboard
	return onDashboard, nil
}

// RefreshUserJobs evaluates new jobs for a user against their preferences.
func RefreshUserJobs(ctx context.Context, fs *internal.FirestoreClient, psh *pusher.Client, logger *slog.Logger, user internal.User, criteria internal.SearchCriteria) error {
	if logger == nil {
		// Create a silent logger (discards all output) or stderr logger based on environment
		// Set REFRESH_USER_JOBS_LOG=stderr to log to stderr, otherwise logs are discarded
		logOutput := "stderr"
		var handler slog.Handler

		if logOutput == "stderr" {
			// Log to stderr with tint formatting
			handler = tint.NewHandler(os.Stderr, &tint.Options{Level: slog.LevelInfo})
		} else {
			// Silent logger - discard all output
			handler = slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})
		}

		logger = slog.New(handler).With("pkg", "RefreshUserJobs")
	}

	weekAgo := time.Now().Add(-7 * 24 * time.Hour)
	jobIDs, err := internal.GetJobIDsForCriteriaCreatedAfter(ctx, fs, criteria.ID, weekAgo)
	if err != nil {
		logger.Error("failed to get job ids for criteria created after", "error", err)
		return err
	}

	userJobs, err := internal.GetUserJobs(ctx, fs, user.ID)
	if err != nil {
		return fmt.Errorf("failed to get user jobs: %w", err)
	}

	// TODO: remove job IDs that the user already has
	for _, job := range userJobs {
		if slices.Contains(jobIDs, job.ID) {
			// Delete removes the elements s[i:j] from s, returning the modified
			// slice. Delete panics if j > len(s) or s[i:j] is not a valid slice
			// of s. Delete is O(len(s)-i), so if many items must be deleted, it
			// is better to make a single call deleting them all together than
			// to delete one at a time. Delete zeroes the elements
			// s[len(s)-(j-i):len(s)].
			i := slices.Index(jobIDs, job.ID)
			jobIDs = slices.Delete(jobIDs, i, i+1)
		}
	}

	if len(jobIDs) == 0 {
		logger.Info("no new jobs found for user/criteria", "userID", user.ID, "criteriaID", criteria.ID)
		internal.SendPusherRedirect(psh, user.ID, "/dashboard")
		return nil // No new jobs to process
	}

	logger = logger.With("userID", user.ID, "criteriaID", criteria.ID)
	logger.Info(internal.HiBGCyan("RefreshUserJobs started"), "jobCount", len(jobIDs))

	// 2. Prepare LLM conversation
	sysPrompt, err := assets.EmbeddedFiles.ReadFile("prompts/jobdetail.match-analysis.v001.txt")
	if err != nil {
		return fmt.Errorf("failed to read match prompt: %w", err)
	}

	resume, found, err := internal.GetLatestResume(ctx, fs, user.ID)
	if err != nil {
		return fmt.Errorf("failed to get latest resume: %w", err)
	}
	if !found {
		logger.Warn("no resume found for user, proceeding without it")
	}

	msgs := minds.Messages{
		{Role: minds.RoleSystem, Content: string(sysPrompt)},
		{Role: minds.RoleUser, Content: fmt.Sprintf("My name is %s.\n\n%s\n\nHere is my latest resume:\n%s\n\nI will send the job description next.", user.FirstName, *user.Statement, resume.RawText)},
		{Role: minds.RoleAssistant, Content: "Ready for the first job description."},
	}

	if len(jobIDs) == 0 {
		logger.Info("no new jobs found for criteria", "criteriaID", criteria.ID)
		return nil // No new jobs to process
	}

	jobs, err := internal.GetJobDetailsByIDs(ctx, fs, jobIDs)
	if err != nil {
		return fmt.Errorf("failed to get job details for IDs %v: %w", jobIDs, err)
	}

	// 4. Queue jobs for evaluation
	processor := NewUserJobProcessor(fs, psh, logger)

	processor.userID = user.ID
	processor.totalJobs = len(jobs)

	processor.Start()

	if len(jobs) > 0 {
		internal.SendPusherProgress(psh, user.ID, "subscription-progress", "Analyzing new jobs for you...", 0, 100)
	}

	for _, job := range jobs {
		task := userJobEvaluationTask{
			User:          user,
			Job:           job,
			Messages:      msgs,
			Prompt:        fmt.Sprintf("<job-description>:\n%s\n</job-description>", job.Description),
			AnalysisModel: tools.DeepSeekChat,
		}
		processor.queue <- task
	}

	logger.Info("queued jobs for evaluation", "count", len(jobs))

	// Signal that no more jobs will be added to the queue.
	// Workers will finish processing remaining jobs and then exit.
	close(processor.queue)

	// Wait for all workers to finish processing all the jobs in the queue.
	processor.Wait()

	return nil
}
