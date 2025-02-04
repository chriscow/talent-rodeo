package database

import (
	"io"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

const defaultTimeout = 5 * time.Second

// DB represents a SQLite database connection
type DB struct {
	*sqlx.DB
	logger     *slog.Logger
	testWriter io.Writer
	inMemory   bool
}

// logDebug logs a debug message if a logger is configured
func (db *DB) logDebug(msg string, args ...any) {
	if db.logger != nil {
		db.logger.Debug(msg, args...)
	}
}

// logError logs an error message with additional arguments
func (db *DB) logError(msg string, err error, args ...any) {
	if db.logger != nil {
		allArgs := append([]any{"error", err}, args...)
		db.logger.Error(msg, allArgs...)
	}
}
