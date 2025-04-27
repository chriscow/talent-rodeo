package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"talent-rodeo/internal"

	"github.com/spf13/cobra"
)

func init() {
	// Add users command
	RootCmd.AddCommand(usersCmd)

	// Add subcommands
	usersCmd.AddCommand(listUsersCmd)
	usersCmd.AddCommand(deleteUserCmd)
	usersCmd.AddCommand(listGeoIDsCmd)
	usersCmd.AddCommand(createUserCmd)

	// Add flags
	listUsersCmd.Flags().IntP("limit", "l", 100, "Maximum number of users to return per page")
	listUsersCmd.Flags().IntP("offset", "o", 0, "Number of users to skip")
	deleteUserCmd.Flags().StringP("id", "i", "", "Delete user by ID")
	deleteUserCmd.Flags().StringP("email", "e", "", "Delete user by email")
	deleteUserCmd.Flags().StringP("prefix", "p", "", "Delete all users with this prefix")

	createUserCmd.Flags().StringP("name", "n", "", "Name of the user")
	createUserCmd.Flags().StringP("email", "e", "", "Email of the user")
	createUserCmd.Flags().StringP("phone", "p", "", "Phone number of the user")
}

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "Manage users",
	Long:  `Users command allows you to manage users in the system`,
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
			output, err := json.MarshalIndent(userInfos, "", "  ")
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
		id, _ := cmd.Flags().GetString("id")
		email, _ := cmd.Flags().GetString("email")
		prefix, _ := cmd.Flags().GetString("prefix")

		if id == "" && email == "" && prefix == "" {
			return fmt.Errorf("must specify either --id, --email, or --prefix")
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
