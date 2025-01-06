package main

import (
	"net/http"

	"talent-rodeo/assets"
	"talent-rodeo/cmd/web/templates/errors"

	"github.com/a-h/templ"
	"github.com/alexedwards/flow"
)

func (app *application) routes() http.Handler {
	mux := flow.New()
	mux.NotFound = templ.Handler(errors.NotFound())

	mux.Use(app.debugHeaders)
	mux.Use(app.recoverPanic)
	mux.Use(app.securityHeaders)

	mux.Group(func(mux *flow.Mux) {
		mux.HandleFunc("/call/status/:session-id", app.callStatus, "GET")
	})

	mux.Use(app.logAccess)

	fileServer := http.FileServer(http.FS(assets.EmbeddedFiles))
	mux.Handle("/static/...", fileServer, "GET")

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
			mux.HandleFunc("/logout", app.logout, "POST")
		})

		mux.Group(func(mux *flow.Mux) {
			mux.Use(app.requireSuperUser)

			mux.HandleFunc("/admin", app.admin, "GET")
			mux.HandleFunc("/chat", app.handleChat, "GET", "POST")
			mux.HandleFunc("/messages", app.handleMessages, "GET")
		})
	})

	return mux
}
