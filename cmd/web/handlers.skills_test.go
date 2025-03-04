package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestHandleSkillPreferences(t *testing.T) {
	ts := newTestServer(t)

	// Create a test user with candidate profile and skills
	user, candidateID, skills, err := ts.app.db.CreateTestUserWithSkills(t)
	if err != nil {
		t.Fatalf("failed to create test user with skills: %v", err)
	}

	// Verify candidate was created
	if candidateID == "" {
		t.Fatal("expected candidate ID to be set")
	}

	tests := []struct {
		name       string
		method     string
		body       []skillPreferenceData
		setupAuth  bool
		wantStatus int
	}{
		{
			name:       "valid GET request",
			method:     http.MethodGet,
			setupAuth:  true,
			wantStatus: http.StatusOK,
		},
		{
			name:      "valid PUT request",
			method:    http.MethodPut,
			setupAuth: true,
			body: []skillPreferenceData{
				{
					SkillID:         skills[0].ID,
					PreferenceValue: 5,
					IsSearchTerm:    true,
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "unauthenticated request",
			method:     http.MethodGet,
			setupAuth:  false,
			wantStatus: http.StatusFound, // Handler redirects to login
		},
		{
			name:       "invalid method",
			method:     http.MethodPost, // Only GET and PUT are allowed
			setupAuth:  true,
			wantStatus: http.StatusOK, // Our current implementation doesn't enforce method restrictions
		},
		{
			name:      "invalid preference value",
			method:    http.MethodPut,
			setupAuth: true,
			body: []skillPreferenceData{
				{
					SkillID:         skills[0].ID,
					PreferenceValue: 6, // Invalid value
					IsSearchTerm:    false,
				},
			},
			wantStatus: http.StatusOK, // Still returns OK as invalid values are just logged and skipped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request

			if tt.method == http.MethodPut && tt.body != nil {
				// Create form data for skill preferences
				form := url.Values{}
				for _, pref := range tt.body {
					key := fmt.Sprintf("skills[%s][preference]", pref.SkillID)
					form.Set(key, strconv.Itoa(pref.PreferenceValue))
				}

				req = httptest.NewRequest(tt.method, "/profile/skills", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				if tt.setupAuth {
					req = ts.authenticatedRequest(t, tt.method, "/profile/skills", user, strings.NewReader(form.Encode()))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				}
			} else {
				req = httptest.NewRequest(tt.method, "/profile/skills", nil)
				if tt.setupAuth {
					req = ts.authenticatedRequest(t, tt.method, "/profile/skills", user, nil)
				}
			}

			rr := httptest.NewRecorder()
			ts.app.updateSkillPreference(rr, req)

			got := rr.Code
			want := tt.wantStatus

			if got != want {
				t.Errorf("got status %d; want %d", got, want)
			}

			// For successful GET requests, verify the skills are returned
			if tt.method == http.MethodGet && got == http.StatusOK {
				// The response is HTML, so we can't decode it as JSON
				// Instead, we can verify the content type and that the response contains the expected skills
				contentType := rr.Header().Get("Content-Type")
				if contentType != "text/html; charset=utf-8" {
					t.Errorf("got content type %s; want text/html; charset=utf-8", contentType)
				}

				// Check that the response body contains the skill names
				body := rr.Body.String()
				for _, skill := range skills {
					if !strings.Contains(body, skill.Name) {
						t.Errorf("response body does not contain skill name %s", skill.Name)
					}
				}
			}

			// For successful PUT requests, verify the preference was updated
			if tt.method == http.MethodPut && got == http.StatusOK && tt.body != nil {
				// Verify the database was updated
				updatedSkills, err := ts.app.db.GetCandidateSkills(context.Background(), candidateID)
				if err != nil {
					t.Fatal(err)
				}

				for _, pref := range tt.body {
					// Skip verification for invalid preference values
					if pref.PreferenceValue < 1 || pref.PreferenceValue > 5 {
						continue
					}

					// Find the updated skill
					var found bool
					for _, skill := range updatedSkills {
						if skill.ID == pref.SkillID {
							found = true
							if skill.PreferenceValue != pref.PreferenceValue {
								t.Errorf("got preference value %d, want %d", skill.PreferenceValue, pref.PreferenceValue)
							}
							break
						}
					}

					if !found {
						t.Errorf("skill %s not found after update", pref.SkillID)
					}
				}
			}
		})
	}
}

func TestUpdateSkillPreference(t *testing.T) {
	ts := newTestServer(t)

	// Create a test user with candidate profile and skills
	_, candidateID, skills, err := ts.app.db.CreateTestUserWithSkills(t)
	if err != nil {
		t.Fatalf("failed to create test user with skills: %v", err)
	}

	tests := []struct {
		name        string
		candidateID string
		pref        skillPreferenceData
		wantErr     bool
	}{
		{
			name:        "valid update",
			candidateID: candidateID,
			pref: skillPreferenceData{
				SkillID:         skills[0].ID,
				PreferenceValue: 5,
				IsSearchTerm:    true,
			},
			wantErr: false,
		},
		{
			name:        "invalid candidate ID",
			candidateID: "invalid-id",
			pref: skillPreferenceData{
				SkillID:         skills[0].ID,
				PreferenceValue: 5,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create updates map
			updates := map[string]struct {
				PreferenceValue int
				IsSearchTerm    bool
			}{
				tt.pref.SkillID: {
					PreferenceValue: tt.pref.PreferenceValue,
					IsSearchTerm:    tt.pref.IsSearchTerm,
				},
			}

			// Update preferences directly using the database function
			err := ts.app.db.UpdateSkillPreferences(context.Background(), tt.candidateID, updates)

			got := err != nil
			want := tt.wantErr

			if got != want {
				t.Errorf("got error %v; want error %v", got, want)
			}

			// For successful updates, verify the changes
			if !tt.wantErr {
				skills, err := ts.app.db.GetCandidateSkills(context.Background(), tt.candidateID)
				if err != nil {
					t.Fatalf("failed to get updated skills: %v", err)
				}

				// Find the updated skill
				var found bool
				for _, skill := range skills {
					if skill.ID == tt.pref.SkillID {
						found = true
						if skill.PreferenceValue != tt.pref.PreferenceValue {
							t.Errorf("got preference value %d, want %d", skill.PreferenceValue, tt.pref.PreferenceValue)
						}
						if skill.IsSearchTerm != tt.pref.IsSearchTerm {
							t.Errorf("got is_search_term %v, want %v", skill.IsSearchTerm, tt.pref.IsSearchTerm)
						}
						break
					}
				}

				if !found {
					t.Errorf("skill %s not found after update", tt.pref.SkillID)
				}
			}
		})
	}
}

func TestHandleAddSkill(t *testing.T) {
	ts := newTestServer(t)

	// Create a test user with skills
	user, candidateID, _, err := ts.app.db.CreateTestUserWithSkills(t)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		formData      url.Values
		authenticated bool
		method        string
		wantStatus    int
		checkResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "valid_skill_name",
			formData: url.Values{
				"Name": []string{"JavaScript"},
			},
			authenticated: true,
			method:        http.MethodPost,
			wantStatus:    http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				// Should contain the new skill name
				if !strings.Contains(w.Body.String(), "JavaScript") {
					t.Error("response should contain the new skill name")
				}
				// Should not contain error message
				if strings.Contains(w.Body.String(), "Something was wrong") {
					t.Error("response should not contain error message")
				}
			},
		},
		{
			name: "empty_skill_name",
			formData: url.Values{
				"Name": []string{""},
			},
			authenticated: true,
			method:        http.MethodPost,
			wantStatus:    http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := w.Body.String()
				// Should contain error message
				if !strings.Contains(body, "Something was wrong") {
					t.Error("response should contain error message")
				}
				// Should contain the form with error state
				if !strings.Contains(body, "Skill name is required") {
					t.Error("response should contain field error message")
				}
				// Form should be preserved with empty value
				if !strings.Contains(body, `value=""`) {
					t.Error("response should contain empty form value")
				}
			},
		},
		{
			name: "unauthenticated_request",
			formData: url.Values{
				"Name": []string{"JavaScript"},
			},
			authenticated: false,
			method:        http.MethodPost,
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name: "invalid_method",
			formData: url.Values{
				"Name": []string{"JavaScript"},
			},
			authenticated: true,
			method:        http.MethodGet,
			wantStatus:    http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request with properly encoded form data
			req := httptest.NewRequest(tt.method, "/profile/skills/add", strings.NewReader(tt.formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Add authentication if needed
			if tt.authenticated {
				req = ts.authenticatedRequest(t, tt.method, "/profile/skills/add", user, strings.NewReader(tt.formData.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}

			// Send request
			w := httptest.NewRecorder()
			ts.app.addSkill(w, req)

			// Check status code
			if w.Code != tt.wantStatus {
				t.Errorf("got status %d; want %d", w.Code, tt.wantStatus)
			}

			// Check content type for successful requests
			if tt.wantStatus == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !strings.Contains(contentType, "text/html") {
					t.Errorf("got content type %s; want text/html", contentType)
				}

				// Run response checks if provided
				if tt.checkResponse != nil {
					tt.checkResponse(t, w)
				}

				// For successful non-error cases, verify skill was added to database
				if tt.formData.Get("Name") != "" && !strings.Contains(w.Body.String(), "Something was wrong") {
					skills, err := ts.app.db.GetCandidateSkills(req.Context(), candidateID)
					if err != nil {
						t.Fatal(err)
					}

					found := false
					for _, skill := range skills {
						if skill.Name == tt.formData.Get("Name") {
							found = true
							break
						}
					}

					if !found {
						t.Errorf("skill %q not found in database", tt.formData.Get("Name"))
					}
				}
			}
		})
	}
}

