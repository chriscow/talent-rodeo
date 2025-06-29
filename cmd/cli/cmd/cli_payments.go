package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"talent-rodeo/internal"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/spf13/cobra"
	"google.golang.org/api/iterator"
)

func init() {
	// Add payments command
	RootCmd.AddCommand(paymentsCmd)

	// Add subcommands
	paymentsCmd.AddCommand(listPaymentsCmd)
	paymentsCmd.AddCommand(listProcessedWebhooksCmd)
	paymentsCmd.AddCommand(upgradeUserCmd)
	paymentsCmd.AddCommand(resetUserCmd)

	// Add flags
	listPaymentsCmd.Flags().StringP("user-id", "u", "", "Filter payments by user ID")
	listPaymentsCmd.Flags().IntP("limit", "l", 20, "Limit number of results")

	listProcessedWebhooksCmd.Flags().StringP("user-id", "u", "", "Filter processed webhooks by user ID")
	listProcessedWebhooksCmd.Flags().IntP("limit", "l", 20, "Limit number of results")

	upgradeUserCmd.Flags().StringP("user-id", "u", "", "User ID to upgrade (required)")
	upgradeUserCmd.Flags().StringP("plan-type", "p", "weekly", "Plan type: weekly or monthly")
	upgradeUserCmd.MarkFlagRequired("user-id")

	resetUserCmd.Flags().StringP("user-id", "u", "", "User ID to reset (required)")
	resetUserCmd.MarkFlagRequired("user-id")
}

var paymentsCmd = &cobra.Command{
	Use:   "payments",
	Short: "Manage payment data",
	Long:  `Payments command allows you to manage payment data in the system`,
}

var listPaymentsCmd = &cobra.Command{
	Use:   "list",
	Short: "List payment records",
	Long:  `List payment records from the payments collection`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		userID, _ := cmd.Flags().GetString("user-id")
		limit, _ := cmd.Flags().GetInt("limit")

		query := fs.Collection("payments").OrderBy("created_at", firestore.Desc)
		if userID != "" {
			query = query.Where("user_id", "==", userID)
		}
		if limit > 0 {
			query = query.Limit(limit)
		}

		iter := query.Documents(ctx)
		defer iter.Stop()

		type PaymentRecord struct {
			ID              string    `json:"id"`
			UserID          string    `json:"user_id"`
			StripeSessionID string    `json:"stripe_session_id"`
			StripeEventID   string    `json:"stripe_event_id"`
			PlanType        string    `json:"plan_type"`
			AmountPaid      int64     `json:"amount_paid"`
			Currency        string    `json:"currency"`
			Status          string    `json:"status"`
			CreatedAt       time.Time `json:"created_at"`
		}

		var payments []PaymentRecord
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to iterate payments: %w", err)
			}

			var payment PaymentRecord
			if err := doc.DataTo(&payment); err != nil {
				return fmt.Errorf("failed to unmarshal payment data: %w", err)
			}
			payment.ID = doc.Ref.ID
			payments = append(payments, payment)
		}

		if len(payments) == 0 {
			fmt.Println("No payments found")
			return nil
		}

		// Output as JSON
		jsonOutput, _ := cmd.Root().PersistentFlags().GetBool("json")
		var output []byte
		if jsonOutput {
			output, err = json.MarshalIndent(payments, "", "  ")
		} else {
			output, err = json.Marshal(payments)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal payment data: %w", err)
		}

		fmt.Printf("Found %d payments\n", len(payments))
		fmt.Println(string(output))

		return nil
	},
}

var listProcessedWebhooksCmd = &cobra.Command{
	Use:   "webhooks",
	Short: "List processed webhook events",
	Long:  `List processed webhook events from the processed_webhook_events collection`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		userID, _ := cmd.Flags().GetString("user-id")
		limit, _ := cmd.Flags().GetInt("limit")

		query := fs.Collection("processed_webhook_events").OrderBy("processed_at", firestore.Desc)
		if userID != "" {
			query = query.Where("user_id", "==", userID)
		}
		if limit > 0 {
			query = query.Limit(limit)
		}

		iter := query.Documents(ctx)
		defer iter.Stop()

		type ProcessedWebhookEvent struct {
			ID          string    `json:"id"`
			EventID     string    `json:"event_id"`
			EventType   string    `json:"event_type"`
			UserID      string    `json:"user_id"`
			ProcessedAt time.Time `json:"processed_at"`
		}

		var events []ProcessedWebhookEvent
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to iterate processed webhook events: %w", err)
			}

			var event ProcessedWebhookEvent
			if err := doc.DataTo(&event); err != nil {
				return fmt.Errorf("failed to unmarshal processed webhook event data: %w", err)
			}
			event.ID = doc.Ref.ID
			events = append(events, event)
		}

		if len(events) == 0 {
			fmt.Println("No processed webhook events found")
			return nil
		}

		// Output as JSON
		jsonOutput, _ := cmd.Root().PersistentFlags().GetBool("json")
		var output []byte
		if jsonOutput {
			output, err = json.MarshalIndent(events, "", "  ")
		} else {
			output, err = json.Marshal(events)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal processed webhook event data: %w", err)
		}

		fmt.Printf("Found %d processed webhook events\n", len(events))
		fmt.Println(string(output))

		return nil
	},
}

var upgradeUserCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Manually upgrade a user to Pro tier",
	Long:  `Manually upgrade a user to Pro tier and set their subscription end date`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create Firestore client: %w", err)
		}
		defer fs.Close()

		userID, _ := cmd.Flags().GetString("user-id")
		planType, _ := cmd.Flags().GetString("plan-type")

		// Get the user
		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found: %s", userID)
		}

		// Calculate subscription end date based on plan type
		var duration time.Duration
		switch planType {
		case "weekly":
			duration = internal.StripeWeeklyProDuration
		case "monthly":
			duration = internal.StripeMonthlyProDuration
		default:
			return fmt.Errorf("invalid plan type: %s (must be 'weekly' or 'monthly')", planType)
		}

		// Calculate new subscription end date
		now := time.Now()
		var newSubscriptionEnd time.Time

		if user.SubscriptionEnd != nil && user.SubscriptionEnd.After(now) {
			// Subscription is still active, extend from current end date
			newSubscriptionEnd = user.SubscriptionEnd.Add(duration)
		} else {
			// Subscription is expired or doesn't exist, start from now
			newSubscriptionEnd = now.Add(duration)
		}

		// Update user in a transaction
		err = fs.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			// Update user tier and subscription end
			err := internal.UpdateUserTierTx(ctx, fs, tx, user, internal.UserTierPro)
			if err != nil {
				return fmt.Errorf("failed to update user tier: %w", err)
			}

			err = internal.UpdateUserSubscriptionEndAtTx(ctx, fs, tx, user, newSubscriptionEnd)
			if err != nil {
				return fmt.Errorf("failed to update subscription end: %w", err)
			}

			// Record a manual payment entry
			payment := map[string]interface{}{
				"user_id":           user.ID,
				"stripe_session_id": "manual_upgrade",
				"stripe_event_id":   "manual_upgrade",
				"plan_type":         planType,
				"amount_paid":       0, // Manual upgrade
				"currency":          "usd",
				"subscription_end":  newSubscriptionEnd,
				"created_at":        time.Now(),
				"status":            "manual_upgrade",
			}

			err = tx.Create(fs.Client.Collection("payments").NewDoc(), payment)
			if err != nil {
				return fmt.Errorf("failed to record manual payment: %w", err)
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to upgrade user: %w", err)
		}

		fmt.Printf("Successfully upgraded user %s to Pro tier\n", userID)
		fmt.Printf("Plan type: %s\n", planType)
		fmt.Printf("Subscription end: %s\n", newSubscriptionEnd.Format(time.RFC3339))

		return nil
	},
}

var resetUserCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset a user's tier to free and clear subscription",
	Long:  "Reset a user's tier to 'free' and clear their subscription end date for testing purposes",
	RunE: func(cmd *cobra.Command, args []string) error {
		userID, _ := cmd.Flags().GetString("user-id")

		ctx := context.Background()

		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Firestore: %w", err)
		}
		defer fs.Close()

		// Get the user to verify they exist
		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found: %s", userID)
		}

		// Reset user tier and subscription
		updates := map[string]any{
			"Tier":            internal.UserTierFree,
			"SubscriptionEnd": nil,
		}

		err = internal.UpdateUser(ctx, fs, userID, updates)
		if err != nil {
			return fmt.Errorf("failed to reset user: %w", err)
		}

		fmt.Printf("Successfully reset user %s (%s) to free tier\n", user.Email, userID)
		fmt.Printf("- Tier: %s -> %s\n", user.Tier, internal.UserTierFree)
		fmt.Printf("- SubscriptionEnd: cleared\n")

		return nil
	},
}
