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
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/validator"
	"time"

	"github.com/alexedwards/flow"
	"github.com/fumiama/go-docx"
	"github.com/getsentry/sentry-go"
	"github.com/ledongthuc/pdf"
)

type resumeUploadForm struct {
	Phone     string
	Filename  string
	Terms     bool // Add terms field
	Validator validator.Validator
}

// isValidFileType checks if the given filename has an allowed extension (.pdf or .docx or .txt).
// Used for resume upload validation.
func isValidFileType(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".pdf" || ext == ".docx" || ext == ".txt"
}

// deleteResume handles DELETE requests to /profile/resume.
// It removes the user's resume and resets their onboarding state to initial.
// After successful deletion, redirects to the resume upload page.
// Requires: Authenticated user
func (app *application) deleteResume(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusSeeOther)
		return
	}

	resumeID := flow.Param(r.Context(), "resumeID")

	// Reset resume fields
	err := internal.DeleteResume(context.Background(), app.fs, user.ID, resumeID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to reset resume: %w", err))
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"userID":   user.ID,
			"resumeID": resumeID,
			"action":   "internal.DeleteResume",
		})
		return
	}

	if err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusPendingResumeUpload); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to update onboarding state: %w", err))
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"userID":   user.ID,
			"resumeID": resumeID,
			"action":   "internal.UpdateUserStatus",
		})
		return
	}

	http.Redirect(w, r, "/profile/resume/upload", http.StatusSeeOther)
}

// showUploadPage handles GET requests to /profile/resume/upload.
// Displays the resume upload form page.
// Template: pages/profile-resume-upload.html
// Requires: Authenticated user
func (app *application) showUploadPage(w http.ResponseWriter, r *http.Request) {
	app.logger.Info("showUploadPage", "method", r.Method, "url", r.URL.String())

	data := app.newTemplateData(r)
	if err := response.Page(w, http.StatusOK, data, "pages/profile-resume-upload.html"); err != nil {
		app.logger.Error("Failed to render resume upload page", "error", err)
		app.serverError(w, r, fmt.Errorf("failed to render resume upload page: %w", err))
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"template": "pages/profile-resume-upload.html",
			"action":   "response.Page",
			"method":   r.Method,
		})
	}
}

// createResume handles POST requests to /profile/resume/upload.
// Processes resume file upload and phone number submission.
// Flow:
//  1. Validates file type and size
//  2. Extracts text from resume
//  3. Updates user's resume and phone number
//  4. Updates onboarding state
//  5. Redirects to scheduling page on success
//
// Template: pages/profile-schedule-phone-call.html (on success)
//
//	partials/resume-form.html (on validation error)
//
// Requires: Authenticated user
func (app *application) createResume(w http.ResponseWriter, r *http.Request) {
	app.logger.Info("createResume", "method", r.Method, "url", r.URL.String())

	data := app.newTemplateData(r)
	form := &resumeUploadForm{
		Validator: validator.Validator{},
	}
	data["Form"] = form

	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		form.Validator.AddFieldError("resume", "File too large. Maximum size is 10MB")
		internal.SentryReportError(r.Context(), err, sentry.LevelWarning, map[string]string{
			"action":  "parseMultipartForm",
			"maxSize": "10MB",
		})
		if err := response.Page(w, http.StatusUnprocessableEntity, data, "pages/profile-resume-upload.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"template": "pages/profile-resume-upload.html",
				"action":   "response.Page",
			})
		}
		return
	}

	// Check terms agreement
	terms := r.FormValue("terms")
	form.Terms = terms == "on"
	form.Validator.CheckField(form.Terms, "terms", "You must agree to the Terms of Service to continue")

	file, header, err := r.FormFile("resume")
	if err != nil {
		form.Validator.AddFieldError("resume", "Please select a file")
		internal.SentryReportError(r.Context(), err, sentry.LevelWarning, map[string]string{
			"action":    "getFormFile",
			"fieldName": "resume",
		})
		if err := response.Page(w, http.StatusUnprocessableEntity, data, "pages/profile-resume-upload.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"template": "pages/profile-resume-upload.html",
				"action":   "response.Page",
			})
		}
		return
	}
	defer file.Close()

	form.Phone = r.FormValue("phone")

	// Ensure phone number is prefixed with +1 (E.164 format)
	if !strings.HasPrefix(form.Phone, "+1") && !strings.HasPrefix(form.Phone, "1") {
		form.Phone = "+1" + form.Phone
	}

	// Update regex to validate E.164 format
	phoneRegex := regexp.MustCompile(`^\+1\d{10}$`)
	form.Validator.CheckField(validator.Matches(form.Phone, phoneRegex), "phone", "Invalid phone number. Please use the format +1XXXXXXXXXX.")

	form.Validator.CheckField(isValidFileType(header.Filename), "resume", "Invalid file type. Please upload a DOCX, PDF, or TXT file.")

	if form.Validator.HasErrors() {
		if err := response.Page(w, http.StatusUnprocessableEntity, data, "pages/profile-resume-upload.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"template": "pages/profile-resume-upload.html",
				"action":   "response.Page",
			})
		}
		return
	}

	form.Filename = header.Filename

	tempFile, err := os.CreateTemp("", "resume-*."+filepath.Ext(header.Filename))
	if err != nil {
		form.Validator.AddFieldError("resume", "Failed to process file. Please try again.")
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":   "os.CreateTemp",
			"filename": header.Filename,
		})
		if err := response.Page(w, http.StatusUnprocessableEntity, data, "pages/profile-resume-upload.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"template": "pages/profile-resume-upload.html",
				"action":   "response.Page",
			})
		}
		return
	}
	defer os.Remove(tempFile.Name())

	_, err = io.Copy(tempFile, file)
	if err != nil {
		form.Validator.AddFieldError("resume", "Failed to process file. Please try again.")
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":   "copyFileToTemp",
			"filename": header.Filename,
		})
		if err := response.Page(w, http.StatusUnprocessableEntity, data, "pages/profile-resume-upload.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"template": "pages/profile-resume-upload.html",
				"action":   "renderTemplate",
			})
		}
		return
	}

	resumeText, err := app.extractResumeText(tempFile, header.Filename)
	if err != nil {
		form.Validator.AddFieldError("resume", fmt.Sprintf("Failed to extract text from resume: %v", err))
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":   "extractResumeText",
			"filename": header.Filename,
			"fileType": strings.ToLower(filepath.Ext(header.Filename)),
		})
		if err := response.Page(w, http.StatusUnprocessableEntity, data, "pages/profile-resume-upload.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"template": "pages/profile-resume-upload.html",
				"action":   "renderTemplate",
			})
		}
		return
	}

	user := contextGetAuthenticatedUser(r)
	resume := internal.Resume{
		RawText: resumeText,
	}
	if err := internal.CreateResume(r.Context(), app.fs, *user, &resume); err != nil {
		form.Validator.AddFieldError("resume", "Failed to save resume. Please try again.")
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":   "internal.CreateResume",
			"userID":   user.ID,
			"filename": header.Filename,
		})
		if err := response.Page(w, http.StatusUnprocessableEntity, data, "pages/profile-resume-upload.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
			internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
				"template": "pages/profile-resume-upload.html",
				"action":   "renderTemplate",
			})
		}
		return
	}

	userState := internal.UserState{
		Status:  internal.UserStatusProcessingResume,
		Started: time.Now(),
	}

	user.Phone = &form.Phone

	// Save the phone number and update state
	if err := internal.UpdateUser(context.Background(), app.fs, user.ID, map[string]any{
		"Phone": form.Phone,
		"State": userState,
	}); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to update user: %w", err))
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action": "internal.UpdateUser",
			"userID": user.ID,
			"phone":  form.Phone,
		})
		return
	}

	// Start resume processing in background
	go func() {
		if err := internal.AnnotateResume(context.Background(), app.fs, app.logger, user, &resume); err != nil {
			app.logger.Error("Failed to annotate resume", "error", err)
			internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
				"action":   "internal.AnnotateResume",
				"userID":   user.ID,
				"resumeID": resume.ID,
			})
		}
		internal.SendPusherRedirect(app.pusher, user.ID, "/dashboard")
	}()

	// Show processing page
	data = app.newTemplateData(r)
	if err := response.Page(w, http.StatusOK, data, "pages/profile-resume-processing.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render processing page: %w", err))
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"template": "pages/profile-resume-processing.html",
			"action":   "renderTemplate",
		})
	}
}

