package cmd

import (
	"context"
	"fmt"
	"talent-rodeo/internal"

	"github.com/spf13/cobra"
)

// filteredEmailsCmd represents the filtered-emails command
var filteredEmailsCmd = &cobra.Command{
	Use:   "filtered-emails",
	Short: "Manage filtered email records",
	Long: `Manage email records that were filtered out by the system.
	
This command allows you to:
- List filtered emails by filter type or reason
- View statistics about filtered emails
- Debug email filtering issues`,
}

// listFilteredEmailsCmd lists filtered emails
var listFilteredEmailsCmd = &cobra.Command{
	Use:   "list [filter-name]",
	Short: "List filtered emails",
	Long: `List emails that were filtered out, optionally filtered by filter name.
	
Examples:
  cli filtered-emails list                    # List all filtered emails  
  cli filtered-emails list user-verification # List emails filtered by user-verification filter`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Get filter parameter
		filterName := ""
		if len(args) > 0 {
			filterName = args[0]
		}

		// Get limit flag
		limit, _ := cmd.Flags().GetInt("limit")

		// Initialize services
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Firestore: %w", err)
		}
		defer fs.Close()

		filteredEmailService := internal.NewFilteredEmailService(fs)

		// Get filtered emails
		var filteredEmails []*internal.FilteredEmail
		if filterName != "" {
			filteredEmails, err = filteredEmailService.GetFilteredEmailsByFilter(ctx, filterName, limit)
		} else {
			filteredEmails, err = internal.ListFilteredEmails(ctx, fs, "", "", limit)
		}

		if err != nil {
			return fmt.Errorf("failed to get filtered emails: %w", err)
		}

		// Display results
		if len(filteredEmails) == 0 {
			if filterName != "" {
				fmt.Printf("No filtered emails found for filter: %s\n", filterName)
			} else {
				fmt.Printf("No filtered emails found.\n")
			}
			return nil
		}

		if filterName != "" {
			fmt.Printf("Found %d filtered email(s) for filter '%s':\n\n", len(filteredEmails), filterName)
		} else {
			fmt.Printf("Found %d filtered email(s):\n\n", len(filteredEmails))
		}

		for i, email := range filteredEmails {
			fmt.Printf("%d. Email ID: %s\n", i+1, email.ID)
			fmt.Printf("   From: %s\n", email.From)
			if email.ExtractedEmail != "" && email.ExtractedEmail != email.From {
				fmt.Printf("   Extracted Email: %s\n", email.ExtractedEmail)
			}
			fmt.Printf("   Subject: %s\n", email.Subject)
			fmt.Printf("   Filter: %s\n", email.FilterName)
			fmt.Printf("   Reason: %s\n", email.FilterReason)
			if email.ReasonDetails != "" {
				fmt.Printf("   Details: %s\n", email.ReasonDetails)
			}
			fmt.Printf("   Created: %s\n", email.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Println()
		}

		return nil
	},
}

// statsFilteredEmailsCmd shows statistics for filtered emails
var statsFilteredEmailsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show statistics for filtered emails",
	Long:  `Show statistics including counts by filter and reason.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Initialize services
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Firestore: %w", err)
		}
		defer fs.Close()

		filteredEmailService := internal.NewFilteredEmailService(fs)

		// Get statistics
		stats, err := filteredEmailService.GetStats(ctx)
		if err != nil {
			return fmt.Errorf("failed to get filtered email stats: %w", err)
		}

		totalFiltered := stats["total_filtered"].(int)
		filterCounts := stats["by_filter"].(map[string]int)
		reasonCounts := stats["by_reason"].(map[string]int)

		fmt.Printf("Filtered Email Statistics:\n")
		fmt.Printf("==========================\n\n")

		fmt.Printf("Total Filtered Emails: %d\n\n", totalFiltered)

		if len(filterCounts) > 0 {
			fmt.Printf("By Filter:\n")
			for filter, count := range filterCounts {
				percentage := float64(count) / float64(totalFiltered) * 100
				fmt.Printf("  %s: %d (%.1f%%)\n", filter, count, percentage)
			}
			fmt.Println()
		}

		if len(reasonCounts) > 0 {
			fmt.Printf("By Reason:\n")
			for reason, count := range reasonCounts {
				percentage := float64(count) / float64(totalFiltered) * 100
				fmt.Printf("  %s: %d (%.1f%%)\n", reason, count, percentage)
			}
			fmt.Println()
		}

		if totalFiltered == 0 {
			fmt.Println("No filtered emails found.")
		}

		return nil
	},
}

// detailFilteredEmailCmd shows detailed information for a filtered email
var detailFilteredEmailCmd = &cobra.Command{
	Use:   "detail <email-id>",
	Short: "Show detailed information for a filtered email",
	Long:  `Show detailed information including original email content and filter details.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		emailID := args[0]

		// Initialize services
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Firestore: %w", err)
		}
		defer fs.Close()

		// Get the filtered email
		filteredEmail, exists, err := internal.GetFilteredEmail(ctx, fs, emailID)
		if err != nil {
			return fmt.Errorf("failed to get filtered email: %w", err)
		}
		if !exists {
			return fmt.Errorf("filtered email not found: %s", emailID)
		}

		// Display detailed information
		fmt.Printf("Filtered Email Details:\n")
		fmt.Printf("=======================\n\n")

		fmt.Printf("ID: %s\n", filteredEmail.ID)
		fmt.Printf("Original Email ID: %s\n", filteredEmail.OriginalEmailID)
		fmt.Printf("Created: %s\n", filteredEmail.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Filter: %s\n", filteredEmail.FilterName)
		fmt.Printf("Reason: %s\n", filteredEmail.FilterReason)
		if filteredEmail.ReasonDetails != "" {
			fmt.Printf("Details: %s\n", filteredEmail.ReasonDetails)
		}
		fmt.Println()

		fmt.Printf("Original Email:\n")
		fmt.Printf("---------------\n")
		fmt.Printf("From: %s\n", filteredEmail.From)
		if filteredEmail.ExtractedEmail != "" && filteredEmail.ExtractedEmail != filteredEmail.From {
			fmt.Printf("Extracted Email: %s\n", filteredEmail.ExtractedEmail)
		}
		fmt.Printf("Subject: %s\n", filteredEmail.Subject)
		fmt.Printf("Body:\n%s\n", filteredEmail.Body)

		return nil
	},
}

func init() {
	RootCmd.AddCommand(filteredEmailsCmd)

	// Add subcommands
	filteredEmailsCmd.AddCommand(listFilteredEmailsCmd)
	filteredEmailsCmd.AddCommand(statsFilteredEmailsCmd)
	filteredEmailsCmd.AddCommand(detailFilteredEmailCmd)

	// Add flags
	listFilteredEmailsCmd.Flags().Int("limit", 50, "Maximum number of filtered emails to return")
}
