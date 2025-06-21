package nudge

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/cloudflare"
	"talent-rodeo/internal/smtp"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/getsentry/sentry-go"
	"google.golang.org/api/iterator"
)

const (
	// NudgeProcessorKey is the feature flag key for nudge processor settings
	NudgeProcessorKey = "nudge-processor"
)

// Processor handles periodic nudge email processing
type Processor struct {
	fs                      *internal.FirestoreClient
	emailGenerator          EmailContentGenerator
	emailSender             EmailSender
	logger                  *slog.Logger
	settings                NudgeProcessorSettings
	stopCh                  chan struct{}
	isRunning               bool
	kv                      *cloudflare.KV // Cloudflare KV client for feature flags
	emailsSentInTestSession int            // Track total emails sent in test mode
}

// NewProcessor creates a new nudge processor
func NewProcessor(fs *internal.FirestoreClient, emailGenerator EmailContentGenerator, emailSender EmailSender, logger *slog.Logger) *Processor {
	return &Processor{
		fs:             fs,
		emailGenerator: emailGenerator,
		emailSender:    emailSender,
		logger:         logger,
		settings:       DefaultNudgeSettings,
		stopCh:         make(chan struct{}),
	}
}

// NewProcessorWithDefaults creates a new nudge processor with default implementations
func NewProcessorWithDefaults(fs *internal.FirestoreClient, smtpConfig *smtp.Config, logger *slog.Logger) *Processor {
	templateReader := NewAssetTemplateReader()
	promptProcessor := NewLLMPromptProcessor()
	emailGenerator := NewEmailGenerator(templateReader, promptProcessor)
	emailSender := NewSMTPEmailSender(smtpConfig, logger)

	return NewProcessor(fs, emailGenerator, emailSender, logger)
}

// Start begins the periodic nudge processing
func (p *Processor) Start(ctx context.Context) error {
	if p.isRunning {
		return fmt.Errorf("nudge processor is already running")
	}

	if !p.settings.Enabled {
		p.logger.Info("nudge processor is disabled")
		return nil
	}

	// Initialize Cloudflare KV client for feature flags
	if err := p.initCloudflareKV(); err != nil {
		// Report init error to Sentry
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"operation": "init_cloudflare_kv",
			"component": "nudge_processor",
		})
		p.logger.Error("failed to initialize Cloudflare KV client", "error", err)
		// Continue without feature flags
	}

	// Refresh settings from feature flags
	if err := p.RefreshSettings(); err != nil {
		// Report refresh error to Sentry
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"operation": "refresh_settings",
			"component": "nudge_processor",
		})
		p.logger.Error("failed to refresh nudge processor settings", "error", err)
		// Continue with current settings
	}

	p.isRunning = true
	p.logger.Info("starting nudge processor",
		"interval", p.settings.Interval,
		"max_emails_per_run", p.settings.MaxEmailsPerRun,
		"dry_run", p.settings.DryRun,
		"test_mode", p.settings.TestModeEnabled)

	go p.processLoop(ctx)
	return nil
}

// Stop gracefully shuts down the processor
func (p *Processor) Stop() error {
	if !p.isRunning {
		return nil
	}

	p.logger.Info("stopping nudge processor")
	close(p.stopCh)
	p.isRunning = false
	return nil
}

// UpdateSettings updates the processor configuration
func (p *Processor) UpdateSettings(settings NudgeProcessorSettings) {
	p.settings = settings
	p.logger.Info("updated nudge processor settings",
		"enabled", settings.Enabled,
		"interval", settings.Interval)
}

// initCloudflareKV initializes the Cloudflare KV client
func (p *Processor) initCloudflareKV() error {
	kv, err := cloudflare.NewKV()
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare KV client: %w", err)
	}
	p.kv = kv
	return nil
}

// RefreshSettings refreshes the processor settings from Cloudflare KV
func (p *Processor) RefreshSettings() error {
	if p.kv == nil {
		return nil // No KV client available
	}

	settings := p.settings
	if err := p.kv.Get(NudgeProcessorKey, &settings); err != nil {
		if err != cloudflare.ErrNotFound {
			return fmt.Errorf("failed to get nudge processor settings: %w", err)
		}
		// If not found, use current settings as default

		if err := p.kv.Set(NudgeProcessorKey, p.settings, 0); err != nil {
			return fmt.Errorf("failed to set default nudge processor settings: %w", err)
		}
		return nil
	}

	p.settings = settings
	p.logger.Info("refreshed nudge processor settings from feature flags",
		"enabled", settings.Enabled,
		"dry_run", settings.DryRun,
		"test_mode", settings.TestModeEnabled)
	return nil
}

