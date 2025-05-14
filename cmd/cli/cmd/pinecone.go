package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"talent-rodeo/internal"
)

var namespace string
var topK int
var denseIndex bool
var sparseIndex bool
var deleteAll bool
var indexHost string

func init() {
	RootCmd.AddCommand(pineconeCmd)

	pineconeCmd.AddCommand(similaritySearchCmd)
	pineconeCmd.AddCommand(deleteAllRecordsCmd)

	similaritySearchCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID to search for")
	similaritySearchCmd.MarkFlagRequired("user")

	similaritySearchCmd.Flags().IntVarP(&topK, "top-k", "k", 2, "Number of results to return")
	similaritySearchCmd.Flags().StringVarP(&indexHost, "index-host", "i", "", "Pinecone index host (overrides environment variables)")
	similaritySearchCmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Pinecone namespace to search")
	similaritySearchCmd.Flags().BoolVarP(&denseIndex, "dense", "d", false, "Use Pinecone dense index")
	similaritySearchCmd.Flags().BoolVarP(&sparseIndex, "sparse", "s", false, "Use Pinecone sparse index")

	deleteAllRecordsCmd.Flags().BoolVar(&denseIndex, "dense", false, "Use Pinecone dense index")
	deleteAllRecordsCmd.Flags().BoolVar(&sparseIndex, "sparse", false, "Use Pinecone sparse index")
	deleteAllRecordsCmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Pinecone namespace to delete")
	deleteAllRecordsCmd.Flags().BoolVarP(&deleteAll, "all", "a", false, "Delete all records from the index")
	deleteAllRecordsCmd.Flags().StringVarP(&indexHost, "index-host", "i", "", "Pinecone index host (overrides environment variables)")
}

var pineconeCmd = &cobra.Command{
	Use:     "pinecone",
	Short:   "Pinecone commands",
	Long:    `Pinecone commands`,
	Aliases: []string{"pc"},
}

var deleteAllRecordsCmd = &cobra.Command{
	Use:     "delete [id1 id2 id3...]",
	Short:   "Delete records from a Pinecone index",
	Long:    `Delete records from a Pinecone index by ID or delete all records`,
	Aliases: []string{"del"},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := internal.NewPineconeClient()
		if err != nil {
			return fmt.Errorf("failed to create pinecone client: %w", err)
		}

		// Validate flags
		if !denseIndex && !sparseIndex && indexHost == "" {
			return fmt.Errorf("one of --dense or --sparse must be provided, or specify an index host with --index-host")
		}

		if !deleteAll && len(args) == 0 {
			return fmt.Errorf("either --all flag or a list of IDs must be provided")
		}

		// Process IDs from arguments (support both space and comma-delimited)
		var ids []string
		if len(args) > 0 {
			for _, arg := range args {
				// Check if the argument contains commas
				if strings.Contains(arg, ",") {
					// Split by comma and add each part
					parts := strings.Split(arg, ",")
					for _, part := range parts {
						trimmed := strings.TrimSpace(part)
						if trimmed != "" {
							ids = append(ids, trimmed)
						}
					}
				} else {
					// Add as is
					ids = append(ids, arg)
				}
			}
		}

		// Get hosts from environment if not directly specified
		var denseHost, sparseHost string

		if denseIndex {
			denseHost = os.Getenv("PINECONE_DENSE_INDEX_HOST")
			if denseHost == "" {
				return fmt.Errorf("PINECONE_DENSE_INDEX_HOST is not set")
			}
		}

		if sparseIndex {
			sparseHost = os.Getenv("PINECONE_SPARSE_INDEX_HOST")
			if sparseHost == "" {
				return fmt.Errorf("PINECONE_SPARSE_INDEX_HOST is not set")
			}
		}

		// If index host is directly specified, use it
		if indexHost != "" {
			// Use the provided host directly
			if deleteAll {
				if err := client.DeleteAllRecords(ctx, indexHost, namespace); err != nil {
					return fmt.Errorf("failed to delete all records from index: %w", err)
				}
				fmt.Println("Successfully deleted all records from index")
			} else {
				if err := client.DeleteRecords(ctx, indexHost, namespace, ids); err != nil {
					return fmt.Errorf("failed to delete records from index: %w", err)
				}
				fmt.Printf("Successfully deleted %d records from index\n", len(ids))
			}
			return nil
		}

		// Handle dense index operations
		if denseIndex {
			if deleteAll {
				if err := client.DeleteAllRecords(ctx, denseHost, namespace); err != nil {
					return fmt.Errorf("failed to delete all records from dense index: %w", err)
				}
				fmt.Println("Successfully deleted all records from dense index")
			} else {
				if err := client.DeleteRecords(ctx, denseHost, namespace, ids); err != nil {
					return fmt.Errorf("failed to delete records from dense index: %w", err)
				}
				fmt.Printf("Successfully deleted %d records from dense index\n", len(ids))
			}
		}

		// Handle sparse index operations
		if sparseIndex {
			if deleteAll {
				if err := client.DeleteAllRecords(ctx, sparseHost, namespace); err != nil {
					return fmt.Errorf("failed to delete all records from sparse index: %w", err)
				}
				fmt.Println("Successfully deleted all records from sparse index")
			} else {
				if err := client.DeleteRecords(ctx, sparseHost, namespace, ids); err != nil {
					return fmt.Errorf("failed to delete records from sparse index: %w", err)
				}
				fmt.Printf("Successfully deleted %d records from sparse index\n", len(ids))
			}
		}

		return nil
	},
}

