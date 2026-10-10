package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/justinas/nosurf"
)

func (app *application) newTemplateData(r *http.Request) map[string]any {
	data := map[string]any{
		"PersonalMode":  app.mode.Personal,
		"CSRFToken":     nosurf.Token(r),
		"TestingMode":   os.Getenv("TESTING_MODE") == "true",
		"PusherKey":     os.Getenv("PUSHER_KEY"),
		"PusherCluster": os.Getenv("PUSHER_CLUSTER"),
	}

	if r != nil {
		data["User"] = contextGetAuthenticatedUser(r)
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

func (app *application) sendNotification(templateName string, data map[string]any) error {
	if app.mode.Personal || app.config.notifications.email == "" {
		app.logger.Debug("notification email not configured, skipping notification")
		return nil
	}

	emailData := app.newEmailData()
	for key, value := range data {
		emailData[key] = value
	}

	err := app.mailer.SendFromTemplate(app.config.notifications.email, emailData, templateName)
	if err != nil {
		app.logger.Error("failed to send notification email", "template", templateName, "error", err)
		return fmt.Errorf("failed to send notification email: %w", err)
	}

	app.logger.Debug("notification email sent successfully", "template", templateName, "to", app.config.notifications.email)
	return nil
}
