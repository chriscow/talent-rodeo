package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"talent-rodeo/internal"

	"github.com/spf13/cobra"
)

func init() {
	// Add webhooks command
	RootCmd.AddCommand(webhooksCmd)

	// Add subcommands
	webhooksCmd.AddCommand(listWebhooksCmd)
	webhooksCmd.AddCommand(deleteWebhooksCmd)

	// Add flags
	listWebhooksCmd.Flags().Bool("id-only", false, "Only output document IDs (one per line)")

	// Add flags for delete command
	deleteWebhooksCmd.Flags().StringP("id", "i", "", "Webhook ID to delete")
	deleteWebhooksCmd.Flags().StringP("prefix", "p", "", "Delete all webhooks with this prefix")
	deleteWebhooksCmd.Flags().Bool("all", false, "Delete all webhooks in the collection (requires confirmation)")
	deleteWebhooksCmd.Flags().Bool("force", false, "Skip confirmation prompt when using --all")
}

var webhooksCmd = &cobra.Command{
	Use:   "webhooks",
	Short: "Manage webhook data and webhook history",
	Long: `Manage webhook data received from external services like VAPI.

Webhooks are HTTP callbacks received from external services when events occur.
The system stores webhook data for debugging, processing, and audit purposes.
This includes call status updates, payment notifications, and other event data.

This command group allows you to:
- List webhook data for debugging and analysis
- Delete webhook records for cleanup and maintenance`,
}

var deleteWebhooksCmd = &cobra.Command{
	Use:   "delete [--id <webhook-id> | --prefix <prefix> | --all]",
	Short: "Delete webhook records for cleanup and maintenance",
	Long: `Delete webhook records from the webhook storage collection.

Webhooks accumulate over time and may need cleanup for maintenance or
to remove test data. Use with caution as deleted webhook data cannot
be recovered.

Deletion options (choose one):
  --id, -i: Delete specific webhook by ID
  --prefix, -p: Delete all webhooks whose IDs start with prefix (BULK)
  --all: Delete ALL webhooks in the collection (DANGEROUS)

Safety options:
  --force: Skip confirmation prompt when using --all

Examples:
  # Delete specific webhook
  trcli webhooks delete --id webhook123
  
  # Delete test webhooks
  trcli webhooks delete --prefix "test-"
  
  # Delete all webhooks (with confirmation)
  trcli webhooks delete --all
  
  # Delete all webhooks (skip confirmation)
  trcli webhooks delete --all --force`,
	RunE: func(cmd *cobra.Command, args []string) error {
		webhookID, _ := cmd.Flags().GetString("id")
		prefix, _ := cmd.Flags().GetString("prefix")
		all, _ := cmd.Flags().GetBool("all")
		force, _ := cmd.Flags().GetBool("force")

		if !all && webhookID == "" && prefix == "" && len(args) == 0 {
			return fmt.Errorf("must specify either --id, --prefix, --all, or provide IDs as arguments")
		}

		if all && (webhookID != "" || prefix != "" || len(args) > 0) {
			return fmt.Errorf("--all cannot be used with other deletion options")
		}

		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		// Handle --all flag
		if all {
			if !force {
				fmt.Fprintf(os.Stderr, "WARNING: This will delete ALL webhooks in the collection.\nAre you sure? [y/N]: ")
				reader := bufio.NewReader(os.Stdin)
				response, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read response: %w", err)
				}
				response = strings.ToLower(strings.TrimSpace(response))
				if response != "y" && response != "yes" {
					fmt.Fprintln(os.Stderr, "Operation cancelled")
					return nil
				}
			}

			deleted := 0
			iter := fs.Collection("webhook-data").Doc("vapi").Collection("webhooks").Documents(ctx)
			defer iter.Stop()

			batch := fs.Batch()
			batchSize := 0
			maxBatchSize := 100 // Reduced from 500 to handle transaction size limits

			for {
				doc, err := iter.Next()
				if err != nil {
					break // Stop at end of iterator
				}

				batch.Delete(doc.Ref)
				batchSize++
				deleted++

				// If batch is full, commit it and start a new one
				if batchSize == maxBatchSize {
					if _, err := batch.Commit(ctx); err != nil {
						return fmt.Errorf("failed to commit batch delete: %w", err)
					}
					fmt.Fprintf(os.Stderr, "Progress: deleted %d webhooks...\n", deleted)
					batch = fs.Batch()
					batchSize = 0
				}
			}

			// Commit any remaining documents
			if batchSize > 0 {
				if _, err := batch.Commit(ctx); err != nil {
					return fmt.Errorf("failed to commit final batch delete: %w", err)
				}
			}

			fmt.Fprintf(os.Stderr, "Successfully deleted %d webhooks\n", deleted)
			return nil
		}

		// If we have a prefix, delete all matching documents
		if prefix != "" {
			deleted := 0
			iter := fs.Collection("webhook-data").Doc("vapi").Collection("webhooks").Documents(ctx)
			defer iter.Stop()

			batch := fs.Batch()
			batchSize := 0
			maxBatchSize := 100 // Reduced from 500 to handle transaction size limits

			for {
				doc, err := iter.Next()
				if err != nil {
					break // Stop at end of iterator
				}

				if strings.HasPrefix(doc.Ref.ID, prefix) {
					batch.Delete(doc.Ref)
					batchSize++
					deleted++

					// If batch is full, commit it and start a new one
					if batchSize == maxBatchSize {
						if _, err := batch.Commit(ctx); err != nil {
							return fmt.Errorf("failed to commit batch delete: %w", err)
						}
						fmt.Fprintf(os.Stderr, "Progress: deleted %d webhooks...\n", deleted)
						batch = fs.Batch()
						batchSize = 0
					}
				}
			}

			// Commit any remaining documents
			if batchSize > 0 {
				if _, err := batch.Commit(ctx); err != nil {
					return fmt.Errorf("failed to commit final batch delete: %w", err)
				}
			}

			fmt.Fprintf(os.Stderr, "Successfully deleted %d webhooks with prefix %s\n", deleted, prefix)
			return nil
		}

		// Handle single ID from flag
		if webhookID != "" {
			args = append(args, webhookID)
		}

		// Delete each provided ID
		for _, id := range args {
			docRef := fs.Collection("webhook-data").Doc("vapi").Collection("webhooks").Doc(id)
			if _, err := docRef.Delete(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to delete webhook %s: %v\n", id, err)
				continue
			}
			fmt.Fprintf(os.Stderr, "Successfully deleted webhook: %s\n", id)
		}

		return nil
	},
}

