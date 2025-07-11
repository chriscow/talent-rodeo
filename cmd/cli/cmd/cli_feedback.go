package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/feedback"
	"talent-rodeo/internal/inbox"
	"talent-rodeo/internal/smtp"
	"time"

	"github.com/spf13/cobra"
)

var feedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Manage and analyze user feedback on job listings",
	Long: `Manage user feedback on job listings to improve job matching accuracy.

User feedback includes likes, dislikes, and ratings on job titles, companies,
descriptions, and other job attributes. This feedback is processed to update
user preferences and improve future job recommendations.

This command group allows you to:
- Process feedback to update user preferences
- Analyze feedback patterns and statistics
- Export feedback data for analysis
- Clean up orphaned feedback records`,
}

var processFeedbackCmd = &cobra.Command{
	Use:   "process",
	Short: "Process all unprocessed user feedback to update preferences",
	Long: `Process all unprocessed user feedback and update SteeringQuery preferences.

This command analyzes user feedback (likes/dislikes on job attributes) and
updates each user's SteeringQuery to improve future job matching. Run this
as a daily batch job to keep user preferences current.

Examples:
  trcli feedback process                               # Process all pending feedback`,
	RunE: processFeedback,
}

var processFeedbackUserCmd = &cobra.Command{
	Use:   "process-user <user-id>",
	Short: "Process feedback for a specific user to update their preferences",
	Long: `Process unprocessed feedback for a specific user and update their SteeringQuery preferences.

This command focuses on a single user's feedback to immediately update their
job matching preferences. Useful for testing or when you want to update a
specific user's preferences without processing all users.

Examples:
  trcli feedback process-user user123                  # Process feedback for specific user`,
	Args: cobra.ExactArgs(1),
	RunE: processFeedbackForUser,
}

var showFeedbackStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show overview statistics about unprocessed feedback",
	Long: `Display statistics about unprocessed user feedback across the system.

Shows counts of likes vs dislikes, number of users with pending feedback,
and identifies the most active users. Useful for monitoring feedback volume
and identifying users who need preference updates.

Examples:
  trcli feedback stats                                 # Show feedback statistics
  trcli feedback stats --json                         # Get statistics in JSON format`,
	RunE: showFeedbackStats,
}

var cleanupOrphanedFeedbackCmd = &cobra.Command{
	Use:   "cleanup-orphaned",
	Short: "Clean up feedback records for deleted users",
	Long: `Mark feedback as processed for users that no longer exist in the system.

This prevents feedback processing errors when users have been deleted but
their feedback records remain. Run this periodically to maintain data
consistency and avoid processing errors.

Examples:
  trcli feedback cleanup-orphaned                     # Clean up orphaned feedback`,
	RunE: cleanupOrphanedFeedback,
}

var listUserFeedbackCmd = &cobra.Command{
	Use:   "list-user <user-id>",
	Short: "List all feedback items for a specific user",
	Long: `Display all feedback items for a specific user, both processed and unprocessed.

Shows the user's feedback history including what they liked/disliked, when
the feedback was given, and whether it has been processed into their preferences.
Useful for understanding a user's feedback patterns.

Examples:
  trcli feedback list-user user123                    # Show user's feedback
  trcli feedback list-user user123 --json             # Get feedback in JSON format`,
	Args: cobra.ExactArgs(1),
	RunE: listUserFeedback,
}

var showFeedbackCmd = &cobra.Command{
	Use:   "show <feedback-id>",
	Short: "Show detailed information about a specific feedback item",
	Long: `Display detailed information about a specific feedback item including all fields.

Shows the complete feedback record including user ID, job ID, feedback type,
text content, and processing status. Useful for debugging feedback processing
or understanding specific feedback details.

Examples:
  trcli feedback show feedback123                     # Show feedback details
  trcli feedback show feedback123 --json              # Get details in JSON format`,
	Args: cobra.ExactArgs(1),
	RunE: showFeedback,
}

var listJobFeedbackCmd = &cobra.Command{
	Use:   "list-job <job-id>",
	Short: "List all feedback for a specific job across all users",
	Long: `Display all feedback items for a specific job across all users.

Shows how different users have rated or provided feedback on the same job.
Useful for understanding job popularity, identifying problematic job listings,
or analyzing feedback patterns for specific positions.

Examples:
  trcli feedback list-job scrapingdog:12345           # Show job feedback
  trcli feedback list-job scrapingdog:12345 --json    # Get feedback in JSON format`,
	Args: cobra.ExactArgs(1),
	RunE: listJobFeedback,
}

