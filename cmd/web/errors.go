package main

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"talent-rodeo/internal/response"
)

func (app *application) reportServerError(r *http.Request, err error) {
	var (
		message = err.Error()
		method  = r.Method
		url     = r.URL.String()
		trace   = string(debug.Stack())
	)

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

		err := app.mailer.Send(app.config.notifications.email, data, "error-notification.html")
		if err != nil {
			trace = string(debug.Stack())
			app.logger.Error(err.Error(), requestAttrs, "trace", trace)
		}
	}
}

func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	// Log the error
	app.reportServerError(r, err)

	// Prevent infinite recursion by handling errors in rendering directly
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