// processLoop runs the main processing loop
func (p *Processor) processLoop(ctx context.Context) {
	ticker := time.NewTicker(p.settings.Interval)
	defer ticker.Stop()

	// Run immediately on startup
	p.runProcessingCycle(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("nudge processor stopped due to context cancellation")
			return
		case <-p.stopCh:
			p.logger.Info("nudge processor stopped")
			return
		case <-ticker.C:
			if p.settings.Paused {
				p.logger.Debug("nudge processor is paused, skipping cycle")
				continue
			}

			if !p.isOperatingHours() {
				p.logger.Debug("outside operating hours, skipping cycle")
				continue
			}

			p.runProcessingCycle(ctx)
		}
	}
}

// runProcessingCycle executes one complete processing cycle
func (p *Processor) runProcessingCycle(ctx context.Context) {
	p.logger.Debug("starting nudge processing cycle")
	startTime := time.Now()

	stats := &ProcessingStats{
		StartTime: startTime,
	}

	// Process each nudge type in priority order
	nudgeTypes := []NudgeType{
		NudgeTypeNoResume,        // Priority 1
		NudgeTypeNoCall,          // Priority 2
		NudgeTypeFiveStarMatches, // Priority 3
	}

	for _, nudgeType := range nudgeTypes {
		if stats.TotalEmailsSent >= p.settings.MaxEmailsPerRun {
			p.logger.Info("reached max emails per run limit", "limit", p.settings.MaxEmailsPerRun)
			break
		}

		typeStats := p.processNudgeType(ctx, nudgeType, p.settings.MaxEmailsPerRun-stats.TotalEmailsSent)
		stats.AddTypeStats(nudgeType, typeStats)
	}

	stats.EndTime = time.Now()
	stats.Duration = stats.EndTime.Sub(stats.StartTime)

	p.logger.Info("completed nudge processing cycle",
		"duration", stats.Duration,
		"total_emails", stats.TotalEmailsSent,
		"total_errors", stats.TotalErrors,
		"no_resume", stats.TypeStats[NudgeTypeNoResume].EmailsSent,
		"no_call", stats.TypeStats[NudgeTypeNoCall].EmailsSent,
		"five_star", stats.TypeStats[NudgeTypeFiveStarMatches].EmailsSent)
}

// processNudgeType processes all users eligible for a specific nudge type
func (p *Processor) processNudgeType(ctx context.Context, nudgeType NudgeType, maxEmails int) *TypeStats {
	stats := &TypeStats{}

	p.logger.Debug("processing nudge type", "type", nudgeType, "max_emails", maxEmails)

	// Get eligible users
	users, err := GetNudgeEligibleUsers(ctx, p.logger, p.fs, nudgeType, p.settings)
	if err != nil {
		p.handleError(ctx, err, "get_eligible_users", string(nudgeType))
		stats.Errors++
		return stats
	}

	stats.EligibleUsers = len(users)
	p.logger.Debug("found eligible users", "type", nudgeType, "count", len(users))

	// Sort users by priority if needed (e.g., by last access time)
	p.sortUsersByPriority(users, nudgeType)

	// Process users up to email limit
	processed := 0
	for _, user := range users {
		if processed >= maxEmails {
			break
		}

		sent, err := p.processUser(ctx, user, nudgeType)
		if err != nil {
			stats.Errors++
		} else if sent {
			stats.EmailsSent++
		}
		processed++
	}

	return stats
}

