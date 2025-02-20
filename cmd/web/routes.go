package main

import (
	"net/http"

	"talent-rodeo/assets"

	"github.com/alexedwards/flow"
)

func (app *application) routes() http.Handler {
	mux := flow.New()
	mux.NotFound = http.HandlerFunc(app.notFound)

	// mux.Use(app.debugHeaders)
	mux.Use(app.recoverPanic)
	mux.Use(app.securityHeaders)
	mux.Use(app.logAccess)

	fileServer := http.FileServer(http.FS(assets.EmbeddedFiles))
	mux.Handle("/static/...", fileServer, "GET")

	mux.HandleFunc("/clear-session", app.clearSession)

	mux.Group(func(mux *flow.Mux) {
		// Only apply CSRF middleware if not in testing mode
		if !app.config.testing {
			mux.Use(app.preventCSRF)
		}
		mux.Use(app.authenticate)

		mux.HandleFunc("/", app.home, "GET", "POST") // we need post because of the logout form

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
			mux.HandleFunc("/profile/resume", app.deleteResume, "DELETE")
			// Direct resume upload route - always shows the resume upload form regardless of onboarding state
			// Used for:
			// - Direct access to resume upload
			// - Uploading a different resume
			// - Resetting onboarding process
			mux.HandleFunc("/profile/resume/upload", app.showUploadPage, "GET")
			mux.HandleFunc("/profile/resume/upload", app.createResume, "POST")
			mux.HandleFunc("/profile/schedule", app.showSchedulePage, "GET")
			mux.HandleFunc("/profile/schedule/preferences", app.updateSchedulePreferences, "POST")
			mux.HandleFunc("/profile/schedule/appointment", app.createAppointment, "POST")
			mux.HandleFunc("/profile/schedule/appointment/:id", app.cancelAppointment, "DELETE")
			mux.HandleFunc("/profile/schedule/reschedule", app.rescheduleAppointment, "POST")
			mux.HandleFunc("/interview", app.showInterviewPage, "GET")
			mux.HandleFunc("/interview/start", app.startInterview, "GET")
			mux.HandleFunc("/interview/end", app.completeInterview, "POST")
			mux.HandleFunc("/interview/audio", app.handleAudioUpload, "POST")
			mux.HandleFunc("/logout", app.logoutUser, "POST")
		})

		mux.Group(func(mux *flow.Mux) {
			mux.Use(app.requireSuperUser)
			mux.HandleFunc("/user/:id/keywords", app.editKeywords, "GET")
			mux.HandleFunc("/user/:id/keywords", app.saveKeywords, "POST")
			// Add tasks routes
			mux.HandleFunc("/admin/tasks", app.showTasksPage, "GET")
			mux.HandleFunc("/admin/tasks/partial", app.getTasksPartial, "GET")
		})
	})

	return mux
}
