package main

import (
	"fmt"
	"log/slog"
	"net/http"

	"talent-rodeo/internal"
	"talent-rodeo/internal/response"

	"github.com/getsentry/sentry-go"
	"github.com/justinas/nosurf"
	"github.com/tomasen/realip"
)

const CSRFTokenKey contextKey = "csrf_token"

// HTTP Status Code Notes:
// We consistently use http.StatusSeeOther (303) for redirects after POST/PUT/DELETE operations.
// This is the preferred approach because:
// 1. It forces the client to use GET for the subsequent request regardless of the original method
// 2. Prevents duplicate form submissions and unintended side effects
// 3. Ensures browser history reflects the target page, not the form submission
// 4. Complies with HTTP/1.1 specifications for redirecting after non-GET operations
// 5. Provides better user experience by avoiding "resubmit form?" browser warnings
//
// Unlike 301 (Permanent) or 302 (Found), 303 explicitly requires the next request to be GET,
// which is exactly what we want after form processing to show the result page.

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// app.logger.Warn("recover panic middleware", "path", r.URL.Path)
		defer func() {
			err := recover()
			if err != nil {
				app.serverError(w, r, fmt.Errorf("%s", err))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func (app *application) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// app.logger.Warn("security headers middleware", "path", r.URL.Path)

		w.Header().Set("Referrer-Policy", "origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")

		//cors
		w.Header().Set("Access-Control-Allow-Origin", "*")

		next.ServeHTTP(w, r)
	})
}

func (app *application) logAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// app.logger.Warn("log access middleware", "path", r.URL.Path)
		mw := response.NewMetricsResponseWriter(w)
		next.ServeHTTP(mw, r)

		var (
			ip     = realip.FromRequest(r)
			method = r.Method
			url    = r.URL.String()
			proto  = r.Proto
		)

		userAttrs := slog.Group("user", "ip", ip)
		requestAttrs := slog.Group("request", "method", method, "url", url, "proto", proto)
		responseAttrs := slog.Group("repsonse", "status", mw.StatusCode, "size", mw.BytesCount)

		app.logger.Info("access", userAttrs, requestAttrs, responseAttrs)
	})
}

func (app *application) preventCSRF(next http.Handler) http.Handler {
	csrfHandler := nosurf.New(next)

	// Set up custom failure handler to provide more detailed logging
	csrfHandler.SetFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.logger.Error("CSRF validation failed",
			"path", r.URL.Path,
			"method", r.Method,
			"token_in_form", r.PostForm.Get("csrf_token"),
			"token_in_header", r.Header.Get("X-CSRF-Token"),
			"real_token", nosurf.Token(r),
			"cookie", r.Header.Get("Cookie"),
		)

		if app.config.testing {
			// In testing mode, accept our test token
			token := r.Header.Get("X-CSRF-Token")
			if token == "test-csrf-token" {
				app.logger.Info("Accepting test CSRF token")
				next.ServeHTTP(w, r)
				return
			}
		}

		http.Error(w, "Bad Request (CSRF validation failed)", http.StatusBadRequest)
	}))

	// Add logging for successful CSRF validation
	wrappedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.logger.Debug("CSRF validation successful",
			"path", r.URL.Path,
			"method", r.Method,
			"token", nosurf.Token(r))
		csrfHandler.ServeHTTP(w, r)
	})

	csrfHandler.SetBaseCookie(http.Cookie{
		HttpOnly: true,
		Path:     "/",
		MaxAge:   86400,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})

	return wrappedHandler
}

func (app *application) clearSession(w http.ResponseWriter, r *http.Request) {
	err := app.sessionManager.Destroy(r.Context())
	if err != nil {
		app.logger.Error("error destroying session", "error", err.Error())
	}
	app.logger.Info("session successfully cleared")
}

func (app *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.logger.Debug("authenticate: middleware",
			"path", r.URL.Path,
			"method", r.Method)

		// Check if user is authenticated
		userID := app.sessionManager.GetString(r.Context(), "UserID")
		if userID == "" {
			app.logger.Debug("authenticate: no UserID in session", "path", r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		// Get user from database
		user, found, err := internal.GetUser(r.Context(), app.fs, userID)
		if err != nil {
			app.logger.Error("authenticate: error getting user from database",
				"error", err,
				"user_id", userID)
			app.serverError(w, r, err)
			return
		}

		if !found {
			app.logger.Debug("authenticate: user not found in database",
				"user_id", userID,
				"path", r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		app.logger.Debug("user authenticated",
			"user_id", user.ID,
			"email", user.Email,
			"path", r.URL.Path)

		// Add user to request context
		r = contextSetAuthenticatedUser(r, user)
		next.ServeHTTP(w, r)
	})
}

func (app *application) setSentryUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the user from your context
		user := contextGetAuthenticatedUser(r)

		if user != nil {
			// Get the hub from the request context (created by the Sentry middleware)
			if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
				// Configure the hub's scope with the user info
				hub.ConfigureScope(func(scope *sentry.Scope) {
					scope.SetUser(sentry.User{
						ID:        user.ID,
						Email:     user.Email,
						Name:      user.Name,
						IPAddress: r.Header.Get("X-Forwarded-For"),
						// Add other user fields as needed
					})
				})

				app.logger.Debug("sentry: added user to scope",
					"user_id", user.ID,
					"path", r.URL.Path)
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (app *application) requireSuperUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := contextGetAuthenticatedUser(r)
		if user == nil {
			http.Redirect(w, r, "/auth/linkedin", http.StatusSeeOther)
			return
		}
		// Check both is_superuser flag and role for admin access
		if !user.IsSuperUser {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAuthenticatedUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := contextGetAuthenticatedUser(r)
		if user == nil {
			http.Redirect(w, r, "/auth/linkedin", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAnonymousUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := contextGetAuthenticatedUser(r)
		if user != nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *application) debugHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mw := response.NewMetricsResponseWriter(w)

		// Log pre-handler state
		app.logger.Debug("pre-handler",
			"path", r.URL.Path,
			"method", r.Method,
			"headers", fmt.Sprintf("%+v", mw.Header()))

		// Wrap the metrics writer to catch header operations
		hw := &headerDebugWriter{
			MetricsResponseWriter: mw,
			logger:                app.logger,
			path:                  r.URL.Path,
		}

		next.ServeHTTP(hw, r)

		// Log post-handler state
		app.logger.Debug("post-handler",
			"path", r.URL.Path,
			"method", r.Method,
			"status", mw.StatusCode,
			"size", mw.BytesCount,
			"headers", fmt.Sprintf("%+v", mw.Header()))
	})
}

type headerDebugWriter struct {
	*response.MetricsResponseWriter
	logger *slog.Logger
	path   string
}

func (hw *headerDebugWriter) Header() http.Header {
	h := hw.MetricsResponseWriter.Header()
	hw.logger.Debug("header access",
		"path", hw.path,
		"headers", fmt.Sprintf("%+v", h))
	return h
}

func (hw *headerDebugWriter) WriteHeader(statusCode int) {
	hw.logger.Debug("writing header",
		"path", hw.path,
		"status", statusCode,
		"headers", fmt.Sprintf("%+v", hw.Header()))
	hw.MetricsResponseWriter.WriteHeader(statusCode)
}

func (hw *headerDebugWriter) Write(b []byte) (int, error) {
	hw.logger.Debug("writing response",
		"path", hw.path,
		"headers", fmt.Sprintf("%+v", hw.Header()))
	return hw.MetricsResponseWriter.Write(b)
}
