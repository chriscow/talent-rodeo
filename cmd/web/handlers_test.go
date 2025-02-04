package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal/database"

	"github.com/gorilla/sessions"
)

type testServer struct {
	*testing.T
	app *application
}

func newTestServer(t *testing.T) *testServer {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Initialize test database using the Postgres setup
	db := database.SetupTestDB(t)

	// Create test user
	user := &database.User{
		ID:      1,
		Email:   "test@example.com",
		Created: time.Now(),
	}
	_, err := db.InsertUser(*user)
	if err != nil {
		t.Fatal(err)
	}

	sessionStore := sessions.NewCookieStore([]byte("test-session-secret-key"))
	sessionStore.Options = &sessions.Options{
		HttpOnly: true,
		MaxAge:   86400 * 7,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	}

	app := &application{
		config: config{
			baseURL:  "http://localhost:2304",
			httpPort: 2304,
			testing:  true,
		},
		logger:       logger,
		sessionStore: sessionStore,
		db:           db,
	}

	ts := &testServer{
		T:   t,
		app: app,
	}

	return ts
}

func (ts *testServer) get(t *testing.T, urlPath string) (int, http.Header, string) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", urlPath, nil)
	ts.app.routes().ServeHTTP(w, r)
	return w.Code, w.Header(), w.Body.String()
}

func (ts *testServer) authenticatedRequest(t *testing.T, method, urlPath string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, urlPath, body)

	// Set up the user
	user := &database.User{
		ID:      1,
		Email:   "test@example.com",
		Created: time.Now(),
	}
	req = contextSetAuthenticatedUser(req, user)

	// Create a session and save it
	session, err := ts.app.sessionStore.Get(req, "session")
	if err != nil {
		t.Fatal(err)
	}
	session.Values["UserID"] = user.ID
	w := httptest.NewRecorder()
	err = session.Save(req, w)
	if err != nil {
		t.Fatal(err)
	}

	// Create a new request with the session cookie
	req = httptest.NewRequest(method, urlPath, body)
	req.Header.Add("Cookie", w.Header().Get("Set-Cookie"))

	// Add CSRF token for POST requests
	if method == "POST" {
		req.Header.Set("X-CSRF-Token", "test-csrf-token")
		cookie := &http.Cookie{
			Name:     "csrf_token",
			Value:    "test-csrf-token",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		}
		req.AddCookie(cookie)
	}

	// Set the user in the context
	req = contextSetAuthenticatedUser(req, user)

	return req
}

func (ts *testServer) postForm(t *testing.T, urlPath string, form url.Values) (int, http.Header, string) {
	req := httptest.NewRequest("POST", urlPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Add CSRF token for POST requests
	if req.Method == "POST" {
		req.Header.Set("X-CSRF-Token", "test-csrf-token")
		cookie := &http.Cookie{
			Name:     "csrf_token",
			Value:    "test-csrf-token",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		}
		req.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	ts.app.routes().ServeHTTP(w, req)
	return w.Code, w.Header(), w.Body.String()
}

func (ts *testServer) authenticatedPostForm(t *testing.T, urlPath string, form url.Values) (int, http.Header, string) {
	req := httptest.NewRequest("POST", urlPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Add CSRF token for POST requests
	if req.Method == "POST" {
		req.Header.Set("X-CSRF-Token", "test-csrf-token")
		cookie := &http.Cookie{
			Name:     "csrf_token",
			Value:    "test-csrf-token",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		}
		req.AddCookie(cookie)
	}

	user := &database.User{
		ID:      1,
		Email:   "test@example.com",
		Created: time.Now(),
	}
	req = contextSetAuthenticatedUser(req, user)
	w := httptest.NewRecorder()
	ts.app.routes().ServeHTTP(w, req)
	return w.Code, w.Header(), w.Body.String()
}
