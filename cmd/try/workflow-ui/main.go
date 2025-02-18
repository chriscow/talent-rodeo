// main.go
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"talent-rodeo/assets"

	"github.com/alexedwards/flow"
	"github.com/gorilla/websocket"
)

func main() {
	mux := flow.New()

	// Serve static files from embedded filesystem
	fileServer := http.FileServer(http.FS(assets.EmbeddedFiles))
	mux.Handle("/static/...", fileServer, "GET")

	// WebSocket endpoint
	mux.HandleFunc("/ws", HandleWebSocket, "GET")

	// Serve index.html
	mux.HandleFunc("/", serveIndex, "GET")

	log.Printf("Server starting on :2304")
	if err := http.ListenAndServe(":2304", mux); err != nil {
		log.Fatal(err)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	content, err := assets.EmbeddedFiles.ReadFile("static/workflow-ui.html")
	if err != nil {
		http.Error(w, "Could not load index.html", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

// WebsocketMsg represents messages sent over websocket
type WebsocketMsg struct {
	Type    string          `json:"type"` // "save_workflow", "load_workflow", "execute"
	Payload json.RawMessage `json:"payload"`
}

// Node represents a handler node in the workflow
type Node struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"` // "sequential", "must", "first", "for", "range", "policy"
	Name     string          `json:"name"`
	Config   json.RawMessage `json:"config"`   // Type-specific configuration
	Children []string        `json:"children"` // IDs of child nodes
}

// Workflow represents the entire handler workflow
type Workflow struct {
	Nodes map[string]Node `json:"nodes"`
	Root  string          `json:"root"` // ID of root node
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Configure this appropriately for production
	},
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("New WebSocket connection from %s", r.RemoteAddr)

	for {
		var msg WebsocketMsg
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		switch msg.Type {
		case "save_workflow":
			handleSaveWorkflow(conn, msg)
		case "load_workflow":
			handleLoadWorkflow(conn, msg)
		case "execute":
			handleExecuteWorkflow(conn, msg)
		default:
			log.Printf("Unknown message type: %s", msg.Type)
		}
	}
}

func handleSaveWorkflow(conn *websocket.Conn, msg WebsocketMsg) {
	var workflow Workflow
	if err := json.Unmarshal(msg.Payload, &workflow); err != nil {
		sendError(conn, "Invalid workflow data")
		return
	}

	// TODO: Implement workflow saving logic

	sendSuccess(conn, "workflow_saved", nil)
}

func handleLoadWorkflow(conn *websocket.Conn, msg WebsocketMsg) {
	// TODO: Implement workflow loading logic

	sendSuccess(conn, "workflow_loaded", nil)
}

func handleExecuteWorkflow(conn *websocket.Conn, msg WebsocketMsg) {
	var workflow Workflow
	if err := json.Unmarshal(msg.Payload, &workflow); err != nil {
		sendError(conn, "Invalid workflow data")
		return
	}

	// TODO: Implement workflow execution logic

	sendSuccess(conn, "execution_complete", nil)
}

func sendError(conn *websocket.Conn, message string) {
	conn.WriteJSON(map[string]any{
		"type":    "error",
		"message": message,
	})
}

func sendSuccess(conn *websocket.Conn, msgType string, data any) {
	conn.WriteJSON(map[string]any{
		"type":    msgType,
		"payload": data,
	})
}
