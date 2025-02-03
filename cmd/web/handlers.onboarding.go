package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/onboarding"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/rodeo"
)

func (app *application) restartOnboarding(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.badRequest(w, r, fmt.Errorf("restartOnboarding: user not found"))
		return
	}

	if err := app.db.UpdateOnboardingStep(user.ID, onboarding.StepInitial); err != nil {
		app.serverError(w, r, fmt.Errorf("restartOnboarding: failed to update onboarding: %w", err))
		return
	}

	data := app.newTemplateData(r)
	if err := response.NamedTemplateWithHeaders(w, http.StatusOK, data, nil, "partial:profile-resume", "partials/*.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("restartOnboarding: failed to render template: %w", err))
	}
}

func (app *application) startOnboarding(r *http.Request, user *database.User, resumeText string) error {
	data := app.newTemplateData(r)
	sessionID, ok := data["SessionID"].(string)
	if !ok {
		return fmt.Errorf("startOnboarding: sessionID not found")
	}

	app.backgroundTask(r, func() error {
		app.SendStatus(sessionID, "status", "info", "Extracting keywords...")
		defer app.SendStatus(sessionID, "status", "success", "Keywords extracted")

		keywords, err := rodeo.GetResumeKeywords(resumeText)
		if err != nil {
			return fmt.Errorf("failed to extract keywords: %w", err)
		}

		userKeywords := make([]database.UserKeyword, len(keywords))
		for i, kw := range keywords {
			userKeywords[i] = database.UserKeyword{
				UserID:   user.ID,
				Keyword:  kw,
				Priority: i,
			}
		}

		if err := app.db.UpdateUserKeywords(user.ID, userKeywords); err != nil {
			return fmt.Errorf("failed to save keywords: %w", err)
		}

		return nil
	})

	return nil
}
