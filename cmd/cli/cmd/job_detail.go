package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"talent-rodeo/internal"
	"time"

	"github.com/cheggaaa/pb"
	"github.com/spf13/cobra"
)

func init() {
	// Add jobs command
	RootCmd.AddCommand(jobsCmd)

	// Add subcommands
	jobsCmd.AddCommand(listJobsCmd)
	jobsCmd.AddCommand(updateJobCmd)
	jobsCmd.AddCommand(deleteJobCmd)
	jobsCmd.AddCommand(listJobListingsCmd)
	jobsCmd.AddCommand(listJobDetailsCmd)
	jobsCmd.AddCommand(deleteJobListingsCmd)
	jobsCmd.AddCommand(deleteJobDetailsCmd)
	jobsCmd.AddCommand(deleteAllUserJobsCmd)
	jobsCmd.AddCommand(upsertJobDetailsCmd)

	// Add flags
	jobsCmd.PersistentFlags().StringP("user", "u", "", "User ID (optional for list operation)")

	updateJobCmd.Flags().StringP("id", "i", "", "Job ID to update")
	updateJobCmd.Flags().StringP("status", "s", "", "Update job status (viewed, saved, applied, archived)")
	updateJobCmd.Flags().StringP("notes", "n", "", "Update job notes")

	deleteJobCmd.Flags().StringP("id", "i", "", "Job ID to delete")
	deleteJobCmd.Flags().StringP("prefix", "p", "", "Delete all jobs with this prefix")

	// Add flags for job listings and details commands
	listJobListingsCmd.Flags().IntP("limit", "l", 100, "Maximum number of listings to return per page")
	listJobListingsCmd.Flags().IntP("offset", "o", 0, "Number of listings to skip")

	listJobDetailsCmd.Flags().IntP("limit", "l", 100, "Maximum number of details to return per page")
	listJobDetailsCmd.Flags().IntP("offset", "o", 0, "Number of details to skip")

	// Add flags for delete commands
	deleteJobListingsCmd.Flags().StringP("prefix", "p", "", "Delete all job listings with this prefix")
	deleteJobDetailsCmd.Flags().StringP("prefix", "p", "", "Delete all job details with this prefix")

	// Add flag for delete all user jobs command
	deleteAllUserJobsCmd.Flags().StringP("prefix", "p", "", "Delete all user jobs with this prefix")
}

var jobsCmd = &cobra.Command{
	Use:   "jobs",
	Short: "Manage jobs",
	Long:  `Jobs command allows you to manage jobs in the system`,
}

var listJobsCmd = &cobra.Command{
	Use:   "list",
	Short: "List jobs",
	Long:  `List all jobs for a user, or all job details if no user is specified`,
	RunE: func(cmd *cobra.Command, args []string) error {
		userID, _ := cmd.Flags().GetString("user")
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		if userID == "" {
			// No user ID provided, list all job details
			details, err := internal.ListJobDetails(ctx, fs, 100, 0)
			if err != nil {
				return fmt.Errorf("failed to list job details: %w", err)
			}

			if len(details) == 0 {
				fmt.Println("No job details found")
				return nil
			}

			// Convert to JSON and display
			var output []byte
			if jsonOutput {
				output, err = json.MarshalIndent(details, "", "  ")
			} else {
				output, err = json.Marshal(details)
			}

			if err != nil {
				return fmt.Errorf("failed to marshal job details: %w", err)
			}

			fmt.Printf("Retrieved %d job details\n", len(details))
			fmt.Println(string(output))

			return nil
		}

		// User ID provided, list jobs for that user
		jobs, err := internal.GetUserJobs(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user jobs: %w", err)
		}

		if len(jobs) == 0 {
			fmt.Printf("User %s has no jobs\n", userID)
			return nil
		}

		type SerializableJob struct {
			ID         string             `json:"id"`
			Status     internal.JobStatus `json:"status"`
			Notes      string             `json:"notes,omitempty"`
			SearchHash string             `json:"search_hash"`
			JobRef     string             `json:"job_ref"`
		}

		serJobs := make([]SerializableJob, len(jobs))
		for i, job := range jobs {
			serJobs[i] = SerializableJob{
				ID:     job.ID,
				Status: job.Status,
				Notes:  job.Notes,
				JobRef: job.JobRef.Path,
			}
		}

		var output []byte
		if jsonOutput {
			output, err = json.MarshalIndent(serJobs, "", "  ")
		} else {
			output, err = json.Marshal(serJobs)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal jobs: %w", err)
		}

		fmt.Println(string(output))
		return nil
	},
}

var updateJobCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a job",
	Long:  `Update a job's status or notes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		userID, _ := cmd.Flags().GetString("user")
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

var deleteJobCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete job(s)",
	Long:  `Delete job(s) by ID or prefix`,
	RunE: func(cmd *cobra.Command, args []string) error {
		userID, _ := cmd.Flags().GetString("user")
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
			jobs, err := internal.GetUserJobs(ctx, fs, userID)
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

var listJobListingsCmd = &cobra.Command{
	Use:   "listings",
	Short: "List job listings",
	Long:  `List job listings from the job-listings collection`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")

		listings, err := internal.ListJobListings(ctx, fs, limit, offset)
		if err != nil {
			return fmt.Errorf("failed to list job listings: %w", err)
		}

		var output []byte
		if jsonOutput {
			output, err = json.MarshalIndent(listings, "", "  ")
		} else {
			output, err = json.Marshal(listings)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal job listings: %w", err)
		}

		fmt.Printf("Retrieved %d job listings (offset: %d)\n", len(listings), offset)
		fmt.Println(string(output))

		// If we got a full page of results, indicate there might be more
		if len(listings) == limit {
			fmt.Printf("\nThere might be more listings. Use --offset %d to see the next page.\n", offset+limit)
		}

		return nil
	},
}

var listJobDetailsCmd = &cobra.Command{
	Use:   "details",
	Short: "List job details",
	Long:  `List job details from the job-details collection`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")

		details, err := internal.ListJobDetails(ctx, fs, limit, offset)
		if err != nil {
			return fmt.Errorf("failed to list job details: %w", err)
		}

		var output []byte
		if jsonOutput {
			output, err = json.MarshalIndent(details, "", "  ")
		} else {
			output, err = json.Marshal(details)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal job details: %w", err)
		}

		fmt.Printf("Retrieved %d job details (offset: %d)\n", len(details), offset)
		fmt.Println(string(output))

		// If we got a full page of results, indicate there might be more
		if len(details) == limit {
			fmt.Printf("\nThere might be more details. Use --offset %d to see the next page.\n", offset+limit)
		}

		return nil
	},
}

var deleteJobListingsCmd = &cobra.Command{
	Use:   "delete-listings",
	Short: "Delete job listing(s)",
	Long:  `Delete job listing(s) by prefix`,
	RunE: func(cmd *cobra.Command, args []string) error {
		prefix, _ := cmd.Flags().GetString("prefix")
		if prefix == "" {
			return fmt.Errorf("prefix is required (use --prefix or -p)")
		}

		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		deleted, err := internal.DeleteJobListingsByPrefix(ctx, fs, prefix)
		if err != nil {
			return fmt.Errorf("failed to delete job listings: %w", err)
		}

		fmt.Printf("Successfully deleted %d job listings with prefix: %s\n", deleted, prefix)
		return nil
	},
}

var deleteJobDetailsCmd = &cobra.Command{
	Use:   "delete-details",
	Short: "Delete job detail(s)",
	Long:  `Delete job detail(s) by prefix`,
	RunE: func(cmd *cobra.Command, args []string) error {
		prefix, _ := cmd.Flags().GetString("prefix")
		if prefix == "" {
			return fmt.Errorf("prefix is required (use --prefix or -p)")
		}

		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		deleted, err := internal.DeleteJobDetailsByPrefix(ctx, fs, prefix)
		if err != nil {
			return fmt.Errorf("failed to delete job details: %w", err)
		}

		fmt.Printf("Successfully deleted %d job details with prefix: %s\n", deleted, prefix)
		return nil
	},
}

var deleteAllUserJobsCmd = &cobra.Command{
	Use:   "delete-user-jobs",
	Short: "Delete jobs across all users",
	Long:  `Delete jobs with a specific prefix across all users`,
	RunE: func(cmd *cobra.Command, args []string) error {
		prefix, _ := cmd.Flags().GetString("prefix")
		if prefix == "" {
			return fmt.Errorf("prefix is required (use --prefix or -p)")
		}

		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		users, err := internal.ListUsers(ctx, fs, 0, 0)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		totalDeleted := 0
		for _, user := range users {
			jobs, err := internal.GetUserJobs(ctx, fs, user.ID)
			if err != nil {
				return fmt.Errorf("failed to get jobs for user %s: %w", user.ID, err)
			}

			userDeleted := 0
			for _, job := range jobs {
				if len(job.ID) >= len(prefix) && job.ID[:len(prefix)] == prefix {
					if err := internal.DeleteUserJob(ctx, fs, user.ID, job.ID); err != nil {
						return fmt.Errorf("failed to delete job %s for user %s: %w", job.ID, user.ID, err)
					}
					userDeleted++
				}
			}

			if userDeleted > 0 {
				fmt.Printf("Deleted %d jobs for user %s\n", userDeleted, user.ID)
				totalDeleted += userDeleted
			}
		}

		fmt.Printf("Successfully deleted %d jobs with prefix: %s across all users\n", totalDeleted, prefix)
		return nil
	},
}

var upsertJobDetailsCmd = &cobra.Command{
	Use:   "upsert",
	Short: "Upsert job details",
	Long:  `Upsert job details to Pinecone`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		criterium, err := internal.ListSearchCriteria(ctx, fs)
		if err != nil {
			return fmt.Errorf("failed to list search criteria: %w", err)
		}

		denseIndex := os.Getenv("PINECONE_DENSE_INDEX_HOST")
		println("Dense index host:", denseIndex)

		bar := pb.StartNew(len(criterium))
		defer bar.Finish()

		for _, crit := range criterium {
			ids, err := internal.GetJobIDsForCriteriaCreatedAfter(ctx, fs, crit.ID, time.Time{})
			if err != nil {
				return fmt.Errorf("failed to get job IDs for criteria %s: %w", crit.ID, err)
			}
			if len(ids) == 0 {
				continue
			}

			jobs, err := internal.GetJobDetailsByIDs(ctx, fs, ids)
			if err != nil {
				return fmt.Errorf("failed to get job details for criteria %s: %w", crit.ID, err)
			}
			if len(jobs) == 0 {
				continue
			}

			internal.UpsertJobDetailEmbeddings(ctx, crit, jobs)
			bar.Add(1)
		}

		return nil
	},
}
