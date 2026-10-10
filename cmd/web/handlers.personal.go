package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"time"
)

type personalPreferencesForm struct {
	TargetRoles   string
	Locations     string
	WorkTypes     []string
	Seniority     string
	DesiredTraits string
	AvoidedTraits string
	Timezone      string
	Confirmed     bool
	Errors        map[string]string
}

func (f personalPreferencesForm) HasWorkType(value string) bool {
	for _, selected := range f.WorkTypes {
		if selected == value {
			return true
		}
	}
	return false
}

func (app *application) personalPreferencesGet(w http.ResponseWriter, r *http.Request) {
	if !app.mode.Personal {
		http.NotFound(w, r)
		return
	}
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusSeeOther)
		return
	}
	if !app.mode.AllowsUser(user.ID) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if user.Status == internal.UserStatusProcessingResume {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}
	resume, found, err := internal.GetLatestResume(r.Context(), app.fs, user.ID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get resume for preferences: %w", err))
		return
	}
	if !found || !resume.IsValid {
		http.Redirect(w, r, "/profile/resume/upload", http.StatusSeeOther)
		return
	}
	form := personalPreferencesForm{Errors: make(map[string]string)}
	if user.SteeringQuery != nil {
		form.TargetRoles = strings.Join(user.SteeringQuery.Queries, "\n")
		form.Seniority = string(user.SteeringQuery.ExperienceLevel)
		for _, workType := range user.SteeringQuery.WorkTypes {
			form.WorkTypes = append(form.WorkTypes, string(workType))
		}
	}
	if user.LocationPreference != nil {
		form.Locations = *user.LocationPreference
	}
	if user.Timezone != nil {
		form.Timezone = *user.Timezone
	}
	version, err := internal.GetLatestStatement(r.Context(), app.fs, user.ID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get preferences source: %w", err))
		return
	}
	if version != nil {
		if version.SourceData["source_type"] != "profile" {
			versions, _, err := internal.GetStatementHistory(r.Context(), app.fs, user.ID, 1000, nil)
			if err != nil {
				app.serverError(w, r, fmt.Errorf("find confirmed profile source: %w", err))
				return
			}
			for _, candidate := range versions {
				if candidate.GeneratedBy == "profile" && candidate.SourceData["source_type"] == "profile" {
					version = &candidate
					break
				}
			}
		}
		if source, ok := version.SourceData["preferences"].(map[string]any); ok {
			if value, ok := source["desired_traits"].(string); ok {
				form.DesiredTraits = value
			}
			if value, ok := source["avoided_traits"].(string); ok {
				form.AvoidedTraits = value
			}
		}
	}
	app.renderPersonalPreferences(w, r, http.StatusOK, form)
}

