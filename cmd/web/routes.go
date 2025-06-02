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

	// VAPI webhook endpoint
	mux.HandleFunc("/vapi/outbound-call", app.makeOutboundCall, "POST")
	mux.HandleFunc("/webhooks/vapi", app.handleVapiWebhook, "POST")

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
			// Main profile route - displays different templates based on user's onboarding state:
			// - StepInitial: profile-resume-upload.html
			// - StepResumeUploaded: profile-schedule-phone-call.html
			// - StepPhoneCallScheduled: profile-waiting-call.html
			// - StepPhoneCallCompleted: profile-preferences.html
			// - StepPreferencesExtracted: profile-job-search.html
			// - StepJobSearchStarted/Complete: profile-jobs.html
			mux.HandleFunc("/profile", app.showProfilePage, "GET")
			mux.HandleFunc("/profile/onboarding/reset", app.resetOnboardingState, "POST")
			mux.HandleFunc("/profile/resume/:resumeID", app.deleteResume, "DELETE")

			// Direct resume upload route - always shows the resume upload form regardless of onboarding state
			// Used for:
			// - Direct access to resume upload
			// - Uploading a different resume
			// - Resetting onboarding process
			mux.HandleFunc("/profile/resume/upload", app.showUploadPage, "GET")
			mux.HandleFunc("/profile/resume/upload", app.createResume, "POST")

			mux.HandleFunc("/profile/schedule", app.showSchedulePage, "GET")

			mux.HandleFunc("/profile/schedule/appointment", app.createAppointment, "POST")
			mux.HandleFunc("/profile/schedule/appointment/:id", app.cancelAppointment, "DELETE")
			mux.HandleFunc("/profile/schedule/reschedule", app.rescheduleAppointment, "POST")
			mux.HandleFunc("/profile/schedule/call-now", app.callNow, "POST")

			mux.HandleFunc("/account/delete", app.deleteAccount, "POST")

			mux.HandleFunc("/dashboard", app.showDashboard, "GET")

			// Feedback routes
			mux.HandleFunc("/feedback/job", app.handleJobFeedback, "POST")

			mux.HandleFunc("/testing/reset-user-state", app.handleTestingResetUserState, "POST")

			mux.HandleFunc("/logout", app.logoutUser, "POST")
		})

		mux.Group(func(mux *flow.Mux) {
			mux.Use(app.requireSuperUser)
			// Admin index page
			mux.HandleFunc("/admin", app.adminIndex, "GET")

			// Add tasks routes
			mux.HandleFunc("/admin/tasks", app.showTasksPage, "GET")
			mux.HandleFunc("/admin/tasks/partial", app.getTasksPartial, "GET")

			mux.HandleFunc("/admin/voice", app.adminVoice, "GET", "POST")

			// Add feature flags admin page
			mux.HandleFunc("/admin/feature-flags", app.adminFeatureFlags, "GET", "POST")

			// Add user dashboard admin page
			mux.HandleFunc("/admin/user-dashboard", app.adminUserDashboard, "GET", "POST")
		})
	})

	// Testing routes - only enabled in test mode
	if os.Getenv("TESTING_MODE") == "true" {
		app.logger.Info(internal.Red("TESTING_MODE is true - setting up testing routes"))
		mux.HandleFunc("/testing/authenticate", app.handleTestingAuthenticate, "POST")
		mux.HandleFunc("/testing/reset", app.handleTestingResetUserState, "POST")
	}

	return mux
}
