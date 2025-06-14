package cmd

import (
	"context"
	"fmt"
	"talent-rodeo/internal"

	"cloud.google.com/go/firestore"
	"github.com/spf13/cobra"
)

// UserJobs update command
var userJobsCmd = &cobra.Command{
	Use:   "userjobs",
	Short: "Manage user jobs",
	Long:  `UserJobs command allows you to manage user jobs in the system`,
}

var userJobsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update user job ratings from JobMatchAnalysis",
	Long:  `Update the top-level Rating field of user jobs from JobMatchAnalysis.Rating.NumericRating for all users or a specific user`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		allUsers, _ := cmd.Flags().GetBool("all-users")
		userID, _ := cmd.Flags().GetString("user-id")

		var users []*internal.User
		if allUsers {
			users, err = internal.ListUsers(ctx, fs, 0, 0)
			if err != nil {
				return fmt.Errorf("failed to list users: %w", err)
			}
			if len(users) == 0 {
				fmt.Println("No users found.")
				return nil
			}
		} else if userID != "" {
			user, found, err := internal.GetUser(ctx, fs, userID)
			if err != nil {
				return fmt.Errorf("failed to get user %s: %w", userID, err)
			}
			if !found {
				return fmt.Errorf("user %s not found", userID)
			}
			users = append(users, user)
		} else {
			return fmt.Errorf("either --all-users or --user-id must be specified")
		}

		updated := 0
		unchanged := 0
		skipped := 0
		for _, user := range users {
			jobs, err := internal.GetUserJobs(ctx, fs, user.ID)
			if err != nil {
				fmt.Printf("Failed to get jobs for user %s: %v\n", user.ID, err)
				skipped++
				continue
			}
			err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
				for _, job := range jobs {
					analysis := job.JobMatchAnalysis
					if analysis.Rating.NumericRating == 0 && analysis.Rating.RatingExplanation == "" {
						skipped++
						continue
					}
					if job.Rating == analysis.Rating.NumericRating {
						unchanged++
						continue
					}
					err := internal.UpdateUserJobRatingTx(ctx, fs, tx, user.ID, job.ID, analysis.Rating.NumericRating)
					if err != nil {
						fmt.Printf("Failed to update rating for user %s job %s: %v\n", user.ID, job.ID, err)
						skipped++
						continue
					}
					updated++
				}
				return nil
			})
			if err != nil {
				fmt.Printf("Failed to update rating for user %s: %v\n", user.ID, err)
				skipped++
			}
		}

		fmt.Printf("\nDone. Updated %d jobs. %d unchanged. %d skipped.\n", updated, unchanged, skipped)
		return nil
	},
}
