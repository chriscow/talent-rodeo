package voice

import (
	"testing"

	"talent-rodeo/internal/database"

	_ "github.com/mattn/go-sqlite3"
)

// setupVoiceTestDB creates a new in-memory SQLite database for testing voice package
func setupVoiceTestDB(t *testing.T) *database.DB {
	t.Helper()
	return database.SetupTestDB(t)
}
