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
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/onboarding"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/rodeo"
	"talent-rodeo/internal/validator"
)

type initialOnboarding struct {
	Phone     string
	Filename  string
	Validator validator.Validator
}

func isValidFileType(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".docx" || ext == ".pdf"
}

func (app *application) resetResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		app.badRequest(w, r, fmt.Errorf("resetResume: method not allowed"))
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := app.db.UpdateFullResume(user.ID, "", ""); err != nil {
		app.serverError(w, r, fmt.Errorf("resetResume: failed to reset resume: %w", err))
		return
	}

	// Reset the onboarding state
	if err := app.db.UpdateOnboardingStep(user.ID, onboarding.StepInitial); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to reset onboarding state: %w", err))
		return
	}

	// If it's an HTMX request, return the profile page content
	if r.Header.Get("HX-Request") == "true" {
		data := app.newTemplateData(r)
		err := response.Page(w, http.StatusOK, data, "pages/profile.html")
		if err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	// For non-HTMX requests, use HX-Refresh
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
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
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data := app.newTemplateData(r)
	data["Form"] = &initialOnboarding{
		Validator: validator.Validator{},
	}

	err := response.Page(w, http.StatusOK, data, "pages/profile-resume-upload.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}
