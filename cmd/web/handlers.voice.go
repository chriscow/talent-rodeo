package main

import (
	"net/http"

	voicesession "talent-rodeo/internal/voice"

	"github.com/gorilla/websocket"
)

func (app *application) handleVoiceWebSocket(w http.ResponseWriter, r *http.Request) {
	app.logger.Debug("Handling voice websocket connection")

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.logger.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	// Create a new voice session
	session, err := voicesession.NewSession(conn, app.logger)
	if err != nil {
		app.logger.Error("Failed to create voice session", "error", err)
		return
	}
	defer session.Close()

	// Start the session
	if err := session.Start(r.Context()); err != nil {
		app.logger.Error("Voice session failed", "error", err)
	}
}
