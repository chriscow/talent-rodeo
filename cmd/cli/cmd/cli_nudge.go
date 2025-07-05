package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/nudge"

	"github.com/spf13/cobra"
	"google.golang.org/api/iterator"
)

// PredictedNudge represents a predicted nudge for a user
type PredictedNudge struct {
	User          *internal.User
	NudgeType     nudge.NudgeType
	WillBeSentAt  time.Time
	DaysUntilSent int
	Reason        string
}

var nudgeCmd = &cobra.Command{
	Use:   "nudge",
	Short: "Manage nudge emails",
	Long:  `Nudge command allows you to manage and predict nudge emails in the system`,
}

var nudgeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List predicted nudges for users",
	Long:  `List users who will receive nudges in the future based on their current state and nudge settings`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Create logger
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

		// Create Firestore client
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		// Get nudge settings (use defaults for prediction)
		settings := nudge.DefaultNudgeSettings

		// Get predicted nudges
		predictions, err := getPredictedNudges(ctx, logger, fs, settings)
		if err != nil {
			return fmt.Errorf("failed to get predicted nudges: %w", err)
		}

		// Display results
		displayPredictions(predictions)

		return nil
	},
}

var nudgeStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show nudge statistics",
	Long:  `Show statistics about nudge emails that have been sent`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Create Firestore client
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		// Get nudge statistics
		stats, err := nudge.GetNudgeStats(ctx, fs)
		if err != nil {
			return fmt.Errorf("failed to get nudge stats: %w", err)
		}

		// Display stats
		fmt.Printf("Nudge Email Statistics:\n")
		fmt.Printf("Total users with nudges: %d\n", stats["total_users_with_nudges"])
		fmt.Printf("No resume nudges sent: %d\n", stats["no_resume_nudges"])
		fmt.Printf("No call nudges sent: %d\n", stats["no_call_nudges"])
		fmt.Printf("Five star match nudges sent: %d\n", stats["five_star_nudges"])
		fmt.Printf("Trial welcome nudges sent: %d\n", stats["trial_welcome_nudges"])
		fmt.Printf("Trial expectations nudges sent: %d\n", stats["trial_expectations_nudges"])
		fmt.Printf("Trial engagement nudges sent: %d\n", stats["trial_engagement_nudges"])
		fmt.Printf("Trial conversion nudges sent: %d\n", stats["trial_conversion_nudges"])

		return nil
	},
}

var nudgeTestEmailCmd = &cobra.Command{
	Use:   "test-email",
	Short: "Generate a test email for preview",
	Long:  `Generate a test email using either real user data or synthetic test data for email template testing`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Get flags
		emailType, _ := cmd.Flags().GetString("type")
		userID, _ := cmd.Flags().GetString("user-id")
		useTestData, _ := cmd.Flags().GetBool("test-data")
		outputFormat, _ := cmd.Flags().GetString("format")
		qualityMatches, _ := cmd.Flags().GetInt("quality-matches")

		// Validate email type
		var nudgeType nudge.NudgeType
		switch emailType {
		case "trial_welcome":
			nudgeType = nudge.NudgeTypeTrialWelcome
		case "trial_expectations":
			nudgeType = nudge.NudgeTypeTrialExpectations
		case "trial_engagement":
			nudgeType = nudge.NudgeTypeTrialEngagement
		case "trial_conversion":
			nudgeType = nudge.NudgeTypeTrialConversion
		case "no_resume":
			nudgeType = nudge.NudgeTypeNoResume
		case "no_call":
			nudgeType = nudge.NudgeTypeNoCall
		case "five_star_matches":
			nudgeType = nudge.NudgeTypeFiveStarMatches
		default:
			return fmt.Errorf("invalid email type: %s. Valid types: trial_welcome, trial_expectations, trial_engagement, trial_conversion, no_resume, no_call, five_star_matches", emailType)
		}

		// Create Firestore client
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		// Create logger
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

		var user *internal.User

		if useTestData {
			// Create synthetic test user
			user = createTestUser(nudgeType)
		} else if userID != "" {
			// Get real user data
			var found bool
			user, found, err = internal.GetUser(ctx, fs, userID)
			if err != nil {
				return fmt.Errorf("failed to get user %s: %w", userID, err)
			}
			if !found {
				return fmt.Errorf("user %s not found", userID)
			}
		} else {
			return fmt.Errorf("must specify either --user-id or --test-data")
		}

		// Initialize template cache for email generation
		if err := nudge.LoadAndCacheTemplates(); err != nil {
			return fmt.Errorf("failed to load email templates: %w", err)
		}

		// Create email generator
		templateReader := nudge.NewAssetTemplateReader()
		emailGenerator := nudge.NewEmailGenerator(templateReader, nil)

		// Build additional data
		additionalData := make(map[string]any)
		if qualityMatches >= 0 {
			additionalData["qualityMatches"] = qualityMatches
		}

		// Build email data
		emailData := emailGenerator.BuildEmailData(user, nudgeType, additionalData)

		// Generate email
		email, err := emailGenerator.GenerateEmailContent(ctx, fs, logger, user, nudgeType, emailData)
		if err != nil {
			return fmt.Errorf("failed to generate email: %w", err)
		}

		// Output the result
		switch outputFormat {
		case "plain":
			fmt.Printf("Subject: %s\n\n%s\n", email.Subject, email.PlainBody)
		case "html":
			fmt.Printf("Subject: %s\n\n%s\n", email.Subject, email.HTMLBody)
		case "both":
			fmt.Printf("Subject: %s\n\n=== PLAIN TEXT ===\n%s\n\n=== HTML ===\n%s\n",
				email.Subject, email.PlainBody, email.HTMLBody)
		case "json":
			emailJSON := map[string]string{
				"subject": email.Subject,
				"plain":   email.PlainBody,
				"html":    email.HTMLBody,
			}
			jsonBytes, _ := json.MarshalIndent(emailJSON, "", "  ")
			fmt.Println(string(jsonBytes))
		default:
			return fmt.Errorf("invalid format: %s. Valid formats: plain, html, both, json", outputFormat)
		}

		return nil
	},
}