var userStatsCmd = &cobra.Command{
	Use:   "user-stats <user-id>",
	Short: "Show comprehensive feedback statistics for a user",
	Long: `Display detailed statistics about a user's feedback activity and patterns.

Shows total feedback count, like/dislike ratios, most recent activity, and
processing status. Useful for understanding user engagement levels and
feedback behavior patterns.

Examples:
  trcli feedback user-stats user123                   # Show user feedback stats
  trcli feedback user-stats user123 --json            # Get stats in JSON format`,
	Args: cobra.ExactArgs(1),
	RunE: showUserStats,
}

var searchDateCmd = &cobra.Command{
	Use:   "search-date <user-id> <start-date> <end-date>",
	Short: "Search user feedback within a specific date range",
	Long: `Search for feedback within a date range for a specific user.

Useful for analyzing feedback patterns over time, understanding how user
preferences have changed, or finding feedback related to specific time periods.

Date format: YYYY-MM-DD

Examples:
  trcli feedback search-date user123 2024-01-01 2024-01-31    # January 2024 feedback
  trcli feedback search-date user123 2024-01-01 2024-01-31 --json`,
	Args: cobra.ExactArgs(3),
	RunE: searchFeedbackByDate,
}

var exportFeedbackCmd = &cobra.Command{
	Use:   "export <user-id> [--format json|csv]",
	Short: "Export user feedback data for external analysis",
	Long: `Export feedback data in JSON or CSV format for analysis in external tools.

The exported data includes all feedback fields: IDs, timestamps, feedback types,
text content, and processing status. Useful for data analysis, reporting, or
migrating feedback data.

Examples:
  trcli feedback export user123                       # Export as JSON (default)
  trcli feedback export user123 --format csv          # Export as CSV
  trcli feedback export user123 --format csv > feedback.csv`,
	Args: cobra.ExactArgs(1),
	RunE: exportFeedback,
}

var inboxFeedbackCmd = &cobra.Command{
	Use:   "inbox",
	Short: "Process inbox emails for feedback analysis (read-only by default)",
	Long: `Process inbox emails for feedback analysis with safety-first design.

By default, this command shows a read-only summary of emails that would be processed
by the feedback system. Use flags to control actual processing.

IMPORTANT: This command only runs the feedback email handler. Other email handlers
(follow-up questions, etc.) that are active in the main web application will NOT
be executed by this command.

Safety Features:
- Default mode is read-only (no changes made)
- Prompts for confirmation when 5+ emails are found
- Continues processing on individual email failures
- Requires explicit flags for actual processing

Examples:
  trcli feedback inbox                                # Read-only summary
  trcli feedback inbox --process                     # Actually process emails
  trcli feedback inbox --test                        # Dry-run with mock email
  trcli feedback inbox --test --user-email user@example.com  # Custom mock email`,
	RunE: inboxFeedback,
}

func init() {
	feedbackCmd.AddCommand(processFeedbackCmd)
	feedbackCmd.AddCommand(processFeedbackUserCmd)
	feedbackCmd.AddCommand(showFeedbackStatsCmd)
	feedbackCmd.AddCommand(cleanupOrphanedFeedbackCmd)
	feedbackCmd.AddCommand(listUserFeedbackCmd)
	feedbackCmd.AddCommand(showFeedbackCmd)
	feedbackCmd.AddCommand(listJobFeedbackCmd)
	feedbackCmd.AddCommand(userStatsCmd)
	feedbackCmd.AddCommand(searchDateCmd)
	feedbackCmd.AddCommand(exportFeedbackCmd)
	feedbackCmd.AddCommand(inboxFeedbackCmd)

	// Add flags
	exportFeedbackCmd.Flags().StringP("format", "f", "json", "Export format (json or csv)")
	inboxFeedbackCmd.Flags().BoolP("process", "p", false, "Actually process emails (default is read-only)")
	inboxFeedbackCmd.Flags().BoolP("test", "t", false, "Run dry-run test with mock email")
	inboxFeedbackCmd.Flags().StringP("user-email", "u", "", "Specific user email for test mode (default: test@example.com)")
	RootCmd.AddCommand(feedbackCmd)
}

func processFeedback(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	fmt.Printf("Starting feedback processing at %s\n", time.Now().Format(time.RFC3339))

	// Process all users' feedback
	err = feedback.ProcessAllUsersFeedback(ctx, fs, logger)
	if err != nil {
		return fmt.Errorf("failed to process user feedback: %w", err)
	}

	fmt.Printf("Feedback processing completed at %s\n", time.Now().Format(time.RFC3339))
	return nil
}

