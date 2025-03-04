package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"talent-rodeo/internal/request"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/validator"

	"github.com/alexedwards/flow"
)

type skillPreferenceData struct {
	SkillID         string `form:"SkillID"`
	PreferenceValue int    `form:"PreferenceValue"`
	IsSearchTerm    bool   `form:"IsSearchTerm"`
	validator.Validator
}

type addSkillData struct {
	Name string `form:"Name"`
	validator.Validator
}

// handleSkill is a consolidated handler for skill operations that uses HTTP verbs
// to determine the action to take.
//
// Routes:
//   - PUT /profile/skills/:id - Update single skill preference
//   - DELETE /profile/skills/:id - Delete a skill
//
// Templates:
//   - partials/skill-item.html (for DELETE responses)
func (app *application) handleSkill(w http.ResponseWriter, r *http.Request) {
	logger := app.logger.With(
		slog.String("handler", "handleSkill"),
		slog.String("method", r.Method),
	)

	// Get authenticated user
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		if r.Method == http.MethodDelete {
			response.JSON(w, http.StatusUnauthorized, map[string]interface{}{
				"error": "Unauthorized",
			})
		} else {
			http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		}
		return
	}

	// Get candidate ID
	candidateID, err := app.db.GetCandidateIDForUser(r.Context(), user.ID)
	if err != nil {
		logger.Error("failed to get candidate ID", "error", err)
		if r.Method == http.MethodDelete {
			response.JSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": "Failed to get candidate ID",
			})
		} else {
			app.serverError(w, r, fmt.Errorf("failed to get candidate ID: %w", err))
		}
		return
	}

	// Get skill ID from URL path for both PUT and DELETE
	skillID := flow.Param(r.Context(), "id")
	if skillID == "" {
		response.JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "skill ID is required",
		})
		return
	}

	// Check if the skill exists
	skills, err := app.db.GetCandidateSkills(r.Context(), candidateID)
	if err != nil {
		logger.Error("failed to get skills", "error", err)
		response.JSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to get skills",
		})
		return
	}

	skillExists := false
	for _, skill := range skills {
		if skill.ID == skillID {
			skillExists = true
			break
		}
	}

	if !skillExists {
		response.JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "Skill ID not found in candidate's skills",
		})
		return
	}

	switch r.Method {
	case http.MethodPut:
		// Parse form data
		if err := r.ParseForm(); err != nil {
			logger.Error("error parsing form",
				slog.String("error", err.Error()))
			app.badRequest(w, r, err)
			return
		}

		// Get preference value from form
		preferenceStr := r.FormValue("preference")
		if preferenceStr == "" {
			response.JSON(w, http.StatusBadRequest, map[string]interface{}{
				"error": "preference value is required",
			})
			return
		}

		// Parse preference value
		preferenceValue, err := strconv.Atoi(preferenceStr)
		if err != nil {
			logger.Error("error parsing preference value",
				slog.String("error", err.Error()),
				slog.String("value", preferenceStr))
			response.JSON(w, http.StatusBadRequest, map[string]interface{}{
				"error": "invalid preference value",
			})
			return
		}

		// Validate preference value
		if preferenceValue < 1 || preferenceValue > 5 {
			logger.Error("invalid preference value",
				slog.Int("value", preferenceValue))
			response.JSON(w, http.StatusBadRequest, map[string]interface{}{
				"error": "preference value must be between 1 and 5",
			})
			return
		}

		// Update single skill preference
		updates := map[string]struct {
			PreferenceValue int
			IsSearchTerm    bool
		}{
			skillID: {
				PreferenceValue: preferenceValue,
				IsSearchTerm:    preferenceValue > 3, // Consider "Prefer" and "Must Have" as search terms
			},
		}

		err = app.db.UpdateSkillPreferences(r.Context(), candidateID, updates)
		if err != nil {
			logger.Error("error updating skill preference",
				slog.String("error", err.Error()),
				slog.String("skill_id", skillID))
			app.serverError(w, r, fmt.Errorf("failed to update skill preference: %w", err))
			return
		}

		w.WriteHeader(http.StatusOK)

	case http.MethodDelete:
		// Delete skill
		err = app.db.DeleteCandidateSkill(r.Context(), candidateID, skillID)
		if err != nil {
			logger.Error("failed to delete skill", "error", err)
			response.JSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": "Failed to delete skill",
			})
			return
		}

		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// addSkill handles adding a new skill to a candidate's profile.
//
// Routes:
//   - POST /profile/skills/add
func (app *application) addSkill(w http.ResponseWriter, r *http.Request) {
	logger := app.logger.With(
		slog.String("handler", "handleAddSkill"),
	)

	// Check method first
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	// Get candidate ID for the user
	candidateID, err := app.db.GetCandidateIDForUser(r.Context(), user.ID)
	if err != nil {
		logger.Error("error getting candidate ID",
			slog.String("error", err.Error()),
			slog.Int("user_id", user.ID))
		app.serverError(w, r, fmt.Errorf("failed to get candidate ID: %w", err))
		return
	}

	var form addSkillData

	err = request.DecodePostForm(r, &form)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	// Log the form data
	logger.Debug("form data",
		slog.String("name", form.Name),
		slog.Any("form", form))

	template := "page:skills"
	if r.Header.Get("HX-Request") == "true" {
		template = "partial:skills"
	}

	// Validate the skill name
	form.CheckField(form.Name != "", "Name", "Skill name is required")
	if form.HasErrors() {
		data := app.newTemplateData(r)
		data["Form"] = form

		// Get all skills to include in the response
		skills, err := app.db.GetCandidateSkills(r.Context(), candidateID)
		if err != nil {
			logger.Error("error getting skills",
				slog.String("error", err.Error()),
				slog.Int("user_id", user.ID))
			app.serverError(w, r, fmt.Errorf("failed to get skills: %w", err))
			return
		}
		data["Skills"] = skills

		err = response.NamedTemplate(w, http.StatusOK, data, template, "partials/skills.html", "partials/skill-item.html")
		if err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	// Add the skill to the database
	_, err = app.db.AddCandidateSkill(r.Context(), candidateID, form.Name)
	if err != nil {
		logger.Error("error adding skill",
			slog.String("error", err.Error()),
			slog.String("skill_name", form.Name))
		app.serverError(w, r, fmt.Errorf("failed to add skill: %w", err))
		return
	}

	// Get all skills to include in the response
	skills, err := app.db.GetCandidateSkills(r.Context(), candidateID)
	if err != nil {
		logger.Error("error getting skills",
			slog.String("error", err.Error()),
			slog.Int("user_id", user.ID))
		app.serverError(w, r, fmt.Errorf("failed to get skills: %w", err))
		return
	}

	// Return the complete skills partial
	data := app.newTemplateData(r)
	data["Skills"] = skills
	err = response.NamedTemplate(w, http.StatusOK, data, template, "partials/skills.html", "partials/skill-item.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}
