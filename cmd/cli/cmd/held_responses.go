package cmd

import (
	"context"
	"fmt"
	"strings"
	"talent-rodeo/internal"

	"github.com/spf13/cobra"
)

// heldResponsesCmd represents the held-responses command
var heldResponsesCmd = &cobra.Command{
	Use:   "held-responses",
	Short: "Manage held email responses",
	Long: `Manage held email responses that are pending admin approval.
	
This command allows you to:
- List held responses by status
- Approve responses for sending
- Request revisions with feedback
- View statistics and details`,
}

// listHeldResponsesCmd lists held responses
var listHeldResponsesCmd = &cobra.Command{
	Use:   "list [status]",
	Short: "List held responses",
	Long: `List held responses, optionally filtered by status.
	
Available statuses: pending, approved, revision, expired, all (default)`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Get status filter
		status := "all"
		if len(args) > 0 {
			status = args[0]
		}

		// Validate status
		validStatuses := []string{"all", "pending", "approved", "revision", "expired"}
		isValid := false
		for _, validStatus := range validStatuses {
			if status == validStatus {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid status: %s. Valid statuses: %s", status, strings.Join(validStatuses, ", "))
		}

		// Initialize services
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Firestore: %w", err)
		}
		defer fs.Close()

		heldResponseService := internal.NewHeldResponseService(fs, nil, nil, nil)

		// Get responses based on status
		var responses []*internal.HeldResponse
		if status == "all" {
			// Get all responses (we'll implement this method)
			responses, err = getAllHeldResponses(ctx, heldResponseService)
		} else {
			// Parse status
			var heldStatus internal.HeldResponseStatus
			switch status {
			case "pending":
				heldStatus = internal.HeldResponseStatusPending
			case "approved":
				heldStatus = internal.HeldResponseStatusApproved
			case "revision":
				heldStatus = internal.HeldResponseStatusRevision
			case "expired":
				heldStatus = internal.HeldResponseStatusExpired
			}
			responses, err = heldResponseService.ListHeldResponses(ctx, heldStatus, 100)
		}

		if err != nil {
			return fmt.Errorf("failed to get held responses: %w", err)
		}

		// Display results
		if len(responses) == 0 {
			fmt.Printf("No %s held responses found.\n", status)
			return nil
		}

		fmt.Printf("Found %d %s held response(s):\n\n", len(responses), status)

		for _, response := range responses {
			fmt.Printf("ID: %s\n", response.ID)
			fmt.Printf("  Status: %s\n", response.Status)
			fmt.Printf("  From: %s\n", response.OriginalFrom)
			fmt.Printf("  To: %s\n", response.RecipientEmail)
			fmt.Printf("  Subject: %s\n", response.OriginalSubject)
			fmt.Printf("  Created: %s\n", response.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Expires: %s\n", response.ExpiresAt.Format("2006-01-02 15:04:05"))
			if response.RevisionCount > 0 {
				fmt.Printf("  Revisions: %d\n", response.RevisionCount)
			}
			if response.HoldReason != "" {
				fmt.Printf("  Hold Reason: %s\n", response.HoldReason)
			}
			fmt.Println()
		}

		return nil
	},
}

// approveHeldResponseCmd approves a held response
var approveHeldResponseCmd = &cobra.Command{
	Use:   "approve <response-id>",
	Short: "Approve a held response for sending",
	Long:  `Approve a held response and send it to the recipient.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		responseID := args[0]

		// Initialize services
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Firestore: %w", err)
		}
		defer fs.Close()

		heldResponseService := internal.NewHeldResponseService(fs, nil, nil, nil)

		// Get admin user ID from flag or default
		adminUserID, _ := cmd.Flags().GetString("admin-user")
		if adminUserID == "" {
			adminUserID = "cli-admin"
		}

		// Approve the response
		err = heldResponseService.ApproveResponse(ctx, responseID, adminUserID)
		if err != nil {
			return fmt.Errorf("failed to approve response: %w", err)
		}

		fmt.Printf("Successfully approved and sent response %s\n", responseID)
		return nil
	},
}

// requestRevisionCmd requests revision for a held response
var requestRevisionCmd = &cobra.Command{
	Use:   "request-revision <response-id> <feedback>",
	Short: "Request revision for a held response",
	Long:  `Request revision for a held response with admin feedback.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		responseID := args[0]
		feedback := args[1]

		// Initialize services
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Firestore: %w", err)
		}
		defer fs.Close()

		heldResponseService := internal.NewHeldResponseService(fs, nil, nil, nil)

		// Get admin user ID from flag or default
		adminUserID, _ := cmd.Flags().GetString("admin-user")
		if adminUserID == "" {
			adminUserID = "cli-admin"
		}

		// Request revision
		err = heldResponseService.RequestRevision(ctx, responseID, adminUserID, feedback)
		if err != nil {
			return fmt.Errorf("failed to request revision: %w", err)
		}

		fmt.Printf("Successfully requested revision for response %s\n", responseID)
		return nil
	},
}