func processFeedbackForUser(cmd *cobra.Command, args []string) error {
	userID := args[0]
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	fmt.Printf("Processing feedback for user %s at %s\n", userID, time.Now().Format(time.RFC3339))

	// Process specific user's feedback
	err = feedback.ProcessUserFeedback(ctx, fs, logger, userID)
	if err != nil {
		return fmt.Errorf("failed to process feedback for user %s: %w", userID, err)
	}

	fmt.Printf("Feedback processing completed for user %s at %s\n", userID, time.Now().Format(time.RFC3339))
	return nil
}

func showFeedbackStats(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	// Get all unprocessed feedback
	feedbacks, err := internal.GetAllUnprocessedFeedback(ctx, fs, 1000)
	if err != nil {
		return fmt.Errorf("failed to get unprocessed feedback: %w", err)
	}

	// Calculate statistics
	totalFeedback := len(feedbacks)
	likedCount := 0
	dislikedCount := 0
	userMap := make(map[string]int)
	jobMap := make(map[string]int)

	for _, feedback := range feedbacks {
		if feedback.Liked {
			likedCount++
		} else {
			dislikedCount++
		}
		userMap[feedback.UserID]++
		jobMap[feedback.JobID]++
	}

	// Output results
	if jsonOutput {
		stats := map[string]interface{}{
			"total_unprocessed": totalFeedback,
			"liked_count":       likedCount,
			"disliked_count":    dislikedCount,
			"unique_users":      len(userMap),
			"unique_jobs":       len(jobMap),
		}

		// Print JSON output
		jsonBytes, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal stats to JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Feedback Statistics:\n")
		fmt.Printf("==================\n")
		fmt.Printf("Total unprocessed feedback: %d\n", totalFeedback)
		fmt.Printf("Liked items: %d\n", likedCount)
		fmt.Printf("Disliked items: %d\n", dislikedCount)
		fmt.Printf("Unique users with feedback: %d\n", len(userMap))
		fmt.Printf("Unique jobs with feedback: %d\n", len(jobMap))

		if totalFeedback > 0 {
			fmt.Printf("\nTop users by feedback count:\n")
			count := 0
			for userID, feedbackCount := range userMap {
				if count >= 5 { // Show top 5
					break
				}
				fmt.Printf("  %s: %d feedback items\n", userID, feedbackCount)
				count++
			}
		}
	}

	return nil
}

func cleanupOrphanedFeedback(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	fmt.Printf("Starting orphaned feedback cleanup at %s\n", time.Now().Format(time.RFC3339))

	// Clean up orphaned feedback
	err = feedback.CleanupOrphanedFeedback(ctx, fs, logger)
	if err != nil {
		return fmt.Errorf("failed to cleanup orphaned feedback: %w", err)
	}

	fmt.Printf("Orphaned feedback cleanup completed at %s\n", time.Now().Format(time.RFC3339))
	return nil
}