var similaritySearchCmd = &cobra.Command{
	Use:     "similarity",
	Short:   "Search for jobs using cosine similarity that match the input user job description",
	Long:    `Search for jobs using cosine similarity that match the input user job description`,
	Aliases: []string{"sim"},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create firestore client: %w", err)
		}
		defer fs.Close()

		if userID == "" {
			return fmt.Errorf("user ID is required")
		}

		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found: %s", userID)
		}

		subscriptions, err := internal.GetSubscriptionsForUser(ctx, fs, user.ID)
		if err != nil {
			return fmt.Errorf("failed to get subscriptions: %w", err)
		}

		if len(subscriptions) == 0 {
			return fmt.Errorf("no subscriptions found for user: %s", userID)
		}

		sub := subscriptions[0]

		// Validate flags
		if !denseIndex && !sparseIndex && indexHost == "" {
			return fmt.Errorf("one of --dense or --sparse must be provided, or specify an index host with --index-host")
		}

		if namespace == "" {
			return fmt.Errorf("--namespace flag is required")
		}

		client, err := internal.NewPineconeClient()
		if err != nil {
			return fmt.Errorf("failed to create pinecone client: %w", err)
		}

		// Determine which index host to use
		var host string
		var vector []float32
		if indexHost != "" {
			// Use the explicitly provided host
			host = indexHost
			vector, err = user.SteeringQuery.GetVector(ctx, client, internal.PineconeEmbedDense)
			if err != nil {
				return fmt.Errorf("failed to get vector: %w", err)
			}
		} else if denseIndex {
			// Use dense index from environment
			host = os.Getenv("PINECONE_DENSE_INDEX_HOST")
			if host == "" {
				return fmt.Errorf("PINECONE_DENSE_INDEX_HOST is not set")
			}
			vector, err = user.SteeringQuery.GetVector(ctx, client, internal.PineconeEmbedDense)
			if err != nil {
				return fmt.Errorf("failed to get vector: %w", err)
			}

		} else if sparseIndex {
			// Use sparse index from environment
			host = os.Getenv("PINECONE_SPARSE_INDEX_HOST")
			if host == "" {
				return fmt.Errorf("PINECONE_SPARSE_INDEX_HOST is not set")
			}
			vector, err = user.SteeringQuery.GetVector(ctx, client, internal.PineconeEmbedSparse)
			if err != nil {
				return fmt.Errorf("failed to get vector: %w", err)
			}
		}

		filter := map[string]any{
			"criteriaID": sub.CriteriaID,
		}

		res, err := client.QueryByVector(ctx, host, namespace, vector, internal.WithFilter(filter), internal.WithTopK(topK))
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		jsonBytes, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		fmt.Println(string(jsonBytes))
		return nil
	},
}