// detailHeldResponseCmd shows details for a held response
var detailHeldResponseCmd = &cobra.Command{
	Use:   "detail <response-id>",
	Short: "Show detailed information for a held response",
	Long:  `Show detailed information including original email and generated response.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		responseID := args[0]

		// Initialize services
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Firestore: %w", err)
		}
		defer fs.Close()

		// Get the held response
		heldResponse, exists, err := internal.GetHeldResponse(ctx, fs, responseID)
		if err != nil {
			return fmt.Errorf("failed to get held response: %w", err)
		}
		if !exists {
			return fmt.Errorf("held response not found: %s", responseID)
		}

		// Display detailed information
		fmt.Printf("Held Response Details:\n")
		fmt.Printf("======================\n\n")

		fmt.Printf("ID: %s\n", heldResponse.ID)
		fmt.Printf("Status: %s\n", heldResponse.Status)
		fmt.Printf("Created: %s\n", heldResponse.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Expires: %s\n", heldResponse.ExpiresAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Hold Criteria: %s\n", heldResponse.HoldCriteria)
		fmt.Printf("Hold Reason: %s\n", heldResponse.HoldReason)
		if heldResponse.RevisionCount > 0 {
			fmt.Printf("Revisions: %d\n", heldResponse.RevisionCount)
		}
		fmt.Println()

		fmt.Printf("Original Email:\n")
		fmt.Printf("---------------\n")
		fmt.Printf("From: %s\n", heldResponse.OriginalFrom)
		fmt.Printf("To: %s\n", heldResponse.RecipientEmail)
		fmt.Printf("Subject: %s\n", heldResponse.OriginalSubject)
		fmt.Printf("Date: %s\n", heldResponse.OriginalDate.Format("2006-01-02 15:04:05"))
		fmt.Printf("Body:\n%s\n\n", heldResponse.OriginalBody)

		fmt.Printf("Generated Response:\n")
		fmt.Printf("-------------------\n")
		fmt.Printf("Subject: %s\n", heldResponse.ResponseSubject)
		fmt.Printf("Type: %s\n", heldResponse.ResponseType)
		fmt.Printf("Body:\n%s\n", heldResponse.ResponseBody)
		if heldResponse.ResponseHTMLBody != "" {
			fmt.Printf("HTML Body:\n%s\n", heldResponse.ResponseHTMLBody)
		}

		if heldResponse.AdminFeedback != "" {
			fmt.Printf("\nAdmin Feedback:\n")
			fmt.Printf("---------------\n")
			fmt.Printf("%s\n", heldResponse.AdminFeedback)
		}

		if heldResponse.AdminUserID != "" {
			fmt.Printf("\nAdmin Info:\n")
			fmt.Printf("-----------\n")
			fmt.Printf("Admin User: %s\n", heldResponse.AdminUserID)
			if !heldResponse.ReviewedAt.IsZero() {
				fmt.Printf("Reviewed: %s\n", heldResponse.ReviewedAt.Format("2006-01-02 15:04:05"))
			}
			if !heldResponse.ApprovedAt.IsZero() {
				fmt.Printf("Approved: %s\n", heldResponse.ApprovedAt.Format("2006-01-02 15:04:05"))
			}
		}

		return nil
	},
}

// statsHeldResponseCmd shows statistics for held responses
var statsHeldResponseCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show statistics for held responses",
	Long:  `Show statistics including counts by status and hold criteria.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Initialize services
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Firestore: %w", err)
		}
		defer fs.Close()

		heldResponseService := internal.NewHeldResponseService(fs, nil, nil, nil)

		// Get counts for each status
		pendingResponses, err := heldResponseService.GetPendingResponses(ctx)
		if err != nil {
			return fmt.Errorf("failed to get pending responses: %w", err)
		}

		approvedResponses, err := heldResponseService.GetApprovedResponses(ctx)
		if err != nil {
			return fmt.Errorf("failed to get approved responses: %w", err)
		}

		revisionResponses, err := heldResponseService.GetRevisionResponses(ctx)
		if err != nil {
			return fmt.Errorf("failed to get revision responses: %w", err)
		}

		expiredResponses, err := heldResponseService.GetExpiredResponses(ctx)
		if err != nil {
			return fmt.Errorf("failed to get expired responses: %w", err)
		}

		// Calculate totals and percentages
		totalResponses := len(pendingResponses) + len(approvedResponses) + len(revisionResponses) + len(expiredResponses)

		fmt.Printf("Held Response Statistics:\n")
		fmt.Printf("=========================\n\n")

		fmt.Printf("Total Responses: %d\n\n", totalResponses)

		if totalResponses > 0 {
			fmt.Printf("Status Breakdown:\n")
			fmt.Printf("-----------------\n")
			fmt.Printf("Pending:   %d (%.1f%%)\n", len(pendingResponses), float64(len(pendingResponses))*100/float64(totalResponses))
			fmt.Printf("Approved:  %d (%.1f%%)\n", len(approvedResponses), float64(len(approvedResponses))*100/float64(totalResponses))
			fmt.Printf("Revision:  %d (%.1f%%)\n", len(revisionResponses), float64(len(revisionResponses))*100/float64(totalResponses))
			fmt.Printf("Expired:   %d (%.1f%%)\n", len(expiredResponses), float64(len(expiredResponses))*100/float64(totalResponses))
		} else {
			fmt.Printf("No held responses found.\n")
		}

		return nil
	},
}

