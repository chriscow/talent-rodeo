package main

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"talent-rodeo/cmd/web/templates/errors"
)

func (app *application) reportServerError(r *http.Request, err error) {
	var (
		message = err.Error()
		method  = r.Method
		url     = r.URL.String()
		trace   = string(debug.Stack())
	)

	requestAttrs := slog.Group("request", "method", method, "url", url)
	app.logger.Error(message, requestAttrs, "trace", trace)

	if app.config.notifications.email != "" {
		data := app.newEmailData()
		data["Message"] = message
		data["RequestMethod"] = method
		data["RequestURL"] = url
		data["Trace"] = trace

		err := app.mailer.Send(app.config.notifications.email, data, "error-notification.tmpl")
		if err != nil {
			trace = string(debug.Stack())
			app.logger.Error(err.Error(), requestAttrs, "trace", trace)
		}
	}
}

func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	app.reportServerError(r, err)
	errors.ServerError().Render(r.Context(), w)
}

func (app *application) notFound(w http.ResponseWriter, r *http.Request) {
	errors.NotFound().Render(r.Context(), w)
}

func (app *application) badRequest(w http.ResponseWriter, r *http.Request, err error) {
	errors.BadRequest(err.Error()).Render(r.Context(), w)
}