// Helper functions below

// extractResumeText extracts text content from a resume file.
// Supports both PDF, DOCX, and TXT formats.
func (app *application) extractResumeText(file *os.File, filename string) (string, error) {
	if strings.HasSuffix(strings.ToLower(filename), ".docx") {
		return extractDOCX(file)
	} else if strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return extractPDF(file)
	} else if strings.HasSuffix(strings.ToLower(filename), ".txt") {
		return extractTXT(file)
	}
	return "", fmt.Errorf("unsupported file type")
}

func extractDOCX(file *os.File) (string, error) {
	fileinfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("extractDOCX: failed to get file info")
	}

	size := fileinfo.Size()

	doc, err := docx.Parse(file, size)
	if err != nil {
		return "", fmt.Errorf("extractDOCX: failed to parse DOCX file")
	}

	sb := strings.Builder{}
	for _, it := range doc.Document.Body.Items {
		switch it.(type) {
		case *docx.Paragraph, *docx.Table: // printable
			s, ok := it.(fmt.Stringer)
			if !ok {
				return "", fmt.Errorf("extractDOCX: failed to convert inteface to string")
			}
			sb.WriteString(s.String())
		}
	}

	println(sb.String())

	return sb.String(), nil
}

func extractPDF(file *os.File) (string, error) {
	// Get file info for size
	fileinfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("extractPDF: failed to get file info: %w", err)
	}

	// Create a reader for the PDF
	reader, err := pdf.NewReader(file, fileinfo.Size())
	if err != nil {
		return "", fmt.Errorf("extractPDF: failed to create PDF reader: %w", err)
	}

	var sb strings.Builder
	numPages := reader.NumPage()

	// Extract text from each page
	for pageNum := 1; pageNum <= numPages; pageNum++ {
		page := reader.Page(pageNum)
		if page.V.IsNull() {
			continue // Skip empty pages
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("extractPDF: failed to extract text from page %d: %w", pageNum, err)
		}
		sb.WriteString(text)
	}

	return sb.String(), nil
}

func extractTXT(file *os.File) (string, error) {
	// Reset file pointer to beginning
	if _, err := file.Seek(0, 0); err != nil {
		return "", fmt.Errorf("extractTXT: failed to reset file pointer: %w", err)
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("extractTXT: failed to read file: %w", err)
	}

	return string(content), nil
}