// processUser processes a single user for nudge email
func (p *Processor) processUser(ctx context.Context, user *internal.User, nudgeType NudgeType) (sent bool, err error) {
	p.logger.Debug("processing user for nudge",
		"user_id", user.ID,
		"email", user.Email,
		"type", nudgeType)

	// Safety checks for testing
	if !p.isEmailSafeToSend(user.Email) {
		p.logger.Info("email not safe to send in current mode, skipping",
			"user_id", user.ID,
			"email", user.Email,
			"test_mode", p.settings.TestModeEnabled,
			"dry_run", p.settings.DryRun)
		return false, nil
	}

	var additionalData map[string]any

	// Additional eligibility checks and data gathering for five-star matches
	if nudgeType == NudgeTypeFiveStarMatches {
		const rating = 5
		const daysSince = 90 // Look back for 90 days
		userJobs, err := internal.GetNewUserJobsWithRatingOrBetter(ctx, p.fs, user.ID, rating, daysSince)
		if err != nil {
			p.handleError(ctx, err, "check_five_star_matches", user.ID)
			return false, err
		}

		// Filter out jobs that have already been sent in a nudge
		var newJobs []internal.UserJob
		for _, job := range userJobs {
			if job.FiveStarNudgeSentAt == nil {
				newJobs = append(newJobs, job)
			}
		}

		if len(newJobs) == 0 {
			p.logger.Debug("user has no new five-star matches to be notified about, skipping", "user_id", user.ID)
			return false, nil // Not an error, just a successful skip
		}

		// Get Job Details for all matches
		jobIDs := make([]string, len(newJobs))
		for i, uj := range newJobs {
			jobIDs[i] = uj.ID
		}
		jobDetails, err := internal.GetJobDetailsByIDs(ctx, p.fs, jobIDs)
		if err != nil {
			p.handleError(ctx, err, "get_job_details_for_nudge", user.ID)
			return false, err
		}
		jobDetailsMap := make(map[string]internal.JobDetail, len(jobDetails))
		for _, jd := range jobDetails {
			jobDetailsMap[jd.ID] = jd
		}

		// Construct match data for template
		matches := make([]FiveStarMatchData, 0, len(newJobs))
		for _, uj := range newJobs {
			jd, ok := jobDetailsMap[uj.ID]
			if !ok {
				continue // Should not happen, but good to be safe
			}
			var summary string
			if jd.ExtractedData != nil {
				summary = jd.ExtractedData.Summary
			}
			matches = append(matches, FiveStarMatchData{
				Title:    jd.Title,
				Summary:  summary,
				Analysis: uj.JobMatchAnalysis.Summary,
			})
		}
		additionalData = map[string]any{
			"matches":  matches,
			"userJobs": newJobs,
		}
	}

	// Send nudge email with transaction
	success := p.sendNudgeWithTracking(ctx, user, nudgeType, additionalData)

	// Track emails sent in test mode
	if p.settings.TestModeEnabled && success {
		p.emailsSentInTestSession++
	}

	return success, nil
}

