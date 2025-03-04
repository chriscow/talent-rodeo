package database

import (
	"context"
	"testing"
)

func TestSkills(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Create a test user with candidate profile and skills
	_, candidateID, skills, err := db.CreateTestUserWithSkills(t)
	if err != nil {
		t.Fatalf("failed to create test user with skills: %v", err)
	}

	t.Run("StoreSkills", func(t *testing.T) {
		// Test storing skills
		err := db.StoreSkills(ctx, candidateID, skills)
		if err != nil {
			t.Fatalf("failed to store skills: %v", err)
		}

		// Verify skills were stored
		stored, err := db.GetCandidateSkills(ctx, candidateID)
		if err != nil {
			t.Fatalf("failed to get stored skills: %v", err)
		}

		if got, want := len(stored), len(skills); got != want {
			t.Errorf("got %d skills, want %d", got, want)
		}

		// Verify skill data
		for i, want := range skills {
			got := stored[i]
			if got.Name != want.Name {
				t.Errorf("got name %q, want %q", got.Name, want.Name)
			}
			if got.Level != want.Level {
				t.Errorf("got level %q, want %q", got.Level, want.Level)
			}
			if got.YearsUsed != want.YearsUsed {
				t.Errorf("got years_used %f, want %f", got.YearsUsed, want.YearsUsed)
			}
			if got.Confidence != want.Confidence {
				t.Errorf("got confidence %q, want %q", got.Confidence, want.Confidence)
			}
			if got.Summary == nil != (want.Summary == nil) {
				t.Errorf("got summary nil %v, want %v", got.Summary == nil, want.Summary == nil)
			}
			if got.Summary != nil && want.Summary != nil && *got.Summary != *want.Summary {
				t.Errorf("got summary %q, want %q", *got.Summary, *want.Summary)
			}
			if got.PreferenceValue != want.PreferenceValue {
				t.Errorf("got preference_value %d, want %d", got.PreferenceValue, want.PreferenceValue)
			}
			if got.IsSearchTerm != want.IsSearchTerm {
				t.Errorf("got is_search_term %v, want %v", got.IsSearchTerm, want.IsSearchTerm)
			}
		}
	})

	t.Run("GetCandidateSkills", func(t *testing.T) {
		// Create another test user and candidate for testing non-existent skills
		anotherUser, err := db.CreateTestUser(t)
		if err != nil {
			t.Fatalf("failed to create another test user: %v", err)
		}

		anotherCandidateID, err := db.CreateTestCandidate(t, anotherUser.ID)
		if err != nil {
			t.Fatalf("failed to create another test candidate: %v", err)
		}

		// Test getting skills for a candidate with no skills
		skills, err := db.GetCandidateSkills(ctx, anotherCandidateID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(skills) != 0 {
			t.Errorf("got %d skills for candidate without skills, want 0", len(skills))
		}
	})

	t.Run("UpdateSkillPreferences", func(t *testing.T) {
		// Get stored skills first
		stored, err := db.GetCandidateSkills(ctx, candidateID)
		if err != nil {
			t.Fatalf("failed to get stored skills: %v", err)
		}

		// Update preferences
		updates := make(map[string]struct {
			PreferenceValue int
			IsSearchTerm    bool
		})
		for _, skill := range stored {
			updates[skill.ID] = struct {
				PreferenceValue int
				IsSearchTerm    bool
			}{
				PreferenceValue: 4,    // Change all to 4
				IsSearchTerm:    true, // Change all to true
			}
		}

		err = db.UpdateSkillPreferences(ctx, candidateID, updates)
		if err != nil {
			t.Fatalf("failed to update preferences: %v", err)
		}

		// Verify updates
		updated, err := db.GetCandidateSkills(ctx, candidateID)
		if err != nil {
			t.Fatalf("failed to get updated skills: %v", err)
		}

		for _, skill := range updated {
			if skill.PreferenceValue != 4 {
				t.Errorf("got preference_value %d, want 4", skill.PreferenceValue)
			}
			if !skill.IsSearchTerm {
				t.Errorf("got is_search_term false, want true")
			}
		}
	})
}
