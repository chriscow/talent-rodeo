// Package internal provides shared functionality for the Talent Rodeo application.
package subscription

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"talent-rodeo/assets"
	"talent-rodeo/internal"
	tools "talent-rodeo/internal/llmtools"

	"sync/atomic"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds"
	"github.com/getsentry/sentry-go"
	"github.com/lmittmann/tint"
	"github.com/pusher/pusher-http-go/v5"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

const (
	userJobProcessorConcurrency = 15 // Number of concurrent workers
	personalMatchPromptVersion  = "match-v1"
)

var errPersonalEvaluationStale = errors.New("personal evaluation inputs changed")

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

	// Search context
	searchCriteria *internal.SearchCriteria
	jobProvider    internal.JobProvider
	mode           internal.RuntimeMode
}

type userJobEvaluationTask struct {
	User          internal.User
	Job           internal.JobDetail
	Messages      minds.Messages
	Prompt        string
	AnalysisModel string
	Version       *internal.PersonalEvaluationVersion
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

// SetJobProvider sets the job provider for fetching individual job details.
// This is optional - if not set, related jobs processing will be skipped.
func (p *UserJobProcessor) SetJobProvider(provider internal.JobProvider) {
	p.jobProvider = provider
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

	logger := p.logger.With("name", task.User.Name, "userID", p.userID, "jobID", task.Job.ID)

	logger.Debug("evaluating job for user")
	if err := p.admitPersonalUser(ctx, task.User.ID); err != nil {
		logger.Warn("skipping ineligible personal job", "error", err)
		return
	}

	resp := openai.ChatCompletionResponse{}
	analysis, err := tools.Ask(ctx, task.Prompt, tools.WithMessages(task.Messages),
		tools.WithModel(task.AnalysisModel), tools.WithMaxTokens(8192),
		tools.WantCompletionResponse(&resp))
	if err != nil {
		logger.Error("failed to analyze job description", "error", err)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"jobID":  task.Job.ID,
			"userID": p.userID,
		})
		if costErr := internal.RecordLLMCost(ctx, p.fs, &task.User.ID, task.AnalysisModel, resp, "job_analysis", &task.Job.ID); costErr != nil {
			logger.Error("failed to record LLM cost for failed job analysis", "error", costErr)
		}
		return
	}
	// Record LLM cost for job analysis
	if err := internal.RecordLLMCost(ctx, p.fs, &task.User.ID, task.AnalysisModel, resp, "job_analysis", &task.Job.ID); err != nil {
		logger.Error("failed to record LLM cost for job analysis", "error", err)
	}
	if err := p.admitPersonalUser(ctx, task.User.ID); err != nil {
		logger.Warn("stopping ineligible personal job analysis", "error", err)
		return
	}

	structuredResp := openai.ChatCompletionResponse{}
	matchAnalysis, err := tools.StructuredAsk[internal.JobMatchAnalysis](ctx, "Please extract this analysis into structured JSON data:\n"+analysis,
		tools.WithModel(tools.GPT6Luna), tools.WithMaxTokens(8192), tools.WantCompletionResponse(&structuredResp))
	if err != nil {
		logger.Error("failed to extract job match analysis", "error", err)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"jobID":  task.Job.ID,
			"userID": p.userID,
		})
		if costErr := internal.RecordLLMCost(ctx, p.fs, &task.User.ID, tools.GPT6Luna, structuredResp, "job_analysis_extraction", &task.Job.ID); costErr != nil {
			logger.Error("failed to record LLM cost for failed extraction", "error", costErr)
		}
		return
	}
	// Record LLM cost for structured extraction before validating its content.
	if err := internal.RecordLLMCost(ctx, p.fs, &task.User.ID, tools.GPT6Luna, structuredResp, "job_analysis_extraction", &task.Job.ID); err != nil {
		logger.Error("failed to record LLM cost for job analysis extraction", "error", err)
	}
	if err := validateMatchAnalysis(matchAnalysis); err != nil {
		logger.Error("invalid structured job match analysis", "error", err)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{"jobID": task.Job.ID, "userID": p.userID})
		return
	}

	matchAnalysis.RawAnalysis = analysis
	matchAnalysis.Model = resp.Model
	matchAnalysis.PromptTokens = resp.Usage.PromptTokens
	matchAnalysis.CompletionTokens = resp.Usage.CompletionTokens
	matchAnalysis.ProcessingComplete = true

	// If the score is 4+ set the status as JobStatusNew otherwise set it to JobStatusArchived
	status := internal.JobStatusNew
	if matchAnalysis.Rating.NumericRating < internal.QualityJobRatingThreshold {
		status = internal.JobStatusArchived
	}

	now := time.Now()
	userJob := internal.UserJob{
		ID:                       task.Job.ID,
		UserID:                   p.userID,
		CreatedAt:                now,
		UpdatedAt:                now,
		Status:                   status,
		Rating:                   matchAnalysis.Rating.NumericRating,
		AutoArchivedByEvaluation: status == internal.JobStatusArchived,
		JobMatchAnalysis:         matchAnalysis,
		PersonalEvaluation:       task.Version,
		JobRef:                   p.fs.Collection(internal.JobDetailsCollection).Doc(task.Job.ID),
	}
	if status == internal.JobStatusArchived {
		userJob.ArchivedAt = &now
	}

	if err := p.admitPersonalUser(ctx, task.User.ID); err != nil {
		logger.Warn("skipping ineligible personal job result", "error", err)
		return
	}

	// Read and update the job in one transaction so manual statuses survive reevaluation.
	err = p.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if p.mode.Personal {
			if task.Version == nil || p.searchCriteria == nil {
				return errPersonalEvaluationStale
			}
			ownerDoc, err := tx.Get(p.fs.Collection("users").Doc(p.mode.UserID))
			if grpcstatus.Code(err) == codes.NotFound {
				return errPersonalEvaluationStale
			}
			if err != nil {
				return fmt.Errorf("read personal owner before match write: %w", err)
			}
			var owner internal.User
			if err := ownerDoc.DataTo(&owner); err != nil {
				return fmt.Errorf("decode personal owner before match write: %w", err)
			}
			if !p.mode.CanMatch(&owner, time.Now()) || owner.PersonalSearchStartedAt == nil ||
				!owner.PersonalSearchStartedAt.Equal(task.Version.Activation) || owner.CurrentStatementVersionID == nil ||
				*owner.CurrentStatementVersionID != task.Version.StatementVersionID || owner.Statement == nil ||
				fmt.Sprintf("%x", sha256.Sum256([]byte(*owner.Statement))) != task.Version.StatementHash {
				return errPersonalEvaluationStale
			}
			jobDoc, err := tx.Get(p.fs.Collection(internal.JobDetailsCollection).Doc(task.Job.ID))
			if grpcstatus.Code(err) == codes.NotFound {
				return errPersonalEvaluationStale
			}
			if err != nil {
				return fmt.Errorf("read personal job detail before match write: %w", err)
			}
			var currentJob internal.JobDetail
			if err := jobDoc.DataTo(&currentJob); err != nil {
				return fmt.Errorf("decode personal job detail before match write: %w", err)
			}
			if personalJobInputHash(currentJob) != task.Version.JobInputHash {
				return errPersonalEvaluationStale
			}
		}
		ref := p.fs.Collection("users").Doc(userJob.UserID).Collection("user_jobs").Doc(userJob.ID)
		doc, err := tx.Get(ref)
		if grpcstatus.Code(err) == codes.NotFound {
			return tx.Set(ref, userJob)
		}
		if err != nil {
			return fmt.Errorf("read existing user job: %w", err)
		}
		var existing internal.UserJob
		if err := doc.DataTo(&existing); err != nil {
			return fmt.Errorf("decode existing user job: %w", err)
		}
		if mayUpdateEvaluatedStatus(existing) {
			// The status helper marks explicit user changes; reset that marker for an evaluation update.
			if err := internal.UpdateUserJobStatusTx(ctx, p.fs, tx, userJob.UserID, userJob.ID, userJob.Status); err != nil {
				return fmt.Errorf("update user job status: %w", err)
			}
			if err := tx.Update(ref, []firestore.Update{
				{Path: "AutoArchivedByEvaluation", Value: userJob.AutoArchivedByEvaluation},
				{Path: "StatusManuallySet", Value: false},
			}); err != nil {
				return fmt.Errorf("update automatic archive marker: %w", err)
			}
			if existing.Status == internal.JobStatusArchived && userJob.Status == internal.JobStatusNew {
				if err := tx.Update(ref, []firestore.Update{{Path: "ArchivedAt", Value: nil}}); err != nil {
					return fmt.Errorf("clear automatic archive timestamp: %w", err)
				}
			}
		}
		if err := internal.UpdateUserJobAnalysisTx(ctx, p.fs, tx, userJob.UserID, userJob.ID, matchAnalysis); err != nil {
			return fmt.Errorf("failed to update user job: %w", err)
		}
		if err := internal.UpdateUserJobRatingTx(ctx, p.fs, tx, userJob.UserID, userJob.ID, matchAnalysis.Rating.NumericRating); err != nil {
			return fmt.Errorf("failed to update user job rating: %w", err)
		}
		if p.mode.Personal {
			if err := tx.Update(ref, []firestore.Update{{Path: "PersonalEvaluation", Value: task.Version}}); err != nil {
				return fmt.Errorf("update personal evaluation version: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errPersonalEvaluationStale) {
			logger.Info("discarding match analysis for changed personal profile or criteria", "jobID", task.Job.ID)
			return
		}
		logger.Error("failed to update user job", "error", err)
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"jobID":  task.Job.ID,
			"userID": p.userID,
		})
		return
	}

	onDashboard, _ := p.checkUserOnDashboard()
	logger.Info("successfully evaluated and saved job for user", "rating", matchAnalysis.Rating.NumericRating, "onDashboard", onDashboard)

	// Process related jobs if this job is rated 3 stars or higher
	if matchAnalysis.Rating.NumericRating >= internal.MinRatingForRelatedJobs && p.searchCriteria != nil && p.jobProvider != nil {
		logger.Debug("processing related jobs for high-rated job", "rating", matchAnalysis.Rating.NumericRating)
		if err := internal.ProcessRelatedJobs(ctx, p.fs, logger, p.userID, task.Job, *p.searchCriteria, p.jobProvider); err != nil {
			logger.Error("failed to process related jobs", "error", err)
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action": "evaluateJob: failed to process related jobs",
				"jobID":  task.Job.ID,
				"userID": p.userID,
				"rating": fmt.Sprintf("%d", matchAnalysis.Rating.NumericRating),
			})
			// Don't return error - related job processing failure shouldn't break main flow
		}
	}

	// Update and notify progress
	processed := p.processedJobs.Add(1)
	// if p.onDashboard {
	progress := int(float32(processed) / float32(p.totalJobs) * 100)
	internal.SendPusherProgress(p.psh, p.userID, "subscription-progress", "Analyzing new jobs for you...", progress, 100)
	// }

	// Check for completion
	if processed == int32(p.totalJobs) {
		logger.Info("finished processing all jobs for user")
		// if p.onDashboard {
		internal.SendPusherRedirect(p.psh, p.userID, "/dashboard")
		// }
	}
}

