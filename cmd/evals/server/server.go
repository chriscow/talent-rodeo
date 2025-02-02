package server

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Conversation struct {
	ID            string    `json:"id"`
	RunID         string    `json:"run_id"`
	PromptFile    string    `json:"prompt_file"`
	PromptVersion string    `json:"prompt_version"`
	ProviderName  string    `json:"provider_name"`
	Model         string    `json:"model"`
	CreatedAt     time.Time `json:"created_at"`
}

type Message struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// StartServer starts the HTTP server
func StartServer() {
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)

	srv := &http.Server{
		Addr: ":2304",
	}

	http.HandleFunc("/", homePage)
	http.HandleFunc("/conversation", conversationDetail)

	go func() {
		fmt.Println("🚀 Server started at http://localhost:2304")
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("Server shutting down: %v", err)
		}
	}()

	<-sigint
	fmt.Println("🚀 Server shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	fmt.Println("🚀 Server shutdown complete")
}

// Fetch conversations
func getConversations() []Conversation {
	db, _ := sql.Open("sqlite3", "./evals.db")
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, run_id, prompt_filename, prompt_version, provider_name, model, created_at 
		FROM conversations 
		ORDER BY created_at DESC`)
	if err != nil {
		log.Printf("Error querying conversations: %v", err)
		return nil
	}
	defer rows.Close()

	var conversations []Conversation
	for rows.Next() {
		var c Conversation
		err := rows.Scan(&c.ID, &c.RunID, &c.PromptFile, &c.PromptVersion, &c.ProviderName, &c.Model, &c.CreatedAt)
		if err != nil {
			log.Printf("Error scanning conversation: %v", err)
			continue
		}
		conversations = append(conversations, c)
	}
	return conversations
}

// Get messages for a conversation
func getMessages(conversationID string) []Message {
	db, _ := sql.Open("sqlite3", "./evals.db")
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, name, content, created_at 
		FROM messages 
		WHERE conversation_id = ? 
		ORDER BY created_at ASC`, conversationID)
	if err != nil {
		log.Printf("Error querying messages: %v", err)
		return nil
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		err := rows.Scan(&m.ID, &m.Name, &m.Content, &m.CreatedAt)
		if err != nil {
			log.Printf("Error scanning message: %v", err)
			continue
		}
		messages = append(messages, m)
	}
	return messages
}

// Home page
func homePage(w http.ResponseWriter, r *http.Request) {
	conversations := getConversations()
	html := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AI Conversations</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@picocss/pico@1/css/pico.min.css">
    <style>
        .grid {
            grid-template-columns: repeat(auto-fill, minmax(min(100%, 450px), 1fr));
            max-width: 1000px;
            margin: 0 auto;
        }
    </style>
</head>
<body>
    <main class="container">
        <h1>AI Conversations</h1>
        <div class="grid">
            {{range .}}
            <article>
                <header>
                    <strong>{{.PromptFile}}</strong>
                    <p><small>Version: {{.PromptVersion}}</small></p>
                </header>
                <p>Run ID: {{.RunID}}</p>
                <p>Provider: {{.ProviderName}} ({{.Model}})</p>
                <footer>
                    <small>{{.CreatedAt.Format "Jan 02, 2006 15:04:05"}}</small>
                    <a href="/conversation?id={{.ID}}" role="button">View Conversation</a>
                </footer>
            </article>
            {{end}}
        </div>
    </main>
</body>
</html>`

	tmpl := template.Must(template.New("home").Parse(html))
	tmpl.Execute(w, conversations)
}

// Conversation detail
func conversationDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	messages := getMessages(id)

	html := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Conversation Details</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@picocss/pico@1/css/pico.min.css">
    <style>
        .message { 
            margin-bottom: 1rem; 
            padding: 1rem;
            border-radius: 8px;
        }
        .message header { 
            margin-bottom: 0.5rem;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .message.recruiter { 
            background: var(--primary);
            color: var(--primary-inverse);
            margin-right: 2rem;
        }
        .message.candidate { 
            background: var(--secondary);
            color: var(--secondary-inverse);
            margin-left: 2rem;
        }
        .message p {
            margin: 0;
            white-space: pre-wrap;
        }
        .timestamp {
            font-size: 0.8em;
            opacity: 0.8;
        }
    </style>
</head>
<body>
    <main class="container">
        <nav>
            <ul>
                <li><a href="/" role="button" class="outline">← Back to Conversations</a></li>
            </ul>
        </nav>
        <h1>Conversation Details</h1>
        {{range .}}
        <article class="message {{if eq .Name "Shirely"}}recruiter{{else}}candidate{{end}}">
            <header>
                <strong>{{.Name}}</strong>
                <span class="timestamp">{{.CreatedAt.Format "15:04:05"}}</span>
            </header>
            <p>{{.Content}}</p>
        </article>
        {{end}}
    </main>
</body>
</html>`

	tmpl := template.Must(template.New("detail").Parse(html))
	tmpl.Execute(w, messages)
}
