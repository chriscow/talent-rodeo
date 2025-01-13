package main

import (
	"fmt"
	"log/slog"
	"net/http"

	"talent-rodeo/internal/response"

	"github.com/google/uuid"
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
	session, _ := app.sessionStore.Get(r, "session")

	// Set MaxAge to -1 to expire the cookie immediately
	session.Options.MaxAge = -1

	// Save will remove the cookie since MaxAge < 0
	err := session.Save(r, w)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
}

func (app *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// app.logger.Warn("authenticate middleware", "path", r.URL.Path)
		session, err := app.sessionStore.Get(r, "session")
		if err != nil {
			app.logger.Error("session error",
				"error", err,
				"cookie", r.Header.Get("Cookie"))
			// First, clear the corrupted cookie
			cookie := &http.Cookie{
				Name:     "session",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   false, // Set to true in production
				SameSite: http.SameSiteLaxMode,
			}
			http.SetCookie(w, cookie)

			// Create a fresh request without the corrupted cookie
			r.Header.Del("Cookie")

			// Now try to create a new session with the clean request
			session, err = app.sessionStore.New(r, "session")
			if err != nil {
				app.serverError(w, r, err)
				return
			}

			// Save the new session
			err = session.Save(r, w)
			if err != nil {
				app.serverError(w, r, err)
				return
			}

			next.ServeHTTP(w, r)
			return
		}

		sessionID, ok := session.Values["SessionID"].(string)
		if !ok || sessionID == "" {
			sessionID = uuid.New().String()
			session.Values["SessionID"] = sessionID
			if err := session.Save(r, w); err != nil {
				app.serverError(w, r, err)
				return
			}
		}

		userID, ok := session.Values["UserID"].(int)
		if ok {
			user, found, err := app.db.GetUser(userID)
			if err != nil {
				app.serverError(w, r, err)
				return
			}

			app.logger.Debug("authenticated user", "found", found)
			if found {
				r = contextSetAuthenticatedUser(r, user)
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (app *application) requireSuperUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// app.logger.Warn("require super user middleware", "path", r.URL.Path)
		authenticatedUser := contextGetAuthenticatedUser(r)

		if authenticatedUser == nil || !authenticatedUser.IsSuperUser {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAuthenticatedUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// app.logger.Warn("require authenticated user middleware", "path", r.URL.Path)
		authenticatedUser := contextGetAuthenticatedUser(r)

		if authenticatedUser == nil {
			session, err := app.sessionStore.Get(r, "session")
			if err != nil {
				app.serverError(w, r, err)
				return
			}

			session.Values["redirectPathAfterLogin"] = r.URL.Path

			err = session.Save(r, w)
			if err != nil {
				app.serverError(w, r, err)
				return
			}

			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		w.Header().Add("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAnonymousUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// app.logger.Warn("require anonymous user middleware", "path", r.URL.Path)
		authenticatedUser := contextGetAuthenticatedUser(r)

		if authenticatedUser != nil {
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
