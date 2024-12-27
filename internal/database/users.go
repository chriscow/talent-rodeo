package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/markbates/goth"
)

type User struct {
	ID           int       `db:"id"`
	Created      time.Time `db:"created"`
	Email        string    `db:"email"`
	HashedSecret string    `db:"hashed_secret"`

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
