package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

type Conversation struct {
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	Messages  string `json:"messages"`
	Response  string `json:"response"`
	CreatedAt string `json:"created_at"`
}

// StartServer starts the HTTP server
func StartServer() {
	http.HandleFunc("/", homePage)
	http.HandleFunc("/conversation", conversationDetail)
	fmt.Println("🚀 Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// Fetch conversations
func getConversations() []Conversation {
	db, _ := sql.Open("sqlite3", "./conversations.db")
	defer db.Close()

	rows, _ := db.Query("SELECT id, run_id, messages, response, created_at FROM conversations ORDER BY created_at DESC")
	defer rows.Close()

	var conversations []Conversation
	for rows.Next() {
		var c Conversation
		rows.Scan(&c.ID, &c.RunID, &c.Messages, &c.Response, &c.CreatedAt)
		conversations = append(conversations, c)
	}
	return conversations
}

// Home page
func homePage(w http.ResponseWriter, r *http.Request) {
	conversations := getConversations()
	tmpl, _ := template.ParseFiles("templates/index.html")
	tmpl.Execute(w, conversations)
}

// Conversation detail
func conversationDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	db, _ := sql.Open("sqlite3", "./conversations.db")
	defer db.Close()

	row := db.QueryRow("SELECT messages, response FROM conversations WHERE id=?", id)
	var c Conversation
	row.Scan(&c.Messages, &c.Response)

	var messages []map[string]string
	json.Unmarshal([]byte(c.Messages), &messages)

	tmpl, _ := template.ParseFiles("templates/conversation.html")
	tmpl.Execute(w, messages)
}