func listUserFeedback(cmd *cobra.Command, args []string) error {
	userID := args[0]
	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	// Get all feedback for the user
	feedbacks, err := internal.ListUserFeedback(ctx, fs, userID)
	if err != nil {
		return fmt.Errorf("failed to list user feedback: %w", err)
	}

	if len(feedbacks) == 0 {
		fmt.Printf("No feedback found for user %s\n", userID)
		return nil
	}

	// Output results
	if jsonOutput {
		jsonBytes, err := json.MarshalIndent(feedbacks, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal feedback to JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Feedback for user %s:\n", userID)
		fmt.Printf("=====================\n")
		for i, feedback := range feedbacks {
			fmt.Printf("%d. ID: %s\n", i+1, feedback.ID)
			fmt.Printf("   Job ID: %s\n", feedback.JobID)
			fmt.Printf("   Item ID: %s\n", feedback.ItemID)
			fmt.Printf("   Type: %s\n", feedback.FeedbackType)
			fmt.Printf("   Liked: %t\n", feedback.Liked)
			fmt.Printf("   Created: %s\n", feedback.CreatedAt.Format(time.RFC3339))
			if feedback.ProcessedAt != nil {
				fmt.Printf("   Processed: %s\n", feedback.ProcessedAt.Format(time.RFC3339))
			} else {
				fmt.Printf("   Processed: Not processed\n")
			}
			fmt.Printf("   Item Text: %s\n", feedback.ItemText)
			fmt.Println()
		}
		fmt.Printf("Total: %d feedback items\n", len(feedbacks))
	}

	return nil
}

func showFeedback(cmd *cobra.Command, args []string) error {
	feedbackID := args[0]
	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	// Get specific feedback
	feedback, found, err := internal.GetUserFeedback(ctx, fs, feedbackID)
	if err != nil {
		return fmt.Errorf("failed to get feedback: %w", err)
	}

	if !found {
		fmt.Printf("Feedback with ID %s not found\n", feedbackID)
		return nil
	}

	// Output results
	if jsonOutput {
		jsonBytes, err := json.MarshalIndent(feedback, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal feedback to JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Feedback Details:\n")
		fmt.Printf("================\n")
		fmt.Printf("ID: %s\n", feedback.ID)
		fmt.Printf("User ID: %s\n", feedback.UserID)
		fmt.Printf("Job ID: %s\n", feedback.JobID)
		fmt.Printf("Item ID: %s\n", feedback.ItemID)
		fmt.Printf("Feedback Type: %s\n", feedback.FeedbackType)
		fmt.Printf("Liked: %t\n", feedback.Liked)
		fmt.Printf("Created At: %s\n", feedback.CreatedAt.Format(time.RFC3339))
		if feedback.ProcessedAt != nil {
			fmt.Printf("Processed At: %s\n", feedback.ProcessedAt.Format(time.RFC3339))
		} else {
			fmt.Printf("Processed At: Not processed\n")
		}
		fmt.Printf("Item Text: %s\n", feedback.ItemText)
	}

	return nil
}

func listJobFeedback(cmd *cobra.Command, args []string) error {
	jobID := args[0]
	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	// Get all unprocessed feedback first to search through it
	feedbacks, err := internal.GetAllUnprocessedFeedback(ctx, fs, 0)
	if err != nil {
		return fmt.Errorf("failed to get all feedback: %w", err)
	}

	// Filter by job ID
	var jobFeedbacks []*internal.UserFeedback
	for _, feedback := range feedbacks {
		if feedback.JobID == jobID {
			jobFeedbacks = append(jobFeedbacks, feedback)
		}
	}

	if len(jobFeedbacks) == 0 {
		fmt.Printf("No feedback found for job %s\n", jobID)
		return nil
	}

	// Output results
	if jsonOutput {
		jsonBytes, err := json.MarshalIndent(jobFeedbacks, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal feedback to JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Feedback for job %s:\n", jobID)
		fmt.Printf("====================\n")
		for i, feedback := range jobFeedbacks {
			fmt.Printf("%d. User: %s\n", i+1, feedback.UserID)
			fmt.Printf("   Item ID: %s\n", feedback.ItemID)
			fmt.Printf("   Type: %s\n", feedback.FeedbackType)
			fmt.Printf("   Liked: %t\n", feedback.Liked)
			fmt.Printf("   Created: %s\n", feedback.CreatedAt.Format(time.RFC3339))
			if feedback.ProcessedAt != nil {
				fmt.Printf("   Processed: %s\n", feedback.ProcessedAt.Format(time.RFC3339))
			} else {
				fmt.Printf("   Processed: Not processed\n")
			}
			fmt.Printf("   Item Text: %s\n", feedback.ItemText)
			fmt.Println()
		}
		fmt.Printf("Total: %d feedback items\n", len(jobFeedbacks))
	}

	return nil
}

func showUserStats(cmd *cobra.Command, args []string) error {
	userID := args[0]
	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	// Get user feedback statistics
	stats, err := internal.GetUserFeedbackStats(ctx, fs, userID)
	if err != nil {
		return fmt.Errorf("failed to get user feedback stats: %w", err)
	}

	// Output results
	if jsonOutput {
		jsonBytes, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal stats to JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("User Feedback Statistics for %s:\n", userID)
		fmt.Printf("=================================\n")
		fmt.Printf("Total Feedback: %d\n", stats.TotalFeedback)
		fmt.Printf("Liked Items: %d\n", stats.LikedItems)
		fmt.Printf("Disliked Items: %d\n", stats.DislikedItems)
		fmt.Printf("Unprocessed Count: %d\n", stats.UnprocessedCount)
		if stats.LastFeedbackAt != nil {
			fmt.Printf("Last Feedback: %s\n", stats.LastFeedbackAt.Format(time.RFC3339))
			fmt.Printf("Most Recent Job ID: %s\n", stats.MostRecentJobID)
		} else {
			fmt.Printf("Last Feedback: None\n")
		}

		if stats.TotalFeedback > 0 {
			likePercentage := float64(stats.LikedItems) / float64(stats.TotalFeedback) * 100
			fmt.Printf("Like Percentage: %.1f%%\n", likePercentage)
		}
	}

	return nil
}

func searchFeedbackByDate(cmd *cobra.Command, args []string) error {
	userID := args[0]
	startDateStr := args[1]
	endDateStr := args[2]
	ctx := context.Background()

	// Parse dates
	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		return fmt.Errorf("invalid start date format (expected YYYY-MM-DD): %w", err)
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		return fmt.Errorf("invalid end date format (expected YYYY-MM-DD): %w", err)
	}

	// Set end date to end of day
	endDate = endDate.Add(24*time.Hour - time.Nanosecond)

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	// Get feedback by date range
	feedbacks, err := internal.GetUserFeedbackByDateRange(ctx, fs, userID, startDate, endDate)
	if err != nil {
		return fmt.Errorf("failed to get feedback by date range: %w", err)
	}

	if len(feedbacks) == 0 {
		fmt.Printf("No feedback found for user %s between %s and %s\n", userID, startDateStr, endDateStr)
		return nil
	}

	// Output results
	if jsonOutput {
		jsonBytes, err := json.MarshalIndent(feedbacks, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal feedback to JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Feedback for user %s from %s to %s:\n", userID, startDateStr, endDateStr)
		fmt.Printf("==========================================\n")
		for i, feedback := range feedbacks {
			fmt.Printf("%d. ID: %s\n", i+1, feedback.ID)
			fmt.Printf("   Job ID: %s\n", feedback.JobID)
			fmt.Printf("   Item ID: %s\n", feedback.ItemID)
			fmt.Printf("   Type: %s\n", feedback.FeedbackType)
			fmt.Printf("   Liked: %t\n", feedback.Liked)
			fmt.Printf("   Created: %s\n", feedback.CreatedAt.Format(time.RFC3339))
			if feedback.ProcessedAt != nil {
				fmt.Printf("   Processed: %s\n", feedback.ProcessedAt.Format(time.RFC3339))
			} else {
				fmt.Printf("   Processed: Not processed\n")
			}
			fmt.Printf("   Item Text: %s\n", feedback.ItemText)
			fmt.Println()
		}
		fmt.Printf("Total: %d feedback items\n", len(feedbacks))
	}

	return nil
}

func exportFeedback(cmd *cobra.Command, args []string) error {
	userID := args[0]
	format, _ := cmd.Flags().GetString("format")
	ctx := context.Background()

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	// Get all feedback for the user
	feedbacks, err := internal.ListUserFeedback(ctx, fs, userID)
	if err != nil {
		return fmt.Errorf("failed to list user feedback: %w", err)
	}

	if len(feedbacks) == 0 {
		fmt.Printf("No feedback found for user %s\n", userID)
		return nil
	}

	// Export in requested format
	switch strings.ToLower(format) {
	case "json":
		jsonBytes, err := json.MarshalIndent(feedbacks, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal feedback to JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))

	case "csv":
		writer := csv.NewWriter(os.Stdout)
		defer writer.Flush()

		// Write header
		header := []string{"ID", "UserID", "JobID", "ItemID", "ItemText", "FeedbackType", "Liked", "CreatedAt", "ProcessedAt"}
		if err := writer.Write(header); err != nil {
			return fmt.Errorf("failed to write CSV header: %w", err)
		}

		// Write data rows
		for _, feedback := range feedbacks {
			processedAt := ""
			if feedback.ProcessedAt != nil {
				processedAt = feedback.ProcessedAt.Format(time.RFC3339)
			}

			row := []string{
				feedback.ID,
				feedback.UserID,
				feedback.JobID,
				feedback.ItemID,
				feedback.ItemText,
				string(feedback.FeedbackType),
				strconv.FormatBool(feedback.Liked),
				feedback.CreatedAt.Format(time.RFC3339),
				processedAt,
			}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("failed to write CSV row: %w", err)
			}
		}

	default:
		return fmt.Errorf("unsupported format: %s (supported: json, csv)", format)
	}

	return nil
}

func inboxFeedback(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Get flags
	processEmails, _ := cmd.Flags().GetBool("process")
	testMode, _ := cmd.Flags().GetBool("test")
	userEmail, _ := cmd.Flags().GetString("user-email")

	// Validate flag combinations
	if processEmails && testMode {
		return fmt.Errorf("cannot use both --process and --test flags together")
	}

	// Handle test mode (keep existing functionality)
	if testMode {
		return runTestMode(ctx, logger, userEmail)
	}

	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	// Check Gmail configuration
	credentialsJSON := os.Getenv("GMAIL_READER_CREDENTIALS")
	if credentialsJSON == "" {
		return fmt.Errorf(`Gmail is not configured for inbox processing.

To configure Gmail access:
1. Set the GMAIL_READER_CREDENTIALS environment variable with your service account JSON
2. Run 'go run cmd/cli/cli_main.go gmail setup' to configure OAuth token

Without Gmail configuration, you can only use test mode:
  trcli feedback inbox --test`)
	}

	// Verify Gmail token exists
	tokenPath := "gmail_token.json"
	if _, err := os.Stat(tokenPath); err != nil {
		return fmt.Errorf(`Gmail token not found at %s.

Run the following command to set up Gmail access:
  go run cmd/cli/cli_main.go gmail setup

After setup, retry this command.`, tokenPath)
	}

	// Create Gmail reader
	gmail, err := createGmailReader(logger, tokenPath)
	if err != nil {
		return fmt.Errorf("failed to create Gmail reader: %w", err)
	}
	defer gmail.Close()

	// Get unread emails for analysis
	emails, err := gmail.GetUnreadEmails(ctx, 50) // Max 50 for safety
	if err != nil {
		return fmt.Errorf("failed to get unread emails: %w", err)
	}

	if len(emails) == 0 {
		fmt.Printf("📬 No unread emails found in inbox\n")
		return nil
	}

	// Show email summary
	fmt.Printf("Inbox Feedback Analysis\n")
	fmt.Printf("=======================\n")
	fmt.Printf("Found %d unread email(s) that could be processed by the feedback system:\n\n", len(emails))

	for i, email := range emails {
		fmt.Printf("%d. From: %s\n", i+1, email.From)
		fmt.Printf("   Subject: %s\n", email.Subject)
		fmt.Printf("   Date: %s\n", email.Date.Format("2006-01-02 15:04:05"))
		fmt.Printf("   Preview: %s\n", truncateString(email.Body, 100))
		fmt.Printf("\n")
	}

	// Safety check for 5+ emails
	if len(emails) >= 5 && processEmails {
		fmt.Printf("⚠️  WARNING: %d emails will be processed.\n", len(emails))
		fmt.Printf("This will:\n")
		fmt.Printf("- Analyze each email using OpenAI\n")
		fmt.Printf("- Update user statements in the database\n")
		fmt.Printf("- Mark emails as read in Gmail\n")
		fmt.Printf("- Send notification emails to admins\n\n")

		if !confirmProcessing() {
			fmt.Printf("❌ Processing cancelled by user\n")
			return nil
		}
	}

	// Handle read-only mode (default)
	if !processEmails {
		fmt.Printf("📋 READ-ONLY MODE (default)\n")
		fmt.Printf("To actually process these emails, use: trcli feedback inbox --process\n")
		fmt.Printf("To test with mock data, use: trcli feedback inbox --test\n")
		return nil
	}

	// Process emails for real
	fmt.Printf("🚀 PROCESSING EMAILS...\n")
	fmt.Printf("========================\n")

	return processRealEmails(ctx, fs, gmail, emails, logger)
}

// Helper functions for the new inbox command

func runTestMode(ctx context.Context, logger *slog.Logger, userEmail string) error {
	// Initialize Firestore client for test mode
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Firestore client: %w", err)
	}
	defer fs.Close()

	// Use default email if not provided
	if userEmail == "" {
		userEmail = "test@example.com"
	}

	fmt.Printf("Testing inbox feedback processing system\n")
	fmt.Printf("==========================================\n")
	fmt.Printf("User email: %s\n", userEmail)
	fmt.Printf("Dry-run mode: Changes will NOT be saved to database\n")
	fmt.Printf("Time: %s\n\n", time.Now().Format(time.RFC3339))

	// Create mock email with search adjustment content
	mockEmail := &inbox.Email{
		ID:      fmt.Sprintf("test-email-%d", time.Now().Unix()),
		From:    fmt.Sprintf("Test User <%s>", userEmail),
		Subject: "Update my job search preferences",
		Body:    "I would like to add remote work options to my search and focus on senior-level positions instead of entry-level roles. Also, I prefer companies with good work-life balance.",
		Date:    time.Now(),
	}

	fmt.Printf("MOCK EMAIL CREATED:\n")
	fmt.Printf("ID: %s\n", mockEmail.ID)
	fmt.Printf("From: %s\n", mockEmail.From)
	fmt.Printf("Subject: %s\n", mockEmail.Subject)
	fmt.Printf("Body: %s\n\n", mockEmail.Body)

	// Create dry-run inbox feedback handler
	// This will use real OpenAI but won't save changes
	openaiClient := feedback.NewOpenAIClient(os.Getenv("OPENAI_API_KEY"), logger)

	// Create dry-run user service that simulates database operations
	dryRunUserService := &dryRunUserService{
		realUserService: &firestoreUserService{client: fs},
		logger:          logger,
	}

	// Create real mailer for actual email sending (if enabled)
	var mailer smtp.Mailer
	if os.Getenv("SEND_FEEDBACK_ADJUSTMENTS") == "true" {
		// Create mailer using environment variables
		smtpHost := os.Getenv("SMTP_HOST")
		smtpPort := 587 // default SMTP port
		if portStr := os.Getenv("SMTP_PORT"); portStr != "" {
			var err error
			smtpPort, err = strconv.Atoi(portStr)
			if err != nil {
				return fmt.Errorf("invalid SMTP_PORT environment variable: %w", err)
			}
		}
		smtpUsername := os.Getenv("SMTP_USERNAME")
		smtpPassword := os.Getenv("SMTP_PASSWORD")
		smtpFrom := os.Getenv("SMTP_FROM")

		if smtpHost == "" {
			fmt.Printf("WARNING: SMTP_HOST not set, using dry-run mailer\n")
			mailer = &dryRunMailer{logger: logger}
		} else {
			var err error
			mailer, err = smtp.NewMailSender(smtpHost, smtpPort, smtpUsername, smtpPassword, smtpFrom)
			if err != nil {
				return fmt.Errorf("failed to create SMTP mailer: %w", err)
			}
		}
		fmt.Printf("EMAIL NOTIFICATIONS: Enabled - actual notification email will be sent\n\n")
	} else {
		mailer = &dryRunMailer{logger: logger}
		fmt.Printf("EMAIL NOTIFICATIONS: Disabled - no notification email will be sent\n\n")
	}

	// Create handler with dry-run components
	handler := feedback.NewInboxFeedbackHandlerWithServices(
		openaiClient,
		dryRunUserService,
		mailer,
		"admin@example.com", // Replace with actual admin email
		logger,
	)

	// Process the mock email
	fmt.Printf("PROCESSING EMAIL...\n")
	fmt.Printf("====================\n")

	result, err := handler.HandleEmail(ctx, mockEmail)
	if err != nil {
		fmt.Printf("❌ ERROR: %v\n", err)
		return err
	}

	if result != nil && result.Handled {
		fmt.Printf("✅ SUCCESS: Email processed successfully - %s\n", result.Summary)
	} else {
		fmt.Printf("ℹ️ INFO: Email was not handled by the feedback processor\n")
	}
	fmt.Printf("\nDRY-RUN COMPLETE\n")
	fmt.Printf("================\n")
	fmt.Printf("- No changes were saved to the database\n")
	fmt.Printf("- All processing steps were executed and validated\n")
	fmt.Printf("- Check logs above for detailed processing information\n")

	if os.Getenv("SEND_FEEDBACK_ADJUSTMENTS") == "true" {
		fmt.Printf("- Notification email was sent to admin (if configured)\n")
	}

	return nil
}

