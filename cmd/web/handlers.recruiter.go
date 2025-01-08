package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"talent-rodeo/cmd/web/components/partials"
	"talent-rodeo/internal/response"
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
}

func (app *application) uploadResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html")

	// Parse multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		partials.UploadError("Failed to parse form").Render(r.Context(), w)
		return
	}

	// Get phone number
	phone := r.FormValue("phone")
	if phone == "" {
		partials.UploadError("Phone number is required").Render(r.Context(), w)
		return
	}

	// Get resume file
	file, header, err := r.FormFile("resume")
	if err != nil {
		partials.UploadError("Failed to get resume file").Render(r.Context(), w)
		return
	}
	defer file.Close()

	tempDir := os.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "resume-*.docx")
	if err != nil {
		http.Error(w, "Error creating temporary file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempFile.Name()) // Clean up temp file after we're done

	// Copy the uploaded file to the temp file
	_, err = io.Copy(tempFile, file)
	if err != nil {
		http.Error(w, "Error copying file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tempFile.Close()

	// Read file content
	var textContent string
	if strings.HasSuffix(strings.ToLower(header.Filename), ".docx") {
		textContent, err = extractDOCX(tempFile)
	} else if strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		textContent, err = extractPDF(tempFile)
	} else {
		partials.UploadError("Unsupported file format").Render(r.Context(), w)
		return
	}

	if err != nil {
		partials.UploadError("Failed to extract text from resume").Render(r.Context(), w)
		return
	}

	data := app.newTemplateData(r)
	sessionID, ok := data["SessionID"].(string)
	if !ok {
		app.serverError(w, r, fmt.Errorf("uploadResume: failed to get session ID"))
		return
	}

	app.backgroundTask(r, func() error {
		return app.startWorkflow(sessionID, textContent)
	})

	if err := response.NamedTemplate(w, http.StatusOK, data, "partials/call-status.tmpl"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) sendStatus(sessionID, level, message string) {
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
