package cmd

import (
	"context"
	"fmt"
	"talent-rodeo/internal"
	"talent-rodeo/internal/subscription"

	"github.com/spf13/cobra"
)

var matchUserCmd = &cobra.Command{
	Use:   "match",
	Short: "Match users to jobs",
	Long:  `Match users to jobs based on various criteria`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		users := []*internal.User{}

		if allUsers {
			// List all users
			users, err = internal.ListUsers(ctx, fs, 1000, 0)
			if err != nil {
				return fmt.Errorf("failed to list users: %w", err)
			}
			if len(users) == 0 {
				fmt.Println("No users found.")
				return nil
			}
			fmt.Printf("Found %d users.\n", len(users))
		} else if userID != "" {
			// Get user by ID
			user, found, err := internal.GetUser(ctx, fs, userID)
			if err != nil {
				return fmt.Errorf("failed to get user %s: %w", userID, err)
			}
			if !found {
				return fmt.Errorf("user %s not found", userID)
			}
			users = append(users, user)
		} else {
			return fmt.Errorf("either --all or --user must be specified")
		}

		if len(users) == 0 {
			fmt.Println("No users to process.")
			return nil
		}

		for _, user := range users {
			fmt.Printf("Processing user: %s (%s)\n", user.ID, user.Email)

			subs, err := internal.GetSubscriptionsForUser(ctx, fs, user.ID)
			if err != nil {
				return fmt.Errorf("failed to get subscriptions for user %s: %w", user.ID, err)
			}
			if len(subs) == 0 {
				fmt.Println("No subscriptions found for the user.")
				return nil
			}

			// Collect all job IDs from all subscriptions
			allJobIDSet := map[string]struct{}{}
			subscriptionMap := map[string]internal.SearchCriteria{} // Map criteria ID to criteria

			for _, sub := range subs {
				ids, err := internal.GetJobIDsForCriteria(ctx, fs, sub.CriteriaID)
				if err != nil {
					return fmt.Errorf("failed to get job IDs for criteria %s: %w", sub.CriteriaID, err)
				}

				criteria, found, err := internal.GetSearchCriteria(ctx, fs, sub.CriteriaID)
				if err != nil {
					return fmt.Errorf("failed to get criteria %s: %w", sub.CriteriaID, err)
				}

				if !found {
					return fmt.Errorf("criteria %s not found", sub.CriteriaID)
				}

				subscriptionMap[sub.CriteriaID] = *criteria

				for _, id := range ids {
					allJobIDSet[id] = struct{}{}
				}

			}

			// Get existing user jobs to exclude them
			userJobs, err := internal.GetUserJobs(ctx, fs, user.ID)
			if err != nil {
				return fmt.Errorf("failed to get user jobs for user %s: %w", user.ID, err)
			}

			// Create a set of existing user job IDs for fast lookup
			existingJobIDs := map[string]struct{}{}
			for _, job := range userJobs {
				existingJobIDs[job.ID] = struct{}{}
			}

			// Convert set to slice and remove already processed jobs
			newJobIDs := []string{}
			for jobID := range allJobIDSet {
				if _, exists := existingJobIDs[jobID]; !exists {
					newJobIDs = append(newJobIDs, jobID)
				}
			}

			if len(newJobIDs) == 0 {
				fmt.Println("No new jobs to process for this user.")
				return nil
			}

			fmt.Printf("Processing %d new jobs for user %s...\n", len(newJobIDs), user.ID)

			// Refresh jobs for each subscription's criteria
			for criteriaID, criteria := range subscriptionMap {
				fmt.Printf("Refreshing jobs for criteria %s...\n", criteriaID)
				subscription.RefreshUserJobs(ctx, fs, nil, nil, *user, criteria)
			}
		}

		return nil
	},
}
