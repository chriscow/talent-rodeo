package main

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"talent-rodeo/experimental/rodeo"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/onboarding"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/validator"
)

type initialOnboarding struct {
	Phone     string
	Filename  string
	Validator validator.Validator
}

func isValidFileType(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".pdf" || ext == ".docx"
}

func (app *application) resetResume(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}

	// Reset resume fields
	err := app.db.UpdateFullResume(user.ID, "", "")
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to reset resume: %w", err))
		return
	}

	// Update onboarding state back to initial
	if err := app.db.UpdateOnboardingStep(user.ID, onboarding.StepInitial); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to update onboarding state: %w", err))
		return
	}

	http.Redirect(w, r, "/profile/resume/upload", http.StatusFound)
}

func (app *application) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := app.newTemplateData(r)
	form := &initialOnboarding{
		Validator: validator.Validator{},
	}
	data["Form"] = form

	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		form.Validator.AddFieldError("resume", "File too large. Maximum size is 10MB")
		if err := response.NamedTemplate(w, http.StatusUnprocessableEntity, data, "partial:resume-form", "partials/resume-form.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
		}
		return
	}

	file, header, err := r.FormFile("resume")
	if err != nil {
		form.Validator.AddFieldError("resume", "Please select a file")
		if err := response.NamedTemplate(w, http.StatusUnprocessableEntity, data, "partial:resume-form", "partials/resume-form.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
		}
		return
	}
	defer file.Close()

	form.Phone = r.FormValue("phone")
	form.Filename = header.Filename

	phoneRegex := regexp.MustCompile(`^\+?1?\d{10,}$`)
	form.Validator.CheckField(validator.Matches(form.Phone, phoneRegex), "phone", "Invalid phone number")
	form.Validator.CheckField(isValidFileType(header.Filename), "resume", "Invalid file type. Please upload a DOCX or PDF file.")

	if form.Validator.HasErrors() {
		if err := response.NamedTemplate(w, http.StatusUnprocessableEntity, data, "partial:resume-form", "partials/resume-form.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
		}
		return
	}

	tempFile, err := os.CreateTemp("", "resume-*."+filepath.Ext(header.Filename))
	if err != nil {
		form.Validator.AddFieldError("resume", "Failed to process file. Please try again.")
		if err := response.NamedTemplate(w, http.StatusUnprocessableEntity, data, "partial:resume-form", "partials/resume-form.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
		}
		return
	}
	defer os.Remove(tempFile.Name())

	_, err = io.Copy(tempFile, file)
	if err != nil {
		form.Validator.AddFieldError("resume", "Failed to process file. Please try again.")
		if err := response.NamedTemplate(w, http.StatusUnprocessableEntity, data, "partial:resume-form", "partials/resume-form.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
		}
		return
	}

	resumeText, err := app.extractResumeText(tempFile, header.Filename)
	if err != nil {
		form.Validator.AddFieldError("resume", fmt.Sprintf("Failed to extract text from resume: %v", err))
		if err := response.NamedTemplate(w, http.StatusUnprocessableEntity, data, "partial:resume-form", "partials/resume-form.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
		}
		return
	}

	user := contextGetAuthenticatedUser(r)
	if err := app.updateUserResume(user, header.Filename, resumeText); err != nil {
		form.Validator.AddFieldError("resume", "Failed to save resume. Please try again.")
		if err := response.NamedTemplate(w, http.StatusUnprocessableEntity, data, "partial:resume-form", "partials/resume-form.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
		}
		return
	}

	// Save the phone number
	if err := app.db.UpdateUserPhone(user.ID, form.Phone); err != nil {
		form.Validator.AddFieldError("phone", "Failed to save phone number. Please try again.")
		if err := response.NamedTemplate(w, http.StatusUnprocessableEntity, data, "partial:resume-form", "partials/resume-form.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
		}
		return
	}

	// Update onboarding state
	if err := app.db.UpdateOnboardingStep(user.ID, onboarding.StepResumeUploaded); err != nil {
		form.Validator.AddFieldError("resume", "Failed to update onboarding state. Please try again.")
		if err := response.NamedTemplate(w, http.StatusUnprocessableEntity, data, "partial:resume-form", "partials/resume-form.html"); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to render form with error: %w", err))
		}
		return
	}

	app.startOnboarding(r, user, resumeText)

	app.logger.Info("Resume uploaded successfully", "filename", header.Filename, "phone", form.Phone)
	if err := response.Page(w, http.StatusOK, data, "pages/profile-phone-call.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render next page: %w", err))
	}
}

