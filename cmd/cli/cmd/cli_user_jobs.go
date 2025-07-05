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
	Short: "Manage user jobs (jobs assigned to users)",
	Long: `Manage user jobs, which are job records assigned to specific users.
	
User jobs contain job details plus user-specific metadata like ratings, status,
and notes. This command group allows you to:
- List jobs for a user
- Update job ratings and metadata
- Delete user jobs
- View follow-up questions for jobs`,
}

var userJobsUpdateCmd = &cobra.Command{
	Use:   "update [user-id] [--all]",
	Short: "Update user job ratings from AI match analysis",
	Long: `Update the top-level Rating field of user jobs from JobMatchAnalysis results.

This command syncs the Rating field (1-5 stars) from the JobMatchAnalysis.Rating.NumericRating
field for better performance and easier querying. Run this after changes to the
job matching algorithm or when ratings seem out of sync.

Options:
  --all: Update ratings for all users in the system (bulk operation)
  user-id: Update ratings for a specific user only

Examples:
  trcli userjobs update user123                       # Update specific user
  trcli userjobs update --all                         # Update all users (bulk)`,
	Args: cobra.RangeArgs(0, 1),
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
	Use:   "followup <user-id> [--dump]",
	Short: "Show or export follow-up questions from job matches",
	Long: `Display follow-up questions generated from JobMatchAnalysis for user jobs.

Follow-up questions are AI-generated conversation starters based on job matches
that recruiters can use during appointments. They help personalize conversations
and explore specific aspects of job opportunities.

Examples:
  trcli userjobs followup user123                     # Show summary of questions
  trcli userjobs followup user123 --dump              # Export all questions, one per line`,
	Args: cobra.ExactArgs(1),
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
	Short: "List all jobs assigned to a user with status and ratings",
	Long: `List all jobs in a user's collection with their status and metadata.

Shows job IDs, statuses (new, saved, applied, archived), titles, and ratings.
This gives an overview of all jobs the user has received and their current
status in the job application process.

Examples:
  trcli userjobs list user123                         # Show jobs with basic info
  trcli userjobs list user123 --json                  # Get full job details in JSON`,
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
	Use:   "update <user-id> --id <job-id> [--status <status>] [--notes <notes>]",
	Short: "Update status or notes for a specific user job",
	Long: `Update the status or notes for a specific job in a user's collection.

This allows manual updates to job status (tracking application progress) or
adding notes for follow-up. Useful for managing the job application pipeline
and tracking user interactions with specific jobs.

Required flags:
  --id, -i: Job ID to update

Optional flags:
  --status, -s: New status (new, saved, applied, archived)
  --notes, -n: Notes to add or update

Examples:
  # Update job status to applied
  trcli userjobs update user123 --id job456 --status applied
  
  # Add notes to a job
  trcli userjobs update user123 --id job456 --notes "Great company culture"
  
  # Update both status and notes
  trcli userjobs update user123 --id job456 --status applied --notes "Applied via company website"`,
	Args: cobra.ExactArgs(1),
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
	Use:   "delete <user-id> [--id <job-id> | --prefix <prefix> | --all]",
	Short: "Delete jobs from a user's collection",
	Long: `Delete one or more jobs from a user's job collection.

WARNING: This permanently removes job records from the user's collection.
The jobs may be re-added if they match future searches.

Deletion options (choose one):
  --id, -i: Delete specific job by ID
  --prefix, -p: Delete all jobs whose IDs start with prefix (BULK)
  --all: Delete ALL jobs for the user (DANGEROUS)

Examples:
  # Delete specific job
  trcli userjobs delete user123 --id job456
  
  # Delete test jobs
  trcli userjobs delete user123 --prefix "test-"
  
  # Delete all jobs for user (careful!)
  trcli userjobs delete user123 --all`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := args[0]
		jobID, _ := cmd.Flags().GetString("id")
		prefix, _ := cmd.Flags().GetString("prefix")
		all, _ := cmd.Flags().GetBool("all")

		if jobID == "" && prefix == "" && !all {
			return fmt.Errorf("must specify either --id, --prefix, or --all")
		}

		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		if all {
			// Delete all jobs for the user
			jobs, err := internal.ListUserJobs(ctx, fs, userID)
			if err != nil {
				return fmt.Errorf("failed to get user jobs: %w", err)
			}

			deleted := 0
			for _, job := range jobs {
				if err := internal.DeleteUserJob(ctx, fs, userID, job.ID); err != nil {
					return fmt.Errorf("failed to delete job %s: %w", job.ID, err)
				}
				deleted++
			}
			fmt.Printf("Deleted %d jobs for user %s\n", deleted, userID)
			return nil
		}

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
