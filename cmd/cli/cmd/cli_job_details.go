package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search/providers/scrapingdog"
	"time"

	"github.com/cheggaaa/pb"
	"github.com/spf13/cobra"
)

// scrapingDogProviderWrapper wraps the ScrapingDog provider to implement JobProvider interface
type scrapingDogProviderWrapper struct {
	provider *scrapingdog.Provider
}

func (w *scrapingDogProviderWrapper) Name() string {
	return w.provider.Name()
}

func (w *scrapingDogProviderWrapper) Validate(ctx context.Context) error {
	return w.provider.Validate(ctx)
}

func (w *scrapingDogProviderWrapper) SearchJobListings(ctx context.Context, criteria internal.SearchCriteria) ([]internal.JobListing, error) {
	// For the CLI command, we don't need SearchJobListings, so we can return an error or empty slice
	return nil, fmt.Errorf("SearchJobListings not implemented for CLI wrapper")
}

func (w *scrapingDogProviderWrapper) GetJobDetails(ctx context.Context, userID *string, jobID string) (*internal.JobDetail, error) {
	return w.provider.GetJobDetails(ctx, userID, jobID)
}

func init() {
	// Add jobs command
	RootCmd.AddCommand(jobDetailsCmd)

	// Add subcommands
	jobDetailsCmd.AddCommand(listJobListingsCmd)
	jobDetailsCmd.AddCommand(listJobDetailsCmd)
	jobDetailsCmd.AddCommand(deleteJobListingsCmd)
	jobDetailsCmd.AddCommand(deleteJobDetailsCmd)
	jobDetailsCmd.AddCommand(deleteAllUserJobsCmd)
	jobDetailsCmd.AddCommand(upsertJobDetailsCmd)
	jobDetailsCmd.AddCommand(processRelatedJobsCmd)

	// Add flags for job listings and details commands
	listJobListingsCmd.Flags().IntP("limit", "l", 100, "Maximum number of listings to return per page")
	listJobListingsCmd.Flags().IntP("offset", "o", 0, "Number of listings to skip")
	listJobListingsCmd.Flags().BoolP("all", "a", false, "List all job listings instead of a specific one")

	listJobDetailsCmd.Flags().IntP("limit", "l", 100, "Maximum number of details to return per page")
	listJobDetailsCmd.Flags().IntP("offset", "o", 0, "Number of details to skip")
	listJobDetailsCmd.Flags().BoolP("all", "a", false, "List all job details instead of a specific one")

	// Add flags for delete commands
	deleteJobListingsCmd.Flags().StringP("prefix", "p", "", "Delete all job listings with this prefix")
	deleteJobDetailsCmd.Flags().StringP("prefix", "p", "", "Delete all job details with this prefix")

	// Add flags for process related jobs command
	processRelatedJobsCmd.Flags().StringP("job-id", "j", "", "Job detail ID to process related jobs for")
	processRelatedJobsCmd.Flags().StringP("criteria-id", "c", "", "Search criteria ID to associate related jobs with")
	processRelatedJobsCmd.Flags().BoolP("dry-run", "d", false, "Show what would be processed without actually doing it")
}

var jobDetailsCmd = &cobra.Command{
	Use:     "job-details",
	Aliases: []string{"job"},
	Short:   "Manage job details",
	Long:    `Job details command allows you to manage job details in the system`,
}

var listJobListingsCmd = &cobra.Command{
	Use:   "listings [job-listing-id] [--all] [--limit N] [--offset N]",
	Short: "Get a specific job listing or list all job listings",
	Long:  `Get a specific job listing by ID, or list all job listings with --all flag`,
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		all, _ := cmd.Flags().GetBool("all")

		// If --all flag is used or no job listing ID is provided, list all job listings
		if all || len(args) == 0 {
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
		}

		// Get specific job listing
		jobListingID := args[0]

		// Get the specific job listing
		doc, err := fs.Collection("job-listings").Doc(jobListingID).Get(ctx)
		if err != nil {
			return fmt.Errorf("failed to get job listing: %w", err)
		}
		if !doc.Exists() {
			return fmt.Errorf("job listing not found: %s", jobListingID)
		}

		var jobListing internal.JobListing
		if err := doc.DataTo(&jobListing); err != nil {
			return fmt.Errorf("failed to parse job listing: %w", err)
		}

		var output []byte
		if jsonOutput {
			output, err = json.MarshalIndent(jobListing, "", "  ")
		} else {
			output, err = json.Marshal(jobListing)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal job listing: %w", err)
		}

		fmt.Printf("Job Listing: %s\n", jobListingID)
		fmt.Println(string(output))

		return nil
	},
}

