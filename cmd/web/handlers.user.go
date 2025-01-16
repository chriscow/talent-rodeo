package main

import (
	"context"
	"fmt"
	"net/http"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/rodeo"
	"time"
)

func (app *application) profile(w http.ResponseWriter, r *http.Request) {

	data := app.newTemplateData(r)
	sessionID := data["SessionID"].(string)
	ctx := context.Background()
	ctx = context.WithValue(ctx, rodeo.SessionIDKey, sessionID)

	app.backgroundTask(r, func() error {
		sessionID := ctx.Value(rodeo.SessionIDKey).(string)

		timer := time.NewTicker(1 * time.Second)
		count := 0
		for {
			select {
			case <-timer.C:
				count++
				app.SendStatus(sessionID, "status", "error", fmt.Sprintf("tick %d", count))
				timer.Reset(time.Duration(count) * time.Second)

			case <-ctx.Done():
				timer.Stop()
				app.SendStatus(sessionID, "status", "success", "Done")
				return nil
			}
		}
	})

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
