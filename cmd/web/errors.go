package main

import (
	"log/slog"
	"net/http"
	"runtime/debug"
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
	if err := response.Page(w, http.StatusOK, nil, "pages/500.tmpl"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) notFound(w http.ResponseWriter, r *http.Request) {
	if err := response.Page(w, http.StatusOK, nil, "pages/404.tmpl"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) badRequest(w http.ResponseWriter, r *http.Request, err error) {
	data := app.newTemplateData(r)
	data["ErrorMessage"] = err.Error()

	err = response.Page(w, http.StatusBadRequest, data, "pages/errors/400.tmpl")
	if err != nil {
		app.serverError(w, r, err)
	}
}
