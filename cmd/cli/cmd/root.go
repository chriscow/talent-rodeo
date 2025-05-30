package cmd

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
	envFile    string
)

var RootCmd = &cobra.Command{
	Use:   "cli",
	Short: "Talent Rodeo CLI tool",
	Long:  `A command line interface for managing Talent Rodeo operations`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if err := godotenv.Overload(envFile); err != nil {
			fmt.Printf("Error loading .env file: %s err: %v\n", envFile, err)
			os.Exit(1)
		}
	},
}

func init() {
	RootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	RootCmd.PersistentFlags().StringVar(&envFile, "env", ".env", "Path to environment file for configuration")
}
