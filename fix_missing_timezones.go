package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"talent-rodeo/internal"
	"time"

	"github.com/joho/godotenv"
)

const (
	DefaultTimezone = "America/Los_Angeles"
	BatchSize       = 100
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Printf(`Fix Missing Timezones Script

This script updates all users with missing timezones to "%s".

Usage:
  go run scripts/fix_missing_timezones.go [--dry-run] [--env ENV_FILE]

Options:
  --dry-run     Show what would be updated without making changes
  --env FILE    Path to environment file (default: .env)

Examples:
  go run scripts/fix_missing_timezones.go --dry-run
  go run scripts/fix_missing_timezones.go --env .prod.env
  go run scripts/fix_missing_timezones.go --dry-run --env .prod.env

`, DefaultTimezone)
		os.Exit(0)
	}

	// Parse command line arguments
	dryRun := false
	envFile := ".env"

	for i, arg := range os.Args[1:] {
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--env":
			dir, err := os.Getwd()
			if err != nil {
				log.Fatal(err)
			}
			println("env file", dir)
			if i+1 < len(os.Args[1:]) {
				envFile = os.Args[i+2]
			} else {
				log.Fatal("--env requires a file path")
			}
		}
	}

	if dryRun {
		fmt.Printf("🔍 DRY RUN MODE: No changes will be made\n\n")
	}

	// Load environment
	if err := godotenv.Overload(envFile); err != nil {
		log.Fatalf("Failed to load environment from %s: %v", envFile, err)
	}

	fmt.Printf("Using environment file: %s\n", envFile)
	fmt.Printf("Default timezone: %s\n\n", DefaultTimezone)

	// Initialize Firestore
	ctx := context.Background()
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer fs.Close()

	// Get all users
	fmt.Println("📋 Fetching all users...")
	users, err := internal.ListUsers(ctx, fs, 0, 0) // Get all users
	if err != nil {
		log.Fatalf("Failed to list users: %v", err)
	}

	fmt.Printf("Total users found: %d\n\n", len(users))

	// Find users with missing timezones
	var usersToUpdate []*internal.User
	var usersWithTimezone []*internal.User

	for _, user := range users {
		if user.Timezone == nil {
			usersToUpdate = append(usersToUpdate, user)
		} else {
			usersWithTimezone = append(usersWithTimezone, user)
		}
	}

	fmt.Printf("📊 Analysis:\n")
	fmt.Printf("  ✅ Users with timezone: %d\n", len(usersWithTimezone))
	fmt.Printf("  ❌ Users missing timezone: %d\n", len(usersToUpdate))
	fmt.Printf("\n")

	if len(usersToUpdate) == 0 {
		fmt.Println("🎉 All users already have timezones set!")
		return
	}

	// Show sample of users that would be updated
	fmt.Printf("🔧 Users that would be updated:\n")
	sampleSize := 10
	if len(usersToUpdate) < sampleSize {
		sampleSize = len(usersToUpdate)
	}

	for i := 0; i < sampleSize; i++ {
		user := usersToUpdate[i]
		fmt.Printf("  • %s (%s) - %s\n", user.Name, user.ID[:8], user.Email)
	}

	if len(usersToUpdate) > sampleSize {
		fmt.Printf("  ... and %d more\n", len(usersToUpdate)-sampleSize)
	}
	fmt.Printf("\n")

	if dryRun {
		fmt.Printf("🔍 DRY RUN: Would update %d users to timezone '%s'\n", len(usersToUpdate), DefaultTimezone)
		return
	}

	// Confirm before proceeding
	fmt.Printf("⚠️  Ready to update %d users to timezone '%s'\n", len(usersToUpdate), DefaultTimezone)
	fmt.Print("Continue? (y/N): ")

	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" && response != "yes" {
		fmt.Println("❌ Cancelled by user")
		return
	}

	// Update users in batches
	fmt.Printf("\n🚀 Starting updates...\n")
	startTime := time.Now()
	successCount := 0
	errorCount := 0

	for i := 0; i < len(usersToUpdate); i += BatchSize {
		end := i + BatchSize
		if end > len(usersToUpdate) {
			end = len(usersToUpdate)
		}

		batch := usersToUpdate[i:end]
		fmt.Printf("Processing batch %d-%d of %d...\n", i+1, end, len(usersToUpdate))

		for _, user := range batch {
			err := internal.UpdateUser(ctx, fs, user.ID, map[string]any{
				"Timezone": DefaultTimezone,
			})

			if err != nil {
				fmt.Printf("  ❌ Failed to update %s (%s): %v\n", user.Name, user.ID[:8], err)
				errorCount++
			} else {
				fmt.Printf("  ✅ Updated %s (%s)\n", user.Name, user.ID[:8])
				successCount++
			}
		}

		// Small delay between batches to avoid rate limiting
		if end < len(usersToUpdate) {
			time.Sleep(100 * time.Millisecond)
		}
	}

	duration := time.Since(startTime)

	fmt.Printf("\n📈 Summary:\n")
	fmt.Printf("  ✅ Successfully updated: %d users\n", successCount)
	fmt.Printf("  ❌ Failed updates: %d users\n", errorCount)
	fmt.Printf("  ⏱️  Total time: %v\n", duration.Round(time.Millisecond))

	if errorCount > 0 {
		fmt.Printf("\n⚠️  Some updates failed. Check the logs above for details.\n")
		os.Exit(1)
	} else {
		fmt.Printf("\n🎉 All users successfully updated to timezone '%s'!\n", DefaultTimezone)
	}
}
