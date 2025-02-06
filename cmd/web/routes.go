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
			mux.HandleFunc("/auth/:provider", app.socialAuth, "GET")
			mux.HandleFunc("/auth/:provider/callback", app.socialCallback, "GET")
		})

		mux.Group(func(mux *flow.Mux) {
			mux.Use(app.requireAuthenticatedUser)
			mux.HandleFunc("/profile", app.profile, "GET", "POST")
			mux.HandleFunc("/profile/resume", app.resetResume, "DELETE")
			mux.HandleFunc("/profile/resume/upload", app.resumeUploadPage, "GET", "POST")
			mux.HandleFunc("/profile/schedule", app.scheduleHandler, "GET")
			mux.HandleFunc("/profile/schedule/preferences", app.schedulePreferencesSubmitHandler, "POST")
			mux.HandleFunc("/profile/schedule/appointment", app.scheduleAppointmentHandler, "POST")
			mux.HandleFunc("/reschedule-call", app.rescheduleCall, "POST")
			mux.HandleFunc("/interview", app.InterviewPage, "GET")
			mux.HandleFunc("/interview/start", app.InterviewStart, "GET")
			mux.HandleFunc("/interview/end", app.InterviewEnd, "POST")
			mux.HandleFunc("/interview/audio", app.HandleAudio, "POST")
			mux.HandleFunc("/logout", app.logout, "POST")
		})

		mux.Group(func(mux *flow.Mux) {
			mux.Use(app.requireSuperUser)
			// Job search endpoints
			mux.HandleFunc("/api/v1/jobs/search", app.jobHandler.HandleSearchJobs, "POST")
			mux.HandleFunc("/api/v1/jobs/:id", app.jobHandler.HandleGetJobDetails, "GET")
			mux.HandleFunc("/api/v1/jobs/:id/analyze", app.jobHandler.HandleAnalyzeJob, "POST")

			// Legacy job endpoints
			mux.HandleFunc("/job/search", app.linkedInSearch, "GET")
			mux.HandleFunc("/job/:id", app.jobDetail, "GET")
			mux.HandleFunc("/job/:id/match", app.handleJobMatch, "GET")

			mux.HandleFunc("/user/:id/keywords", app.editKeywords, "GET")
			mux.HandleFunc("/user/:id/keywords", app.saveKeywords, "POST")
		})
	})

	return mux
}
