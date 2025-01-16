package main

import (
	"net/http"
	"talent-rodeo/internal/response"
)

func (app *application) profile(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data := app.newTemplateData(r)
		if err := response.Page(w, http.StatusOK, data, "pages/profile.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	if r.Method == http.MethodPost {
		app.handleResume(w, r)
		return
	}
}
