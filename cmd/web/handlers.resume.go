package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/rodeo"
	"talent-rodeo/internal/validator"
	"time"
)

func isValidFileType(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".docx" || ext == ".pdf"
}

func (app *application) resetResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		app.badRequest(w, r, fmt.Errorf("resetResume: method not allowed"))
		return
	}

	if err := app.db.UpdateFullResume(contextGetAuthenticatedUser(r).ID, "", ""); err != nil {
		app.serverError(w, r, fmt.Errorf("resetResume: failed to reset resume: %w", err))
		return
	}

	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := app.newTemplateData(r)
	sessionID, ok := data["SessionID"].(string)
	if !ok {
		app.serverError(w, r, fmt.Errorf("uploadResume: failed to get session ID"))
		return
	}

	w.Header().Set("Content-Type", "text/html")

	// Parse multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		app.badRequest(w, r, fmt.Errorf("uploadResume: failed to parse form: %w", err))
		return
	}

	phone := r.FormValue("phone")
	file, header, err := r.FormFile("resume")
	if err != nil {
		app.badRequest(w, r, fmt.Errorf("uploadResume: failed to get resume file: %w", err))
		return
	}
	defer file.Close()

	type formData struct {
		Phone     string `form:"phone"`
		Filename  string `form:"resume"`
		Validator validator.Validator
	}

	form := formData{
		Phone:    phone,
		Filename: header.Filename,
	}

	rx := regexp.MustCompile(`^\+?1?\d{10,}$`)
	form.Validator.CheckField(validator.Matches(phone, rx), "Phone", "Your phone number is required")
	form.Validator.CheckField(isValidFileType(header.Filename), "Resume", "Invalid file type. Please upload a DOCX or PDF file.")
	data["Form"] = form

	if form.Validator.HasErrors() {
		for _, err := range form.Validator.Errors {
			app.logger.Warn("uploadResume: invalid form data", "error", err)
		}
		for _, err := range form.Validator.FieldErrors {
			app.logger.Warn("uploadResume: invalid field data", "error", err)
		}

		data["Errors"] = form.Validator.Errors

		if err := response.Page(w, http.StatusOK, data, "pages/profile.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}

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

	user := contextGetAuthenticatedUser(r)
	if err := app.db.UpdateFullResume(user.ID, header.Filename, resumeText); err != nil {
		app.serverError(w, r, fmt.Errorf("uploadResume: failed to save resume: %w", err))
		return
	}

	user.FullResume = &resumeText
	user.ResumeFilename = &header.Filename
	r = contextSetAuthenticatedUser(r, user)

	ctx := context.Background()
	ctx = context.WithValue(ctx, rodeo.SessionIDKey, sessionID)

	app.backgroundTask(r, func() error {
		sessionID := ctx.Value(rodeo.SessionIDKey).(string)

		timer := time.NewTicker(5 * time.Second)
		count := 0
		for {
			select {
			case <-timer.C:
				count++
				app.SendStatus(sessionID, "status", "error", fmt.Sprintf("tick %d", count))

			case <-ctx.Done():
				timer.Stop()
				app.SendStatus(sessionID, "status", "success", "Done")
				return nil
			}
		}

		app.SendStatus(sessionID, "status", "info", "Extracting keywords...")
		defer app.SendStatus(sessionID, "status", "success", "Keywords extracted")
		keywords, err := rodeo.GetResumeKeywords(resumeText)
		if err != nil {
			return fmt.Errorf("uploadResume: failed to get keywords: %w", err)
		}

		user := contextGetAuthenticatedUser(r)
		userKW := make([]database.UserKeyword, len(keywords))
		for i, kw := range keywords {
			keyword := database.UserKeyword{
				UserID:   user.ID,
				Keyword:  kw,
				Priority: i,
			}
			userKW[i] = keyword
		}

		if err := app.db.UpdateUserKeywords(user.ID, userKW); err != nil {
			return fmt.Errorf("uploadResume: failed to save keywords: %w", err)
		}

		return nil
	})

	app.logger.Info("uploadResume: resume uploaded", "filename", header.Filename, "phone", phone)
	if err := response.Page(w, http.StatusOK, data, "pages/profile.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) SendStatus(sessionID, eventName, level, message string) {
	app.logger.Info("SendStatus", "sessionID", sessionID, "eventName", eventName, "level", level, "message", message)
	err := app.pusher.Trigger(sessionID, eventName, map[string]string{
		"level":   level,
		"message": message,
	})

	if err != nil {
		app.logger.Error("SendStatus: failed to send status", "error", err)
	}
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
