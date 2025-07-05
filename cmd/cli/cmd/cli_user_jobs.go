package cmd

import (
	"context"
	"encoding/json"
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
	Use:   "update [user-id] [--all]",
	Short: "Update user job ratings from JobMatchAnalysis",
	Long:  `Update the top-level Rating field of user jobs from JobMatchAnalysis.Rating.NumericRating for all users or a specific user`,
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		allUsers, _ := cmd.Flags().GetBool("all")

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
		} else if len(args) == 1 {
			userID := args[0]
			user, found, err := internal.GetUser(ctx, fs, userID)
			if err != nil {
				return fmt.Errorf("failed to get user %s: %w", userID, err)
			}
			if !found {
				return fmt.Errorf("user %s not found", userID)
			}
			users = append(users, user)
		} else {
			return fmt.Errorf("either --all or user-id must be specified")
		}

		updated := 0
		unchanged := 0
		skipped := 0
		for _, user := range users {
			jobs, err := internal.ListUserJobs(ctx, fs, user.ID)
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

var userJobsFollowupCmd = &cobra.Command{
	Use:   "followup <user-id>",
	Short: "Show or dump follow-up questions for user jobs",
	Long:  `Dump all follow-up questions from JobMatchAnalysis for all jobs of a user, one per line`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		userID := args[0]
		dump, _ := cmd.Flags().GetBool("dump")
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		jobs, err := internal.ListUserJobs(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user jobs: %w", err)
		}
		if dump {
			for _, job := range jobs {
				fq := job.JobMatchAnalysis.FollowUpQuestions
				if fq != "" {
					fmt.Println(fq)
				}
			}
			return nil
		}
		// If not dump, just print a summary (could be extended)
		fmt.Printf("User %s has %d jobs. Use --dump to print follow-up questions.\n", userID, len(jobs))
		return nil
	},
}

var listUserJobsCmd = &cobra.Command{
	Use:   "list <user-id>",
	Short: "List user jobs",
	Long: `List all user jobs for a user

	Example:
		trcli userjobs list --json [user-id]
		trcli userjobs list [user-id]
	`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := args[0]
		jsonOutput, _ := cmd.Flags().GetBool("json")
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		// User ID provided, list userJobs for that user
		userJobs, err := internal.ListUserJobs(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user jobs: %w", err)
		}

		if len(userJobs) == 0 {
			fmt.Printf("User %s has no jobs\n", userID)
			return nil
		}

		type SerializableJob struct {
			ID     string             `json:"id"`
			Status internal.JobStatus `json:"status"`
			Title  string             `json:"title,omitempty"`
			JobRef string             `json:"job_ref"`
		}

		ids := make([]string, len(userJobs))
		serJobs := make([]SerializableJob, len(userJobs))
		for i, job := range userJobs {
			serJobs[i] = SerializableJob{
				ID:     job.ID,
				Status: job.Status,
				JobRef: job.JobRef.Path,
			}

			ids[i] = job.JobRef.Path
		}

		jobs, err := internal.GetJobDetailsByIDs(ctx, fs, ids)
		if err != nil {
			return fmt.Errorf("failed to get job details: %w", err)
		}

		for i, job := range jobs {
			serJobs[i].Title = job.Title
		}

		var output []byte
		if jsonOutput {
			output, err = json.MarshalIndent(serJobs, "", "  ")
		} else {
			for _, job := range serJobs {
				fmt.Printf("%s\t%s\t%s\n", job.ID, job.Status, job.Title)
			}
		}

		if err != nil {
			return fmt.Errorf("failed to marshal jobs: %w", err)
		}

		fmt.Println(string(output))
		return nil
	},
}

var updateUserJobCmd = &cobra.Command{
	Use:   "update <user-id> --id <job-id>",
	Short: "Update a user job",
	Long:  `Update a user job's status or notes`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := args[0]
		jobID, _ := cmd.Flags().GetString("id")
		status, _ := cmd.Flags().GetString("status")
		notes, _ := cmd.Flags().GetString("notes")

		if jobID == "" {
			return fmt.Errorf("job ID is required (use --id or -i)")
		}

		if status == "" && notes == "" {
			return fmt.Errorf("either status or notes must be specified")
		}

		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		if status != "" {
			jobStatus, ok := internal.ParseJobStatus(status)
			if !ok {
				return fmt.Errorf("invalid job status: %s", status)
			}
			if err := internal.UpdateUserJobStatus(ctx, fs, userID, jobID, jobStatus); err != nil {
				return fmt.Errorf("failed to update job status: %w", err)
			}
			fmt.Printf("Successfully updated job %s status to %s\n", jobID, status)
		}

		if notes != "" {
			if err := internal.UpdateUserJobNotes(ctx, fs, userID, jobID, notes); err != nil {
				return fmt.Errorf("failed to update job notes: %w", err)
			}
			fmt.Printf("Successfully updated notes for job %s\n", jobID)
		}

		return nil
	},
}

var deleteUserJobCmd = &cobra.Command{
	Use:   "delete <user-id> [--id <job-id>] [--prefix <prefix>]",
	Short: "Delete user job(s)",
	Long:  `Delete user job(s) by ID or prefix`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := args[0]
		jobID, _ := cmd.Flags().GetString("id")
		prefix, _ := cmd.Flags().GetString("prefix")

		if jobID == "" && prefix == "" {
			return fmt.Errorf("must specify either --id or --prefix")
		}

		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		if prefix != "" {
			// Delete all jobs with prefix
			jobs, err := internal.ListUserJobs(ctx, fs, userID)
			if err != nil {
				return fmt.Errorf("failed to get user jobs: %w", err)
			}

			deleted := 0
			for _, job := range jobs {
				if len(job.ID) >= len(prefix) && job.ID[:len(prefix)] == prefix {
					if err := internal.DeleteUserJob(ctx, fs, userID, job.ID); err != nil {
						return fmt.Errorf("failed to delete job %s: %w", job.ID, err)
					}
					deleted++
				}
			}
			fmt.Printf("Deleted %d jobs with prefix: %s\n", deleted, prefix)
			return nil
		}

		if err := internal.DeleteUserJob(ctx, fs, userID, jobID); err != nil {
			return fmt.Errorf("failed to delete job: %w", err)
		}

		fmt.Printf("Successfully deleted job: %s\n", jobID)
		return nil
	},
}

func init() {
	userJobsFollowupCmd.Flags().Bool("dump", false, "Dump all follow-up questions, one per line")

	listUserJobsCmd.Flags().Bool("json", false, "Output in JSON format")

	userJobsUpdateCmd.Flags().BoolP("all", "a", false, "Update ratings for all users")

	userJobsCmd.AddCommand(userJobsUpdateCmd)
	userJobsCmd.AddCommand(updateUserJobCmd)
	userJobsCmd.AddCommand(deleteUserJobCmd)
	updateUserJobCmd.Flags().StringP("id", "i", "", "Job ID to update")
	updateUserJobCmd.Flags().StringP("status", "s", "", "Update job status (viewed, saved, applied, archived)")
	updateUserJobCmd.Flags().StringP("notes", "n", "", "Update job notes")
	deleteUserJobCmd.Flags().StringP("id", "i", "", "Job ID to delete")
	deleteUserJobCmd.Flags().StringP("prefix", "p", "", "Delete all jobs with this prefix")
	deleteUserJobCmd.Flags().Bool("all", false, "Delete all jobs for a user")

	userJobsCmd.AddCommand(userJobsFollowupCmd)
	userJobsCmd.AddCommand(listUserJobsCmd)
}
