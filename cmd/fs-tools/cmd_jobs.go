package main

import (
	"context"
	"encoding/json"
	"fmt"
	"talent-rodeo/internal"

	"github.com/spf13/cobra"
)

func init() {
	// Add jobs command
	rootCmd.AddCommand(jobsCmd)

	// Add subcommands
	jobsCmd.AddCommand(listJobsCmd)
	jobsCmd.AddCommand(updateJobCmd)
	jobsCmd.AddCommand(deleteJobCmd)
	jobsCmd.AddCommand(listJobListingsCmd)
	jobsCmd.AddCommand(listJobDetailsCmd)
	jobsCmd.AddCommand(deleteJobListingsCmd)
	jobsCmd.AddCommand(deleteJobDetailsCmd)
	jobsCmd.AddCommand(deleteAllUserJobsCmd)

	// Add flags
	jobsCmd.PersistentFlags().StringP("user", "u", "", "User ID (required for user-specific operations)")
	listJobsCmd.Flags().BoolVar(&prettyOutput, "pretty", false, "Pretty print JSON output")

	updateJobCmd.Flags().StringP("id", "i", "", "Job ID to update")
	updateJobCmd.Flags().StringP("status", "s", "", "Update job status (viewed, saved, applied, rejected)")
	updateJobCmd.Flags().StringP("notes", "n", "", "Update job notes")

	deleteJobCmd.Flags().StringP("id", "i", "", "Job ID to delete")
	deleteJobCmd.Flags().StringP("prefix", "p", "", "Delete all jobs with this prefix")

	// Add flags for job listings and details commands
	listJobListingsCmd.Flags().IntP("limit", "l", 100, "Maximum number of listings to return per page")
	listJobListingsCmd.Flags().IntP("offset", "o", 0, "Number of listings to skip")
	listJobListingsCmd.Flags().BoolVar(&prettyOutput, "pretty", false, "Pretty print JSON output")

	listJobDetailsCmd.Flags().IntP("limit", "l", 100, "Maximum number of details to return per page")
	listJobDetailsCmd.Flags().IntP("offset", "o", 0, "Number of details to skip")
	listJobDetailsCmd.Flags().BoolVar(&prettyOutput, "pretty", false, "Pretty print JSON output")

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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Only check for user ID if the command requires it
		if cmd.Name() == "list" || cmd.Name() == "update" || cmd.Name() == "delete" {
			userID, _ := cmd.Flags().GetString("user")
			if userID == "" {
				return fmt.Errorf("user ID is required (use --user or -u)")
			}
		}
		return nil
	},
}

var listJobsCmd = &cobra.Command{
	Use:   "list",
	Short: "List all jobs for a user",
	Long:  `List all jobs for a user with their status and metadata`,
	RunE: func(cmd *cobra.Command, args []string) error {
		userID, _ := cmd.Flags().GetString("user")
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

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
				ID:         job.ID,
				Status:     job.Status,
				Notes:      job.Notes,
				SearchHash: job.SearchHash,
				JobRef:     job.JobRef.Path,
			}
		}

		var output []byte
		if prettyOutput {
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
			if err := internal.UpdateUserJobStatus(ctx, fs, userID, jobID, status); err != nil {
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

			fmt.Printf("Successfully deleted %d jobs with prefix %s\n", deleted, prefix)
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
	Short: "List all job listings",
	Long:  `List all job listings from the job_listings collection`,
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

		if len(listings) == 0 {
			fmt.Println("No job listings found")
			return nil
		}

		var output []byte
		if prettyOutput {
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
	Short: "List all job details",
	Long:  `List all job details from the job_details collection`,
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

		if len(details) == 0 {
			fmt.Println("No job details found")
			return nil
		}

		var output []byte
		if prettyOutput {
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
	Short: "Delete job listings by prefix",
	Long:  `Delete all job listings that have IDs starting with the specified prefix`,
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

		fmt.Printf("Successfully deleted %d job listings with prefix %s\n", deleted, prefix)
		return nil
	},
}

var deleteJobDetailsCmd = &cobra.Command{
	Use:   "delete-details",
	Short: "Delete job details by prefix",
	Long:  `Delete all job details that have IDs starting with the specified prefix`,
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

		fmt.Printf("Successfully deleted %d job details with prefix %s\n", deleted, prefix)
		return nil
	},
}

var deleteAllUserJobsCmd = &cobra.Command{
	Use:   "delete-user-jobs",
	Short: "Delete user jobs by prefix across all users",
	Long:  `Delete all user jobs that have IDs starting with the specified prefix, across all users`,
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

		deleted, err := internal.DeleteAllUserJobsByPrefix(ctx, fs, prefix)
		if err != nil {
			return fmt.Errorf("failed to delete user jobs: %w", err)
		}

		fmt.Printf("Successfully deleted %d user jobs with prefix %s across all users\n", deleted, prefix)
		return nil
	},
}
