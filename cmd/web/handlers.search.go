package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"talent-rodeo/internal/handlers"
	"talent-rodeo/internal/handlers/search"
	"talent-rodeo/internal/response"
	"time"

	"github.com/alexedwards/flow"
	"github.com/nats-io/nats.go/jetstream"
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

	// Return the search progress page
	data := app.newTemplateData(r)
	data["CandidateID"] = candidateID
	data["MustHaveSkills"] = mustHaveSkills
	data["PreferredSkills"] = preferredSkills
	err = response.Page(w, http.StatusOK, data, "pages/search-progress.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}

// mockSearchProgress simulates a search in progress for testing
func (app *application) mockSearchProgress(w http.ResponseWriter, r *http.Request) {
	app.logger.Debug("mock search progress request started",
		"method", r.Method,
		"url", r.URL.String(),
		"headers", r.Header)

	// Get authenticated user
	user := contextGetAuthenticatedUser(r)
	app.logger.Debug("authenticated user check",
		"user", user != nil,
		"user_details", user)
	if user == nil {
		app.logger.Error("no authenticated user found")
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}

	// Validate CSRF token
	token := r.Header.Get("X-CSRF-Token")
	app.logger.Debug("csrf validation",
		"token_present", token != "",
		"token", token)

	// Get session ID from cookie
	session, err := app.sessionStore.Get(r, "session")
	if err != nil {
		app.logger.Error("failed to get session",
			"error", err,
			"cookies", r.Cookies())
		app.serverError(w, r, fmt.Errorf("failed to get session: %w", err))
		return
	}

	app.logger.Debug("session details",
		"is_new", session.IsNew,
		"values", session.Values,
		"options", session.Options)

	// Generate a session ID if one doesn't exist
	sessionID, ok := session.Values["SessionID"].(string)
	app.logger.Debug("session ID check",
		"session_id_exists", ok,
		"session_id", sessionID,
		"raw_value", session.Values["SessionID"])

	if !ok || sessionID == "" {
		sessionID = fmt.Sprintf("mock-%d-%d", user.ID, time.Now().UnixNano())
		session.Values["SessionID"] = sessionID
		app.logger.Debug("generated new session ID",
			"session_id", sessionID)

		err = session.Save(r, w)
		if err != nil {
			app.logger.Error("failed to save session",
				"error", err,
				"session_id", sessionID,
				"session_values", session.Values)
			app.serverError(w, r, fmt.Errorf("failed to save session: %w", err))
			return
		}
		app.logger.Debug("session saved successfully")
	}

	// Validate NATS connection
	if app.jetstream == nil {
		app.logger.Error("jetstream not initialized")
		app.serverError(w, r, fmt.Errorf("jetstream not initialized"))
		return
	}

	// Create KV store for tracking progress
	kv, err := app.getKV(jetstream.KeyValueConfig{
		Bucket:      "search_progress",
		Description: "Search Progress Tracking",
		TTL:         5 * time.Minute,
		MaxBytes:    1024 * 1024, // 1MB
	})
	if err != nil {
		app.logger.Error("failed to create KV store",
			"error", err)
		app.serverError(w, r, fmt.Errorf("failed to create KV store: %w", err))
		return
	}

	app.logger.Debug("mock search progress configuration",
		"user_id", user.ID,
		"session_id", sessionID,
		"kv_bucket", "search_progress")

	// Simulate search progress in a goroutine
	go func() {
		app.logger.Debug("starting mock search simulation",
			"session_id", sessionID)

		// Simulate finding 100 total jobs over 10 updates
		totalJobs := 100
		analyzedJobs := 0
		goodFitJobs := 0

		for i := 1; i <= 10; i++ {
			time.Sleep(1 * time.Second) // Simulate processing time

			// Update progress
			analyzedJobs = i * 10
			goodFitJobs = i * 2 // 20% of analyzed jobs are good fits

			progress := search.ProgressUpdate{
				TotalJobs:       totalJobs,
				AnalyzedJobs:    analyzedJobs,
				GoodFitJobs:     goodFitJobs,
				Status:          fmt.Sprintf("Analyzing job %d of %d...", analyzedJobs, totalJobs),
				IsComplete:      i == 10,
				PercentComplete: float64(analyzedJobs) / float64(totalJobs) * 100,
			}

			// Add mock results when complete
			if progress.IsComplete {
				progress.Status = "Search complete!"
				progress.Results = []search.JobResult{
					{
						Title:      "Senior Go Developer",
						Company:    "TechCorp",
						Location:   "Remote",
						MatchScore: 95,
						SkillMatch: map[string]bool{
							"golang":     true,
							"docker":     true,
							"kubernetes": true,
						},
					},
					{
						Title:      "Backend Engineer",
						Company:    "StartupCo",
						Location:   "San Francisco",
						MatchScore: 85,
						SkillMatch: map[string]bool{
							"golang": true,
							"python": true,
						},
					},
				}
				app.logger.Debug("mock search complete",
					"num_results", len(progress.Results))
			}

			// Store progress in KV store
			progressJSON, err := json.Marshal(progress)
			if err != nil {
				app.logger.Error("failed to marshal progress update",
					"error", err,
					"progress", progress)
				return
			}

			_, err = kv.Put(context.Background(), sessionID, progressJSON)
			if err != nil {
				app.logger.Error("failed to store progress update",
					"error", err,
					"session_id", sessionID)
				return
			}

			app.logger.Debug("progress update stored",
				"session_id", sessionID,
				"iteration", i,
				"percent_complete", progress.PercentComplete)
		}
	}()

	// Return the search progress page
	data := app.newTemplateData(r)
	data["SessionID"] = sessionID
	data["MustHaveSkills"] = []string{"golang", "docker"}
	data["PreferredSkills"] = []string{"kubernetes", "python"}

	app.logger.Debug("rendering search progress page",
		"session_id", sessionID,
		"template_data", data)

	err = response.Page(w, http.StatusOK, data, "pages/search-progress.html")
	if err != nil {
		app.logger.Error("failed to render search progress page",
			"error", err,
			"session_id", sessionID)
		app.serverError(w, r, err)
		return
	}

	app.logger.Debug("mock search progress request completed successfully",
		"session_id", sessionID)
}

// getSearchProgress returns the current search progress from the KV store
func (app *application) getSearchProgress(w http.ResponseWriter, r *http.Request) {
	// Get session ID from URL
	sessionID := flow.Param(r.Context(), "id")
	if sessionID == "" {
		app.logger.Error("no session ID provided")
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	app.logger.Debug("fetching search progress",
		"session_id", sessionID)

	// Get KV store
	kv, err := app.getKV(jetstream.KeyValueConfig{
		Bucket:      "search_progress",
		Description: "Search Progress Tracking",
		TTL:         5 * time.Minute,
		MaxBytes:    1024 * 1024, // 1MB
	})
	if err != nil {
		app.logger.Error("failed to get KV store",
			"error", err)
		app.serverError(w, r, fmt.Errorf("failed to get KV store: %w", err))
		return
	}

	// Get progress from KV store
	entry, err := kv.Get(r.Context(), sessionID)
	if err != nil {
		app.logger.Error("failed to get progress from KV store",
			"error", err,
			"session_id", sessionID)
		app.serverError(w, r, fmt.Errorf("failed to get progress: %w", err))
		return
	}

	// Log the response data
	var progress search.ProgressUpdate
	if err := json.Unmarshal(entry.Value(), &progress); err != nil {
		app.logger.Error("failed to unmarshal progress data",
			"error", err,
			"raw_data", string(entry.Value()))
		app.serverError(w, r, fmt.Errorf("failed to unmarshal progress: %w", err))
		return
	}

	app.logger.Debug("returning search progress",
		"session_id", sessionID,
		"progress", progress,
		"percent_complete", progress.PercentComplete,
		"is_complete", progress.IsComplete,
		"analyzed_jobs", progress.AnalyzedJobs,
		"good_fit_jobs", progress.GoodFitJobs)

	// Return progress as JSON
	w.Header().Set("Content-Type", "application/json")
	w.Write(entry.Value())
}
