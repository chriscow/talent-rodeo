package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"talent-rodeo/internal"
)

var namespace string
var topK int

func init() {
	similaritySearchCmd.Flags().StringVar(&namespace, "namespace", "", "Pinecone namespace to search")
	RootCmd.AddCommand(similaritySearchCmd)

	similaritySearchCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID to search for")
	similaritySearchCmd.MarkFlagRequired("user")

	similaritySearchCmd.Flags().IntVarP(&topK, "top-k", "k", 2, "Number of results to return")
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

		_, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found: %s", userID)
		}

		appt, found, err := internal.GetLatestCompletedAppointment(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get latest completed appointment: %w", err)
		}
		if !found {
			return fmt.Errorf("no completed appointment found for user: %s", userID)
		}

		jobDesc := appt.ExtractedData.JobDescription
		if jobDesc == nil || *jobDesc == "" {
			return fmt.Errorf("no job description found for user: %s", userID)
		}

		indexHost := os.Getenv("PINECONE_DENSE_INDEX_HOST")
		if indexHost == "" {
			return fmt.Errorf("PINECONE_DENSE_INDEX_HOST is not set")
		}

		if namespace == "" {
			return fmt.Errorf("-namespace flag is required")
		}

		client, err := internal.NewPineconeClient()
		if err != nil {
			return fmt.Errorf("failed to create pinecone client: %w", err)
		}

		res, err := client.SearchRecords(ctx, indexHost, namespace, *jobDesc, internal.WithTopK(topK))
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
