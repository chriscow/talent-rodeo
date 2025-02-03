package database

import (
	"context"
	"fmt"
	"time"

	"talent-rodeo/internal/onboarding"
)

// GetOnboardingState retrieves the current onboarding state for a user
func (db *DB) GetOnboardingState(userID int) (*onboarding.State, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var state onboarding.State
	query := `
		SELECT 
			onboarding_step,
			onboarding_started_at,
			onboarding_completed_at,
			onboarding_step_updated_at
		FROM users 
		WHERE id = $1`

	err := db.GetContext(ctx, &state, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get onboarding state: %w", err)
	}

	return &state, nil
}

// UpdateOnboardingStep updates a user's onboarding step and related timestamps
func (db *DB) UpdateOnboardingStep(userID int, step onboarding.Step) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	now := time.Now()
	query := `
		UPDATE users 
		SET 
			onboarding_step = $1,
			onboarding_step_updated_at = $2,
			onboarding_started_at = COALESCE(onboarding_started_at, $2),
			onboarding_completed_at = CASE 
				WHEN $1 = 'complete' THEN $2
				ELSE onboarding_completed_at
			END
		WHERE id = $3`

	_, err := db.ExecContext(ctx, query, step, now, userID)
	if err != nil {
		return fmt.Errorf("failed to update onboarding step: %w", err)
	}

	return nil
}

// GetUsersInOnboardingStep retrieves all users currently in a specific onboarding step
func (db *DB) GetUsersInOnboardingStep(step onboarding.Step) ([]User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var users []User
	query := `SELECT * FROM users WHERE onboarding_step = $1`

	err := db.SelectContext(ctx, &users, query, step)
	if err != nil {
		return nil, fmt.Errorf("failed to get users in step %s: %w", step, err)
	}

	return users, nil
}

// GetIncompleteOnboarding retrieves all users who have started but not completed onboarding
func (db *DB) GetIncompleteOnboarding() ([]User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var users []User
	query := `
		SELECT * FROM users 
		WHERE onboarding_started_at IS NOT NULL 
		AND onboarding_completed_at IS NULL`

	err := db.SelectContext(ctx, &users, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get incomplete onboarding users: %w", err)
	}

	return users, nil
}