var listWebhooksCmd = &cobra.Command{
	Use:   "list [--id-only]",
	Short: "List webhook data for debugging and analysis",
	Long: `List webhook data received from external services.

Shows webhook payloads and metadata stored in the system. Useful for
debugging webhook processing, analyzing event patterns, or investigating
issues with external service integrations.

Examples:
  trcli webhooks list                                  # List all webhook data
  trcli webhooks list --json                          # Pretty formatted JSON
  trcli webhooks list --id-only                       # List webhook IDs only`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		// Get all documents from the webhook-data/vapi/webhooks collection
		iter := fs.Collection("webhook-data").Doc("vapi").Collection("webhooks").Documents(ctx)
		defer iter.Stop()

		idOnly, _ := cmd.Flags().GetBool("id-only")

		if idOnly {
			for {
				doc, err := iter.Next()
				if err != nil {
					break // Stop at end of iterator
				}
				fmt.Println(doc.Ref.ID)
			}
			return nil
		}

		// Collect webhook data
		type WebhookData struct {
			ID   string      `json:"id"`
			Data interface{} `json:"data"`
		}

		var webhooks []WebhookData
		for {
			doc, err := iter.Next()
			if err != nil {
				break // Stop at end of iterator
			}

			webhook := WebhookData{
				ID:   doc.Ref.ID,
				Data: doc.Data(),
			}
			webhooks = append(webhooks, webhook)
		}

		if len(webhooks) == 0 {
			fmt.Println("No webhooks found")
			return nil
		}

		// Output as JSON
		var output []byte
		if jsonOutput {
			output, err = json.MarshalIndent(webhooks, "", "  ")
		} else {
			output, err = json.Marshal(webhooks)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal webhook data: %w", err)
		}

		fmt.Printf("Retrieved %d webhooks\n", len(webhooks))
		fmt.Println(string(output))

		return nil
	},
}