// getPredictedNudges analyzes all users and predicts when they will receive nudges
func getPredictedNudges(ctx context.Context, logger *slog.Logger, fs *internal.FirestoreClient, settings nudge.NudgeProcessorSettings) ([]PredictedNudge, error) {
	// Get all users from the database
	var allUsers []*internal.User
	offset := 0
	limit := 200 // Process in batches

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

	var predictions []PredictedNudge
	now := time.Now()

	// Define nudge types in priority order
	nudgeTypes := []nudge.NudgeType{
		nudge.NudgeTypeNoResume,
		nudge.NudgeTypeNoCall,
		nudge.NudgeTypeFiveStarMatches,
		nudge.NudgeTypeTrialWelcome,
		nudge.NudgeTypeTrialExpectations,
		nudge.NudgeTypeTrialEngagement,
		nudge.NudgeTypeTrialConversion,
	}

	for _, user := range allUsers {
		// Skip users with expired subscriptions
		if user.SubscriptionEnd != nil && user.SubscriptionEnd.Before(now) {
			continue
		}

		// Find the next nudge this user will receive
		prediction := predictNextNudge(ctx, fs, user, nudgeTypes, settings, now)
		if prediction != nil {
			predictions = append(predictions, *prediction)
		}
	}

	return predictions, nil
}

// predictNextNudge predicts the next nudge a user will receive
func predictNextNudge(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, nudgeTypes []nudge.NudgeType, settings nudge.NudgeProcessorSettings, now time.Time) *PredictedNudge {
	// Check each nudge type in priority order
	for _, nudgeType := range nudgeTypes {
		prediction := predictNudgeForType(ctx, fs, user, nudgeType, settings, now)
		if prediction != nil {
			return prediction
		}
	}
	return nil
}

// checkUserHasFiveStarMatches checks if a user has actual five-star matches that haven't been sent
func checkUserHasFiveStarMatches(ctx context.Context, fs *internal.FirestoreClient, userID string) (bool, error) {
	// Query for user jobs with high scores created within last 90 days
	cutoffDate := time.Now().Add(-90 * 24 * time.Hour)

	iter := fs.Collection("users").Doc(userID).Collection("user_jobs").
		Where("Score", ">=", 4.5).
		Where("CreatedAt", ">=", cutoffDate).
		Where("FiveStarNudgeSentAt", "==", nil).
		Limit(1). // We only need to know if any exist
		Documents(ctx)

	_, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return false, nil // No matches found
		}
		return false, fmt.Errorf("failed to check for five-star matches: %w", err)
	}

	return true, nil // Found at least one match
}