func (app *application) personalPreferencesPost(w http.ResponseWriter, r *http.Request) {
	if !app.mode.Personal {
		http.NotFound(w, r)
		return
	}
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusSeeOther)
		return
	}
	if !app.mode.AllowsUser(user.ID) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if user.Status == internal.UserStatusProcessingResume {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid preferences form", http.StatusBadRequest)
		return
	}
	form := personalPreferencesForm{
		TargetRoles:   strings.TrimSpace(r.PostForm.Get("target_roles")),
		Locations:     strings.TrimSpace(r.PostForm.Get("locations")),
		WorkTypes:     r.PostForm["work_types"],
		Seniority:     strings.TrimSpace(r.PostForm.Get("seniority")),
		DesiredTraits: strings.TrimSpace(r.PostForm.Get("desired_traits")),
		AvoidedTraits: strings.TrimSpace(r.PostForm.Get("avoided_traits")),
		Timezone:      strings.TrimSpace(r.PostForm.Get("timezone")),
		Confirmed:     r.PostForm.Get("confirm") == "on",
		Errors:        make(map[string]string),
	}
	preferences := internal.PersonalPreferences{
		TargetRoles:   splitPersonalRoles(form.TargetRoles),
		Locations:     form.Locations,
		Seniority:     internal.ExperienceLevel(form.Seniority),
		DesiredTraits: form.DesiredTraits,
		AvoidedTraits: form.AvoidedTraits,
		Timezone:      form.Timezone,
	}
	for _, value := range form.WorkTypes {
		preferences.WorkTypes = append(preferences.WorkTypes, internal.WorkType(value))
	}
	if !form.Confirmed {
		form.Errors["confirm"] = "Confirm these preferences before starting your search."
	}
	if err := preferences.Validate(); err != nil {
		form.Errors["preferences"] = err.Error()
	}
	if len(form.Errors) > 0 {
		app.renderPersonalPreferences(w, r, http.StatusUnprocessableEntity, form)
		return
	}
	resume, found, err := internal.GetLatestResume(r.Context(), app.fs, user.ID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get resume for confirmation: %w", err))
		return
	}
	if !found || !resume.IsValid {
		http.Redirect(w, r, "/profile/resume/upload", http.StatusSeeOther)
		return
	}
	geoID, err := internal.ExtractGeoID(r.Context(), app.fs, &user.ID, preferences.Locations)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("resolve personal search location: %w", err))
		return
	}
	if strings.TrimSpace(geoID) == "" {
		app.serverError(w, r, fmt.Errorf("location preference did not resolve to a search area"))
		return
	}
	criteria, err := app.preparePersonalCriteria(r, preferences, geoID)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("prepare personal search criteria: %w", err))
		return
	}
	criteriaIDs := make([]string, 0, len(criteria))
	for _, item := range criteria {
		criteriaIDs = append(criteriaIDs, item.ID)
	}
	activation, err := internal.SavePersonalProfile(r.Context(), app.fs, user.ID, *resume, preferences, criteriaIDs)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("save confirmed personal preferences: %w", err))
		return
	}
	if err := app.queuePersonalSearches(r, user.ID, criteria, activation); err != nil {
		app.serverError(w, r, fmt.Errorf("queue personal searches: %w", err))
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func splitPersonalRoles(value string) []string {
	var roles []string
	for _, item := range strings.Split(value, "\n") {
		for _, role := range strings.Split(item, ",") {
			role = strings.TrimSpace(role)
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	return roles
}

func (app *application) renderPersonalPreferences(w http.ResponseWriter, r *http.Request, status int, form personalPreferencesForm) {
	data := app.newTemplateData(r)
	data["Form"] = form
	if err := response.Page(w, status, data, "pages/personal-preferences.html"); err != nil {
		app.serverError(w, r, fmt.Errorf("render personal preferences: %w", err))
	}
}

func (app *application) preparePersonalCriteria(r *http.Request, p internal.PersonalPreferences, geoID string) ([]internal.SearchCriteria, error) {
	criteriaList := make([]internal.SearchCriteria, 0, len(p.TargetRoles)*len(p.WorkTypes))
	for _, role := range p.TargetRoles {
		for _, workType := range p.WorkTypes {
			criteriaID := internal.MakeSearchCriteriaID(role, geoID, workType, p.Seniority)
			criteria, found, err := internal.GetSearchCriteria(r.Context(), app.fs, criteriaID)
			if err != nil {
				return nil, fmt.Errorf("get search criteria: %w", err)
			}
			if !found {
				criteria, err = internal.CreateSearchCriteria(r.Context(), app.fs, role, geoID, workType, p.Seniority, internal.DefaultMaxPages, internal.DefaultListingAge)
				if err != nil {
					criteria, found, err = internal.GetSearchCriteria(r.Context(), app.fs, criteriaID)
					if err != nil {
						return nil, fmt.Errorf("create search criteria: %w", err)
					}
					if !found {
						return nil, fmt.Errorf("search criteria %s unavailable after create", criteriaID)
					}
				}
			}
			criteriaList = append(criteriaList, *criteria)
		}
	}
	return criteriaList, nil
}

func (app *application) queuePersonalSearches(r *http.Request, userID string, criteria []internal.SearchCriteria, activation time.Time) error {
	for _, item := range criteria {
		_, err := internal.CreatePersonalSearchTask(r.Context(), app.fs, item, "scrapingdog", userID, activation)
		if err != nil && !errors.Is(err, internal.ErrSearchTaskExists) {
			return fmt.Errorf("create owner search task: %w", err)
		}
	}
	return nil
}
