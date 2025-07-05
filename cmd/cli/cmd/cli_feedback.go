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

	// Add flags
	exportFeedbackCmd.Flags().StringP("format", "f", "json", "Export format (json or csv)")
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