// predictNudgeForType predicts when a user will receive a specific nudge type
func predictNudgeForType(ctx context.Context, fs *internal.FirestoreClient, user *internal.User, nudgeType nudge.NudgeType, settings nudge.NudgeProcessorSettings, now time.Time) *PredictedNudge {
	// Check if user's status matches the nudge type
	var statusMatches bool
	var eligibleTime time.Time

	switch nudgeType {
	case nudge.NudgeTypeNoResume:
		statusMatches = (user.Status == internal.UserStatusPendingResumeUpload)
		// Use the time they became eligible (status update time or creation time)
		if user.StatusUpdatedAt.IsZero() {
			eligibleTime = user.CreatedAt
		} else {
			eligibleTime = user.StatusUpdatedAt
		}
	case nudge.NudgeTypeNoCall:
		statusMatches = (user.Status == internal.UserStatusResumeUploaded)
		if user.StatusUpdatedAt.IsZero() {
			eligibleTime = user.CreatedAt
		} else {
			eligibleTime = user.StatusUpdatedAt
		}
	case nudge.NudgeTypeFiveStarMatches:
		statusMatches = (user.Status >= internal.UserStatusCallCompleted)
		// For five star matches, also check if they actually have matches
		if statusMatches {
			hasMatches, err := checkUserHasFiveStarMatches(ctx, fs, user.ID)
			if err != nil {
				// Log error but don't fail completely - could add logging here
				statusMatches = false
			} else {
				statusMatches = hasMatches
			}
		}
		// For five star matches, use last access time as the reference point
		eligibleTime = user.LastAccessedAt
	case nudge.NudgeTypeTrialWelcome, nudge.NudgeTypeTrialExpectations, nudge.NudgeTypeTrialEngagement, nudge.NudgeTypeTrialConversion:
		// Trial emails are for users in trial period
		statusMatches = (user.Status >= internal.UserStatusCallCompleted && user.SubscriptionEnd != nil && user.SubscriptionEnd.After(now))
		if user.SubscriptionEnd != nil {
			// Trial starts 7 days before subscription end
			eligibleTime = user.SubscriptionEnd.Add(-7 * 24 * time.Hour)
		}
	}

	if !statusMatches {
		return nil
	}

	// Calculate when the nudge would be sent
	var sendTime time.Time
	var delayDays int

	switch nudgeType {
	case nudge.NudgeTypeNoResume:
		delayDays = settings.NoResumeDelayDays
		sendTime = eligibleTime.Add(time.Duration(delayDays) * 24 * time.Hour)
	case nudge.NudgeTypeNoCall:
		delayDays = settings.NoCallDelayDays
		sendTime = eligibleTime.Add(time.Duration(delayDays) * 24 * time.Hour)
	case nudge.NudgeTypeFiveStarMatches:
		delayDays = settings.MatchAlertDelayDays
		sendTime = eligibleTime.Add(time.Duration(delayDays) * 24 * time.Hour)
	case nudge.NudgeTypeTrialWelcome:
		delayDays = settings.TrialWelcomeDelayDays
		sendTime = eligibleTime.Add(time.Duration(delayDays-1) * 24 * time.Hour) // Day 1 = 0 days after start
	case nudge.NudgeTypeTrialExpectations:
		delayDays = settings.TrialExpectationsDelayDays
		sendTime = eligibleTime.Add(time.Duration(delayDays-1) * 24 * time.Hour) // Day 2 = 1 day after start
	case nudge.NudgeTypeTrialEngagement:
		delayDays = settings.TrialEngagementDelayDays
		sendTime = eligibleTime.Add(time.Duration(delayDays-1) * 24 * time.Hour) // Day 3 = 2 days after start
	case nudge.NudgeTypeTrialConversion:
		delayDays = settings.TrialConversionDelayDays
		sendTime = eligibleTime.Add(time.Duration(delayDays-1) * 24 * time.Hour) // Day 6 = 5 days after start
	}

	// For trial emails, use the same precise timing logic as the processor
	if isTrialEmail(nudgeType) {
		// Must have timezone and subscription end date
		if user.Timezone == nil {
			return nil // No timezone set
		}
		if user.SubscriptionEnd == nil {
			return nil // No trial end date
		}

		// Must be in active trial
		if user.SubscriptionEnd.Before(now) {
			return nil // Trial expired
		}

		// Check if it's the right time to send this email (within 24 hours after target)
		timeSinceTarget := now.Sub(sendTime)
		if timeSinceTarget < 0 {
			// Too early - keep the original send time
			// Don't return nil, just use the future send time
		} else if timeSinceTarget > 24*time.Hour {
			return nil // Too late for trial email
		} else {
			// Within the 24-hour window after target - can send now
			sendTime = now
		}
	} else {
		// For non-trial emails, use the original logic
		if sendTime.Before(now) {
			// For other nudge types, if it's overdue, it would be sent immediately
			sendTime = now
		}
	}

	// Check rate limiting - skip for trial emails (they have their own timing logic)
	// Trial emails are limited by per-type limits (max 1 each) and specific timing windows
	if !isTrialEmail(nudgeType) {
		if user.LastNudgeEmailSent != nil {
			timeSinceLastNudge := now.Sub(*user.LastNudgeEmailSent)
			if timeSinceLastNudge < settings.WeeklyRateLimit {
				// User is rate limited, calculate when they can receive the next nudge
				nextAllowedTime := user.LastNudgeEmailSent.Add(settings.WeeklyRateLimit)
				if nextAllowedTime.After(sendTime) {
					sendTime = nextAllowedTime
				}
			}
		}
	}

	// Check per-type limits
	if user.NudgeTypeCount != nil {
		var maxAllowed int
		switch nudgeType {
		case nudge.NudgeTypeNoResume:
			maxAllowed = settings.MaxNudgesPerType.NoResume
		case nudge.NudgeTypeNoCall:
			maxAllowed = settings.MaxNudgesPerType.NoCall
		case nudge.NudgeTypeFiveStarMatches:
			maxAllowed = settings.MaxNudgesPerType.FiveStarMatches
		case nudge.NudgeTypeTrialWelcome:
			maxAllowed = settings.MaxNudgesPerType.TrialWelcome
		case nudge.NudgeTypeTrialExpectations:
			maxAllowed = settings.MaxNudgesPerType.TrialExpectations
		case nudge.NudgeTypeTrialEngagement:
			maxAllowed = settings.MaxNudgesPerType.TrialEngagement
		case nudge.NudgeTypeTrialConversion:
			maxAllowed = settings.MaxNudgesPerType.TrialConversion
		}

		currentCount := user.NudgeTypeCount[string(nudgeType)]
		if currentCount >= maxAllowed {
			return nil // User has reached the limit for this nudge type
		}
	}

	// Check recent activity limitations
	var limitOnActivity bool
	switch nudgeType {
	case nudge.NudgeTypeNoResume:
		limitOnActivity = settings.LimitOnRecentActivityPerType.NoResume
	case nudge.NudgeTypeNoCall:
		limitOnActivity = settings.LimitOnRecentActivityPerType.NoCall
	case nudge.NudgeTypeFiveStarMatches:
		limitOnActivity = settings.LimitOnRecentActivityPerType.FiveStarMatches
	}

	if limitOnActivity {
		activityCutoff := now.Add(-time.Duration(delayDays) * 24 * time.Hour)
		if !user.LastAccessedAt.IsZero() && user.LastAccessedAt.After(activityCutoff) {
			return nil // User has been active too recently
		}
	}

	// Calculate days until sent
	daysUntilSent := int(sendTime.Sub(now).Hours() / 24)
	if daysUntilSent < 0 {
		daysUntilSent = 0
	}

	// Generate reason
	reason := fmt.Sprintf("Status: %s, Eligible since: %s", user.Status.String(), eligibleTime.Format("2006-01-02"))
	if nudgeType == nudge.NudgeTypeFiveStarMatches {
		reason = fmt.Sprintf("Status: %s, Has 5-star matches, Last access: %s", user.Status.String(), eligibleTime.Format("2006-01-02"))
	}

	return &PredictedNudge{
		User:          user,
		NudgeType:     nudgeType,
		WillBeSentAt:  sendTime,
		DaysUntilSent: daysUntilSent,
		Reason:        reason,
	}
}

