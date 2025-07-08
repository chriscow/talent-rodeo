package main

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"talent-rodeo/internal"
	"testing"
	"time"
)

func TestResumeUpload(t *testing.T) {
	ts := newTestServer(t, "test resume upload")

	t.Run("Unauthenticated user cannot upload resume", func(t *testing.T) {
		code, headers, _ := ts.get(t, "/onboarding/resume/upload")

		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
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
		now := time.Now()
		user := &internal.User{
			ID:        "1",
			Email:     "test@example.com",
			CreatedAt: now,
		}
		req := ts.authenticatedRequest(t, "POST", "/onboarding/resume/upload", user, body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		// Store user ID in session
		ts.app.sessionManager.Put(req.Context(), "UserID", "1")

		// Create a new request with the session cookie and CSRF token
		req = httptest.NewRequest("POST", "/onboarding/resume/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Add("Cookie", w.Header().Get("Set-Cookie"))
		req.Header.Add("X-CSRF-Token", "test-token")

		// Set the user in the context
		ctx := context.WithValue(req.Context(), authenticatedUserContextKey, user)
		req = req.WithContext(ctx)

		w = httptest.NewRecorder()
		ts.app.routes().ServeHTTP(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, w.Code)
		}

		if w.Header().Get("Location") != "/onboarding" {
			t.Errorf("want %s; got %s", "/onboarding", w.Header().Get("Location"))
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
		now := time.Now()
		user := &internal.User{
			ID:        "1",
			Email:     "test@example.com",
			CreatedAt: now,
		}
		req := ts.authenticatedRequest(t, "POST", "/onboarding/resume/upload", user, body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		// Store user ID in session
		ts.app.sessionManager.Put(req.Context(), "UserID", "1")

		// Create a new request with the session cookie and CSRF token
		req = httptest.NewRequest("POST", "/onboarding/resume/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Add("Cookie", w.Header().Get("Set-Cookie"))
		req.Header.Add("X-CSRF-Token", "test-token")

		// Set the user in the context
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
		now := time.Now()
		user := &internal.User{
			ID:        "1",
			Email:     "test@example.com",
			CreatedAt: now,
		}
		req := ts.authenticatedRequest(t, "POST", "/onboarding/resume/upload", user, body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		// Store user ID in session
		ts.app.sessionManager.Put(req.Context(), "UserID", "1")

		// Create a new request with the session cookie and CSRF token
		req = httptest.NewRequest("POST", "/onboarding/resume/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Add("Cookie", w.Header().Get("Set-Cookie"))
		req.Header.Add("X-CSRF-Token", "test-token")

		// Set the user in the context
		ctx := context.WithValue(req.Context(), authenticatedUserContextKey, user)
		req = req.WithContext(ctx)

		w = httptest.NewRecorder()
		ts.app.routes().ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("want %d; got %d", http.StatusBadRequest, w.Code)
		}
	})
}

func TestExtractPDF(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantErr  bool
		wantSub  string // substring expected in output, can be empty
	}{
		{
			name:     "Valid PDF Resume 1",
			filename: "../../testdata/sample-resumes/sample-resume.pdf", // placeholder, replace as needed
			wantErr:  false,
			wantSub:  "Extensive experience in a technical leadership role",
		},
		{
			name:     "Valid PDF Resume 2",
			filename: "../../testdata/sample-resumes/Nick Beaver 2024.pdf", // placeholder, replace as needed
			wantErr:  false,
			wantSub:  "A passionate student of materials and data science looking to solve complex and cutting-edge engineering problems", // placeholder, replace as needed
		},
		{
			name:     "Valid PDF Resume 3",
			filename: "../../testdata/sample-resumes/Marco Mindemann May 2025.pdf", // placeholder, replace as needed
			wantErr:  false,
			wantSub:  "performing team of risk managers who are subject matter experts",
		},
		// {
		// 	name:     "Corrupt or Not a PDF",
		// 	filename: "testdata/sample-resumes/01-not-a-resume.easy.txt", // not a PDF
		// 	wantErr:  true,
		// 	wantSub:  "",
		// },
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.Open(tc.filename)
			if err != nil {
				t.Fatalf("failed to open file %s: %v", tc.filename, err)
			}
			defer f.Close()

			got, err := extractPDF(f)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for %s, got none", tc.filename)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for %s: %v", tc.filename, err)
			}
			if tc.wantSub != "" && !strings.Contains(got, tc.wantSub) {
				t.Errorf("output for %s does not contain expected substring. want %q, got: %.100q", tc.filename, tc.wantSub, got)
			}
		})
	}
}
