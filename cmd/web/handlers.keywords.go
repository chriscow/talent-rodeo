package main

import (
	"encoding/json"
	"net/http"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/response"
)

type KeywordList struct {
	Keywords []string `json:"keywords"`
}

var keywords = []string{}

func (app *application) keywords(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		app.editKeywords(w, r)
		return
	}

	if r.Method == http.MethodPost {
		app.saveKeywords(w, r)
		return
	}

	app.badRequest(w, r, nil)
}

func (app *application) editKeywords(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)

	app.keywordsMutex.RLock()
	keywords, err := app.db.GetUserKeywords(user.ID)
	app.keywordsMutex.RUnlock()

	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data["Keywords"] = keywords

	if err := response.Page(w, http.StatusOK, data, "pages/resume-keywords.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) saveKeywords(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Keywords []database.UserKeyword `json:"keywords"`
	}

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	app.keywordsMutex.Lock()
	// HTMX posts the item IDs in the order they appear in the DOM
	// err = app.db.SaveUserKeywords(user.ID, request.Keywords)
	app.keywordsMutex.Unlock()

	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}
