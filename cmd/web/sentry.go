package main

import (
	"net/http"

	"github.com/getsentry/sentry-go"
)

// setSentryContext sets the Sentry context for the current request. Pass in a blank
// string for userID if you don't want to set a user. The extras map can contain
// any additional context you want to attach to the event. This is useful for
// adding custom fields or metadata that can help with debugging and understanding
// the context of the error.
func setSentryUserContext(r *http.Request, userID string, extras map[string]any) {
	if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
		hub.ConfigureScope(func(scope *sentry.Scope) {
			// Set user if provided
			if userID != "" {
				scope.SetUser(sentry.User{
					ID:        userID,
					IPAddress: r.Header.Get("X-Forwarded-For"),
				})
			}

			// Set any extra fields
			for k, v := range extras {
				scope.SetExtra(k, v)
			}
		})
	}
}

func setSentryContext(r *http.Request, extras map[string]any) {
	if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
		hub.ConfigureScope(func(scope *sentry.Scope) {
			// Set any extra fields
			for k, v := range extras {
				scope.SetExtra(k, v)
			}
		})
	}
}

// reportToSentry captures an error with a specific severity level
func (app *application) reportToSentry(r *http.Request, err error, level sentry.Level, tags map[string]string) error {
	if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(level)
			for k, v := range tags {
				scope.SetTag(k, v)
			}
			hub.CaptureException(err)
		})
	}
	return err
}
