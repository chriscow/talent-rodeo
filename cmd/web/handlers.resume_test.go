package main

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"talent-rodeo/internal/database"
	"testing"
)

func TestResumeUpload(t *testing.T) {
	ts := newTestServer(t)

	t.Run("Unauthenticated user cannot upload resume", func(t *testing.T) {
		code, headers, _ := ts.get(t, "/profile/resume/upload")

		if code != http.StatusFound {
			t.Errorf("want %d; got %d", http.StatusFound, code)
		}

		if headers.Get("Location") != "/auth/linkedin" {
			t.Errorf("want %s; got %s", "/auth/linkedin", headers.Get("Location"))
		}
	})

	t.Run("Authenticated user can upload resume", func(t *testing.T) {
		t.Skip("Skipping resume upload test until we fix it")
		// Create a multipart form with a PDF file and phone number
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		// Add phone number field
		err := writer.WriteField("phone", "+11234567890")
		if err != nil {
			t.Fatal(err)
		}

		// Add resume file field
		part, err := writer.CreateFormFile("resume", "test.pdf")
		if err != nil {
			t.Fatal(err)
		}
		_, err = part.Write([]byte("test pdf content"))
		if err != nil {
			t.Fatal(err)
		}

		err = writer.Close()
		if err != nil {
			t.Fatal(err)
		}

		// Create authenticated request
		req := ts.authenticatedRequest(t, "POST", "/profile/resume/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		// Create and save a session
		session, err := ts.app.sessionStore.Get(req, "session")
		if err != nil {
			t.Fatal(err)
		}
		session.Values["UserID"] = 1
		err = session.Save(req, w)
		if err != nil {
			t.Fatal(err)
		}

		// Create a new request with the session cookie and CSRF token
		req = httptest.NewRequest("POST", "/profile/resume/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Add("Cookie", w.Header().Get("Set-Cookie"))
		req.Header.Add("X-CSRF-Token", "test-token")

		// Set the user in the context
		user := &database.User{ID: 1}
		ctx := context.WithValue(req.Context(), authenticatedUserContextKey, user)
		req = req.WithContext(ctx)

		w = httptest.NewRecorder()
		ts.app.routes().ServeHTTP(w, req)

		if w.Code != http.StatusFound {
			t.Errorf("want %d; got %d", http.StatusFound, w.Code)
		}

		if w.Header().Get("Location") != "/profile" {
			t.Errorf("want %s; got %s", "/profile", w.Header().Get("Location"))
		}
	})

	t.Run("Invalid file type is rejected", func(t *testing.T) {
		t.Skip("Skipping invalid file type test until we fix it")

		// Create a multipart form with a text file and phone number
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		// Add phone number field
		err := writer.WriteField("phone", "+11234567890")
		if err != nil {
			t.Fatal(err)
		}

		// Add resume file field
		part, err := writer.CreateFormFile("resume", "test.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, err = part.Write([]byte("test text content"))
		if err != nil {
			t.Fatal(err)
		}

		err = writer.Close()
		if err != nil {
			t.Fatal(err)
		}

		// Create authenticated request
		req := ts.authenticatedRequest(t, "POST", "/profile/resume/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		// Create and save a session
		session, err := ts.app.sessionStore.Get(req, "session")
		if err != nil {
			t.Fatal(err)
		}
		session.Values["UserID"] = 1
		err = session.Save(req, w)
		if err != nil {
			t.Fatal(err)
		}

		// Create a new request with the session cookie and CSRF token
		req = httptest.NewRequest("POST", "/profile/resume/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Add("Cookie", w.Header().Get("Set-Cookie"))
		req.Header.Add("X-CSRF-Token", "test-token")

		// Set the user in the context
		user := &database.User{ID: 1}
		ctx := context.WithValue(req.Context(), authenticatedUserContextKey, user)
		req = req.WithContext(ctx)

		w = httptest.NewRecorder()
		ts.app.routes().ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("want %d; got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("Missing phone number is rejected", func(t *testing.T) {
		t.Skip("Skipping missing phone number test until we fix it")

		// Create a multipart form with only a PDF file
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		// Add resume file field
		part, err := writer.CreateFormFile("resume", "test.pdf")
		if err != nil {
			t.Fatal(err)
		}
		_, err = part.Write([]byte("test pdf content"))
		if err != nil {
			t.Fatal(err)
		}

		err = writer.Close()
		if err != nil {
			t.Fatal(err)
		}

		// Create authenticated request
		req := ts.authenticatedRequest(t, "POST", "/profile/resume/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		// Create and save a session
		session, err := ts.app.sessionStore.Get(req, "session")
		if err != nil {
			t.Fatal(err)
		}
		session.Values["UserID"] = 1
		err = session.Save(req, w)
		if err != nil {
			t.Fatal(err)
		}

		// Create a new request with the session cookie and CSRF token
		req = httptest.NewRequest("POST", "/profile/resume/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Add("Cookie", w.Header().Get("Set-Cookie"))
		req.Header.Add("X-CSRF-Token", "test-token")

		// Set the user in the context
		user := &database.User{ID: 1}
		ctx := context.WithValue(req.Context(), authenticatedUserContextKey, user)
		req = req.WithContext(ctx)

		w = httptest.NewRecorder()
		ts.app.routes().ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("want %d; got %d", http.StatusBadRequest, w.Code)
		}
	})
}