func (app *application) parseResumeForm(r *http.Request) (*initialOnboarding, io.ReadCloser, *multipart.FileHeader, error) {
	phone := r.FormValue("phone")
	file, header, err := r.FormFile("resume")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get resume file: %w", err)
	}

	form := &initialOnboarding{
		Phone:     phone,
		Filename:  header.Filename,
		Validator: validator.Validator{},
	}

	phoneRegex := regexp.MustCompile(`^\+?1?\d{10,}$`)
	form.Validator.CheckField(validator.Matches(phone, phoneRegex), "Phone", "Invalid phone number")
	form.Validator.CheckField(isValidFileType(header.Filename), "Resume", "Invalid file type. Please upload a DOCX or PDF file.")

	return form, file, header, nil
}

func (app *application) handleFormErrors(w http.ResponseWriter, r *http.Request, v validator.Validator, data map[string]interface{}) {
	app.logger.Warn("Invalid form submission", "errors", v.Errors, "fieldErrors", v.FieldErrors)
	data["Errors"] = v.Errors
	if err := response.Page(w, http.StatusOK, data, "pages/profile.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) saveTempFile(file io.Reader) (*os.File, error) {
	tempFile, err := os.CreateTemp(os.TempDir(), "resume-*.docx")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(tempFile, file); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	return tempFile, nil
}

func (app *application) extractResumeText(file *os.File, filename string) (string, error) {
	if strings.HasSuffix(strings.ToLower(filename), ".docx") {
		return rodeo.ExtractDOCX(file)
	} else if strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return rodeo.ExtractPDF(file)
	}
	return "", fmt.Errorf("unsupported file type")
}

func (app *application) updateUserResume(user *database.User, filename, resumeText string) error {
	if err := app.db.UpdateFullResume(user.ID, filename, resumeText); err != nil {
		return fmt.Errorf("failed to save resume: %w", err)
	}
	user.FullResume = &resumeText
	user.ResumeFilename = &filename
	return nil
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

func (app *application) resumeUploadPage(w http.ResponseWriter, r *http.Request) {
	app.logger.Info("Handling resume upload request", "method", r.Method, "url", r.URL.String())

	if r.Method == http.MethodGet {
		data := app.newTemplateData(r)
		if err := response.Page(w, http.StatusOK, data, "pages/profile-resume-upload.html"); err != nil {
			app.logger.Error("Failed to render resume upload page", "error", err)
			app.serverError(w, r, fmt.Errorf("failed to render resume upload page: %w", err))
		}
		return
	}

	if r.Method != http.MethodPost {
		app.logger.Info("Invalid method", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Max upload size of 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		app.logger.Error("Failed to parse multipart form", "error", err)
		app.badRequest(w, r, fmt.Errorf("file too large: %w", err))
		return
	}

	// Get phone number
	phone := r.FormValue("phone")
	if phone == "" {
		app.logger.Info("Missing phone number")
		app.badRequest(w, r, fmt.Errorf("phone number is required"))
		return
	}

	// Get resume file
	file, header, err := r.FormFile("resume")
	if err != nil {
		app.logger.Error("Failed to get resume file", "error", err)
		app.badRequest(w, r, fmt.Errorf("resume file is required"))
		return
	}
	defer file.Close()

	// Validate file type
	if !isValidFileType(header.Filename) {
		app.logger.Info("Invalid file type", "filename", header.Filename)
		app.badRequest(w, r, fmt.Errorf("invalid file type: only PDF and DOCX files are allowed"))
		return
	}

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		app.logger.Error("Failed to read file", "error", err)
		app.serverError(w, r, fmt.Errorf("failed to read file: %w", err))
		return
	}

	// Get authenticated user
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.logger.Info("User not authenticated, redirecting to login")
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}

	// Update user's resume
	contentStr := string(content)
	err = app.db.UpdateFullResume(user.ID, header.Filename, contentStr)
	if err != nil {
		app.logger.Error("Failed to update resume", "error", err, "user_id", user.ID)
		app.serverError(w, r, fmt.Errorf("failed to update resume: %w", err))
		return
	}

	// Update onboarding state
	if err := app.db.UpdateOnboardingStep(user.ID, onboarding.StepResumeUploaded); err != nil {
		app.logger.Error("Failed to update onboarding state", "error", err, "user_id", user.ID)
		app.serverError(w, r, fmt.Errorf("failed to update onboarding state: %w", err))
		return
	}

	app.logger.Info("Resume uploaded successfully", "user_id", user.ID, "filename", header.Filename)
	http.Redirect(w, r, "/profile", http.StatusFound)
}