func (p *UserJobProcessor) admitPersonalUser(ctx context.Context, userID string) error {
	if !p.mode.Personal {
		return nil
	}
	if !p.mode.AllowsUser(userID) {
		return fmt.Errorf("user is outside personal mode")
	}
	owner, err := internal.GetPersonalSearchOwner(ctx, p.fs, p.mode.UserID)
	if err != nil {
		return err
	}
	if !p.mode.CanMatch(owner, time.Now()) {
		return fmt.Errorf("personal owner is not ready for matching")
	}
	return nil
}

func personalJobInputHash(job internal.JobDetail) string {
	input := fmt.Sprintf("%s\x00%s\x00%s\x00%+v", job.Description, job.WorkplaceType, job.Location, job.SalaryInfo)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(input)))
}

func newPersonalEvaluationVersion(user internal.User, job internal.JobDetail, prompt []byte, analysisModel, extractionModel string) *internal.PersonalEvaluationVersion {
	statementHash := sha256.Sum256([]byte(*user.Statement))
	promptHash := sha256.Sum256(append(append([]byte(nil), prompt...), []byte(personalMatchPromptVersion)...))
	return &internal.PersonalEvaluationVersion{
		Activation:         *user.PersonalSearchStartedAt,
		StatementVersionID: *user.CurrentStatementVersionID,
		StatementHash:      fmt.Sprintf("%x", statementHash),
		JobInputHash:       personalJobInputHash(job),
		AnalysisModel:      analysisModel,
		ExtractionModel:    extractionModel,
		PromptVersion:      fmt.Sprintf("%x", promptHash),
	}
}

