package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"talent-rodeo/internal"

	"github.com/spf13/cobra"
)

func init() {
	geoidsCmd := &cobra.Command{
		Use:   "geoids",
		Short: "Manage geoids",
		Long:  `Manage geoids in the system`,
	}

	listGeoidsCmd := &cobra.Command{
		Use:   "list",
		Short: "List geoids",
		Long:  `List all geoids in the system`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fs, err := internal.NewFirestoreClient(context.Background())
			if err != nil {
				return fmt.Errorf("failed to create Firestore client: %w", err)
			}
			defer fs.Close()

			geoids, err := internal.ListGeoIDs(context.Background(), fs)
			if err != nil {
				return fmt.Errorf("failed to list geoids: %w", err)
			}

			if len(geoids) == 0 {
				fmt.Println("No geoids found")
				return nil
			}

			if jsonOutput {
				output, err := json.MarshalIndent(geoids, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal geoids: %w", err)
				}
				fmt.Println(string(output))
			} else {
				for _, geoid := range geoids {
					fmt.Printf("ID: %s, Value: %s\n", geoid.ID, geoid.Value)
				}
			}

			return nil
		},
	}

	geoidsCmd.AddCommand(listGeoidsCmd)
	RootCmd.AddCommand(geoidsCmd)
}
