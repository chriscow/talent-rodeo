package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/internal/response"
)

func (app *application) InterviewPage(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	if err := response.Page(w, http.StatusOK, data, "pages/interview.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render interview page: %w", err))
	}
}

func (app *application) HandleAudio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement audio handling
	// This will be implemented when we add the audio interview feature
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}
