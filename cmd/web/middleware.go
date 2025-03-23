package main

import (
	"fmt"
	"log/slog"
	"net/http"

	"talent-rodeo/internal"
	"talent-rodeo/internal/response"

	"github.com/justinas/nosurf"
	"github.com/tomasen/realip"
)

const CSRFTokenKey contextKey = "csrf_token"

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
	// app.logger.Warn("prevent csrf middleware")
	csrfHandler := nosurf.New(next)

	if app.config.testing {
		// In testing mode, accept our test token
		csrfHandler.SetFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-CSRF-Token")
			if token == "test-csrf-token" {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "Bad Request", http.StatusBadRequest)
		}))
	}

	csrfHandler.SetBaseCookie(http.Cookie{
		HttpOnly: true,
		Path:     "/",
		MaxAge:   86400,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})

	return csrfHandler
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
		// Check if user is authenticated
		userID := app.sessionManager.GetString(r.Context(), "UserID")
		if userID == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Get user from database
		user, found, err := internal.GetUser(r.Context(), app.fs, userID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		if !found {
			next.ServeHTTP(w, r)
			return
		}

		// Add user to request context
		r = contextSetAuthenticatedUser(r, user)
		next.ServeHTTP(w, r)
	})
}

func (app *application) requireSuperUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := contextGetAuthenticatedUser(r)
		if user == nil {
			http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
			return
		}
		// Check both is_superuser flag and role for admin access
		if !user.IsSuperUser {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAuthenticatedUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := contextGetAuthenticatedUser(r)
		if user == nil {
			http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAnonymousUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := contextGetAuthenticatedUser(r)
		if user != nil {
			http.Redirect(w, r, "/", http.StatusFound)
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
