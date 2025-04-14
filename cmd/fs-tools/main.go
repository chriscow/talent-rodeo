package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

func init() {
	// Load environment variables from .env file
	if err := godotenv.Overload(); err != nil {
		// Try parent directory if not found
		fmt.Println("Trying parent directory")
		if err := godotenv.Overload("../../.env"); err != nil {
			log.Println("No .env file found, using system env")
		}
	}

	fmt.Println("Using project ID:", os.Getenv("FIREBASE_PROJECT_ID"))
}

var rootCmd = &cobra.Command{
	Use:   "userjobs",
	Short: "A CLI tool to manage users and jobs in Firestore",
	Long: `userjobs is a command line tool for managing users and their jobs in Firestore.
It provides functionality to view, create, update and delete users and their associated jobs.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
