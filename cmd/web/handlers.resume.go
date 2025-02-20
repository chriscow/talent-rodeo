package main

import (
	"fmt"
	"io"
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

type resumeUploadForm struct {
	Phone     string
	Filename  string
	Validator validator.Validator
}

// isValidFileType checks if the given filename has an allowed extension (.pdf or .docx).
// Used for resume upload validation.
func isValidFileType(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".pdf" || ext == ".docx"
}

// deleteResume handles DELETE requests to /profile/resume.
// It removes the user's resume and resets their onboarding state to initial.
// After successful deletion, redirects to the resume upload page.
// Requires: Authenticated user
func (app *application) deleteResume(w http.ResponseWriter, r *http.Request) {
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

// showUploadPage handles GET requests to /profile/resume/upload.
// Displays the resume upload form page.
// Template: pages/profile-resume-upload.html
// Requires: Authenticated user
func (app *application) showUploadPage(w http.ResponseWriter, r *http.Request) {
	app.logger.Info("Showing resume upload page", "method", r.Method, "url", r.URL.String())

	data := app.newTemplateData(r)
	if err := response.Page(w, http.StatusOK, data, "pages/profile-resume-upload.html"); err != nil {
		app.logger.Error("Failed to render resume upload page", "error", err)
		app.serverError(w, r, fmt.Errorf("failed to render resume upload page: %w", err))
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
	app.logger.Info("Processing resume upload", "method", r.Method, "url", r.URL.String())

	data := app.newTemplateData(r)
	form := &resumeUploadForm{
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

	app.processResumeKeywords(r, user, resumeText)

	app.logger.Info("Resume uploaded successfully", "filename", header.Filename, "phone", form.Phone)
	if err := response.Page(w, http.StatusOK, data, "pages/profile-schedule-phone-call.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("failed to render next page: %w", err))
	}
}

// Helper functions below

// extractResumeText extracts text content from a resume file.
// Supports both PDF and DOCX formats.
func (app *application) extractResumeText(file *os.File, filename string) (string, error) {
	if strings.HasSuffix(strings.ToLower(filename), ".docx") {
		return rodeo.ExtractDOCX(file)
	} else if strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return rodeo.ExtractPDF(file)
	}
	return "", fmt.Errorf("unsupported file type")
}

// updateUserResume updates a user's resume information in the database
// and updates the user struct with the new resume data.
func (app *application) updateUserResume(user *database.User, filename, resumeText string) error {
	if err := app.db.UpdateFullResume(user.ID, filename, resumeText); err != nil {
		return fmt.Errorf("failed to save resume: %w", err)
	}
	user.FullResume = &resumeText
	user.ResumeFilename = &filename
	return nil
}

// SendStatus sends a status update through the pusher service.
// Used to provide real-time feedback during resume processing.
func (app *application) SendStatus(sessionID, eventName, level, message string) {
	app.logger.Info("SendStatus", "sessionID", sessionID, "eventName", eventName, "level", level, "message", message)

	/*
		err := app.pusher.Trigger(sessionID, eventName, map[string]string{
			"level":   level,
			"message": message,
		})

		if err != nil {
			app.logger.Error("SendStatus: failed to send status", "error", err)
		}
	*/
}
