package main

import (
	"net/http"
	"talent-rodeo/internal/response"
)

func (app *application) profile(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	if err := response.Page(w, http.StatusOK, data, "pages/user-profile.html"); err != nil {
		app.serverError(w, r, err)
	}
}
