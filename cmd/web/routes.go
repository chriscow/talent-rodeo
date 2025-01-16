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
		mux.Use(app.preventCSRF)
		mux.Use(app.authenticate)

		mux.HandleFunc("/", app.home, "GET", "POST") // we need post because of the logout form

		mux.Group(func(mux *flow.Mux) {
			mux.Use(app.requireAnonymousUser)
			mux.HandleFunc("/auth/:provider", app.socialAuth, "GET")
			mux.HandleFunc("/auth/:provider/callback", app.socialCallback, "GET")
		})

		mux.Group(func(mux *flow.Mux) {
			mux.Use(app.requireAuthenticatedUser)
			mux.HandleFunc("/profile", app.profile, "GET")
			mux.HandleFunc("/resume", app.handleResume, "GET", "POST")
			mux.HandleFunc("/logout", app.logout, "POST")
		})

		mux.Group(func(mux *flow.Mux) {
			mux.Use(app.requireSuperUser)
			mux.HandleFunc("/job/search", app.linkedInSearch, "GET")
			mux.HandleFunc("/job/:id", app.jobDetail, "GET")
			mux.HandleFunc("/job/:id/match", app.handleJobMatch, "GET")

			mux.HandleFunc("/user/:id/keywords", app.editKeywords, "GET")
			mux.HandleFunc("/user/:id/keywords", app.saveKeywords, "POST")

			mux.HandleFunc("/chat", app.handleChat, "GET", "POST")
			mux.HandleFunc("/messages", app.handleMessages, "GET")
		})
	})

	return mux
}
