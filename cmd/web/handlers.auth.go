package main

import (
	"net/http"

	"github.com/alexedwards/flow"
	"github.com/markbates/goth/gothic"
)

func (app *application) socialAuth(w http.ResponseWriter, r *http.Request) {

	// goth looks for the provider in the query string, so we need to add it there
	provider := flow.Param(r.Context(), "provider")
	q := r.URL.Query()
	q.Set("provider", provider)
	r.URL.RawQuery = q.Encode()

	gothic.BeginAuthHandler(w, r)
}

func (app *application) socialCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" {
		app.logger.Error("error", "error", r.URL.Query().Get("error"))
		app.logger.Error("error", "error-description", r.URL.Query().Get("error_description"))
		http.Error(w, "You must grant access to continue.", http.StatusUnauthorized)
		return
	}

	provider := flow.Param(r.Context(), "provider")
	q := r.URL.Query()
	q.Set("provider", provider)
	r.URL.RawQuery = q.Encode()

	user, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	dbUser, found, err := app.db.GetUserByEmail(user.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if !found {
		dbUser, err = app.db.NewUserFromGothUser(user)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
	}

	session, err := app.sessionStore.Get(r, "session")
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	session.Values["userID"] = dbUser.ID

	r = contextSetAuthenticatedUser(r, dbUser)

	redirectPath, ok := session.Values["redirectPathAfterLogin"].(string)
	if ok {
		delete(session.Values, "redirectPathAfterLogin")
	} else {
		redirectPath = "/"
	}

	err = session.Save(r, w)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	http.Redirect(w, r, redirectPath, http.StatusSeeOther)
}

func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	gothic.Logout(w, r)

	session, err := app.sessionStore.Get(r, "session")
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	delete(session.Values, "authenticated")
	delete(session.Values, "socialUserID")
	delete(session.Values, "redirectPathAfterLogin")
	session.Save(r, w)

	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusTemporaryRedirect)
}
