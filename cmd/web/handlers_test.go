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

// createTestUser creates a user with the specified role and superuser status
func (ts *testServer) createTestUser(id int, email string, role string, isSuperUser bool) *database.User {
	now := time.Now()
	user := &database.User{
		ID:          id,
		Email:       email,
		Created:     now,
		UpdatedAt:   &now,
		Role:        &role,
		IsSuperUser: isSuperUser,
	}
	_, err := ts.app.db.InsertUser(*user)
	if err != nil {
		ts.T.Fatal(err)
	}
	return user
}

// createAdminUser creates a user with admin role and superuser status
func (ts *testServer) createAdminUser(id int, email string) *database.User {
	return ts.createTestUser(id, email, "admin", true)
}

// createCandidateUser creates a user with candidate role and non-superuser status
func (ts *testServer) createCandidateUser(id int, email string) *database.User {
	return ts.createTestUser(id, email, "candidate", false)
}

func newTestServer(t *testing.T) *testServer {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Initialize test database using the Postgres setup
	db := database.SetupTestDB(t)

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

	// Create default test users
	ts.createAdminUser(1, "admin@example.com")
	ts.createCandidateUser(2, "candidate@example.com")

	return ts
}

func (ts *testServer) get(t *testing.T, urlPath string) (int, http.Header, string) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", urlPath, nil)
	ts.app.routes().ServeHTTP(w, r)
	return w.Code, w.Header(), w.Body.String()
}

func (ts *testServer) authenticatedRequest(t *testing.T, method, urlPath string, user *database.User, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, urlPath, body)

	// Create a session and save it
	session, err := ts.app.sessionStore.Get(req, "session")
	if err != nil {
		t.Fatal(err)
	}

	if user != nil {
		session.Values["UserID"] = user.ID
	}

	w := httptest.NewRecorder()
	err = session.Save(req, w)
	if err != nil {
		t.Fatal(err)
	}

	// Create a new request with the session cookie, preserving original headers
	req = httptest.NewRequest(method, urlPath, body)
	// Copy all headers from the original request
	for k, v := range req.Header {
		req.Header[k] = v
	}
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

	if user != nil {
		req = contextSetAuthenticatedUser(req, user)
	}

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

	// Set up the user
	now := time.Now()
	role := "candidate"
	user := &database.User{
		ID:        1,
		Email:     "test@example.com",
		Created:   now,
		UpdatedAt: &now,
		Role:      &role,
	}

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
	req = httptest.NewRequest("POST", urlPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Cookie", w.Header().Get("Set-Cookie"))

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

	req = contextSetAuthenticatedUser(req, user)
	w = httptest.NewRecorder()
	ts.app.routes().ServeHTTP(w, req)
	return w.Code, w.Header(), w.Body.String()
}