// displayPredictions displays the predicted nudges in a formatted table
func displayPredictions(predictions []PredictedNudge) {
	if len(predictions) == 0 {
		fmt.Println("No predicted nudges found.")
		return
	}

	// Create a tab writer for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Print header
	fmt.Fprintf(w, "ID\tName\tEmail\tSub Days\tNudge Type\tWhen\tDays\tReason\n")
	fmt.Fprintf(w, "-----\t----\t-----\t--------\t----------\t----\t----\t------\n")

	// Print each prediction
	for _, pred := range predictions {
		// Get first 5 characters of ID
		shortID := pred.User.ID
		if len(shortID) > 5 {
			shortID = shortID[:5]
		}

		// Format name
		name := pred.User.Name
		if name == "" {
			name = fmt.Sprintf("%s %s", pred.User.FirstName, pred.User.LastName)
		}
		if len(name) > 20 {
			name = name[:17] + "..."
		}

		// Format email
		email := pred.User.Email
		if len(email) > 30 {
			email = email[:27] + "..."
		}

		// Format nudge type
		nudgeTypeStr := string(pred.NudgeType)
		if len(nudgeTypeStr) > 15 {
			nudgeTypeStr = nudgeTypeStr[:12] + "..."
		}

		// Format when
		when := funcs.ApproxFuture(pred.WillBeSentAt)

		// Override with "Now" only for immediate sends (within next few minutes)
		if pred.DaysUntilSent == 0 {
			// For trial emails, check if we're actually within the send window
			if isTrialEmail(pred.NudgeType) {
				// Trial email should only show "Now" if it's past the target time
				timeSinceTarget := time.Now().Sub(pred.WillBeSentAt)
				if timeSinceTarget >= 0 && timeSinceTarget <= 30*time.Minute {
					when = "Now"
				}
				// Otherwise use the ApproxFuture result
			} else {
				// For non-trial emails, show "Now" if it's very soon
				timeUntilSend := pred.WillBeSentAt.Sub(time.Now())
				if timeUntilSend <= 5*time.Minute {
					when = "Now"
				}
			}
		}

		// Format days
		daysStr := fmt.Sprintf("%d", pred.DaysUntilSent)
		if pred.DaysUntilSent == 0 {
			daysStr = "0"
		}

		// Format subscription days
		now := time.Now()
		var subDaysStr string
		if pred.User.SubscriptionEnd != nil {
			if pred.User.SubscriptionEnd.After(now) {
				subDays := int(pred.User.SubscriptionEnd.Sub(now).Hours() / 24)
				subDaysStr = fmt.Sprintf("%d", subDays)
			} else {
				subDaysStr = "EXPIRED"
			}
		} else {
			subDaysStr = "N/A"
		}

		// Format reason (truncate if too long)
		reason := pred.Reason
		if len(reason) > 40 {
			reason = reason[:37] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			shortID, name, email, subDaysStr, nudgeTypeStr, when, daysStr, reason)
	}

	fmt.Printf("\nTotal predicted nudges: %d\n", len(predictions))
}