// sendNudgeWithTracking sends a nudge email and updates tracking atomically
func (p *Processor) sendNudgeWithTracking(ctx context.Context, user *internal.User, nudgeType NudgeType, additionalData map[string]any) bool {
	err := p.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Build email data
		emailData := p.emailGenerator.BuildEmailData(user, nudgeType, additionalData)

		// Generate email content
		generatedEmail, err := p.emailGenerator.GenerateEmailContent(ctx, user, nudgeType, emailData)
		if err != nil {
			return fmt.Errorf("failed to generate email: %w", err)
		}

		// Send email (or simulate in dry run mode)
		if p.settings.DryRun {
			p.logger.Info("DRY RUN: would send nudge email",
				"user_id", user.ID,
				"email", user.Email,
				"type", nudgeType,
				"subject", generatedEmail.Subject,
				"body_length", len(generatedEmail.PlainBody))
		} else {
			if err := p.emailSender.SendEmail(ctx, user.Email, generatedEmail, nudgeType); err != nil {
				return fmt.Errorf("failed to send email: %w", err)
			}
		}

		// Update user tracking within transaction
		if err := UpdateUserNudgeTrackingTx(ctx, p.fs, tx, user.ID, nudgeType.String()); err != nil {
			return fmt.Errorf("failed to update user tracking: %w", err)
		}

		// If this is a five-star match nudge, update the UserJob records
		if nudgeType == NudgeTypeFiveStarMatches {
			if userJobs, ok := additionalData["userJobs"].([]internal.UserJob); ok {
				sentAt := time.Now()
				for _, uj := range userJobs {
					if err := internal.SetUserJobFiveStarNudgeSentAtTx(ctx, p.fs, tx, user.ID, uj.ID, sentAt); err != nil {
						return fmt.Errorf("failed to update user job nudge sent at for job %s: %w", uj.ID, err)
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		p.handleError(ctx, err, "send_nudge_with_tracking", user.ID)
		return false
	}

	p.logger.Info("successfully sent nudge email",
		"user_id", user.ID,
		"email", user.Email,
		"type", nudgeType)

	return true
}

// sortUsersByPriority sorts users for processing priority
func (p *Processor) sortUsersByPriority(users []*internal.User, nudgeType NudgeType) {
	sort.Slice(users, func(i, j int) bool {
		// Prioritize users who haven't received any nudges
		if users[i].NudgeEmailsSentCount != users[j].NudgeEmailsSentCount {
			return users[i].NudgeEmailsSentCount < users[j].NudgeEmailsSentCount
		}

		// Then prioritize by last access time (older users first)
		return users[i].LastAccessedAt.Before(users[j].LastAccessedAt)
	})
}

// isOperatingHours checks if current time is within operating hours
func (p *Processor) isOperatingHours() bool {
	if p.settings.StartHour == 0 && p.settings.EndHour == 0 {
		return true // 24/7 operation
	}

	now := time.Now().UTC()
	hour := now.Hour()

	if p.settings.StartHour <= p.settings.EndHour {
		// Same day range (e.g., 9 AM to 5 PM)
		return hour >= p.settings.StartHour && hour < p.settings.EndHour
	} else {
		// Crosses midnight (e.g., 10 PM to 6 AM)
		return hour >= p.settings.StartHour || hour < p.settings.EndHour
	}
}

// handleError reports errors to Sentry and logs them
func (p *Processor) handleError(ctx context.Context, err error, operation string, userID string) {
	internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
		"operation": operation,
		"userID":    userID,
		"component": "nudge_processor",
	})

	p.logger.Error("nudge processor error",
		"operation", operation,
		"user_id", userID,
		"error", err)
}

// ProcessingStats tracks statistics for a processing cycle
type ProcessingStats struct {
	StartTime       time.Time
	EndTime         time.Time
	Duration        time.Duration
	TotalEmailsSent int
	TotalErrors     int
	TypeStats       map[NudgeType]*TypeStats
}

// TypeStats tracks statistics for a specific nudge type
type TypeStats struct {
	EligibleUsers int
	EmailsSent    int
	Errors        int
}

// AddTypeStats adds type-specific stats to overall stats
func (ps *ProcessingStats) AddTypeStats(nudgeType NudgeType, stats *TypeStats) {
	if ps.TypeStats == nil {
		ps.TypeStats = make(map[NudgeType]*TypeStats)
	}
	ps.TypeStats[nudgeType] = stats
	ps.TotalEmailsSent += stats.EmailsSent
	ps.TotalErrors += stats.Errors
}

// GetStats returns current processing statistics
func (p *Processor) GetStats(ctx context.Context) (map[string]interface{}, error) {
	nudgeStats, err := GetNudgeStats(ctx, p.fs)
	if err != nil {
		// Report GetStats error to Sentry
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"operation": "get_nudge_stats",
			"component": "nudge_processor",
		})
		return nil, fmt.Errorf("failed to get nudge stats: %w", err)
	}

	stats := map[string]interface{}{
		"enabled":     p.settings.Enabled,
		"paused":      p.settings.Paused,
		"is_running":  p.isRunning,
		"settings":    p.settings,
		"nudge_stats": nudgeStats,
	}

	return stats, nil
}

// isEmailSafeToSend checks if an email is safe to send based on current settings
func (p *Processor) isEmailSafeToSend(email string) bool {
	// Always log what we're checking
	p.logger.Debug("checking email safety",
		"email", email,
		"test_mode", p.settings.TestModeEnabled,
		"dry_run", p.settings.DryRun)

	// In test mode, apply additional restrictions
	if p.settings.TestModeEnabled {
		// Check if we've exceeded test email limit
		if p.settings.MaxTestEmailsTotal > 0 && p.emailsSentInTestSession >= p.settings.MaxTestEmailsTotal {
			p.logger.Warn("test email limit exceeded",
				"sent", p.emailsSentInTestSession,
				"limit", p.settings.MaxTestEmailsTotal)
			return false
		}

		// Check allowed test emails list
		if len(p.settings.AllowedTestEmails) > 0 {
			for _, allowedEmail := range p.settings.AllowedTestEmails {
				if email == allowedEmail {
					return true
				}
			}
			p.logger.Info("email not in allowed test emails list", "email", email)
			return false
		}

		// Check test domain requirements
		if p.settings.RequireTestDomain {
			emailDomain := getEmailDomain(email)
			for _, testDomain := range p.settings.TestDomains {
				if emailDomain == testDomain {
					return true
				}
			}
			p.logger.Info("email not in allowed test domains",
				"email", email,
				"domain", emailDomain,
				"allowed_domains", p.settings.TestDomains)
			return false
		}
	}

	return true
}