func personalEvaluationMatches(actual, expected *internal.PersonalEvaluationVersion) bool {
	return actual != nil && expected != nil && actual.Activation.Equal(expected.Activation) &&
		actual.StatementVersionID == expected.StatementVersionID && actual.StatementHash == expected.StatementHash &&
		actual.JobInputHash == expected.JobInputHash && actual.AnalysisModel == expected.AnalysisModel &&
		actual.ExtractionModel == expected.ExtractionModel && actual.PromptVersion == expected.PromptVersion
}

func validateMatchAnalysis(analysis internal.JobMatchAnalysis) error {
	if analysis.Rating.NumericRating < 1 || analysis.Rating.NumericRating > 5 {
		return fmt.Errorf("job match rating must be between 1 and 5")
	}
	if strings.TrimSpace(analysis.Summary) == "" || strings.TrimSpace(analysis.Strengths) == "" ||
		strings.TrimSpace(analysis.Concerns) == "" || strings.TrimSpace(analysis.Rating.RatingExplanation) == "" {
		return fmt.Errorf("job match analysis is missing required rationale")
	}
	return nil
}

func mayUpdateEvaluatedStatus(job internal.UserJob) bool {
	return !job.StatusManuallySet && (job.Status == internal.JobStatusUnprocessed || job.Status == internal.JobStatusNew ||
		(job.Status == internal.JobStatusArchived && job.AutoArchivedByEvaluation))
}

