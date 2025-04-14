package main

import (
	"context"
	"encoding/json"
	"fmt"
	"talent-rodeo/internal"

	"github.com/spf13/cobra"
)

var (
	prettyOutput bool
)

func init() {
	// Add users command
	rootCmd.AddCommand(usersCmd)

	// Add subcommands
	usersCmd.AddCommand(listUsersCmd)
	usersCmd.AddCommand(deleteUserCmd)
	usersCmd.AddCommand(listGeoIDsCmd)

	// Add flags
	usersCmd.PersistentFlags().BoolVar(&prettyOutput, "pretty", false, "Pretty print JSON output")
	listUsersCmd.Flags().IntP("limit", "l", 100, "Maximum number of users to return per page")
	listUsersCmd.Flags().IntP("offset", "o", 0, "Number of users to skip")
	deleteUserCmd.Flags().StringP("id", "i", "", "Delete user by ID")
	deleteUserCmd.Flags().StringP("email", "e", "", "Delete user by email")
	deleteUserCmd.Flags().StringP("prefix", "p", "", "Delete all users with this prefix")
}

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "Manage users",
	Long:  `Users command allows you to manage users in the system`,
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
			userInfos[i] = UserInfo{
				ID:    user.ID,
				Name:  user.Name,
				Phone: user.Phone,
				Email: user.Email,
			}
		}

		var output []byte
		if prettyOutput {
			output, err = json.MarshalIndent(userInfos, "", "  ")
		} else {
			output, err = json.Marshal(userInfos)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal user info: %w", err)
		}

		fmt.Printf("Retrieved %d users (offset: %d)\n", len(users), offset)
		fmt.Println(string(output))

		// If we got a full page of results, indicate there might be more
		if len(users) == limit {
			fmt.Printf("\nThere might be more users. Use --offset %d to see the next page.\n", offset+limit)
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

		geoIDs, err := internal.ListGeoIDs(fs)
		if err != nil {
			return fmt.Errorf("failed to list GeoIDs: %w", err)
		}

		var output []byte
		if prettyOutput {
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
