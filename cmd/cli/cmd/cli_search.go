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
	searchCriteriaIDFlag string
)

type jobSinkForwarder struct {
	logger  *slog.Logger
	jobSink *internal.SearchResultProcessor
	done    chan struct{}
}

func (f *jobSinkForwarder) OnSearchStarted(task internal.SearchTask, userID *string) {
	f.logger.Info(internal.BGGreen("Search started"),
		"searchID", task.ID,
		"criteria", task.Criteria)
	// Forward to JobSink with context
	f.jobSink.OnSearchStarted(task, userID)
}

func (f *jobSinkForwarder) OnJobListing(task *internal.SearchTask, listing internal.JobListing, userID *string) bool {
	f.logger.Info(internal.Blue("New job listing found"),
		"jobID", listing.ID,
		"title", listing.Title,
		"company", listing.Company,
		"location", listing.Location)
	return f.jobSink.OnJobListing(task, listing, userID)
}

func (f *jobSinkForwarder) OnJobDetail(task internal.SearchTask, listing internal.JobListing, details internal.JobDetail, userID *string) {
	f.logger.Info(internal.BGBlue("New job detail found"),
		"jobID", details.ID,
		"title", details.Title,
		"company", details.Company,
		"location", details.Location)
	f.jobSink.OnJobDetail(task, listing, details, userID)
}

func (f *jobSinkForwarder) OnPageComplete(task internal.SearchTask, page int, jobsFound int, errors []error, userID *string) {
	if len(errors) > 0 {
		f.logger.Error(internal.BGRed("Page completed with errors"),
			"page", page,
			"jobsFound", jobsFound,
			"errors", errors)
	} else {
		f.logger.Info(internal.BGGreen("Page completed successfully"),
			"page", page,
			"jobsFound", jobsFound)
	}
	f.jobSink.OnPageComplete(task, page, jobsFound, errors, userID)
}

func (f *jobSinkForwarder) OnSearchCompleted(task internal.SearchTask, totalJobs int, errors []error, userID *string) {
	if len(errors) > 0 {
		f.logger.Error(internal.BGRed("Search completed with errors"),
			"totalJobs", totalJobs,
			"errors", errors)
	} else {
		f.logger.Info(internal.BGGreen("Search completed successfully"),
			"totalJobs", totalJobs)
	}
	f.jobSink.OnSearchCompleted(task, totalJobs, errors, userID)
	close(f.done) // Signal completion
}

func (f *jobSinkForwarder) OnJobDetailExtractionComplete(task internal.SearchTask, jobDetail internal.JobDetail, userID *string) {
	f.logger.Info(internal.BGGreen("Job detail extraction complete"),
		"jobID", jobDetail.ID,
		"title", jobDetail.Title,
		"company", jobDetail.Company,
		"location", jobDetail.Location)
	f.jobSink.OnJobDetailExtractionComplete(task, jobDetail, userID)
}

func (f *jobSinkForwarder) OnAllExtractionsComplete(task internal.SearchTask, totalJobs int, userID *string) {
	f.logger.Info(internal.BGGreen("All extractions complete"),
		"totalJobs", totalJobs)
	f.jobSink.OnAllExtractionsComplete(task, totalJobs, userID)
}

func init() {
	searchCmd := &cobra.Command{
		Use:   "search",
		Short: "Manage job searches and search execution",
		Long: `Execute job searches and manage search operations.

Job searches find new job listings based on search criteria and add them
to users' job collections. Searches are queued and processed asynchronously
by job search providers like ScrapingDog.

This command group allows you to:
- Queue new job searches with saved criteria
- Monitor search progress and completion`,
	}

	queueCmd := &cobra.Command{
		Use:   "queue --criteria-id <criteria-id>",
		Short: "Queue a new job search with saved search criteria",
		Long: `Queue a new job search using previously saved search criteria.

This command starts an asynchronous job search that will find new job listings
matching the specified criteria. The search runs in the background and adds
found jobs to users' collections automatically.

The search criteria must already exist in the system. Use the 'criteria' command
group to create and manage search criteria.

Required flags:
  --criteria-id, -c: ID of saved search criteria to use for the search

Common search criteria examples:
  golang-developer-remote    - Go developer positions, remote work
  python-data-scientist      - Python data science roles
  frontend-react-nyc         - Frontend React jobs in NYC area

Examples:
  # Queue search with saved criteria
  trcli search queue --criteria-id golang-developer-remote
  
  # Queue search for specific location
  trcli search queue --criteria-id "python-jobs-san-francisco"`,
		RunE: runSearch,
	}

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
	if searchCriteriaIDFlag == "" {
		return fmt.Errorf("criteria-id is required to load saved search criteria")
	}
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

	// Create JobSink (no JobProcessor for MVP)
	jobSink := internal.NewSearchResultProcessor(fs, logger, nil, nil)
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

	searchTask, err := internal.CreateSearchTask(ctx, fs, criteria, "scrapingdog", nil)
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