func resolvePersonalCanonicalJobs(ctx context.Context, fs *internal.FirestoreClient, jobs []internal.JobDetail) ([]internal.JobDetail, error) {
	var canonicalIDs []string
	seenCanonical := make(map[string]struct{})
	for _, job := range jobs {
		if job.CanonicalJobID == nil {
			continue
		}
		id := *job.CanonicalJobID
		if id == "" || id == job.ID {
			return nil, fmt.Errorf("invalid canonical job ID for %s", job.ID)
		}
		if _, found := seenCanonical[id]; !found {
			seenCanonical[id] = struct{}{}
			canonicalIDs = append(canonicalIDs, id)
		}
	}
	canonicals, err := internal.GetJobDetailsByIDs(ctx, fs, canonicalIDs)
	if err != nil {
		return nil, fmt.Errorf("get canonical personal jobs: %w", err)
	}
	byID := make(map[string]internal.JobDetail, len(canonicals))
	for _, job := range canonicals {
		byID[job.ID] = job
	}
	var result []internal.JobDetail
	seenJobs := make(map[string]struct{})
	for _, job := range jobs {
		if job.CanonicalJobID != nil {
			canonical, found := byID[*job.CanonicalJobID]
			if !found || canonical.CanonicalJobID != nil {
				return nil, fmt.Errorf("canonical job %s unavailable", *job.CanonicalJobID)
			}
			job = canonical
		}
		if _, found := seenJobs[job.ID]; found {
			continue
		}
		seenJobs[job.ID] = struct{}{}
		result = append(result, job)
	}
	return result, nil
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
func RefreshUserJobs(ctx context.Context, fs *internal.FirestoreClient, psh *pusher.Client, logger *slog.Logger, user internal.User, criteria internal.SearchCriteria, modes ...internal.RuntimeMode) error {
	var mode internal.RuntimeMode
	if len(modes) > 0 {
		mode = modes[0]
	}
	if mode.Personal {
		owner, err := internal.GetPersonalSearchOwner(ctx, fs, mode.UserID)
		if err != nil {
			return err
		}
		if user.ID != owner.ID || !mode.CanMatch(owner, time.Now()) {
			return nil
		}
		user = *owner
	}
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

	// Track job evaluation statistics
	var totalJobsEvaluated int
	var newJobsFound int
	evaluationStartTime := time.Now()
	var sysPrompt []byte
	if mode.Personal {
		if user.Statement == nil || strings.TrimSpace(*user.Statement) == "" || user.CurrentStatementVersionID == nil {
			return fmt.Errorf("personal profile is not ready for job matching")
		}
		var err error
		sysPrompt, err = assets.EmbeddedFiles.ReadFile("prompts/jobdetail.match-analysis.v001.txt")
		if err != nil {
			return fmt.Errorf("read match prompt: %w", err)
		}
	}

	weekAgo := time.Now().Add(-7 * 24 * time.Hour)
	var jobIDs []string
	var err error
	if mode.Personal {
		jobIDs, err = internal.GetPersonalSearchJobIDs(ctx, fs, criteria.ID, user.ID, *user.PersonalSearchStartedAt)
	} else {
		jobIDs, err = internal.GetJobIDsForCriteriaCreatedAfter(ctx, fs, criteria.ID, weekAgo)
	}
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action":     "RefreshUserJobs: failed to get job IDs for criteria",
			"userID":     user.ID,
			"criteriaID": criteria.ID,
		})
		logger.Error("failed to get job ids for criteria created after", "error", err)
		return err
	}

	totalJobsEvaluated = len(jobIDs)
	var personalJobs []internal.JobDetail
	if mode.Personal && len(jobIDs) > 0 {
		personalJobs, err = internal.GetJobDetailsByIDs(ctx, fs, jobIDs)
		if err != nil {
			return fmt.Errorf("get personal job details: %w", err)
		}
		personalJobs, err = resolvePersonalCanonicalJobs(ctx, fs, personalJobs)
		if err != nil {
			return err
		}
	}

	userJobs, err := internal.ListUserJobs(ctx, fs, user.ID)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "RefreshUserJobs: failed to get user jobs",
			"userID": user.ID,
		})
		return fmt.Errorf("failed to get user jobs: %w", err)
	}

	if mode.Personal {
		seen := make(map[string]*internal.UserJob, len(userJobs))
		for i := range userJobs {
			job := &userJobs[i]
			seen[job.ID] = job
		}
		var pendingJobs []internal.JobDetail
		var pendingIDs []string
		for _, job := range personalJobs {
			expected := newPersonalEvaluationVersion(user, job, sysPrompt, tools.DeepSeekFlash, tools.GPT6Luna)
			if existing := seen[job.ID]; existing != nil && existing.JobMatchAnalysis.ProcessingComplete &&
				personalEvaluationMatches(existing.PersonalEvaluation, expected) {
				continue
			}
			pendingJobs = append(pendingJobs, job)
			pendingIDs = append(pendingIDs, job.ID)
		}
		personalJobs = pendingJobs
		jobIDs = pendingIDs
	} else {
		// Regular mode keeps its existing one-time evaluation behavior.
		for _, job := range userJobs {
			if !slices.Contains(jobIDs, job.ID) {
				continue
			}
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

	newJobsFound = len(jobIDs)

	if len(jobIDs) == 0 {
		logger.Info("no new jobs found for user/criteria", "name", user.Name, "userID", user.ID, "criteriaID", criteria.ID)
		internal.SendPusherRedirect(psh, user.ID, "/dashboard")

		// Update user statistics even when no new jobs found
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

		// Calculate cumulative statistics for quality and rejected jobs
		cumulativeQualityJobs, err := internal.GetUserJobsQualityCount(ctx, fs, user.ID)
		if err != nil {
			logger.Warn("failed to get cumulative quality jobs count", "error", err)
			cumulativeQualityJobs = 0
		}

		cumulativeRejectedJobs, err := internal.GetUserJobsRejectedCount(ctx, fs, user.ID)
		if err != nil {
			logger.Warn("failed to get cumulative rejected jobs count", "error", err)
			cumulativeRejectedJobs = 0
		}

		err = internal.UpdateUser(ctx, fs, user.ID, map[string]any{
			"LastJobEvaluationRun": evaluationStartTime,
			"LastEvaluationDate":   today,
			"TotalJobsEvaluated":   totalJobsEvaluated,
			"NewJobsFound":         newJobsFound,
			"RejectedJobsCount":    cumulativeRejectedJobs,
			"QualityJobsFound":     cumulativeQualityJobs,
		})
		if err != nil {
			logger.Error("failed to update user job evaluation statistics", "error", err)
		}

		return nil // No new jobs to process
	}

	logger = logger.With("name", user.Name, "userID", user.ID, "criteriaID", criteria.ID)
	logger.Info(internal.HiBGCyan("RefreshUserJobs started"), "jobCount", len(jobIDs))

	// 2. Prepare LLM conversation
	if !mode.Personal {
		sysPrompt, err = assets.EmbeddedFiles.ReadFile("prompts/jobdetail.match-analysis.v001.txt")
		if err != nil {
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action": "RefreshUserJobs: failed to read match prompt",
				"userID": user.ID,
			})
			return fmt.Errorf("failed to read match prompt: %w", err)
		}
	}

	resume, found, err := internal.GetLatestResume(ctx, fs, user.ID)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"action": "RefreshUserJobs: failed to get latest resume",
			"userID": user.ID,
		})
		return fmt.Errorf("failed to get latest resume: %w", err)
	}
	if !found {
		logger.Warn("no resume found for user, proceeding without it")
	}
	resumeText := ""
	if found && resume != nil {
		resumeText = resume.RawText
	}

	if user.Statement == nil || *user.Statement == "" {
		err := fmt.Errorf("user statement is empty")
		internal.SentryReportError(ctx, err, sentry.LevelWarning, map[string]string{
			"name":   user.Name,
			"userID": user.ID,
		})
		logger.Error("user statement is empty", "name", user.Name, "userID", user.ID)
		return err
	}

	msgs := minds.Messages{
		{Role: minds.RoleSystem, Content: string(sysPrompt)},
		{Role: minds.RoleUser, Content: fmt.Sprintf("My name is %s.\n\n%s\n\nHere is my latest resume:\n%s\n\nI will send the job description next.", user.FirstName, *user.Statement, resumeText)},
		{Role: minds.RoleAssistant, Content: "Ready for the first job description."},
	}

	jobs := personalJobs
	if !mode.Personal {
		jobs, err = internal.GetJobDetailsByIDs(ctx, fs, jobIDs)
		if err != nil {
			internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
				"action":     "RefreshUserJobs: failed to get job details",
				"userID":     user.ID,
				"jobIDCount": fmt.Sprintf("%d", len(jobIDs)),
			})
			return fmt.Errorf("failed to get job details for IDs %v: %w", jobIDs, err)
		}
	}

	// 4. Queue jobs for evaluation
	processor := NewUserJobProcessor(fs, psh, logger)
	processor.mode = mode

	processor.userID = user.ID
	processor.totalJobs = len(jobs)
	processor.searchCriteria = &criteria

	processor.Start()

	if len(jobs) > 0 {
		internal.SendPusherProgress(psh, user.ID, "subscription-progress", "Analyzing new jobs for you...", 0, 100)
	}

	for _, job := range jobs {
		// If this job is a known duplicate, skip it. The user will be evaluated
		// against the canonical version of this job if they haven't been already.
		if job.CanonicalJobID != nil {
			processor.logger.Debug("skipping duplicate job for user evaluation", "jobID", job.ID, "canonicalJobID", *job.CanonicalJobID)
			// We must decrement the total job count so the progress bar completes correctly.
			processor.totalJobs--
			continue
		}

		workType := ""
		location := ""

		if mode.Personal {
			if strings.TrimSpace(job.WorkplaceType) != "" {
				workType = fmt.Sprintf("<work_type>%s</work_type>", job.WorkplaceType)
			}
			location = fmt.Sprintf("<job_location>%s</job_location>", job.Location)
		} else if criteria.WorkType == internal.WorkTypeRemote {
			workType = "<work_type>remote position</work_type>"
			location = fmt.Sprintf("<job_location>%s</job_location>", job.Location)
			processor.logger.Debug("criteria.WorkType is remote", "jobID", job.ID, "location", job.Location)
		} else {
			workType = "<work_type>on-site position</work_type>"
			if criteria.WorkType == internal.WorkTypeHybrid {
				workType = "<work_type>hybrid position</work_type>"
			}
			geoLocation, err := internal.GetGeoID(ctx, fs, criteria.GeoID)
			if err == nil {
				location = fmt.Sprintf("%s<job_location>%s</job_location><geo_location>%s</geo_location>", workType, job.Location, geoLocation.Value)
			}
		}

		prompt := fmt.Sprintf("<job-description>:\n%s\n</job-description>\n%s\n%s", job.Description, workType, location)
		if job.SalaryInfo != nil {
			salary, err := json.MarshalIndent(job.SalaryInfo, "", "  ")
			if err != nil {
				logger.Error("failed to marshal salary info", "error", err)
			}
			prompt += fmt.Sprintf("\n\nThe salary range is %s.", string(salary))
		}

		var version *internal.PersonalEvaluationVersion
		if mode.Personal {
			version = newPersonalEvaluationVersion(user, job, sysPrompt, tools.DeepSeekFlash, tools.GPT6Luna)
		}
		task := userJobEvaluationTask{
			User:          user,
			Job:           job,
			Messages:      msgs,
			Prompt:        prompt,
			AnalysisModel: tools.DeepSeekFlash,
			Version:       version,
		}
		processor.queue <- task
	}

	logger.Info("queued jobs for evaluation", "name", user.Name, "userID", user.ID, "count", len(jobs))

	// Signal that no more jobs will be added to the queue.
	// Workers will finish processing remaining jobs and then exit.
	close(processor.queue)

	// Wait for all workers to finish processing all the jobs in the queue.
	processor.Wait()

	// After processing is complete, calculate cumulative statistics for quality and rejected jobs
	cumulativeQualityJobs, err := internal.GetUserJobsQualityCount(ctx, fs, user.ID)
	if err != nil {
		logger.Warn("failed to get cumulative quality jobs count", "error", err)
		cumulativeQualityJobs = 0
	}

	cumulativeRejectedJobs, err := internal.GetUserJobsRejectedCount(ctx, fs, user.ID)
	if err != nil {
		logger.Warn("failed to get cumulative rejected jobs count", "error", err)
		cumulativeRejectedJobs = 0
	}

	// Update user with job evaluation statistics
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	err = internal.UpdateUser(ctx, fs, user.ID, map[string]any{
		"LastJobEvaluationRun": evaluationStartTime,
		"LastEvaluationDate":   today,
		"TotalJobsEvaluated":   totalJobsEvaluated,
		"NewJobsFound":         newJobsFound,
		"RejectedJobsCount":    cumulativeRejectedJobs,
		"QualityJobsFound":     cumulativeQualityJobs,
	})
	if err != nil {
		logger.Error("failed to update user job evaluation statistics", "error", err)
	}

	logger.Info("job evaluation completed", "totalEvaluated", totalJobsEvaluated, "newJobs", newJobsFound, "cumulativeRejected", cumulativeRejectedJobs, "cumulativeQuality", cumulativeQualityJobs)

	return nil
}
