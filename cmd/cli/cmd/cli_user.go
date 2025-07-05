package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"talent-rodeo/internal"
	"time"

	"github.com/cheggaaa/pb"
	"github.com/spf13/cobra"
)

var criteriaID string
var unsubscribe bool
var extendDays int

var userSubscribeCmd = &cobra.Command{
	Use:   "subscribe",
	Short: "Subscribe or unsubscribe a user to a search criteria",
	Long:  `Subscribe a user to a search criteria, or unsubscribe if --unsubscribe is set`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		if userID == "" {
			return fmt.Errorf("--user flag is required")
		}
		if criteriaID == "" {
			return fmt.Errorf("--criteria flag is required")
		}

		if unsubscribe {
			err = internal.UnsubscribeFromCriteria(ctx, fs, userID, criteriaID)
			if err != nil {
				return fmt.Errorf("failed to unsubscribe user from criteria: %w", err)
			}
			fmt.Printf("User %s unsubscribed from criteria %s\n", userID, criteriaID)
		} else {
			err = internal.SubscribeToCriteria(ctx, fs, userID, criteriaID)
			if err != nil {
				return fmt.Errorf("failed to subscribe user to criteria: %w", err)
			}
			fmt.Printf("User %s subscribed to criteria %s\n", userID, criteriaID)
		}
		return nil
	},
}

var userSubscriptionCmd = &cobra.Command{
	Use:   "subscription [user-id]",
	Short: "Manage user subscription settings",
	Long:  `View current subscription status or extend a user's subscription end date by the specified number of days from today`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		userID := args[0]

		// Get the user first
		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found: %s", userID)
		}

		// If --extend flag is not provided or is 0, just display current subscription info
		if extendDays <= 0 {
			fmt.Printf("User: %s (%s)\n", user.Name, userID)
			fmt.Printf("Email: %s\n", user.Email)
			fmt.Printf("Tier: %s\n", user.Tier)
			if user.SubscriptionEnd != nil {
				now := time.Now()
				if user.SubscriptionEnd.After(now) {
					fmt.Printf("Subscription End: %s (ACTIVE - %d days remaining)\n",
						user.SubscriptionEnd.Format("2006-01-02 15:04:05"),
						int(user.SubscriptionEnd.Sub(now).Hours()/24))
				} else {
					fmt.Printf("Subscription End: %s (EXPIRED - %d days ago)\n",
						user.SubscriptionEnd.Format("2006-01-02 15:04:05"),
						int(now.Sub(*user.SubscriptionEnd).Hours()/24))
				}
			} else {
				fmt.Printf("Subscription End: Not set\n")
			}
			return nil
		}

		// Calculate new subscription end date
		var newSubscriptionEnd time.Time
		now := time.Now()

		if user.SubscriptionEnd != nil && user.SubscriptionEnd.After(now) {
			// If subscription is still active, extend from current end date
			newSubscriptionEnd = user.SubscriptionEnd.Add(time.Duration(extendDays) * 24 * time.Hour)
			fmt.Printf("Extended subscription for user %s (%s) from %s to %s (+%d days)\n",
				user.Name, userID,
				user.SubscriptionEnd.Format("2006-01-02 15:04:05"),
				newSubscriptionEnd.Format("2006-01-02 15:04:05"), extendDays)
		} else {
			// If subscription is expired or not set, extend from today
			newSubscriptionEnd = now.Add(time.Duration(extendDays) * 24 * time.Hour)
			fmt.Printf("Extended subscription for user %s (%s) to %s (%d days from today)\n",
				user.Name, userID, newSubscriptionEnd.Format("2006-01-02 15:04:05"), extendDays)
		}

		// Update the user's subscription end date
		err = internal.UpdateUser(ctx, fs, userID, map[string]any{
			"SubscriptionEnd": newSubscriptionEnd,
		})
		if err != nil {
			return fmt.Errorf("failed to update user subscription: %w", err)
		}

		return nil
	},
}

