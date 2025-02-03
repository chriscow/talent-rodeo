package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"talent-rodeo/internal/onboarding"

	"github.com/markbates/goth"
)

type User struct {
	ID           int       `db:"id"`
	Created      time.Time `db:"created"`
	Email        string    `db:"email"`
	HashedSecret string    `db:"hashed_secret"`
	IsSuperUser  bool      `db:"is_superuser"`

	Provider          string `db:"provider"`
	Name              string `db:"name"`
	FirstName         string `db:"first_name"`
	LastName          string `db:"last_name"`
	NickName          string `db:"nick_name"`
	Description       string `db:"description"`
	UserID            string `db:"user_id"`
	AvatarURL         string `db:"avatar_url"`
	Location          string `db:"location"`
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
}

type UserKeyword struct {
	ID       int    `db:"id"`
	UserID   int    `db:"user_id"`
	Keyword  string `db:"keyword"`
	Priority int    `db:"priority"`
}

func (db *DB) NewUserFromGothUser(gothUser goth.User) (*User, error) {
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
		INSERT INTO users (email, hashed_secret, provider, name, first_name, last_name, nick_name, description, user_id, avatar_url, location)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`

	result, err := db.ExecContext(ctx, query, user.Email, user.HashedSecret,
		user.Provider, user.Name, user.FirstName, user.LastName, user.NickName,
		user.Description, user.UserID, user.AvatarURL, user.Location)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return int(id), err
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
	var args []interface{}

	if fileName == "" {
		query = `UPDATE users SET resume_filename=NULL, full_resume=NULL WHERE id = $1`
		args = []interface{}{userID}
	} else {
		query = `UPDATE users SET resume_filename = $1, full_resume = $2 WHERE id = $3`
		args = []interface{}{fileName, resume, userID}
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