// createTestUser creates a synthetic user for testing
func createTestUser(nudgeType nudge.NudgeType) *internal.User {
	now := time.Now()
	timezone := "America/Los_Angeles"

	user := &internal.User{
		ID:              "test-user-12345",
		FirstName:       "Alex",
		LastName:        "Johnson",
		Name:            "Alex Johnson",
		Email:           "alex.johnson@example.com",
		CreatedAt:       now.Add(-10 * 24 * time.Hour), // 10 days ago
		LastAccessedAt:  now.Add(-2 * 24 * time.Hour),  // 2 days ago
		StatusUpdatedAt: now.Add(-5 * 24 * time.Hour),  // 5 days ago
		Timezone:        &timezone,
		Status:          internal.UserStatusCallCompleted,
		NudgeTypeCount:  make(map[string]int),
	}

	// Set trial data for trial emails
	if isTrialEmail(nudgeType) {
		trialEnd := now.Add(3 * 24 * time.Hour) // Trial ends in 3 days
		user.SubscriptionEnd = &trialEnd
	}

	// Adjust user status based on nudge type
	switch nudgeType {
	case nudge.NudgeTypeNoResume:
		user.Status = internal.UserStatusPendingResumeUpload
	case nudge.NudgeTypeNoCall:
		user.Status = internal.UserStatusResumeUploaded
	case nudge.NudgeTypeFiveStarMatches:
		user.Status = internal.UserStatusCallCompleted
		user.LastAccessedAt = now.Add(-15 * 24 * time.Hour) // 15 days ago for inactivity
	}

	return user
}

// isTrialEmail checks if nudge type is a trial email
func isTrialEmail(nudgeType nudge.NudgeType) bool {
	switch nudgeType {
	case nudge.NudgeTypeTrialWelcome, nudge.NudgeTypeTrialExpectations,
		nudge.NudgeTypeTrialEngagement, nudge.NudgeTypeTrialConversion:
		return true
	default:
		return false
	}
}

func init() {
	// Add nudge command to root
	RootCmd.AddCommand(nudgeCmd)

	// Add subcommands
	nudgeCmd.AddCommand(nudgeListCmd)
	nudgeCmd.AddCommand(nudgeStatsCmd)
	nudgeCmd.AddCommand(nudgeTestEmailCmd)

	// Add flags for test-email command
	nudgeTestEmailCmd.Flags().String("type", "trial_welcome", "Email type to generate (trial_welcome, trial_expectations, trial_engagement, trial_conversion, no_resume, no_call, five_star_matches)")
	nudgeTestEmailCmd.Flags().String("user-id", "", "User ID to use for real data (exclusive with --test-data)")
	nudgeTestEmailCmd.Flags().Bool("test-data", false, "Use synthetic test data (exclusive with --user-id)")
	nudgeTestEmailCmd.Flags().String("format", "both", "Output format (plain, html, both, json)")
	nudgeTestEmailCmd.Flags().Int("quality-matches", 7, "Number of quality matches for trial emails (default: 7)")
}