var userTimezoneCmd = &cobra.Command{
	Use:   "timezone [user-id]",
	Short: "Manage user timezone settings",
	Long:  `View current timezone or set a new timezone for a user`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		userID := args[0]

		// Get the user first
		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found: %s", userID)
		}

		setTimezone, _ := cmd.Flags().GetString("set")

		// If --set flag is not provided, just display current timezone
		if setTimezone == "" {
			fmt.Printf("User: %s (%s)\n", user.Name, userID)
			fmt.Printf("Email: %s\n", user.Email)
			if user.Timezone != nil {
				fmt.Printf("Timezone: %s\n", *user.Timezone)
			} else {
				fmt.Printf("Timezone: Not set\n")
			}
			return nil
		}

		// Validate timezone string by trying to load it
		_, err = time.LoadLocation(setTimezone)
		if err != nil {
			return fmt.Errorf("invalid timezone '%s': %w", setTimezone, err)
		}

		// Update the user's timezone
		err = internal.UpdateUser(ctx, fs, userID, map[string]any{
			"Timezone": setTimezone,
		})
		if err != nil {
			return fmt.Errorf("failed to update user timezone: %w", err)
		}

		fmt.Printf("Updated timezone for user %s (%s) to: %s\n", user.Name, userID, setTimezone)
		return nil
	},
}

func init() {
	// Add users command
	RootCmd.AddCommand(usersCmd)

	// Add subcommands
	usersCmd.AddCommand(listUsersCmd)
	usersCmd.AddCommand(deleteUserCmd)
	usersCmd.AddCommand(listGeoIDsCmd)
	usersCmd.AddCommand(createUserCmd)
	usersCmd.AddCommand(importUsersCmd)
	usersCmd.AddCommand(matchUserCmd)
	usersCmd.AddCommand(userSubscribeCmd)
	usersCmd.AddCommand(userSubscriptionCmd)
	usersCmd.AddCommand(userTimezoneCmd)

	// Add flags
	listUsersCmd.Flags().IntP("limit", "l", 100, "Maximum number of users to return per page")
	listUsersCmd.Flags().IntP("offset", "o", 0, "Number of users to skip")

	deleteUserCmd.Flags().StringP("user-id", "u", "", "Delete user by ID")
	deleteUserCmd.Flags().StringP("email", "e", "", "Delete user by email")
	deleteUserCmd.Flags().StringP("prefix", "p", "", "Delete all users with this ID prefix")

	createUserCmd.Flags().StringP("name", "n", "", "Name of the user")
	createUserCmd.Flags().StringP("email", "e", "", "Email of the user")
	createUserCmd.Flags().StringP("phone", "p", "", "Phone number of the user")

	matchUserCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID to match jobs for")
	matchUserCmd.MarkFlagRequired("user")
	matchUserCmd.Flags().IntVarP(&topK, "topk", "t", 3, "Number of top jobs to return")
	matchUserCmd.Flags().BoolVarP(&allUsers, "all", "a", false, "Match jobs for all users instead of a single user")

	// UserJobs update command
	userJobsUpdateCmd.Flags().Bool("all-users", false, "Update jobs for all users")
	userJobsUpdateCmd.Flags().String("user-id", "", "User ID to update jobs for (if not using --all-users)")
	userJobsCmd.AddCommand(userJobsUpdateCmd)
	userJobsCmd.AddCommand(userJobsFollowupCmd)
	RootCmd.AddCommand(userJobsCmd)

	userSubscribeCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID to subscribe/unsubscribe")
	userSubscribeCmd.Flags().StringVarP(&criteriaID, "criteria", "c", "", "Criteria ID to subscribe to")
	userSubscribeCmd.Flags().BoolVar(&unsubscribe, "unsubscribe", false, "Unsubscribe the user from the criteria instead of subscribing")
	userSubscribeCmd.MarkFlagRequired("user")
	userSubscribeCmd.MarkFlagRequired("criteria")

	userSubscriptionCmd.Flags().IntVar(&extendDays, "extend", 0, "Number of days to extend subscription from today (optional - shows current info if not provided)")

	userTimezoneCmd.Flags().String("set", "", "Set a new timezone for the user")
}

var usersCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage a user",
	Long:  `User command allows you to manage a user in the system`,
}

var createUserCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new user",
	Long:  `Create a new user with the given name, email, and phone number`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		name, _ := cmd.Flags().GetString("name")
		email, _ := cmd.Flags().GetString("email")
		phone, _ := cmd.Flags().GetString("phone")

		user := &internal.User{
			Name:   name,
			Email:  email,
			Phone:  &phone,
			Status: internal.UserStatusCallCompleted,
		}

		err = internal.CreateUser(ctx, fs, user)
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}

		fmt.Printf("Successfully created user: %s\n", user.ID)
		return nil
	},
}

var listUsersCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	Long:  `List all users in the system with their IDs, names and phone numbers`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")

		users, err := internal.ListUsers(ctx, fs, limit, offset)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		type UserInfo struct {
			ID    string  `json:"id"`
			Name  string  `json:"name"`
			Phone *string `json:"phone,omitempty"`
			Email string  `json:"email"`
		}

		userInfos := make([]UserInfo, len(users))
		for i, user := range users {
			phone := "[none]      "
			if user.Phone != nil {
				phone = *user.Phone
			}
			userInfos[i] = UserInfo{
				ID:    user.ID,
				Name:  user.Name,
				Phone: &phone,
				Email: user.Email,
			}
		}

		if jsonOutput {
			output, err := json.MarshalIndent(users, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal user info: %w", err)
			}

			fmt.Println(string(output))
		} else {
			for _, u := range userInfos {
				fmt.Printf("%s\t%s\t%s\t%s\n", u.ID, u.Name, *u.Phone, u.Email)
			}
		}

		return nil
	},
}

var deleteUserCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete user(s)",
	Long:  `Delete user(s) by ID, email, or prefix`,
	RunE: func(cmd *cobra.Command, args []string) error {
		id, _ := cmd.Flags().GetString("user-id")
		email, _ := cmd.Flags().GetString("email")
		prefix, _ := cmd.Flags().GetString("prefix")

		if id == "" && email == "" && prefix == "" {
			return fmt.Errorf("must specify either --user-id, --email, or --prefix")
		}

		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		if prefix != "" {
			// Delete all users with prefix
			users, err := internal.ListUsers(ctx, fs, 0, 0)
			if err != nil {
				return fmt.Errorf("failed to list users: %w", err)
			}

			deleted := 0
			for _, user := range users {
				if len(user.ID) >= len(prefix) && user.ID[:len(prefix)] == prefix {
					if err := internal.DeleteUser(ctx, fs, user.ID); err != nil {
						return fmt.Errorf("failed to delete user %s: %w", user.ID, err)
					}
					deleted++
				}
			}
			fmt.Printf("Deleted %d users with prefix: %s\n", deleted, prefix)
			return nil
		}

		if email != "" {
			user, found, err := internal.GetUserByEmail(ctx, fs, email)
			if err != nil {
				return fmt.Errorf("failed to get user by email: %w", err)
			}
			if !found {
				return fmt.Errorf("user with email %s not found", email)
			}
			id = user.ID
		}

		if err := internal.DeleteUser(ctx, fs, id); err != nil {
			return fmt.Errorf("failed to delete user: %w", err)
		}

		fmt.Printf("Successfully deleted user: %s\n", id)
		return nil
	},
}

var listGeoIDsCmd = &cobra.Command{
	Use:   "list-geoids",
	Short: "List all GeoIDs",
	Long:  `List all GeoIDs in the system with their IDs and values`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		geoIDs, err := internal.ListGeoIDs(ctx, fs)
		if err != nil {
			return fmt.Errorf("failed to list GeoIDs: %w", err)
		}

		var output []byte
		if jsonOutput {
			output, err = json.MarshalIndent(geoIDs, "", "  ")
		} else {
			output, err = json.Marshal(geoIDs)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal GeoID info: %w", err)
		}

		fmt.Printf("Retrieved %d GeoIDs\n", len(geoIDs))
		fmt.Println(string(output))

		return nil
	},
}

var importUsersCmd = &cobra.Command{
	Use:   "import [json-file]",
	Short: "Import users from a JSON file",
	Long:  `Import users from a JSON file containing an array of User structs`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		// Read the JSON file
		filePath := args[0]
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		// Parse the JSON into an array of User structs
		var users []internal.User
		if err := json.Unmarshal(data, &users); err != nil {
			return fmt.Errorf("failed to parse JSON from file %s: %w", filePath, err)
		}

		bar := pb.StartNew(len(users))
		defer bar.Finish()

		// Import each user
		successCount := 0
		errorCount := 0
		for i, user := range users {
			if err := internal.CreateUser(ctx, fs, &user); err != nil {
				fmt.Printf("Failed to import user %d (ID: %s, Name: %s): %v\n", i+1, user.ID, user.Name, err)
				errorCount++
			} else {
				successCount++
			}
			bar.Increment()
		}

		fmt.Printf("\nImport completed: %d successful, %d failed\n", successCount, errorCount)

		if errorCount > 0 {
			return fmt.Errorf("import completed with %d errors", errorCount)
		}

		return nil
	},
}
