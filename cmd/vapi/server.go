package main

import (
	"fmt"
	"net/http"
	"os"

	"talent-rodeo/internal/vapi"

	"github.com/alexedwards/flow"
)

func (app *application) runServer() {
	// Create webhook handler
	webhookHandler := vapi.NewWebhookHandler(app.logger)

	// Set up router
	mux := flow.New()
	mux.Use(app.logRequest)
	mux.Use(app.recoverPanic)

	// Add webhook routes
	mux.HandleFunc("/server", webhookHandler.HandleWebhook, "POST")

	// Start server
	app.logger.Info("starting server", "port", app.config.port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", app.config.port), mux)
	app.logger.Error("server stopped", "error", err)
	os.Exit(1)
}

// Middleware functions
func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.logger.Info("request received",
			"method", r.Method,
			"url", r.URL.String(),
			"remote_addr", r.RemoteAddr,
		)
		next.ServeHTTP(w, r)
	})
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.Header().Set("Connection", "close")
				app.logger.Error("panic recovered", "error", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
