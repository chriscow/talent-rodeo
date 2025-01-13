package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"talent-rodeo/assets"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/rodeo"
)

func isValidFileType(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".docx"
}

func (app *application) resume(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	if err := response.Page(w, http.StatusOK, data, "resume.tmpl"); err != nil {
		app.serverError(w, r, err)
	}

	assets.EmbeddedFiles.ReadFile("prompts/recruiter.convert-resume.txt")
}

func (app *application) uploadResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html")

	// Parse multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		app.badRequest(w, r, fmt.Errorf("uploadResume: failed to parse form: %w", err))
		return
	}

	// Get phone number
	phone := r.FormValue("phone")
	if phone == "" {
		app.badRequest(w, r, fmt.Errorf("uploadResume: phone number is required"))
		return
	}

	// Get resume file
	file, header, err := r.FormFile("resume")
	if err != nil {
		app.badRequest(w, r, fmt.Errorf("uploadResume: failed to get resume file: %w", err))
		return
	}
	defer file.Close()

	tempDir := os.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "resume-*.docx")
	if err != nil {
		app.serverError(w, r, fmt.Errorf("uploadResume: failed to create temp file: %w", err))
		return
	}
	defer os.Remove(tempFile.Name()) // Clean up temp file after we're done

	// Copy the uploaded file to the temp file
	_, err = io.Copy(tempFile, file)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("uploadResume: failed to copy file: %w", err))
		return
	}
	defer tempFile.Close()

	// Read file content
	var resumeText string
	if strings.HasSuffix(strings.ToLower(header.Filename), ".docx") {
		resumeText, err = rodeo.ExtractDOCX(tempFile)
	} else if strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		resumeText, err = rodeo.ExtractPDF(tempFile)
	} else {
		app.badRequest(w, r, fmt.Errorf("uploadResume: invalid file type"))
		return
	}

	if err != nil {
		app.serverError(w, r, fmt.Errorf("uploadResume: failed to extract text: %w", err))
		return
	}

	data := app.newTemplateData(r)
	sessionID, ok := data["SessionID"].(string)
	if !ok {
		app.serverError(w, r, fmt.Errorf("uploadResume: failed to get session ID"))
		return
	}

	app.backgroundTask(r, func() error {
		ctx := context.WithValue(r.Context(), rodeo.SessionIDKey, sessionID)
		return rodeo.StartWorkflow(ctx, resumeText, app)
	})

	if err := response.NamedTemplate(w, http.StatusOK, data, "partials/call-status.tmpl"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) SendStatus(sessionID, level, message string) {
	app.pusher.Trigger(sessionID, "call-status", map[string]string{
		"level":   level,
		"message": message,
	})
}

// func (app *application) events(w http.ResponseWriter, r *http.Request) {
// 	sessionID := flow.Param(r.Context(), "session-id")
// 	app.logger.Debug("callStatus-----------------------", "sessionID", sessionID)

// 	// Start session-specific publisher
// 	// go func() {
// 	// 	ticker := time.NewTicker(time.Second)
// 	// 	for range ticker.C {
// 	// 		app.sendStatus(sessionID, "status", "Still working...")
// 	// 	}
// 	// }()

// 	app.sse.OnSession = func(sess *sse.Session) (sse.Subscription, bool) {
// 		return sse.Subscription{
// 			Client: sess,
// 			Topics: []string{sessionID},
// 		}, true
// 	}

// 	app.sse.ServeHTTP(w, r)
// }
