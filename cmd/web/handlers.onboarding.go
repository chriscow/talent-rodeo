package main

import (
	"fmt"
	"net/http"
	"talent-rodeo/experimental/rodeo"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/onboarding"
	"talent-rodeo/internal/response"
)

// resetOnboardingState handles POST requests to reset a user's onboarding progress.
// It sets the user back to the initial onboarding step and refreshes the profile
// resume section of the page.
//
// Flow:
//  1. Verifies user authentication
//  2. Resets onboarding step to initial
//  3. Re-renders the profile resume partial template
//
// Routes:
//   - POST /profile/onboarding/reset
//
// Templates:
//   - partial:profile-resume (partials/*.html)
//
// State Transitions:
//   - Any State → StepInitial
func (app *application) resetOnboardingState(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		app.badRequest(w, r, fmt.Errorf("resetOnboardingState: user not found"))
		return
	}

	if err := app.db.UpdateOnboardingStep(user.ID, onboarding.StepInitial); err != nil {
		app.serverError(w, r, fmt.Errorf("resetOnboardingState: failed to update onboarding: %w", err))
		return
	}

	data := app.newTemplateData(r)
	if err := response.NamedTemplateWithHeaders(w, http.StatusOK, data, nil, "partial:profile-resume", "partials/*.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("resetOnboardingState: failed to render template: %w", err))
	}
}

// processResumeKeywords initiates the background processing of a user's resume
// to extract relevant keywords. This is called after a successful resume upload
// to enhance the user's profile with searchable skills and experience.
//
// Flow:
//  1. Validates session ID
//  2. Starts background task for keyword extraction
//  3. Updates user with real-time status via websocket
//  4. Saves extracted keywords to database
//
// Dependencies:
//   - rodeo.GetResumeKeywords for keyword extraction
//   - database.UpdateUserKeywords for storing results
//   - app.SendStatus for real-time updates
//
// State:
//   - Does not modify onboarding state directly
//   - Called during StepResumeUploaded transition
func (app *application) processResumeKeywords(r *http.Request, user *database.User, resumeText string) error {
	data := app.newTemplateData(r)
	sessionID, ok := data["SessionID"].(string)
	if !ok {
		return fmt.Errorf("processResumeKeywords: sessionID not found")
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