// getEmailDomain extracts the domain from an email address
func getEmailDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}

func GetNudgeEligibleUsers(ctx context.Context, logger *slog.Logger, fs *internal.FirestoreClient, nudgeType NudgeType, settings NudgeProcessorSettings) ([]*internal.User, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}

	// Fetch all users since Firestore queries can be tricky with missing fields.
	// This allows us to apply logic exhaustively and log decisions.
	logger.Debug("fetching all users to check for nudge eligibility")
	var allUsers []*internal.User
	offset := 0
	limit := 200 // Process in batches of 200
	for {
		batch, err := internal.ListUsers(ctx, fs, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("failed to list users: %w", err)
		}
		allUsers = append(allUsers, batch...)
		if len(batch) < limit {
			break
		}
		offset += limit
	}
	logger.Debug("finished fetching users", "total_users", len(allUsers))

	var eligibleUsers []*internal.User
	cutoffTime := time.Now()

	// Determine cutoff time based on nudge type
	switch nudgeType {
	case NudgeTypeNoResume:
		if settings.NoResumeDelayDays > 0 {
			cutoffTime = cutoffTime.Add(-time.Duration(settings.NoResumeDelayDays) * 24 * time.Hour)
		}
	case NudgeTypeNoCall:
		if settings.NoCallDelayDays > 0 {
			cutoffTime = cutoffTime.Add(-time.Duration(settings.NoCallDelayDays) * 24 * time.Hour)
		}
	case NudgeTypeFiveStarMatches:
		if settings.MatchAlertDelayDays > 0 {
			cutoffTime = cutoffTime.Add(-time.Duration(settings.MatchAlertDelayDays) * 24 * time.Hour)
		}
	}

	for _, user := range allUsers {
		// 1. Check user status based on nudge type
		var statusMatches bool
		switch nudgeType {
		case NudgeTypeNoResume:
			statusMatches = (user.State.Status == internal.UserStatusPendingResumeUpload)
		case NudgeTypeNoCall:
			statusMatches = (user.State.Status == internal.UserStatusResumeUploaded)
		case NudgeTypeFiveStarMatches:
			statusMatches = (user.State.Status >= internal.UserStatusCallCompleted)
		}

		if !statusMatches {
			logger.Debug(internal.Black("user not eligible for nudge"), "name", user.Name, "user_id", user.ID, "nudge_type", nudgeType, "reason", "incorrect_status", "user_status", user.State.Status)
			continue
		}

		// 3. Perform other eligibility checks
		var lastNudgeSentLog any = "never"
		if user.LastNudgeEmailSent != nil {
			lastNudgeSentLog = *user.LastNudgeEmailSent
		}
		logger.Debug("checking user eligibility details",
			"user_id", user.ID,
			"last_nudge_sent_at", lastNudgeSentLog,
			"nudge_count", user.NudgeEmailsSentCount,
			"weekly_rate_limit", settings.WeeklyRateLimit.String(),
		)

		eligible, reason := isUserEligibleForNudge(user, nudgeType, cutoffTime, settings)
		if !eligible {
			logger.Debug(internal.Black("user not eligible for nudge"), "name", user.Name, "user_id", user.ID, "nudge_type", nudgeType, "reason", reason)
			continue
		}

		logger.Info(internal.Cyan("user IS eligible for nudge"), "name", user.Name, "user_id", user.ID, "email", user.Email, "nudge_type", nudgeType)
		eligibleUsers = append(eligibleUsers, user)
	}

	return eligibleUsers, nil
}

