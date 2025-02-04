package database

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/jmoiron/sqlx"
)

// Option is a function that configures a DB instance
type Option func(*DB) error

// WithForeignKeys enables foreign key support
func WithForeignKeys() Option {
	return func(db *DB) error {
		db.logDebug("enabling foreign keys")
		_, err := db.Exec("PRAGMA foreign_keys = ON")
		if err != nil {
			return err
		}
		db.logDebug("foreign keys enabled")
		return nil
	}
}

// WithInMemory configures the database to run in memory
func WithInMemory() Option {
	return func(db *DB) error {
		db.inMemory = true
		return nil
	}
}

// WithLogger sets a custom logger for the database
func WithLogger(logger *slog.Logger) Option {
	return func(db *DB) error {
		db.logger = logger
		return nil
	}
}

// WithTestWriter sets a test writer for capturing log output
func WithTestWriter(w io.Writer) Option {
	return func(db *DB) error {
		db.testWriter = w
		return nil
	}
}

// New creates a new database connection
func New(dsn string, opts ...Option) (*DB, error) {
	db := &DB{
		logger:   slog.Default(),
		inMemory: false,
	}

	for _, opt := range opts {
		if err := opt(db); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	conn, err := sqlx.Connect("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	db.DB = conn

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.DB == nil {
		return nil
	}
	return db.DB.Close()
}
