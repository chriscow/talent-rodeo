package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"log/slog"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search"
	"talent-rodeo/internal/search/providers/scrapingdog"

	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

var (
	query      string
	geoID      string
	jobType    []string
	experience []string
	workType   string
	userID     string
	maxPages   int
)

// Map of location names to GeoIDs
var locationGeoIDs = map[string]string{
	"remote":                            "103644278",
	"united states":                     "103644278",
	"greater atlanta area":              "90000052",
	"greater baltimore area":            "90000097",
	"greater boston area":               "90000007",
	"greater chicago area":              "90000014",
	"greater dallas-fort worth area":    "90000031",
	"greater denver area":               "90000034",
	"greater detroit area":              "90000035",
	"greater houston area":              "90000042",
	"greater los angeles area":          "90000049",
	"greater miami area":                "90000056",
	"greater minneapolis-st. paul area": "90000512",
	"greater new york area":             "90000070",
	"greater philadelphia area":         "90000077",
	"greater phoenix area":              "90000620",
	"greater san diego area":            "90010472",
	"greater san francisco area":        "90000084",
	"greater seattle area":              "90000091",
	"greater tampa area":                "90000828",
}

type searchEventHandler struct {
	logger         *slog.Logger
	searchComplete chan error
}

func (h *searchEventHandler) OnNewJob(userID string, criteria search.Criteria, jobID string, details search.JobDetail) error {
	h.logger.Info(internal.BGBlue("New job found"),
		"userID", userID,
		"jobID", jobID,
		"title", details.Title,
		"company", details.Company,
		"location", details.Location)
	return nil
}

func (h *searchEventHandler) OnSearchCompleted(userID string, criteria search.Criteria, totalJobs int, errors []error) error {
	if len(errors) > 0 {
		h.logger.Error(internal.BGRed("Search completed with errors"),
			"userID", userID,
			"totalJobs", totalJobs,
			"errors", errors)
		h.searchComplete <- fmt.Errorf("search completed with %d errors", len(errors))
		return nil
	}
	h.logger.Info(internal.BGGreen("Search completed successfully"),
		"userID", userID,
		"totalJobs", totalJobs)
	h.searchComplete <- nil
	return nil
}

func init() {
	searchCmd := &cobra.Command{
		Use:   "queue",
		Short: "Queue a new job search",
		Long: `Queue a new job search with specified criteria and wait for completion.
Example: search queue --query "golang developer" --geo-id "103644278" --user-id "123"

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
	searchCmd.Flags().StringVarP(&query, "query", "q", "", "Search query (required)")
	searchCmd.Flags().StringVarP(&geoID, "geo-id", "g", "", "Geographic ID for location (required)")
	searchCmd.Flags().StringSliceVarP(&jobType, "job-type", "j", nil, "Job types (temporary,contract,volunteer,full_time,part_time)")
	searchCmd.Flags().StringSliceVarP(&experience, "experience", "e", nil, "Experience levels (internship,entry_level,associate,mid_senior_level,director)")
	searchCmd.Flags().StringVarP(&workType, "work-type", "w", "", "Work type (at_work,remote,hybrid)")
	searchCmd.Flags().StringVarP(&userID, "user-id", "u", "", "User ID (required)")
	searchCmd.Flags().IntVarP(&maxPages, "max-pages", "p", 1, "Maximum number of pages to search")

	// Mark required flags
	searchCmd.MarkFlagRequired("query")
	searchCmd.MarkFlagRequired("geo-id")
	searchCmd.MarkFlagRequired("user-id")

	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

	// Create search criteria
	criteria := search.Criteria{
		Query:      query,
		Location:   geoID,
		JobType:    jobType,
		Experience: experience,
		WorkType:   workType,
		MaxPages:   maxPages,
	}

	// Create event handler with completion channel
	searchComplete := make(chan error, 1)
	eventHandler := &searchEventHandler{
		logger:         logger,
		searchComplete: searchComplete,
	}

	// Initialize ScrapingDog provider
	apiKey := os.Getenv("SCRAPINGDOG_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("SCRAPINGDOG_API_KEY environment variable is required")
	}

	provider := scrapingdog.NewProvider(apiKey,
		scrapingdog.WithJobEventHandler(eventHandler),
		scrapingdog.WithProviderLogger(logger))

	// Start workers
	provider.StartWorkers(ctx, 5)

	// Queue the search
	if err := provider.QueueSearch(ctx, userID, criteria); err != nil {
		return fmt.Errorf("failed to queue search: %w", err)
	}

	logger.Info("Search queued successfully. Waiting for completion...")

	// Wait for search completion or timeout
	select {
	case err := <-searchComplete:
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}
		return nil
	case <-time.After(5 * time.Minute):
		return fmt.Errorf("search timed out after 5 minutes")
	}
}
