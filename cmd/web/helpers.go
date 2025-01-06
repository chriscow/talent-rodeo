package main

import (
	"fmt"
	"net/http"

	"talent-rodeo/internal/version"

	"github.com/justinas/nosurf"
)

func (app *application) newTemplateData(r *http.Request) map[string]any {
	user := contextGetAuthenticatedUser(r)

	var sessionID string
	session, err := app.sessionStore.Get(r, "session")
	if err == nil {
		sessionID, _ = session.Values["sessionID"].(string)
	}

	token := nosurf.Token(r)

	data := map[string]any{
		"Version":   version.Get(),
		"User":      user,
		"SessionID": sessionID,
		"CSRFToken": token,
	}

	return data
}

func (app *application) newEmailData() map[string]any {
	data := map[string]any{
		"BaseURL": app.config.baseURL,
	}

	return data
}

func (app *application) backgroundTask(r *http.Request, fn func() error) {
	app.wg.Add(1)

	go func() {
		defer app.wg.Done()

		defer func() {
			err := recover()
			if err != nil {
				app.reportServerError(r, fmt.Errorf("%s", err))
			}
		}()

		err := fn()
		if err != nil {
			app.reportServerError(r, err)
		}
	}()
}
