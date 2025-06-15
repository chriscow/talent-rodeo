package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"talent-rodeo/internal"

	"github.com/cheggaaa/pb"
	"github.com/spf13/cobra"
)

var criteriaID string
var unsubscribe bool

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
			Name:  name,
			Email: email,
			Phone: &phone,
			State: internal.UserState{
				Status: internal.UserStatusCallCompleted,
			},
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
