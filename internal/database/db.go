package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"talent-rodeo/assets"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/lib/pq"
)

const defaultTimeout = 3 * time.Second

// DBTX is an interface that both *sqlx.DB and *sqlx.Tx satisfy
type DBTX interface {
	sqlx.ExtContext
	sqlx.PreparerContext
	Get(dest interface{}, query string, args ...interface{}) error
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	Select(dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	Queryx(query string, args ...interface{}) (*sqlx.Rows, error)
	QueryRowx(query string, args ...interface{}) *sqlx.Row
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	Exec(query string, args ...interface{}) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// DB wraps sqlx.DB to provide additional functionality
type DB struct {
	dbtx   DBTX
	logger *slog.Logger
}

// Option is a function that configures a DB instance
type Option func(*DB) error

// WithLogger sets the logger for the DB instance
func WithLogger(logger *slog.Logger) Option {
	return func(db *DB) error {
		db.logger = logger
		return nil
	}
}

// WithForeignKeys enables foreign key support
func WithForeignKeys() Option {
	return func(db *DB) error {
		db.logger.Debug("enabling foreign keys")
		_, err := db.dbtx.Exec("PRAGMA foreign_keys = ON")
		if err != nil {
			return fmt.Errorf("failed to enable foreign keys: %w", err)
		}
		db.logger.Debug("foreign keys enabled")
		return nil
	}
}

// WithTasksTable creates and verifies the tasks table
func WithTasksTable() Option {
	return func(db *DB) error {
		db.logger.Debug("creating tasks table")
		_, err := db.dbtx.Exec(`
			CREATE TABLE IF NOT EXISTS tasks (
				id BIGINT PRIMARY KEY,
				type TEXT NOT NULL,
				payload JSONB NOT NULL,
				status TEXT NOT NULL CHECK (
					status IN (
						'pending',
						'running',
						'complete',
						'failed',
						'cancelled'
					)
				),
				attempts INTEGER NOT NULL DEFAULT 0,
				max_attempts INTEGER NOT NULL DEFAULT 3,
				run_at TIMESTAMP NOT NULL,
				error TEXT,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create tasks table: %w", err)
		}

		// Verify table exists and has correct schema
		var count int
		err = db.dbtx.Get(&count, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='tasks'")
		if err != nil {
			return fmt.Errorf("failed to verify tasks table: %w", err)
		}
		if count != 1 {
			return errors.New("tasks table not found after creation")
		}

		db.logger.Debug("tasks table created and verified\n")
		return nil
	}
}

// TxFn is a function that executes in a transaction
type TxFn func(*DB) error

// WithTx executes fn within a transaction
func (db *DB) WithTx(ctx context.Context, fn TxFn) error {
	tx, err := db.dbtx.(*sqlx.DB).BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	txDB := &DB{
		dbtx:   tx,
		logger: db.logger,
	}

	if err := fn(txDB); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx failed: %v, rollback failed: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit tx: %w", err)
	}

	return nil
}

// New creates a new DB instance
func New(dsn string, automigrate bool, opts ...Option) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	sqlxDB, err := sqlx.ConnectContext(ctx, "postgres", "postgres://"+dsn)
	if err != nil {
		return nil, err
	}

	sqlxDB.SetMaxOpenConns(25)
	sqlxDB.SetMaxIdleConns(25)
	sqlxDB.SetConnMaxIdleTime(5 * time.Minute)
	sqlxDB.SetConnMaxLifetime(2 * time.Hour)

	db := &DB{
		dbtx:   sqlxDB,
		logger: slog.Default(),
	}

	for _, opt := range opts {
		if err := opt(db); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	if automigrate {
		iofsDriver, err := iofs.New(assets.EmbeddedFiles, "migrations")
		if err != nil {
			return nil, err
		}

		migrator, err := migrate.NewWithSourceInstance("iofs", iofsDriver, "postgres://"+dsn)
		if err != nil {
			return nil, err
		}

		err = migrator.Up()
		switch {
		case errors.Is(err, migrate.ErrNoChange):
			break
		case err != nil:
			return nil, err
		}
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if sqlxDB, ok := db.dbtx.(*sqlx.DB); ok {
		return sqlxDB.Close()
	}
	return nil
}

// Helper methods to make the DB struct satisfy the DBTX interface
func (db *DB) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return db.dbtx.GetContext(ctx, dest, query, args...)
}

func (db *DB) SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return db.dbtx.SelectContext(ctx, dest, query, args...)
}

func (db *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.dbtx.QueryRowContext(ctx, query, args...)
}

func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.dbtx.ExecContext(ctx, query, args...)
}

func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.dbtx.Exec(query, args...)
}

func (db *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if sqlxDB, ok := db.dbtx.(*sqlx.DB); ok {
		return sqlxDB.QueryContext(ctx, query, args...)
	}
	if sqlxTx, ok := db.dbtx.(*sqlx.Tx); ok {
		return sqlxTx.QueryContext(ctx, query, args...)
	}
	return nil, fmt.Errorf("invalid database connection type")
}

func (db *DB) BeginTxx(ctx context.Context, opts *sql.TxOptions) (*sqlx.Tx, error) {
	if sqlxDB, ok := db.dbtx.(*sqlx.DB); ok {
		return sqlxDB.BeginTxx(ctx, opts)
	}
	return nil, fmt.Errorf("cannot begin transaction on non-DB connection")
}