func createGmailReader(logger *slog.Logger, tokenPath string) (*inbox.GmailReader, error) {
	credentialsJSON := os.Getenv("GMAIL_READER_CREDENTIALS")
	return inbox.NewGmailReader(inbox.InboxConfig{
		CredentialsJSON: []byte(credentialsJSON),
		TokenPath:       tokenPath,
	}, logger)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func confirmProcessing() bool {
	fmt.Printf("Do you want to continue? [y/N]: ")
	var response string
	fmt.Scanln(&response)
	return strings.ToLower(response) == "y" || strings.ToLower(response) == "yes"
}

func processRealEmails(ctx context.Context, fs *internal.FirestoreClient, gmail *inbox.GmailReader, emails []*inbox.Email, logger *slog.Logger) error {
	// Create feedback handler (same as production)
	openaiClient := feedback.NewOpenAIClient(os.Getenv("OPENAI_API_KEY"), logger)

	// Create real user service
	userService := &firestoreUserService{client: fs}

	// Create real mailer
	var mailer smtp.Mailer
	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost != "" {
		smtpPort := 587
		if portStr := os.Getenv("SMTP_PORT"); portStr != "" {
			if p, err := strconv.Atoi(portStr); err == nil {
				smtpPort = p
			}
		}
		smtpUsername := os.Getenv("SMTP_USERNAME")
		smtpPassword := os.Getenv("SMTP_PASSWORD")
		smtpFrom := os.Getenv("SMTP_FROM")

		var err error
		mailer, err = smtp.NewMailSender(smtpHost, smtpPort, smtpUsername, smtpPassword, smtpFrom)
		if err != nil {
			logger.Warn("Failed to create SMTP mailer, using no-op mailer", "error", err)
			mailer = &dryRunMailer{logger: logger}
		}
	} else {
		mailer = &dryRunMailer{logger: logger}
	}

	// Create real handler
	handler := feedback.NewInboxFeedbackHandlerWithServices(
		openaiClient,
		userService,
		mailer,
		os.Getenv("ADMIN_EMAIL"), // Use admin email from env
		logger,
	)

	// Process each email
	successCount := 0
	errorCount := 0

	for i, email := range emails {
		fmt.Printf("Processing email %d/%d: %s\n", i+1, len(emails), email.Subject)

		result, err := handler.HandleEmail(ctx, email)
		if err != nil {
			logger.Error("Failed to process email", "email_id", email.ID, "subject", email.Subject, "error", err)
			fmt.Printf("❌ Error processing email %d: %v\n", i+1, err)
			errorCount++
			continue
		}

		// Mark email as read in Gmail
		err = gmail.MarkAsRead(ctx, email.ID)
		if err != nil {
			logger.Error("Failed to mark email as read", "email_id", email.ID, "error", err)
			fmt.Printf("⚠️  Processed email %d but failed to mark as read: %v\n", i+1, err)
		}

		if result != nil && result.Handled {
			fmt.Printf("✅ Email %d processed successfully - %s\n", i+1, result.Summary)
		} else {
			fmt.Printf("ℹ️ Email %d was not handled by the feedback processor\n", i+1)
		}
		successCount++
	}

	fmt.Printf("\n📊 PROCESSING COMPLETE\n")
	fmt.Printf("======================\n")
	fmt.Printf("Total emails: %d\n", len(emails))
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Errors: %d\n", errorCount)

	if errorCount > 0 {
		fmt.Printf("\n⚠️  Some emails failed to process. Check logs above for details.\n")
	}

	return nil
}

// dryRunUserService simulates database operations without actually saving changes
type dryRunUserService struct {
	realUserService feedback.UserService
	logger          *slog.Logger
}

func (d *dryRunUserService) GetUserByEmail(ctx context.Context, email string) (*internal.User, bool, error) {
	// Use real database lookup
	user, found, err := d.realUserService.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, false, err
	}

	if !found {
		// Create a mock user for testing if not found
		d.logger.Info("User not found in database, creating mock user for testing", "email", email)
		mockUser := &internal.User{
			ID:        fmt.Sprintf("mock-user-%d", time.Now().Unix()),
			Email:     email,
			Name:      "Test User",
			FirstName: "Test",
			LastName:  "User",
			Statement: stringPtr("I am looking for software engineering roles in the Bay Area."),
		}
		return mockUser, true, nil
	}

	d.logger.Info("Found existing user", "userID", user.ID, "email", user.Email)
	return user, true, nil
}

