package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
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

	importGeoidsCmd := &cobra.Command{
		Use:   "import",
		Short: "Import geoids from JSON",
		Long:  `Import geoids from JSON data. Can read from stdin or a file.
		
Examples:
  # Import from file
  cli geoids import --file geoids.json
  
  # Import from stdin
  cat geoids.json | cli geoids import
  
  # Import using pipe
  cli geoids list | cli geoids import`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			
			fs, err := internal.NewFirestoreClient(ctx)
			if err != nil {
				return fmt.Errorf("failed to create Firestore client: %w", err)
			}
			defer fs.Close()

			// Get input source
			var input io.Reader = os.Stdin
			filename, _ := cmd.Flags().GetString("file")
			
			if filename != "" {
				file, err := os.Open(filename)
				if err != nil {
					return fmt.Errorf("failed to open file %s: %w", filename, err)
				}
				defer file.Close()
				input = file
			}

			// Read and parse JSON
			data, err := io.ReadAll(input)
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}

			var geoids []internal.GeoID
			if err := json.Unmarshal(data, &geoids); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}

			// Import each GeoID
			successCount := 0
			errorCount := 0
			
			for _, geoid := range geoids {
				if err := internal.CreateGeoID(ctx, fs, geoid); err != nil {
					fmt.Fprintf(os.Stderr, "Error importing GeoID %s (%s): %v\n", geoid.ID, geoid.Value, err)
					errorCount++
				} else {
					if !jsonOutput {
						fmt.Printf("Imported GeoID: %s (%s)\n", geoid.ID, geoid.Value)
					}
					successCount++
				}
			}

			if jsonOutput {
				result := map[string]interface{}{
					"total":   len(geoids),
					"success": successCount,
					"errors":  errorCount,
				}
				output, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(output))
			} else {
				fmt.Printf("\nImport completed: %d successful, %d errors out of %d total\n", 
					successCount, errorCount, len(geoids))
			}

			if errorCount > 0 {
				return fmt.Errorf("failed to import %d GeoIDs", errorCount)
			}

			return nil
		},
	}

	// Add flags
	importGeoidsCmd.Flags().StringP("file", "f", "", "JSON file to import from (if not specified, reads from stdin)")

	geoidsCmd.AddCommand(listGeoidsCmd)
	geoidsCmd.AddCommand(importGeoidsCmd)
	RootCmd.AddCommand(geoidsCmd)
}
