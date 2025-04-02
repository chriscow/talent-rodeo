package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/internal/response"
)

func (app *application) showDashboard(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	if err := response.Page(w, http.StatusOK, data, "pages/dashboard.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render dashboard page: %w", err))
	}
}