func (d *dryRunUserService) SetUserStatement(ctx context.Context, userID, statement string) error {
	// Simulate saving without actually updating database
	d.logger.Info("DRY-RUN: Would update user statement",
		"userID", userID,
		"newStatement", statement)

	fmt.Printf("DRY-RUN UPDATE:\n")
	fmt.Printf("User ID: %s\n", userID)
	fmt.Printf("New Statement: %s\n\n", statement)

	return nil
}

// dryRunMailer simulates email sending without actually sending
type dryRunMailer struct {
	logger *slog.Logger
}

func (d *dryRunMailer) Send(recipient, subject, plainBody, htmlBody string) error {
	d.logger.Info("DRY-RUN: Would send email",
		"recipient", recipient,
		"subject", subject)

	fmt.Printf("DRY-RUN EMAIL:\n")
	fmt.Printf("To: %s\n", recipient)
	fmt.Printf("Subject: %s\n", subject)
	fmt.Printf("Body: %s\n\n", plainBody)

	return nil
}

func (d *dryRunMailer) SendFromTemplate(recipient string, data any, patterns ...string) error {
	d.logger.Info("DRY-RUN: Would send template email",
		"recipient", recipient,
		"template", patterns)

	fmt.Printf("DRY-RUN TEMPLATE EMAIL:\n")
	fmt.Printf("To: %s\n", recipient)
	fmt.Printf("Template: %v\n", patterns)
	fmt.Printf("Data: %+v\n\n", data)

	return nil
}

// firestoreUserService implements UserService for real database operations
type firestoreUserService struct {
	client *internal.FirestoreClient
}

func (f *firestoreUserService) GetUserByEmail(ctx context.Context, email string) (*internal.User, bool, error) {
	return internal.GetUserByEmail(ctx, f.client, email)
}

func (f *firestoreUserService) SetUserStatement(ctx context.Context, userID, statement string) error {
	return internal.SetUserStatement(ctx, f.client, userID, statement)
}

// stringPtr is a helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
