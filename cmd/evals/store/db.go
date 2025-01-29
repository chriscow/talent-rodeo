package store

import (
	"database/sql"
	"encoding/json"
	"log"
	"talent-rodeo/internal/utils"

	"github.com/chriscow/minds"
	_ "github.com/mattn/go-sqlite3"
)

// InitDB initializes the database
func InitDB() *sql.DB {
	db, err := sql.Open("sqlite3", "./evals.db")
	if err != nil {
		log.Fatal(err)
	}

	// Create table if not exists
	query := `
	CREATE TABLE IF NOT EXISTS conversations (
		id TEXT PRIMARY KEY,
		prompt_filename TEXT,
		prompt_sha256 TEXT,
		prompt_version TEXT,
		run_id TEXT,
		messages TEXT,
		response TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = db.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
	return db
}

// SaveConversation stores a generated conversation
func SaveConversation(header utils.PromptHeader, conversationID, runID string, messages minds.Messages) {
	db := InitDB()
	defer db.Close()

	msgJSON, _ := json.Marshal(messages)

	_, err := db.Exec("INSERT INTO conversations (id, prompt_filename, prompt_sha256, prompt_version, run_id, messages) VALUES (?, ?, ?, ?, ?, ?)",
		conversationID, header.Extra["filename"], header.SHA256, header.Version, runID, string(msgJSON))

	if err != nil {
		log.Println("Failed to save conversation:", err)
	}
}
