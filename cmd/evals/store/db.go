package store

import (
	"database/sql"
	"fmt"
	"log"
	"talent-rodeo/internal/utils"

	_ "github.com/mattn/go-sqlite3"
)

// InitDB initializes the database
func InitDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./evals.db")
	if err != nil {
		log.Fatal(err)
	}

	// Create tables if not exists
	queries := []string{
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			prompt_filename TEXT,
			prompt_sha256 TEXT,
			prompt_version TEXT,
			run_id TEXT,
			provider_name TEXT,
			model TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id TEXT,
			run_id TEXT,
			name TEXT,
			content TEXT,
			created_at TIMESTAMP,
			FOREIGN KEY (conversation_id) REFERENCES conversations(id)
		);`,
	}

	for _, query := range queries {
		_, err = db.Exec(query)
		if err != nil {
			return nil, fmt.Errorf("failed to create table: %w", err)
		}
	}
	return db, nil
}

// SaveConversation stores a generated conversation
func SaveConversation(header utils.PromptHeader, conversationID, runID, providerName, model string) error {
	db, err := InitDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()

	_, err = db.Exec("INSERT INTO conversations (id, prompt_filename, prompt_sha256, prompt_version, run_id, provider_name, model) VALUES (?, ?, ?, ?, ?, ?, ?)",
		conversationID, header.Extra["filename"], header.SHA256, header.Version, runID, providerName, model)

	return err
}
