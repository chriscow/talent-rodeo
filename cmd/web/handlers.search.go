package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"talent-rodeo/internal/handlers"
	"talent-rodeo/internal/handlers/search"
	"talent-rodeo/internal/response"
)

// startJobSearch handles the initial job search request.
// For free tier users, it generates 3 search pages.
func (app *application) startJobSearch(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}

	// Get candidate ID
	candidateID, err := app.db.GetCandidateIDForUser(r.Context(), user.ID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get candidate ID: %w", err))
		return
	}

	// Get candidate skills
	skills, err := app.db.GetCandidateSkills(r.Context(), candidateID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("failed to get skills: %w", err))
		return
	}

	// Categorize skills by preference
	var mustHaveSkills []string
	var preferredSkills []string
	for _, skill := range skills {
		switch skill.PreferenceValue {
		case 5:
			mustHaveSkills = append(mustHaveSkills, skill.Name)
		case 4:
			preferredSkills = append(preferredSkills, skill.Name)
		}
	}

	// For free tier, we'll create 3 search pages
	for page := 1; page <= 3; page++ {
		// Create search criteria
		searchMsg := search.SearchInput{
			CandidateID:     candidateID,
			MustHaveSkills:  mustHaveSkills,
			PreferredSkills: preferredSkills,
			Page:            page,
		}

		searchMsgBytes, err := json.Marshal(searchMsg)
		if err != nil {
			app.serverError(w, r, fmt.Errorf("failed to marshal search message: %w", err))
			return
		}

		// Schedule the message to be published immediately
		_, err = app.jetstream.Publish(
			r.Context(),
			handlers.TopicSearchInput,
			searchMsgBytes,
		)
		if err != nil {
			app.serverError(w, r, fmt.Errorf("failed to schedule search message: %w", err))
			return
		}
	}

	// Return success response
	response.JSON(w, http.StatusOK, map[string]any{
		"message": "Search started",
		"status":  "success",
	})
}
