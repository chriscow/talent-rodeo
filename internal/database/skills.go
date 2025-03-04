package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// CandidateSkill represents a skill in the database
type CandidateSkill struct {
	ID              string    `db:"id"`
	CandidateID     string    `db:"candidate_id"`
	Name            string    `db:"name"`
	Level           string    `db:"level"`
	YearsUsed       float64   `db:"years_used"`
	Confidence      string    `db:"confidence"`
	Summary         *string   `db:"summary"`
	PreferenceValue int       `db:"preference_value"`
	IsSearchTerm    bool      `db:"is_search_term"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}

// StoreSkills stores or updates multiple skills for a candidate
func (db *DB) StoreSkills(ctx context.Context, candidateID string, skills []CandidateSkill) error {
	logger := db.logger.With(
		slog.String("method", "StoreSkills"),
		slog.String("candidate_id", candidateID),
		slog.Int("skill_count", len(skills)),
	)

	logger.Debug("storing skills in database")

	return db.WithTx(ctx, func(tx *DB) error {
		// Insert each skill
		for i := range skills {
			logger.Debug("storing skill",
				slog.String("name", skills[i].Name),
				slog.String("level", skills[i].Level),
				slog.Float64("years_used", skills[i].YearsUsed))

			var id string
			err := tx.GetContext(ctx, &id, `
				INSERT INTO candidate_skills (
					candidate_id, name, level, years_used, confidence, summary,
					preference_value, is_search_term
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8
				)
				ON CONFLICT (candidate_id, name) DO UPDATE SET
					level = EXCLUDED.level,
					years_used = EXCLUDED.years_used,
					confidence = EXCLUDED.confidence,
					summary = EXCLUDED.summary,
					updated_at = NOW()
				RETURNING id`,
				candidateID,
				skills[i].Name,
				skills[i].Level,
				skills[i].YearsUsed,
				skills[i].Confidence,
				skills[i].Summary,
				skills[i].PreferenceValue,
				skills[i].IsSearchTerm,
			)
			if err != nil {
				return fmt.Errorf("failed to insert skill %s: %w", skills[i].Name, err)
			}
			skills[i].ID = id
		}

		return nil
	})
}

// GetCandidateSkills retrieves all skills for a candidate
func (db *DB) GetCandidateSkills(ctx context.Context, candidateID string) ([]CandidateSkill, error) {
	logger := db.logger.With(
		slog.String("method", "GetCandidateSkills"),
		slog.String("candidate_id", candidateID),
	)

	logger.Debug("retrieving skills from database")

	query := `
		SELECT id, candidate_id, name, level, years_used, confidence, summary,
			   preference_value, is_search_term, created_at, updated_at
		FROM candidate_skills
		WHERE candidate_id = $1
		ORDER BY name`

	var skills []CandidateSkill
	err := db.SelectContext(ctx, &skills, query, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to query skills: %w", err)
	}

	logger.Debug("successfully retrieved skills",
		slog.Int("skill_count", len(skills)))

	return skills, nil
}

// UpdateSkillPreferences updates the preference values and search term flags for skills
func (db *DB) UpdateSkillPreferences(ctx context.Context, candidateID string, updates map[string]struct {
	PreferenceValue int
	IsSearchTerm    bool
}) error {
	logger := db.logger.With(
		slog.String("method", "UpdateSkillPreferences"),
		slog.String("candidate_id", candidateID),
		slog.Int("update_count", len(updates)),
	)

	logger.Debug("updating skill preferences")

	// First verify the candidate exists
	var userID int
	err := db.GetContext(ctx, &userID, `
		SELECT user_id FROM candidates WHERE id = $1`,
		candidateID)
	if err != nil {
		return fmt.Errorf("failed to get user ID for candidate %s: %w", candidateID, err)
	}
	if userID == 0 {
		return fmt.Errorf("invalid candidate ID: %s", candidateID)
	}

	return db.WithTx(ctx, func(tx *DB) error {
		for skillID, update := range updates {
			logger.Debug("updating skill preference",
				slog.String("skill_id", skillID),
				slog.Int("preference_value", update.PreferenceValue),
				slog.Bool("is_search_term", update.IsSearchTerm))

			result, err := tx.ExecContext(ctx, `
				UPDATE candidate_skills
				SET preference_value = $1,
					is_search_term = $2,
					updated_at = NOW()
				WHERE id = $3 AND candidate_id = $4`,
				update.PreferenceValue,
				update.IsSearchTerm,
				skillID,
				candidateID)
			if err != nil {
				return fmt.Errorf("failed to update skill %s: %w", skillID, err)
			}

			rows, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("failed to get rows affected: %w", err)
			}
			if rows != 1 {
				return fmt.Errorf("expected to update 1 row for skill %s, but updated %d", skillID, rows)
			}
		}

		return nil
	})
}

// AddCandidateSkill adds a new skill to a candidate's profile.
// The skill is added with default values for level, years used, and confidence.
// These can be updated later through other methods.
func (db *DB) AddCandidateSkill(ctx context.Context, candidateID string, name string) (*CandidateSkill, error) {
	logger := db.logger.With(
		slog.String("method", "AddCandidateSkill"),
		slog.String("candidate_id", candidateID),
		slog.String("skill_name", name),
	)

	logger.Debug("adding new skill")

	skill := &CandidateSkill{
		CandidateID:     candidateID,
		Name:            name,
		Level:           "intermediate", // Must be one of: expert, intermediate, beginner
		YearsUsed:       0,              // Default years
		Confidence:      "medium",       // Must be one of: low, medium, high
		PreferenceValue: 3,              // Default neutral preference
		IsSearchTerm:    false,          // Default not a search term
	}

	err := db.GetContext(ctx, skill, `
		INSERT INTO candidate_skills (
			candidate_id, name, level, years_used, confidence,
			preference_value, is_search_term, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, NOW(), NOW()
		)
		ON CONFLICT (candidate_id, name) DO UPDATE SET
			updated_at = NOW()
		RETURNING id, candidate_id, name, level, years_used, confidence, summary,
				preference_value, is_search_term, created_at, updated_at`,
		skill.CandidateID,
		skill.Name,
		skill.Level,
		skill.YearsUsed,
		skill.Confidence,
		skill.PreferenceValue,
		skill.IsSearchTerm)
	if err != nil {
		logger.Error("failed to add skill",
			slog.String("error", err.Error()),
			slog.String("candidate_id", candidateID),
			slog.String("skill_name", name),
			slog.String("level", skill.Level),
			slog.String("confidence", skill.Confidence),
			slog.Float64("years_used", skill.YearsUsed),
			slog.Int("preference_value", skill.PreferenceValue),
			slog.Bool("is_search_term", skill.IsSearchTerm))
		return nil, fmt.Errorf("failed to add skill: %w", err)
	}

	logger.Debug("successfully added skill", slog.String("skill_id", skill.ID))
	return skill, nil
}

// DeleteCandidateSkill removes a skill from a candidate's profile.
func (db *DB) DeleteCandidateSkill(ctx context.Context, candidateID string, skillID string) error {
	logger := db.logger.With(
		slog.String("method", "DeleteCandidateSkill"),
		slog.String("candidate_id", candidateID),
		slog.String("skill_id", skillID),
	)

	logger.Debug("deleting skill")

	result, err := db.ExecContext(ctx, `
		DELETE FROM candidate_skills
		WHERE id = $1 AND candidate_id = $2`,
		skillID, candidateID)
	if err != nil {
		return fmt.Errorf("failed to delete skill: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("expected to delete 1 row for skill %s, but deleted %d", skillID, rows)
	}

	logger.Debug("successfully deleted skill")
	return nil
}
