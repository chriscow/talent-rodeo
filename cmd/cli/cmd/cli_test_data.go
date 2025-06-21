package cmd

import (
	"context"
	"fmt"
	"talent-rodeo/internal"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	// Add test-data command
	RootCmd.AddCommand(testDataCmd)

	// Add flags
	testDataCmd.Flags().IntP("users", "u", 5, "Number of test users to create")
	testDataCmd.Flags().IntP("jobs", "j", 10, "Number of test jobs to create per user")
	testDataCmd.Flags().StringP("prefix", "p", "test-", "Prefix for test data IDs")
}

var testDataCmd = &cobra.Command{
	Use:   "test-data",
	Short: "Generate test data",
	Long:  `Generate test data for development and testing`,
	RunE: func(cmd *cobra.Command, args []string) error {
		numUsers, _ := cmd.Flags().GetInt("users")
		numJobs, _ := cmd.Flags().GetInt("jobs")
		prefix, _ := cmd.Flags().GetString("prefix")

		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		// Generate users
		userIDs := make([]string, numUsers)
		for i := range numUsers {
			userID := fmt.Sprintf("%s%s-%d", prefix, time.Now().Format("20060102"), i+1)
			userIDs[i] = userID

			// Create user
			user := &internal.User{
				ID:     userID,
				Name:   fmt.Sprintf("Test User %d", i+1),
				Email:  fmt.Sprintf("test-user-%d@example.com", i+1),
				Phone:  nil,
				Status: internal.UserStatusJobSearchComplete,
			}

			if err := internal.CreateUser(ctx, fs, user); err != nil {
				return fmt.Errorf("failed to create test user %s: %w", userID, err)
			}
			fmt.Printf("Created user: %s\n", userID)
		}

		// Generate jobs for each user
		for _, userID := range userIDs {
			for j := range numJobs {
				jobID := fmt.Sprintf("%s-job-%d", userID, j+1)

				job, err := internal.CreateUserJob(ctx, fs, userID, jobID)
				if err != nil {
					return fmt.Errorf("failed to create test job %s for user %s: %w", jobID, userID, err)
				}

				fmt.Printf("Created job: %s\n", job.ID)
			}
			fmt.Printf("Created %d jobs for user: %s\n", numJobs, userID)
		}

		fmt.Printf("Successfully created %d users with %d jobs each\n", numUsers, numJobs)
		fmt.Printf("Use 'cli fs users delete --prefix %s' to clean up test users\n", prefix)
		return nil
	},
}
