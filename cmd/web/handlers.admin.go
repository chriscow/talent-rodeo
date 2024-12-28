package main

import (
	"net/http"
	"talent-rodeo/cmd/web/templates/pages"

	"github.com/justinas/nosurf"
)

func (app *application) admin(w http.ResponseWriter, r *http.Request) {
	token := nosurf.Token(r)
	authenticatedUser := contextGetAuthenticatedUser(r)
	if authenticatedUser == nil || !authenticatedUser.IsSuperUser {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Fetch the list of all users
	users, err := app.db.GetUsers()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	pages.Admin(token, authenticatedUser, users).Render(r.Context(), w)
}