// isUserEligibleForNudge performs detailed eligibility checks
func isUserEligibleForNudge(user *internal.User, nudgeType NudgeType, cutoffTime time.Time, settings NudgeProcessorSettings) (bool, string) {
	if user.SubscriptionEnd != nil && user.SubscriptionEnd.Before(time.Now()) {
		return false, "expired subscription"
	}

	// Global rate limiting: check if any nudge was sent recently
	if user.LastNudgeEmailSent != nil {
		timeSinceLastNudge := time.Since(*user.LastNudgeEmailSent)
		if timeSinceLastNudge < settings.WeeklyRateLimit {
			return false, fmt.Sprintf("rate limited (last nudge sent %v ago)", timeSinceLastNudge.Round(time.Minute))
		}
	}

	// Check for recent user activity based on per-type setting
	var limitOnActivity bool
	switch nudgeType {
	case NudgeTypeNoResume:
		limitOnActivity = settings.LimitOnRecentActivityPerType.NoResume
	case NudgeTypeNoCall:
		limitOnActivity = settings.LimitOnRecentActivityPerType.NoCall
	case NudgeTypeFiveStarMatches:
		limitOnActivity = settings.LimitOnRecentActivityPerType.FiveStarMatches
		// case NudgeTypeFollowUp:
		// 	limitOnActivity = settings.LimitOnRecentActivityPerType.FollowUp
	}

	if limitOnActivity {
		if !user.LastAccessedAt.IsZero() && user.LastAccessedAt.After(cutoffTime) {
			return false, fmt.Sprintf("recent activity (last accessed %s is after cutoff %s)",
				user.LastAccessedAt.Format(time.RFC3339),
				cutoffTime.Format(time.RFC3339))
		}
	}

	// Check type-specific nudge limits
	if user.NudgeTypeCount != nil {
		var maxAllowed int
		switch nudgeType {
		case NudgeTypeNoResume:
			maxAllowed = settings.MaxNudgesPerType.NoResume
		case NudgeTypeNoCall:
			maxAllowed = settings.MaxNudgesPerType.NoCall
		case NudgeTypeFiveStarMatches:
			maxAllowed = settings.MaxNudgesPerType.FiveStarMatches
			// case NudgeTypeFollowUp:
			// 	maxAllowed = settings.MaxNudgesPerType.FollowUp
		}

		currentCount := user.NudgeTypeCount[string(nudgeType)]
		if currentCount >= maxAllowed {
			return false, fmt.Sprintf("reached maximum allowed nudges for type %s (%d)", nudgeType, maxAllowed)
		}
	}

	return true, "eligible"
}

func UpdateUserNudgeTrackingTx(ctx context.Context, fs *internal.FirestoreClient, tx *firestore.Transaction, userID string, nudgeType string) error {
	if fs == nil {
		return fmt.Errorf("firestore client is nil")
	}

	now := time.Now()
	updates := []firestore.Update{
		{Path: "LastNudgeEmailSent", Value: now},
		{Path: "LastNudgeEmailType", Value: nudgeType},
		{Path: "NudgeEmailsSentCount", Value: firestore.Increment(1)},
		{Path: fmt.Sprintf("NudgeTypeCount.%s", nudgeType), Value: firestore.Increment(1)},
		{Path: "UpdatedAt", Value: now},
	}

	return tx.Update(fs.Collection("users").Doc(userID), updates)
}

func GetNudgeStats(ctx context.Context, fs *internal.FirestoreClient) (map[string]int, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}

	stats := map[string]int{
		"total_users_with_nudges": 0,
		"no_resume_nudges":        0,
		"no_call_nudges":          0,
		"five_star_nudges":        0,
	}

	iter := fs.Collection("users").
		Where("NudgeEmailsSentCount", ">", 0).
		Documents(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate nudge stats: %w", err)
		}

		var user internal.User
		if err := doc.DataTo(&user); err != nil {
			continue
		}

		stats["total_users_with_nudges"]++

		if user.LastNudgeEmailType != nil {
			switch *user.LastNudgeEmailType {
			case "no_resume":
				stats["no_resume_nudges"]++
			case "no_call":
				stats["no_call_nudges"]++
			case "five_star_matches":
				stats["five_star_nudges"]++
			}
		}
	}

	return stats, nil
}