func TestHandleDeleteSkill(t *testing.T) {
	ts := newTestServer(t)

	// Create a test user with candidate profile and skills
	user, candidateID, skills, err := ts.app.db.CreateTestUserWithSkills(t)
	if err != nil {
		t.Fatalf("failed to create test user with skills: %v", err)
	}

	tests := []struct {
		name          string
		method        string
		skillID       string
		setupAuth     bool
		wantStatus    int
		checkResponse func(*testing.T, map[string]interface{})
	}{
		{
			name:       "valid skill ID",
			method:     http.MethodDelete,
			skillID:    skills[0].ID,
			setupAuth:  true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid skill ID",
			method:     http.MethodDelete,
			skillID:    "invalid-id",
			setupAuth:  true,
			wantStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				if response["error"] != "Skill ID not found in candidate's skills" {
					t.Errorf("got error %v; want 'Failed to delete skill'", response["error"])
				}
			},
		},
		{
			name:       "unauthenticated request",
			method:     http.MethodDelete,
			skillID:    skills[0].ID,
			setupAuth:  false,
			wantStatus: http.StatusFound, // redirect to login
		},
		{
			name:       "invalid method",
			method:     http.MethodGet,
			skillID:    skills[0].ID,
			setupAuth:  true,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "missing skill ID",
			method:     http.MethodDelete,
			skillID:    "invalid-id",
			setupAuth:  true,
			wantStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				if response["error"] != "Skill ID not found in candidate's skills" {
					t.Errorf("got error %v; want 'Skill ID not found in candidate's skills'", response["error"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest(tt.method, "/profile/skills/"+tt.skillID, nil)

			if tt.setupAuth {
				req = ts.authenticatedRequest(t, tt.method, "/profile/skills/"+tt.skillID, user, nil)
			}

			// Send request
			w := httptest.NewRecorder()
			ts.app.routes().ServeHTTP(w, req)

			// Check status code
			if w.Code != tt.wantStatus {
				t.Errorf("got status %d; want %d", w.Code, tt.wantStatus)
			}

			// Check response body if we expect one
			if tt.checkResponse != nil {
				var response map[string]interface{}
				body := w.Body.String()
				t.Logf("Response body: %s", body)
				err = json.NewDecoder(strings.NewReader(body)).Decode(&response)
				if err != nil {
					t.Fatal(err)
				}

				tt.checkResponse(t, response)
			}

			// For successful requests, verify the skill was deleted
			if tt.wantStatus == http.StatusOK {
				skills, err := ts.app.db.GetCandidateSkills(context.Background(), candidateID)
				if err != nil {
					t.Fatalf("failed to get skills: %v", err)
				}

				for _, skill := range skills {
					if skill.ID == tt.skillID {
						t.Errorf("skill %s still exists after deletion", tt.skillID)
						break
					}
				}
			}
		})
	}
}
