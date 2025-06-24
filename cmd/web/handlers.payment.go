package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/getsentry/sentry-go"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/webhook"
)

// upgradeGet displays the upgrade page with pricing options
func (app *application) upgradeGet(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	err := response.Page(w, http.StatusOK, data, "pages/upgrade.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}

// billingGet displays the current subscription status and billing information
func (app *application) billingGet(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	err := response.Page(w, http.StatusOK, data, "pages/billing.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}

// createCheckoutSessionPost creates a Stripe Checkout session and redirects user to Stripe
func (app *application) createCheckoutSessionPost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	planType := r.PostForm.Get("plan_type")
	if planType != "weekly" && planType != "monthly" {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.serverError(w, r, fmt.Errorf("user not found in context"))
		return
	}

	// Determine pricing based on plan type
	var priceInCents int64
	var description string
	
	switch planType {
	case "weekly":
		priceInCents = internal.StripeWeeklyProPrice
		description = "Talent Rodeo Pro - Weekly (7 days)"
	case "monthly":
		priceInCents = internal.StripeMonthlyProPrice
		description = "Talent Rodeo Pro - Monthly (30 days)"
	default:
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	// Create Stripe Checkout session
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		app.serverError(w, r, fmt.Errorf("BASE_URL environment variable not set"))
		return
	}

	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: []*string{
			stripe.String("card"),
		},
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String("usd"),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name:        stripe.String(description),
						Description: stripe.String("Upgrade to Pro tier with full job application access"),
					},
					UnitAmount: stripe.Int64(priceInCents),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:       stripe.String("payment"),
		SuccessURL: stripe.String(fmt.Sprintf("%s/payment/success?session_id={CHECKOUT_SESSION_ID}", baseURL)),
		CancelURL:  stripe.String(fmt.Sprintf("%s/payment/cancel", baseURL)),
		CustomerEmail: stripe.String(user.Email),
	}
	
	// Add metadata
	params.AddMetadata("user_id", user.ID)
	params.AddMetadata("plan_type", planType)

	s, err := session.New(params)
	if err != nil {
		app.logger.Error("failed to create Stripe checkout session", "error", err, "user_id", user.ID, "plan_type", planType)
		app.serverError(w, r, fmt.Errorf("failed to create checkout session: %w", err))
		return
	}

	app.logger.Info("created Stripe checkout session", "session_id", s.ID, "user_id", user.ID, "plan_type", planType)

	// Redirect to Stripe Checkout
	http.Redirect(w, r, s.URL, http.StatusSeeOther)
}

// paymentSuccessGet displays the payment success page
func (app *application) paymentSuccessGet(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	// Optional: Verify the session with Stripe for additional security
	// For now, we'll trust the redirect and let the webhook handle the actual upgrade
	app.logger.Info("payment success page accessed", "session_id", sessionID)

	data := app.newTemplateData(r)
	err := response.Page(w, http.StatusOK, data, "pages/payment-success.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}

// paymentCancelGet displays the payment cancellation page
func (app *application) paymentCancelGet(w http.ResponseWriter, r *http.Request) {
	app.logger.Info("payment cancelled by user")
	
	data := app.newTemplateData(r)
	err := response.Page(w, http.StatusOK, data, "pages/payment-cancel.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}

// stripeWebhookPost handles Stripe webhook events for payment confirmation
func (app *application) stripeWebhookPost(w http.ResponseWriter, r *http.Request) {
	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		app.logger.Error("error reading webhook body", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	// Verify webhook signature
	endpointSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if endpointSecret == "" {
		app.logger.Error("STRIPE_WEBHOOK_SECRET not configured")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	event, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), endpointSecret)
	if err != nil {
		app.logger.Error("webhook signature verification failed", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	app.logger.Info("stripe webhook received", "event_type", event.Type, "event_id", event.ID)

	// Handle the event
	switch event.Type {
	case "checkout.session.completed":
		err = app.handleCheckoutSessionCompleted(r.Context(), event)
		if err != nil {
			app.logger.Error("failed to handle checkout.session.completed", "error", err, "event_id", event.ID)
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"error_type": "stripe_webhook_processing",
				"event_type": event.Type,
				"event_id":   event.ID,
			})
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	default:
		app.logger.Info("unhandled webhook event type", "event_type", event.Type)
	}

	w.WriteHeader(http.StatusOK)
}

// handleCheckoutSessionCompleted processes successful payment and upgrades user tier
func (app *application) handleCheckoutSessionCompleted(ctx context.Context, event stripe.Event) error {
	// Parse the checkout session from the event data
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return fmt.Errorf("failed to parse checkout session: %w", err)
	}

	app.logger.Info("processing checkout session completed", 
		"session_id", session.ID,
		"customer_email", session.CustomerEmail,
		"amount_total", session.AmountTotal)

	// Check if we've already processed this event (idempotency)
	processed, err := app.isWebhookEventProcessed(ctx, event.ID)
	if err != nil {
		return fmt.Errorf("failed to check webhook event processing status: %w", err)
	}
	if processed {
		app.logger.Info("webhook event already processed", "event_id", event.ID)
		return nil
	}

	// Extract metadata
	userID, ok := session.Metadata["user_id"]
	if !ok || userID == "" {
		return fmt.Errorf("user_id not found in session metadata")
	}

	planType, ok := session.Metadata["plan_type"]
	if !ok || planType == "" {
		return fmt.Errorf("plan_type not found in session metadata")
	}

	// Get the user
	user, found, err := internal.GetUser(ctx, app.fs, userID)
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
		return fmt.Errorf("invalid plan type: %s", planType)
	}

	// Calculate new subscription end date (stacking subscriptions)
	now := time.Now()
	var newSubscriptionEnd time.Time
	
	if user.SubscriptionEnd != nil && user.SubscriptionEnd.After(now) {
		// Subscription is still active, extend from current end date
		newSubscriptionEnd = user.SubscriptionEnd.Add(duration)
	} else {
		// Subscription is expired or doesn't exist, start from now
		newSubscriptionEnd = now.Add(duration)
	}

	// Update user tier and subscription end date
	err = app.upgradeUserToPro(ctx, user, newSubscriptionEnd, event.ID, session.ID, planType, session.AmountTotal)
	if err != nil {
		return fmt.Errorf("failed to upgrade user to pro: %w", err)
	}

	app.logger.Info("successfully upgraded user to pro", 
		"user_id", user.ID, 
		"plan_type", planType,
		"subscription_end", newSubscriptionEnd.Format(time.RFC3339),
		"amount_paid", session.AmountTotal)

	return nil
}

// upgradeUserToPro upgrades a user to Pro tier and records the payment
func (app *application) upgradeUserToPro(ctx context.Context, user *internal.User, subscriptionEnd time.Time, eventID, sessionID, planType string, amountPaid int64) error {
	// Update user in a transaction
	err := app.fs.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Update user tier and subscription end
		err := internal.UpdateUserTierTx(ctx, app.fs, tx, user, internal.UserTierPro)
		if err != nil {
			return fmt.Errorf("failed to update user tier: %w", err)
		}

		err = internal.UpdateUserSubscriptionEndAtTx(ctx, app.fs, tx, user, subscriptionEnd)
		if err != nil {
			return fmt.Errorf("failed to update subscription end: %w", err)
		}

		// Record the payment
		payment := map[string]interface{}{
			"user_id":          user.ID,
			"stripe_session_id": sessionID,
			"stripe_event_id":   eventID,
			"plan_type":        planType,
			"amount_paid":      amountPaid,
			"currency":         "usd",
			"subscription_end": subscriptionEnd,
			"created_at":       time.Now(),
			"status":           "completed",
		}

		err = tx.Create(app.fs.Client.Collection("payments").NewDoc(), payment)
		if err != nil {
			return fmt.Errorf("failed to record payment: %w", err)
		}

		// Mark webhook event as processed
		processedEvent := map[string]interface{}{
			"event_id":    eventID,
			"event_type":  "checkout.session.completed",
			"processed_at": time.Now(),
			"user_id":     user.ID,
		}

		err = tx.Create(app.fs.Client.Collection("processed_webhook_events").NewDoc(), processedEvent)
		if err != nil {
			return fmt.Errorf("failed to mark webhook event as processed: %w", err)
		}

		return nil
	})

	return err
}

// isWebhookEventProcessed checks if a webhook event has already been processed (idempotency check)
func (app *application) isWebhookEventProcessed(ctx context.Context, eventID string) (bool, error) {
	iter := app.fs.Client.Collection("processed_webhook_events").Where("event_id", "==", eventID).Limit(1).Documents(ctx)
	defer iter.Stop()

	_, err := iter.Next()
	if err != nil {
		if err.Error() == "no more items in iterator" {
			return false, nil // Event not processed yet
		}
		return false, fmt.Errorf("failed to check processed events: %w", err)
	}

	return true, nil // Event already processed
}