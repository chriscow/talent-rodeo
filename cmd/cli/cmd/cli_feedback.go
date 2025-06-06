package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"talent-rodeo/internal"
	"talent-rodeo/internal/feedback"
	"time"

	"github.com/spf13/cobra"
)

var feedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Manage user feedback operations",
	Long:  `Commands for processing and managing user feedback on job listings`,
}

var processFeedbackCmd = &cobra.Command{
	Use:   "process",
	Short: "Process unprocessed user feedback",
	Long: `Process all unprocessed user feedback and update SteeringQuery preferences.
This command is designed to be run as a daily batch job.`,
	RunE: processFeedback,
}

var processFeedbackUserCmd = &cobra.Command{
	Use:   "process-user [userID]",
	Short: "Process feedback for a specific user",
	Long:  `Process unprocessed feedback for a specific user and update their SteeringQuery preferences.`,
	Args:  cobra.ExactArgs(1),
	RunE:  processFeedbackForUser,
}

var showFeedbackStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show feedback statistics",
	Long:  `Display statistics about unprocessed user feedback.`,
	RunE:  showFeedbackStats,
}

func init() {
	feedbackCmd.AddCommand(processFeedbackCmd)
	feedbackCmd.AddCommand(processFeedbackUserCmd)
	feedbackCmd.AddCommand(showFeedbackStatsCmd)
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
