package database

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB creates a new in-memory SQLite database for testing
func setupTestDB(t *testing.T) *DB {
	t.Helper()

	// Create an in-memory database
	db, err := sqlx.Connect("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Enable foreign key support
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	// Create the tasks table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
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
			max_attempts INTEGER NOT NULL DEFAULT 3,
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
		t.Fatalf("failed to create tasks table: %v", err)
	}

	return &DB{DB: db}
}
