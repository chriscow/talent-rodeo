package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/alexedwards/flow"
	"github.com/google/uuid"
	"github.com/tmaxmax/go-sse"
)

type application struct {
	sse *sse.Server
}

func main() {
	app := &application{
		sse: &sse.Server{},
	}

	// Start SSE publisher
	go func() {
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			msg := &sse.Message{}
			msg.AppendData(fmt.Sprintf("Server time: %s", time.Now().Format(time.RFC3339)))
			app.sse.Publish(msg)
		}
	}()

	// Routes
	r := flow.New()
	r.HandleFunc("/", app.home, "GET")
	r.HandleFunc("/events/:session-id", app.events, "GET")

	log.Fatal(http.ListenAndServe(":2304", r))
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	sessionID := uuid.New().String()

	// Start session-specific publisher
	go func() {
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			msg := &sse.Message{}
			msg.AppendData(fmt.Sprintf("Session %s time: %s",
				sessionID[:6], time.Now().Format(time.RFC3339)))
			app.sse.Publish(msg, sessionID)
		}
	}()

	html := fmt.Sprintf(`
	<!DOCTYPE html>
	<html>
		<head><title>SSE Demo</title></head>
		<body>
			<h1>Server Time</h1>
			<div>Session ID: %s</div>
			<div id="time"></div>
 
			<script>
				const es = new EventSource("/events/%s");
				const timeDiv = document.getElementById("time");
 
				es.onmessage = (event) => {
					timeDiv.textContent = event.data;
				};
 
				es.onerror = (err) => {
					console.error("SSE Error:", err);
					es.close();
				};
			</script>
		</body>
	</html>
	`, sessionID[:6], sessionID)

	w.Write([]byte(html))
}

func (app *application) events(w http.ResponseWriter, r *http.Request) {
	sessionID := flow.Param(r.Context(), "session-id")

	app.sse.OnSession = func(sess *sse.Session) (sse.Subscription, bool) {
		return sse.Subscription{
			Client: sess,
			Topics: []string{sessionID},
		}, true
	}

	app.sse.ServeHTTP(w, r)
}
