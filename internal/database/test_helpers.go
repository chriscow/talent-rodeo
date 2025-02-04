package database

import (
	"fmt"
	"log/slog"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestDBOption represents a database setup option
type TestDBOption func(*DB) error

// WithTasksTable adds the tasks table to the test database
func WithTasksTable() Option {
	return func(db *DB) error {
		db.logDebug("creating tasks table")

		// Create the tasks table with all constraints
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS tasks (
				id TEXT PRIMARY KEY CHECK (id != ''),
				type TEXT NOT NULL,
				payload BLOB,
				status TEXT NOT NULL CHECK (
					status IN (
						'pending',
						'running',
						'complete',
						'failed',
						'retrying',
						'cancelled'
					)
				),
				attempts INTEGER NOT NULL DEFAULT 0,
				max_attempts INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
				run_at DATETIME NOT NULL,
				error TEXT,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			);

			CREATE INDEX IF NOT EXISTS idx_tasks_status_run_at ON tasks (status, run_at)
			WHERE
				status IN ('pending', 'retrying');

			CREATE TRIGGER IF NOT EXISTS tasks_updated_at 
				AFTER UPDATE ON tasks
			BEGIN
				UPDATE tasks 
				SET updated_at = CURRENT_TIMESTAMP
				WHERE id = NEW.id;
			END;
		`)
		if err != nil {
			return fmt.Errorf("failed to create tasks table: %w", err)
		}

		// Verify table was created
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to verify table creation: %w", err)
		}
		if count != 1 {
			return fmt.Errorf("tasks table was not created")
		}

		db.logDebug("tasks table created and verified")
		return nil
	}
}

// WithTestLogger creates a test logger option
func WithTestLogger(t *testing.T) Option {
	return func(db *DB) error {
		db.testWriter = &testWriter{t: t}
		db.logger = slog.New(slog.NewTextHandler(db.testWriter, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		return nil
	}
}

// testWriter implements io.Writer for test logging
type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p))
	return len(p), nil
}

// SetupTestDB creates a new in-memory SQLite database for testing
func SetupTestDB(t *testing.T) *DB {
	t.Helper()

	db, err := New(":memory:",
		WithTestLogger(t),
		WithForeignKeys(),
		WithTasksTable(),
	)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	return db
}
