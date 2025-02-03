package database

import (
	"testing"
)

func TestDatabase(t *testing.T) {
	db, err := New(":memory:", true)
	if err != nil {
		t.Fatal(err)
	}

	// Create test tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS appointments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			scheduled_time TIMESTAMP NOT NULL,
			timezone TEXT NOT NULL,
			status TEXT NOT NULL,
			call_sid TEXT,
			transcript TEXT,
			transcript_summary TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
	`)
	if err != nil {
		t.Fatal(err)
	}
}