// markExpiredCmd marks expired responses as expired
var markExpiredCmd = &cobra.Command{
	Use:   "mark-expired",
	Short: "Mark expired held responses as expired",
	Long:  `Mark all held responses that have passed their expiration time as expired.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Initialize services
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Firestore: %w", err)
		}
		defer fs.Close()

		heldResponseService := internal.NewHeldResponseService(fs, nil, nil, nil)

		// Mark expired responses
		count, err := heldResponseService.MarkExpired(ctx)
		if err != nil {
			return fmt.Errorf("failed to mark expired responses: %w", err)
		}

		fmt.Printf("Successfully marked %d response(s) as expired\n", count)
		return nil
	},
}

// getAllHeldResponses gets all held responses regardless of status
func getAllHeldResponses(ctx context.Context, service *internal.HeldResponseService) ([]*internal.HeldResponse, error) {
	var allResponses []*internal.HeldResponse

	// Get responses for each status
	statuses := []internal.HeldResponseStatus{
		internal.HeldResponseStatusPending,
		internal.HeldResponseStatusApproved,
		internal.HeldResponseStatusRevision,
		internal.HeldResponseStatusExpired,
	}

	for _, status := range statuses {
		responses, err := service.ListHeldResponses(ctx, status, 1000) // Large limit to get all
		if err != nil {
			return nil, err
		}
		allResponses = append(allResponses, responses...)
	}

	return allResponses, nil
}

func init() {
	RootCmd.AddCommand(heldResponsesCmd)

	// Add subcommands
	heldResponsesCmd.AddCommand(listHeldResponsesCmd)
	heldResponsesCmd.AddCommand(approveHeldResponseCmd)
	heldResponsesCmd.AddCommand(requestRevisionCmd)
	heldResponsesCmd.AddCommand(detailHeldResponseCmd)
	heldResponsesCmd.AddCommand(statsHeldResponseCmd)
	heldResponsesCmd.AddCommand(markExpiredCmd)

	// Add flags for admin user
	approveHeldResponseCmd.Flags().String("admin-user", "", "Admin user ID (defaults to 'cli-admin')")
	requestRevisionCmd.Flags().String("admin-user", "", "Admin user ID (defaults to 'cli-admin')")
}
