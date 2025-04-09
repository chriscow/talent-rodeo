package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"talent-rodeo/internal"

	"github.com/joho/godotenv"
)

func init() {
	// Load environment variables from .env file
	if err := godotenv.Overload(); err != nil {
		// Try parent directory if not found
		if err := godotenv.Load("../../.env"); err != nil {
			log.Println("No .env file found, using system env")
		}
	}

	// Fix newlines in FIREBASE_CREDENTIALS if present
	if creds := os.Getenv("FIREBASE_CREDENTIALS"); strings.Contains(creds, "\n") {
		fmt.Println("FIREBASE_CREDENTIALS contains newlines")
		// Remove any literal newlines that might be present in the environment variable
		creds = strings.ReplaceAll(creds, "\n", "")
		os.Setenv("FIREBASE_CREDENTIALS", creds)
	}
}

func main() {
	// Command line flags
	userID := flag.String("user", "", "User ID to check (required for job operations, get from --list-users)")
	listUsers := flag.Bool("list-users", false, "List all users with their IDs, names and phone numbers")
	listJobs := flag.Bool("list", false, "List jobs for user")
	updateStatus := flag.String("status", "", "Update job status (viewed, saved, applied, rejected)")
	jobID := flag.String("job", "", "Job ID for updates")
	updateNotes := flag.String("notes", "", "Update notes for job")
	pretty := flag.Bool("pretty", false, "Pretty print JSON output")
	createTest := flag.Bool("create-test", false, "Create test user and job data")
	checkSearch := flag.Bool("check-search", false, "Check user's search status")
	flag.Parse()

	// Set up logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create context
	ctx := context.Background()

	// Create Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		logger.Error("Failed to create Firestore client", "error", err)
		os.Exit(1)
	}
	defer fs.Close()

	// List users
	if *listUsers {
		users, err := internal.ListUsers(ctx, fs, 0, 0)
		if err != nil {
			logger.Error("Failed to list users", "error", err)
			os.Exit(1)
		}

		type UserInfo struct {
			ID    string  `json:"id"`
			Name  string  `json:"name"`
			Phone *string `json:"phone,omitempty"`
		}

		userInfos := make([]UserInfo, len(users))
		for i, user := range users {
			userInfos[i] = UserInfo{
				ID:    user.ID,
				Name:  user.Name,
				Phone: user.Phone,
			}
		}

		var output []byte
		if *pretty {
			output, err = json.MarshalIndent(userInfos, "", "  ")
		} else {
			output, err = json.Marshal(userInfos)
		}

		if err != nil {
			logger.Error("Failed to marshal user info", "error", err)
			os.Exit(1)
		}

		fmt.Println(string(output))
		return
	}

	// Create test data
	if *createTest {
		testUserID, testJobID, err := CreateTestUserAndJob(ctx)
		if err != nil {
			logger.Error("Failed to create test data", "error", err)
			os.Exit(1)
		}

		fmt.Printf("Created test user: %s\n", testUserID)
		fmt.Printf("Created test job: %s\n", testJobID)
		fmt.Println("\nYou can now run the following command to view the job:")
		fmt.Printf("  go run ./cmd/userjobs --user %s --list --pretty\n", testUserID)
		return
	}

	// Check if user ID is required for the operation
	if (*listJobs || *checkSearch || *updateStatus != "" || *updateNotes != "") && *userID == "" {
		fmt.Println("User ID is required for this operation. Use --list-users to see available users, or --create-test to create test data")
		flag.Usage()
		os.Exit(1)
	}

	// Check user's search status
	if *checkSearch {
		user, found, err := internal.GetUser(ctx, fs, *userID)
		if err != nil {
			logger.Error("Failed to get user", "error", err)
			os.Exit(1)
		}

		if !found {
			fmt.Printf("User %s not found\n", *userID)
			return
		}

		fmt.Printf("User ID: %s\n", *userID)

		// Output search info as JSON
		searchData := map[string]interface{}{}

		if user.Search != nil {
			if user.Search.QueuedAt != nil {
				searchData["queued_at"] = user.Search.QueuedAt.Format(time.RFC3339)
			}

			if user.Search.CompletedAt != nil {
				searchData["completed_at"] = user.Search.CompletedAt.Format(time.RFC3339)
			}

			if user.Search.Error != "" {
				searchData["error"] = user.Search.Error
			}

			if len(user.Search.Criteria.Keywords) > 0 {
				searchData["keywords"] = user.Search.Criteria.Keywords
			}

			if user.Search.Criteria.Location != "" {
				searchData["location"] = user.Search.Criteria.Location
			}
		}

		// Also check if there are jobs in the user_jobs subcollection
		jobs, err := internal.GetUserJobs(ctx, fs, *userID)
		if err != nil {
			logger.Error("Failed to get user jobs", "error", err)
		} else {
			searchData["job_count"] = len(jobs)
		}

		output, err := json.MarshalIndent(searchData, "", "  ")
		if err != nil {
			logger.Error("Failed to marshal search data", "error", err)
			os.Exit(1)
		}

		fmt.Println("Search status:")
		fmt.Println(string(output))
		return
	}

	// List jobs
	if *listJobs {
		jobs, err := internal.GetUserJobs(ctx, fs, *userID)
		if err != nil {
			logger.Error("Failed to get user jobs", "error", err)
			os.Exit(1)
		}

		if len(jobs) == 0 {
			fmt.Printf("User %s has no jobs\n", *userID)
			return
		}

		fmt.Printf("Found %d jobs for user %s\n", len(jobs), *userID)

		// Convert to a serializable format
		type SerializableJob struct {
			ID         string             `json:"id"`
			Status     internal.JobStatus `json:"status"`
			ViewedAt   *time.Time         `json:"viewed_at,omitempty"`
			SavedAt    *time.Time         `json:"saved_at,omitempty"`
			AppliedAt  *time.Time         `json:"applied_at,omitempty"`
			RejectedAt *time.Time         `json:"rejected_at,omitempty"`
			Notes      string             `json:"notes,omitempty"`
			SearchHash string             `json:"search_hash"`
			CreatedAt  time.Time          `json:"created_at"`
			UpdatedAt  time.Time          `json:"updated_at"`
			JobRef     string             `json:"job_ref"`
		}

		serJobs := make([]SerializableJob, len(jobs))
		for i, job := range jobs {
			serJobs[i] = SerializableJob{
				ID:         job.ID,
				Status:     job.Status,
				ViewedAt:   job.ViewedAt,
				SavedAt:    job.SavedAt,
				AppliedAt:  job.AppliedAt,
				RejectedAt: job.RejectedAt,
				Notes:      job.Notes,
				SearchHash: job.SearchHash,
				CreatedAt:  job.CreatedAt,
				UpdatedAt:  job.UpdatedAt,
				JobRef:     job.JobRef.Path,
			}
		}

		// Output as JSON
		var output []byte
		if *pretty {
			output, err = json.MarshalIndent(serJobs, "", "  ")
		} else {
			output, err = json.Marshal(serJobs)
		}

		if err != nil {
			logger.Error("Failed to marshal jobs", "error", err)
			os.Exit(1)
		}

		fmt.Println(string(output))
		return
	}

	// Update job status
	if *updateStatus != "" {
		if *jobID == "" {
			fmt.Println("Job ID is required when updating status")
			flag.Usage()
			os.Exit(1)
		}

		err := internal.UpdateUserJobStatus(ctx, fs, *userID, *jobID, *updateStatus)
		if err != nil {
			logger.Error("Failed to update job status", "error", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully updated job %s status to %s for user %s\n", *jobID, *updateStatus, *userID)
		return
	}

	// Update job notes
	if *updateNotes != "" {
		if *jobID == "" {
			fmt.Println("Job ID is required when updating notes")
			flag.Usage()
			os.Exit(1)
		}

		err := internal.UpdateUserJobNotes(ctx, fs, *userID, *jobID, *updateNotes)
		if err != nil {
			logger.Error("Failed to update job notes", "error", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully updated notes for job %s for user %s\n", *jobID, *userID)
		return
	}

	// If we got here, no action was specified
	fmt.Println("No action specified. Use --list to view jobs, --check-search to check search status, --status to update status, or --notes to update notes.")
	flag.Usage()
}
