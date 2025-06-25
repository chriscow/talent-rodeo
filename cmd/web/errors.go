package main

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"

	"github.com/getsentry/sentry-go"
)

func (app *application) reportServerError(r *http.Request, err error) {
	var (
		message = err.Error()
		method  = r.Method
		url     = r.URL.String()
		trace   = string(debug.Stack())
	)

	internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
		"action": "reportServerError: server error occurred",
		"method": method,
		"url":    url,
	})

	requestAttrs := slog.Group("request", "method", method, "url", url)
	app.logger.Error(message, requestAttrs)

	// Log each line of the stack trace separately
	for _, line := range strings.Split(trace, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			app.logger.Error("stack trace", "line", line)
		}
	}

	if app.config.notifications.email != "" {
		data := app.newEmailData()
		data["Message"] = message
		data["RequestMethod"] = method
		data["RequestURL"] = url
		data["Trace"] = trace

		err := app.mailer.SendFromTemplate(app.config.notifications.email, data, "error-notification.html")
		if err != nil {
			trace = string(debug.Stack())
			app.logger.Error("failed to send error notification email", "error", err)
		}
	}
}

func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	// Log the error
	app.reportServerError(r, err)

	// For HTMX requests, trigger a full page refresh
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Refresh", "true")
		http.Error(w, "An unexpected error occurred.", http.StatusInternalServerError)
		return
	}

	// For regular requests, render the error page
	if renderErr := response.Page(w, http.StatusOK, nil, "pages/errors/500.html"); renderErr != nil {
		// Fallback to plain text response if rendering fails
		http.Error(w, "An unexpected error occurred while handling another error.", http.StatusInternalServerError)
	}
}

func (app *application) notFound(w http.ResponseWriter, r *http.Request) {
	if err := response.Page(w, http.StatusOK, nil, "pages/errors/404.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) badRequest(w http.ResponseWriter, r *http.Request, err error) {
	data := app.newTemplateData(r)
	data["ErrorMessage"] = err.Error()

	err = response.Page(w, http.StatusBadRequest, data, "pages/errors/400.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) clientError(w http.ResponseWriter, r *http.Request, status int) {
	data := app.newTemplateData(r)
	data["ErrorMessage"] = http.StatusText(status)

	err := response.Page(w, status, data, "pages/errors/400.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}
