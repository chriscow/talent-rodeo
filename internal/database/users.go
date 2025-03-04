package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"talent-rodeo/internal/onboarding"

	"log/slog"

	"github.com/markbates/goth"
)

// User represents a user in the database
type User struct {
	ID           int       `db:"id"`
	Created      time.Time `db:"created"`
	Email        string    `db:"email"`
	HashedSecret string    `db:"hashed_secret"`
	IsSuperUser  bool      `db:"is_superuser"`

	Provider          string  `db:"provider"`
	Name              string  `db:"name"`
	FirstName         string  `db:"first_name"`
	LastName          string  `db:"last_name"`
	NickName          string  `db:"nick_name"`
	Description       string  `db:"description"`
	UserID            string  `db:"user_id"`
	AvatarURL         string  `db:"avatar_url"`
	Location          string  `db:"location"`
	Phone             *string `db:"phone"`
	AccessToken       string
	AccessTokenSecret string
	RefreshToken      string
	ExpiresAt         time.Time
	IDToken           string

	ResumeFilename   *string `db:"resume_filename"`
	FullResume       *string `db:"full_resume"`
	SummarizedResume *string `db:"summarized_resume"`

	// Onboarding fields
	OnboardingStep          onboarding.Step `db:"onboarding_step"`
	OnboardingStartedAt     *time.Time      `db:"onboarding_started_at"`
	OnboardingCompletedAt   *time.Time      `db:"onboarding_completed_at"`
	OnboardingStepUpdatedAt *time.Time      `db:"onboarding_step_updated_at"`

	UpdatedAt *time.Time `db:"updated_at"`
	Role      *string    `db:"role"`
}

type UserKeyword struct {
	ID       int    `db:"id"`
	UserID   int    `db:"user_id"`
	Keyword  string `db:"keyword"`
	Priority int    `db:"priority"`
}

func (db *DB) NewUserFromGothUser(gothUser goth.User, role string) (*User, error) {
	user := User{
		Email:       gothUser.Email,
		Provider:    gothUser.Provider,
		Name:        gothUser.Name,
		FirstName:   gothUser.FirstName,
		LastName:    gothUser.LastName,
		NickName:    gothUser.NickName,
		Description: gothUser.Description,
		UserID:      gothUser.UserID,
		AvatarURL:   gothUser.AvatarURL,
		Location:    gothUser.Location,
		Role:        &role,
	}

	id, err := db.InsertUser(user)
	if err != nil {
		return nil, err
	}

	user.ID = id
	return &user, nil
}

func (db *DB) InsertUser(user User) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	query := `
		INSERT INTO users (
			email, hashed_secret, provider, name, first_name, last_name,
			nick_name, description, user_id, avatar_url, location,
			is_superuser, resume_filename, full_resume, summarized_resume,
			phone, created, updated_at, onboarding_started_at,
			onboarding_completed_at, onboarding_step_updated_at,
			onboarding_step, role
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
		)
		RETURNING id`

	var id int
	err := db.QueryRowContext(ctx, query, user.Email, user.HashedSecret,
		user.Provider, user.Name, user.FirstName, user.LastName, user.NickName,
		user.Description, user.UserID, user.AvatarURL, user.Location,
		user.IsSuperUser, user.ResumeFilename, user.FullResume, user.SummarizedResume,
		user.Phone, user.Created, user.UpdatedAt,
		user.OnboardingStartedAt, user.OnboardingCompletedAt,
		user.OnboardingStepUpdatedAt, user.OnboardingStep, user.Role).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (db *DB) GetUser(id int) (*User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var user User

	query := `SELECT * FROM users WHERE id = $1`

	err := db.GetContext(ctx, &user, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}

	return &user, true, err
}

func (db *DB) GetUsers() ([]User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var users []User

	query := `SELECT * FROM users`

	err := db.SelectContext(ctx, &users, query)
	return users, err
}

func (db *DB) GetUserByEmail(email string) (*User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var user User

	query := `SELECT * FROM users WHERE email = $1`

	err := db.GetContext(ctx, &user, query, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}

	return &user, true, err
}