var listJobDetailsCmd = &cobra.Command{
	Use:   "job [job-id] [--all] [--limit N] [--offset N]",
	Short: "Get a specific job detail or list all job details",
	Long:  `Get a specific job detail by ID, or list all job details with --all flag`,
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		all, _ := cmd.Flags().GetBool("all")

		// If --all flag is used or no job ID is provided, list all job details
		if all || len(args) == 0 {
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
		}

		// Get specific job detail
		jobID := args[0]

		// Direct approach since GetJobDetail seems to have cache issues
		doc, err := fs.Collection("job_details").Doc(jobID).Get(ctx)
		if err != nil {
			return fmt.Errorf("failed to get job detail: %w", err)
		}
		if !doc.Exists() {
			return fmt.Errorf("job detail not found: %s", jobID)
		}

		var jobDetail internal.JobDetail
		if err := doc.DataTo(&jobDetail); err != nil {
			return fmt.Errorf("failed to parse job detail: %w", err)
		}

		var output []byte
		if jsonOutput {
			output, err = json.MarshalIndent(jobDetail, "", "  ")
		} else {
			output, err = json.Marshal(jobDetail)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal job detail: %w", err)
		}

		fmt.Printf("Job Detail: %s\n", jobID)
		fmt.Println(string(output))

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
			jobs, err := internal.ListUserJobs(ctx, fs, user.ID)
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

			// internal.UpsertJobDetailEmbeddings(ctx, crit, jobs)
			bar.Add(1)
		}

		return nil
	},
}

var processRelatedJobsCmd = &cobra.Command{
	Use:   "process-related",
	Short: "Process related jobs for a specific job detail",
	Long: `Process related jobs (SimilarJobs and PeopleAlsoViewed) for a job detail.
This command extracts LinkedIn job IDs from the related jobs, checks if they exist in the database,
adds existing ones to the user's collection, and queues missing ones for fetching.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		userID, _ := cmd.Flags().GetString("user")
		jobID, _ := cmd.Flags().GetString("job-id")
		criteriaID, _ := cmd.Flags().GetString("criteria-id")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if userID == "" {
			return fmt.Errorf("user ID is required (use --user or -u)")
		}
		if jobID == "" {
			return fmt.Errorf("job ID is required (use --job-id or -j)")
		}
		if criteriaID == "" {
			return fmt.Errorf("criteria ID is required (use --criteria-id or -c)")
		}

		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		// Get the job detail (using direct approach due to cache issues)
		doc, err := fs.Collection("job_details").Doc(jobID).Get(ctx)
		if err != nil {
			return fmt.Errorf("failed to get job detail: %w", err)
		}
		if !doc.Exists() {
			return fmt.Errorf("job detail not found: %s", jobID)
		}

		var jobDetail internal.JobDetail
		if err := doc.DataTo(&jobDetail); err != nil {
			return fmt.Errorf("failed to parse job detail: %w", err)
		}

		fmt.Printf("Processing related jobs for job: %s (%s)\n", jobDetail.ID, jobDetail.Title)
		fmt.Printf("Company: %s\n", jobDetail.Company)
		fmt.Printf("User: %s\n", userID)
		fmt.Printf("Search Criteria: %s\n", criteriaID)

		// Extract related job IDs
		relatedJobIDs := internal.ExtractRelatedJobIDs(jobDetail.SimilarJobs, jobDetail.PeopleAlsoViewed)
		fmt.Printf("\nFound %d related job IDs:\n", len(relatedJobIDs))
		for i, id := range relatedJobIDs {
			fmt.Printf("  %d. %s\n", i+1, id)
		}

		if len(relatedJobIDs) == 0 {
			fmt.Println("No related job IDs found. Nothing to process.")
			return nil
		}

		if dryRun {
			fmt.Println("\n[DRY RUN] Would process the above related jobs.")
			fmt.Println("Use --dry-run=false to actually process them.")
			return nil
		}

		// Get search criteria
		searchCriteria, found, err := internal.GetSearchCriteria(ctx, fs, criteriaID)
		if err != nil {
			return fmt.Errorf("failed to get search criteria: %w", err)
		}
		if !found {
			return fmt.Errorf("search criteria not found: %s", criteriaID)
		}

		// Create logger
		logger := slog.Default()

		// Create job provider (ScrapingDog)
		apiKey := os.Getenv("SCRAPINGDOG_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("SCRAPINGDOG_API_KEY environment variable is required")
		}

		scrapingDogProvider := scrapingdog.NewProvider(apiKey, logger)

		// Create a wrapper that implements JobProvider interface
		provider := &scrapingDogProviderWrapper{provider: scrapingDogProvider}

		fmt.Println("\nProcessing related jobs...")

		// Process related jobs
		err = internal.ProcessRelatedJobs(ctx, fs, logger, userID, jobDetail, *searchCriteria, provider)
		if err != nil {
			return fmt.Errorf("failed to process related jobs: %w", err)
		}

		fmt.Println("✅ Successfully processed related jobs!")

		// Show summary of what was done
		fmt.Println("\nTo see the results:")
		fmt.Printf("  ./cli jobs list --user %s\n", userID)
		fmt.Printf("  ./cli search list-queued --criteria %s\n", criteriaID)

		return nil
	},
}
