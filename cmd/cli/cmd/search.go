package cmd

import (
	"context"
	"fmt"
	"os"

	"log/slog"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search/providers/scrapingdog"

	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

var (
	searchQueryFlag      string
	searchGeoIDFlag      string
	searchJobTypeFlag    []string
	searchExperienceFlag []string
	searchWorkTypeFlag   string
	searchMaxPagesFlag   int
	searchCriteriaIDFlag string
)

type jobSinkForwarder struct {
	logger  *slog.Logger
	jobSink *internal.JobSink
	done    chan struct{}
}

func (f *jobSinkForwarder) OnSearchStarted(task internal.SearchTask) {
	f.logger.Info(internal.BGGreen("Search started"),
		"searchID", task.ID,
		"criteria", task.Criteria)
	// Forward to JobSink with context
	f.jobSink.OnSearchStarted(context.Background(), task)
}

func (f *jobSinkForwarder) OnJobListing(task internal.SearchTask, listing internal.JobListing) {
	f.logger.Info(internal.Blue("New job listing found"),
		"jobID", listing.ID,
		"title", listing.Title,
		"company", listing.Company,
		"location", listing.Location)
	f.jobSink.OnJobListing(context.Background(), task, listing)
}

func (f *jobSinkForwarder) OnJobDetail(task internal.SearchTask, details internal.JobDetail) {
	f.logger.Info(internal.BGBlue("New job detail found"),
		"jobID", details.ID,
		"title", details.Title,
		"company", details.Company,
		"location", details.Location)
	f.jobSink.OnJobDetail(context.Background(), task, details)
}

func (f *jobSinkForwarder) OnSearchCompleted(task internal.SearchTask, totalJobs int, errors []error) {
	if len(errors) > 0 {
		f.logger.Error(internal.BGRed("Search completed with errors"),
			"totalJobs", totalJobs,
			"errors", errors)
	} else {
		f.logger.Info(internal.BGGreen("Search completed successfully"),
			"totalJobs", totalJobs)
	}
	f.jobSink.OnSearchCompleted(context.Background(), task, totalJobs, errors)
	close(f.done) // Signal completion
}

func init() {
	searchCmd := &cobra.Command{
		Use:   "search",
		Short: "Manage job searches",
		Long:  `Search for jobs and manage search criteria.`,
	}

	queueCmd := &cobra.Command{
		Use:   "queue",
		Short: "Queue a new job search",
		Long: `Queue a new job search with specified criteria and wait for completion.
Example: cli search queue --query "golang developer" --geo-id "103644278"

Alternatively, you can use a saved criteria:
Example: cli search queue --criteria-id "golang-developer-103644278"

Common GeoIDs:
- Remote/United States: 103644278
- Greater Atlanta Area: 90000052
- Greater Boston Area: 90000007
- Greater Chicago Area: 90000014
- Greater San Francisco Area: 90000084
- Greater New York Area: 90000070`,
		RunE: runSearch,
	}

	// Add flags
	queueCmd.Flags().StringVarP(&searchQueryFlag, "query", "q", "", "Search query")
	queueCmd.Flags().StringVarP(&searchGeoIDFlag, "geo-id", "g", "", "Geographic ID for location")
	queueCmd.Flags().StringSliceVarP(&searchJobTypeFlag, "job-type", "j", nil, "Job types (temporary,contract,volunteer,full_time,part_time)")
	queueCmd.Flags().StringSliceVarP(&searchExperienceFlag, "experience", "e", nil, "Experience levels (internship,entry_level,associate,mid_senior_level,director)")
	queueCmd.Flags().StringVarP(&searchWorkTypeFlag, "work-type", "w", "", "Work type (at_work,remote,hybrid)")
	queueCmd.Flags().IntVarP(&searchMaxPagesFlag, "max-pages", "p", 1, "Maximum number of pages to search")
	queueCmd.Flags().StringVarP(&searchCriteriaIDFlag, "criteria-id", "c", "", "ID of saved search criteria to use")

	// Add subcommands to search command
	searchCmd.AddCommand(queueCmd)

	// Add search command to root command
	RootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

	// Create search criteria
	var criteria internal.SearchCriteria

	// If criteriaID is provided, load the saved criteria
	if searchCriteriaIDFlag != "" {
		// Create Firestore client
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		// Get the criteria by ID
		savedCriteria, found, err := internal.GetSearchCriteria(ctx, fs, searchCriteriaIDFlag)
		if err != nil {
			return fmt.Errorf("failed to get saved criteria with ID %s: %w", searchCriteriaIDFlag, err)
		}
		if !found {
			return fmt.Errorf("saved criteria with ID %s not found", searchCriteriaIDFlag)
		}

		criteria = *savedCriteria
		logger.Info("Using saved search criteria",
			"criteria_id", searchCriteriaIDFlag,
			"query", criteria.Query,
			"geo_id", criteria.GeoID)
	} else {
		// Ensure required parameters are provided if not using a saved criteria
		if searchQueryFlag == "" {
			return fmt.Errorf("query is required when not using a saved criteria")
		}
		if searchGeoIDFlag == "" {
			return fmt.Errorf("geo-id is required when not using a saved criteria")
		}

		criteria = internal.SearchCriteria{
			Query:      searchQueryFlag,
			GeoID:      searchGeoIDFlag,
			JobType:    searchJobTypeFlag,
			Experience: searchExperienceFlag,
			WorkType:   &searchWorkTypeFlag,
			MaxPages:   searchMaxPagesFlag,
		}
	}

	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer fs.Close()

	// Create JobSink (no JobProcessor for MVP)
	jobSink := internal.NewJobSink(fs, logger, nil)
	// Wrap with forwarder for logging
	eventHandler := &jobSinkForwarder{
		logger:  logger,
		jobSink: jobSink,
		done:    make(chan struct{}),
	}

	// Initialize ScrapingDog provider
	apiKey := os.Getenv("SCRAPINGDOG_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("SCRAPINGDOG_API_KEY environment variable is required")
	}

	provider := scrapingdog.NewProvider(apiKey, logger,
		scrapingdog.WithJobEventHandler(eventHandler))

	// Start workers
	provider.Start(ctx)

	searchTask, err := internal.CreateSearchTask(ctx, fs, criteria, "scrapingdog")
	if err != nil {
		return fmt.Errorf("failed to create search task: %w", err)
	}

	// Queue the search
	if err := provider.QueueSearch(ctx, *searchTask); err != nil {
		return fmt.Errorf("failed to queue search: %w", err)
	}

	// Wait for search completion before exiting and closing Firestore client
	<-eventHandler.done

	return nil
}
