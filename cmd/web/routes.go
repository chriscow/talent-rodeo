package main

import (
	"net/http"
	"os"

	"talent-rodeo/assets"
	"talent-rodeo/internal"

	"github.com/alexedwards/flow"
	sentryhttp "github.com/getsentry/sentry-go/http"
)

func (app *application) routes() http.Handler {
	mux := flow.New()
	mux.NotFound = http.HandlerFunc(app.notFound)

	// Create an instance of sentryhttp
	sentryHandler := sentryhttp.New(sentryhttp.Options{
		Repanic:         true,
		WaitForDelivery: true,
	})

	// mux.Use(app.debugHeaders)
	mux.Use(app.recoverPanic)
	mux.Use(sentryHandler.Handle)

	mux.Use(app.securityHeaders)
	mux.Use(app.logAccess)
	mux.Use(app.sessionManager.LoadAndSave)

	// Health check endpoints (no auth required)
	mux.HandleFunc("/health", app.health, "GET")
	mux.HandleFunc("/health/detailed", app.healthDetailed, "GET")
	mux.HandleFunc("/health/gmail", app.gmailHealth, "GET")

	// VAPI webhook endpoint
	mux.HandleFunc("/vapi/outbound-call", app.makeOutboundCall, "POST")
	mux.HandleFunc("/webhooks/vapi", app.handleVapiWebhook, "POST")

	// Stripe webhook endpoint (no CSRF protection needed)
	mux.HandleFunc("/webhooks/stripe", app.stripeWebhookPost, "POST")

	fileServer := http.FileServer(http.FS(assets.EmbeddedFiles))
	mux.Handle("/static/...", fileServer, "GET")

	mux.HandleFunc("/clear-session", app.clearSession)

	mux.Group(func(mux *flow.Mux) {
		// Only apply CSRF middleware if not in testing mode
		if !app.config.testing {
			mux.Use(app.preventCSRF)
		}
		mux.Use(app.authenticate)
		mux.Use(app.setSentryUser)

		mux.HandleFunc("/", app.home, "GET", "POST") // we need post because of the logout form
		mux.HandleFunc("/privacy", app.showPrivacyPage, "GET")
		mux.HandleFunc("/terms", app.showTermsPage, "GET")
		mux.HandleFunc("/about", app.showAboutPage, "GET")
		mux.Group(func(mux *flow.Mux) {
			mux.Use(app.requireAnonymousUser)
			mux.HandleFunc("/auth/:provider", app.startSocialAuth, "GET")
			mux.HandleFunc("/auth/:provider/callback", app.completeSocialAuth, "GET")
		})

		mux.Group(func(mux *flow.Mux) {
			mux.Use(app.requireAuthenticatedUser)

			// Chat routes
			mux.HandleFunc("/chat", app.showChatPage, "GET")
			mux.HandleFunc("/chat/start", app.startChatService, "POST") // Updated to use service
			// mux.HandleFunc("/chat/message", app.sendChatMessage, "POST") // DEPRECATED: WebSocket handles messages now
			mux.HandleFunc("/chat/ws", app.streamChatResponseService, "GET")         // Updated to use service
			mux.HandleFunc("/chat/end", app.endChatService, "POST")                  // Updated to use service
			mux.HandleFunc("/api/chat/history", app.getChatHistoryService, "GET")    // Updated to use service
			mux.HandleFunc("/chat/fast-forward", app.fastForwardChatService, "POST") // Updated to use service

			// Main onboarding route - displays different templates based on user's onboarding state:
			// - StepInitial: onboarding-resume-upload.html
			// - StepResumeUploaded: onboarding-schedule-phone-call.html
			// - StepPhoneCallScheduled: onboarding-waiting-call.html
			// - StepPhoneCallCompleted: onboarding-preferences.html
			// - StepPreferencesExtracted: onboarding-job-search.html
			// - StepJobSearchStarted/Complete: onboarding-jobs.html
			mux.HandleFunc("/onboarding", app.showOnboardingPage, "GET")
			mux.HandleFunc("/onboarding/reset", app.resetOnboardingState, "POST")
			mux.HandleFunc("/onboarding/resume/:resumeID", app.deleteResume, "DELETE")

			// Direct resume upload route - always shows the resume upload form regardless of onboarding state
			// Used for:
			// - Direct access to resume upload
			// - Uploading a different resume
			// - Resetting onboarding process
			mux.HandleFunc("/onboarding/resume/upload", app.showUploadPage, "GET")
			mux.HandleFunc("/onboarding/resume/upload", app.createResume, "POST")

			mux.HandleFunc("/onboarding/schedule", app.showSchedulePage, "GET")

			mux.HandleFunc("/onboarding/schedule/appointment", app.createAppointment, "POST")
			mux.HandleFunc("/onboarding/schedule/appointment/:id", app.cancelAppointment, "DELETE")
			mux.HandleFunc("/onboarding/schedule/reschedule", app.rescheduleAppointment, "POST")
			mux.HandleFunc("/onboarding/schedule/call-now", app.callNow, "POST")

			// New HTMX onboarding step routes
			mux.HandleFunc("/onboarding/step/current", app.handleOnboardingStepRequest, "GET")
			mux.HandleFunc("/onboarding/step/resume-upload", app.createResume, "POST")
			mux.HandleFunc("/onboarding/step/start-voice-call", app.callNow, "POST")
			mux.HandleFunc("/onboarding/step/start-chat", app.startChatFromOnboarding, "POST")

			// API endpoints
			mux.HandleFunc("/api/user/status", app.getUserStatus, "GET")

			mux.HandleFunc("/account/delete", app.deleteAccount, "POST")

			mux.HandleFunc("/dashboard", app.showDashboard, "GET", "POST")

			// Payment and subscription routes
			mux.HandleFunc("/upgrade", app.upgradeGet, "GET")
			mux.HandleFunc("/billing", app.billingGet, "GET")
			mux.HandleFunc("/payment/create-checkout-session", app.createCheckoutSessionPost, "POST")
			mux.HandleFunc("/payment/success", app.paymentSuccessGet, "GET")
			mux.HandleFunc("/payment/cancel", app.paymentCancelGet, "GET")

			// Feedback routes
			mux.HandleFunc("/feedback/job", app.handleJobFeedback, "POST")
			mux.HandleFunc("/feedback/job/dismiss", app.handleJobDismiss, "POST")
			mux.HandleFunc("/feedback/job/title", app.handleJobTitleAction, "POST")

			mux.HandleFunc("/testing/reset-user-state", app.handleTestingResetUserState, "POST")

			mux.HandleFunc("/logout", app.logoutUser, "POST")

			// Pusher authentication
			mux.HandleFunc("/pusher/auth", app.pusherAuth, "POST")
		})

		mux.Group(func(mux *flow.Mux) {
			mux.Use(app.requireSuperUser)
			// Admin index page
			mux.HandleFunc("/admin", app.adminIndex, "GET")

			// Add tasks routes
			mux.HandleFunc("/admin/search-tasks", app.showSearchTasksPage, "GET", "POST")
			mux.HandleFunc("/admin/search-tasks/partial", app.getSearchTasksPartial, "GET")
			mux.HandleFunc("/admin/search-task-analytics", app.adminSearchTaskAnalytics, "GET")

			mux.HandleFunc("/admin/voice", app.adminVoice, "GET", "POST")

			// Add feature flags admin page
			mux.HandleFunc("/admin/feature-flags", app.adminFeatureFlags, "GET", "POST")

			// Add user dashboard admin page
			mux.HandleFunc("/admin/user-dashboard", app.adminUserDashboard, "GET", "POST")

			// Add subscription management admin page
			mux.HandleFunc("/admin/subscriptions", app.adminSubscriptions, "GET", "POST")

			// Add upload resume for user admin page
			mux.HandleFunc("/admin/upload-resume", app.adminUploadResume, "GET", "POST")

			// Add search criteria management page
			mux.HandleFunc("/admin/search-criteria", app.adminSearchCriteria, "GET", "POST")

			// Add appointment management page
			mux.HandleFunc("/admin/appointments", app.adminAppointments, "GET", "POST")

			// Add users management page
			mux.HandleFunc("/admin/users", app.adminUsers, "GET")

			// Add user detail page
			mux.HandleFunc("/admin/users/:id", app.adminUserDetail, "GET")

			// Add feedback processing dashboard
			mux.HandleFunc("/admin/feedback-dashboard", app.showFeedbackDashboard, "GET")

		})
	})

	// Testing routes - only enabled in test mode
	if os.Getenv("TESTING_MODE") == "true" {
		app.logger.Info(internal.Red("TESTING_MODE is true - setting up testing routes"))
		mux.HandleFunc("/testing/authenticate", app.handleTestingAuthenticate, "POST")
		mux.HandleFunc("/testing/reset", app.handleTestingResetUserState, "POST")
		mux.HandleFunc("/testing/pusher", app.showPusherTest, "GET")
		mux.HandleFunc("/testing/pusher/trigger", app.triggerPusherProgress, "POST")
	}

	return mux
}
