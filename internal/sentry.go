package internal

import (
	"context"

	"github.com/getsentry/sentry-go"
)

// setSentryContext sets the Sentry context for the current request. Pass in a blank
// string for userID if you don't want to set a user. The extras map can contain
// any additional context you want to attach to the event. This is useful for
// adding custom fields or metadata that can help with debugging and understanding
// the context of the error.
func SentrySetUserContext(ctx context.Context, userID, ipAddress string, extras map[string]any) {
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		hub.ConfigureScope(func(scope *sentry.Scope) {
			// Set user if provided
			if userID != "" {
				scope.SetUser(sentry.User{
					ID:        userID,
					IPAddress: ipAddress,
				})
			}

			// Set any extra fields
			for k, v := range extras {
				scope.SetExtra(k, v)
			}
		})
	}
}

func SentrySetScope(ctx context.Context, extras map[string]any) {
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		hub.ConfigureScope(func(scope *sentry.Scope) {
			// Set any extra fields
			for k, v := range extras {
				scope.SetExtra(k, v)
			}
		})
	}
}

// SentryReportError captures an error with a specific severity level
func SentryReportError(ctx context.Context, err error, level sentry.Level, tags map[string]string) error {
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
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

func SentryBreadcrumb(ctx context.Context, breadcrumb sentry.Breadcrumb) {
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		hub.AddBreadcrumb(&breadcrumb, nil)
	}
}

func SentryMessage(ctx context.Context, message string, level sentry.Level) {
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(level)
			hub.CaptureMessage(message)
		})
	}
}

func SentryCaptureEvent(ctx context.Context, event *sentry.Event) {
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		hub.CaptureEvent(event)
	}
}