func (db *DB) UpdateUserHashedPassword(id int, hashedPassword string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	query := `UPDATE users SET hashed_secret = $1 WHERE id = $2`

	_, err := db.ExecContext(ctx, query, hashedPassword, id)
	return err
}

func (db *DB) GetUserKeywords(userID int) ([]UserKeyword, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var keywords []UserKeyword

	query := `SELECT * FROM user_keywords WHERE user_id = $1 order by priority ASC`

	err := db.SelectContext(ctx, &keywords, query, userID)
	return keywords, err
}

func (db *DB) UpdateUserKeywords(userID int, keywords []UserKeyword) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	// Delete all existing keywords for the user
	deleteQuery := `DELETE FROM user_keywords WHERE user_id = $1`
	_, err = tx.ExecContext(ctx, deleteQuery, userID)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Insert the new set of keywords
	insertQuery := `INSERT INTO user_keywords (user_id, keyword, priority) VALUES ($1, $2, $3)`

	for _, keyword := range keywords {
		_, err := tx.ExecContext(ctx, insertQuery, userID, keyword.Keyword, keyword.Priority)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) UpdateFullResume(userID int, fileName, resume string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var query string
	var args []any

	if fileName == "" {
		query = `UPDATE users SET resume_filename=NULL, full_resume=NULL WHERE id = $1`
		args = []any{userID}
	} else {
		query = `UPDATE users SET resume_filename = $1, full_resume = $2 WHERE id = $3`
		args = []any{fileName, resume, userID}
	}

	_, err := db.ExecContext(ctx, query, args...)

	return err
}

func (db *DB) UpdateUserOnboarding(userID int, step onboarding.Step) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	query := `UPDATE users SET onboarding_step = $1 WHERE id = $2`

	_, err := db.ExecContext(ctx, query, step, userID)
	return err
}

func (db *DB) UpdateUserPhone(userID int, phone string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var phonePtr *string
	if phone != "" {
		phonePtr = &phone
	}

	query := `UPDATE users SET phone = $1 WHERE id = $2`

	_, err := db.ExecContext(ctx, query, phonePtr, userID)
	return err
}

// GetUserByPhone retrieves a user by their phone number
func (db *DB) GetUserByPhone(phone string) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var user User
	query := `SELECT * FROM users WHERE phone = $1`

	err := db.GetContext(ctx, &user, query, phone)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found with phone number %s", phone)
		}
		return nil, fmt.Errorf("failed to get user by phone: %w", err)
	}

	return &user, nil
}

// GetCandidateIDForUser retrieves the candidate ID for a given user ID.
// Returns an error if the user is not found or if they are not a candidate.
func (db *DB) GetCandidateIDForUser(ctx context.Context, userID int) (string, error) {
	logger := db.logger.With(
		slog.String("method", "GetCandidateIDForUser"),
		slog.Int("user_id", userID),
	)

	logger.Debug("retrieving candidate ID for user")

	var candidateID string
	query := `SELECT id FROM candidates WHERE user_id = $1`

	err := db.GetContext(ctx, &candidateID, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no candidate found for user %d", userID)
		}
		return "", fmt.Errorf("failed to get candidate ID: %w", err)
	}

	logger.Debug("found candidate ID", slog.String("candidate_id", candidateID))
	return candidateID, nil
}

// CreateCandidate creates a new candidate record for a user.
// Returns the candidate ID as a string.
func (db *DB) CreateCandidate(ctx context.Context, userID int) (string, error) {
	logger := db.logger.With(
		slog.String("method", "CreateCandidate"),
		slog.Int("user_id", userID),
	)

	logger.Debug("creating candidate record")

	var candidateID string
	err := db.GetContext(ctx, &candidateID, `
		INSERT INTO candidates (user_id) 
		VALUES ($1) 
		RETURNING id`, userID)
	if err != nil {
		return "", fmt.Errorf("failed to create candidate: %w", err)
	}

	logger.Debug("created candidate record", slog.String("candidate_id", candidateID))
	return candidateID, nil
}
