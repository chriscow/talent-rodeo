package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"talent-rodeo/internal"

	"cloud.google.com/go/firestore"
	"github.com/spf13/cobra"
)

var (
	criteriaQueryFlag    string
	criteriaGeoIDFlag    string
	criteriaMaxPagesFlag int
	criteriaAllFlag      bool
)

func init() {
	// Create criteria command
	criteriaCmd := &cobra.Command{
		Use:   "criteria",
		Short: "Manage search criteria",
		Long:  `Manage search criteria for job searches. Create, list, and delete search criteria.`,
	}

	// 'list' subcommand
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all search criteria",
		Long:  `List all search criteria in the database.`,
		RunE:  runListCriteria,
	}
	listCmd.Flags().BoolVarP(&criteriaAllFlag, "all", "a", false, "Show all details including timestamps")

	// 'needs-search' subcommand
	needsSearchCmd := &cobra.Command{
		Use:   "needs-search",
		Short: "List criteria that need to be searched",
		Long:  `List criteria that have never been searched or haven't been searched in the last 24 hours.`,
		RunE:  runNeedsSearchCriteria,
	}
	needsSearchCmd.Flags().BoolVarP(&criteriaAllFlag, "all", "a", false, "Show all details including timestamps")

	// 'create' subcommand
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new search criteria",
		Long:  `Create a new search criteria with the specified query, geo-id, and max pages.`,
		RunE:  runCreateCriteria,
	}
	createCmd.Flags().StringVarP(&criteriaQueryFlag, "query", "q", "", "Search query (required)")
	createCmd.Flags().StringVarP(&criteriaGeoIDFlag, "geo-id", "g", "103644278", "Geographic ID for location (defaults to 'Remote')")
	createCmd.Flags().IntVarP(&criteriaMaxPagesFlag, "max-pages", "p", 5, "Maximum number of pages to search")
	createCmd.MarkFlagRequired("query")

	// 'delete' subcommand
	deleteCmd := &cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a search criteria",
		Long:  `Delete a search criteria by its ID.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runDeleteCriteria,
	}

	// Add subcommands to criteria command
	criteriaCmd.AddCommand(listCmd, needsSearchCmd, createCmd, deleteCmd)

	// Add criteria command to search command
	RootCmd.AddCommand(criteriaCmd)
}

func runListCriteria(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Create Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer fs.Close()

	// Get all criteria
	criteria, err := internal.ListSearchCriteria(ctx, fs)
	if err != nil {
		return fmt.Errorf("failed to get search criteria: %w", err)
	}

	// Display criteria
	return displayCriteria(criteria, criteriaAllFlag)
}

func runNeedsSearchCriteria(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Create Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer fs.Close()

	// Get criteria that need search
	criteria, err := internal.GetSearchCriteriaNeedsSearch(ctx, fs)
	if err != nil {
		return fmt.Errorf("failed to get criteria that need search: %w", err)
	}

	// Display criteria
	return displayCriteria(criteria, criteriaAllFlag)
}

func runCreateCriteria(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Create Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer fs.Close()

	// geoID, err := internal.GetGeoID(ctx, fs, criteriaGeoIDFlag)
	// if err != nil {
	// 	return fmt.Errorf("failed to get geo ID: %w", err)
	// }

	// Create search criteria
	criteria, err := internal.CreateSearchCriteria(ctx, fs, criteriaQueryFlag, criteriaGeoIDFlag, criteriaMaxPagesFlag, internal.ListingAgeDay)
	if err != nil {
		return fmt.Errorf("failed to create search criteria: %w", err)
	}

	fmt.Printf("Search criteria created successfully with ID: %s\n", criteria.ID)
	return nil
}

func runDeleteCriteria(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Create Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer fs.Close()

	// Get ID from args
	id := args[0]

	subscribers, err := internal.GetCriteriaSubscribers(ctx, fs, id)
	if err != nil {
		return fmt.Errorf("failed to get subscribers for search criteria ID %s: %w", id, err)
	}

	jobIDs, err := internal.GetJobIDsForCriteria(ctx, fs, id)
	if err != nil {
		return fmt.Errorf("failed to get job IDs for search criteria ID %s: %w", id, err)
	}

	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {

		for _, sub := range subscribers {
			// Unsubscribe each user from the criteria
			err = internal.UnsubscribeFromCriteriaTx(ctx, fs, tx, sub.UserID, id)
			if err != nil {
				return fmt.Errorf("failed to unsubscribe user %s from search criteria ID %s: %w", sub.UserID, id, err)
			}
			fmt.Printf("User %s unsubscribed from search criteria ID %s\n", sub.UserID, id)
		}

		for _, jobID := range jobIDs {
			err = internal.UnlinkJobDetailFromCriteriaTx(ctx, fs, tx, jobID, id)
			if err != nil {
				return fmt.Errorf("failed to unlink job ID %s from search criteria ID %s: %w", jobID, id, err)
			}
			fmt.Printf("Job ID %s unlinked from search criteria ID %s\n", jobID, id)
		}

		err = internal.DeleteSearchCriteriaTx(ctx, fs, tx, id)
		if err != nil {
			return fmt.Errorf("failed to delete search criteria ID %s: %w", id, err)
		}
		fmt.Printf("Search criteria deleted successfully: %s\n", id)

		return nil
	})
}

func displayCriteria(criteria []internal.SearchCriteria, showAll bool) error {
	if len(criteria) == 0 {
		fmt.Println("No search criteria found")
		return nil
	}

	// Create tabwriter
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print header
	if showAll {
		fmt.Fprintln(w, "ID\tQuery\tGeo ID\tMax Pages\tLast Searched\tError")
	} else {
		fmt.Fprintln(w, "ID\tQuery\tGeo ID\tMax Pages")
	}

	// Print separator
	if showAll {
		fmt.Fprintln(w, "----\t-----\t------\t---------\t-------------\t-----")
	} else {
		fmt.Fprintln(w, "----\t-----\t------\t---------")
	}

	// Print criteria
	for _, c := range criteria {
		if showAll {
			lastSearched := "Never"
			if c.LastSearchedAt != nil {
				lastSearched = c.LastSearchedAt.Format(time.RFC3339)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
				c.ID, c.Query, c.GeoID, c.MaxPages, lastSearched, c.Error)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", c.ID, c.Query, c.GeoID, c.MaxPages)
		}
	}

	return w.Flush()
}
