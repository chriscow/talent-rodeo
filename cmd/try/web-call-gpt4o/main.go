package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	openairt "github.com/WqyJh/go-openai-realtime"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	if err := godotenv.Overload(); err != nil {
		log.Println("No .env file found")
		return
	}

	http.HandleFunc("/stream", handleWebSocket)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
		<!DOCTYPE html>
		<html lang="en">
		
		<head>
			<meta charset="UTF-8">
			<meta name="viewport" content="width=device-width, initial-scale=1.0">
			<title>Audio Streaming</title>
		</head>
		
		<body>
			<h1>Stream Audio</h1>
			<button id="start">Start Recording</button>
			<button id="stop" disabled>Stop Recording</button>
		
			<script>
				let mediaRecorder;
				let websocket;
		
				document.getElementById('start').addEventListener('click', async () => {
					const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
					mediaRecorder = new MediaRecorder(stream);
					websocket = new WebSocket('ws://localhost:2304/stream');
		
					websocket.onopen = () => console.log('WebSocket connected');
					websocket.onclose = () => console.log('WebSocket closed');
					websocket.onerror = (err) => console.error('WebSocket error', err);
		
					mediaRecorder.ondataavailable = (event) => {
						if (websocket.readyState === WebSocket.OPEN) {
							websocket.send(event.data);
						}
					};
		
					mediaRecorder.start(250); // Emit audio chunks every 250ms
					document.getElementById('start').disabled = true;
					document.getElementById('stop').disabled = false;
				});
		
				document.getElementById('stop').addEventListener('click', () => {
					mediaRecorder.stop();
					websocket.close();
					document.getElementById('start').disabled = false;
					document.getElementById('stop').disabled = true;
				});
			</script>
		</body>
		
		</html>`))
	})

	log.Println("Server listening on :2304")
	log.Fatal(http.ListenAndServe(":2304", nil))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	fmt.Println("WebSocket connected")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Println("OPENAI_API_KEY is not set")
		return
	}

	client := openairt.NewClient(os.Getenv("OPENAI_API_KEY"))
	apiConn, err := client.Connect(ctx)
	if err != nil {
		log.Println("API connection error:", err)
		return
	}
	defer apiConn.Close()

	println("Connected to OpenAI Realtime")
	err = apiConn.SendMessage(ctx, openairt.SessionUpdateEvent{
		Session: openairt.ClientSession{
			Modalities:              []openairt.Modality{openairt.ModalityAudio},
			InputAudioTranscription: &openairt.InputAudioTranscription{Model: "whisper-1"},
		},
	})
	if err != nil {
		log.Println("Session configuration error:", err)
		return
	}

	for {
		println("Reading WebSocket message")
		_, audioData, err := conn.ReadMessage()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Println("WebSocket read error:", err)
			break
		}

		println("Sending audio to OpenAI")

		base64Audio := base64.StdEncoding.EncodeToString(audioData)
		err = apiConn.SendMessage(ctx, openairt.InputAudioBufferAppendEvent{
			Audio: base64Audio,
		})
		if err != nil {
			log.Println("API send error:", err)
			break
		}

		err = apiConn.SendMessage(ctx, openairt.InputAudioBufferCommitEvent{})
		if err != nil {
			log.Println("API commit error:", err)
			break
		}

		println("Audio sent to OpenAI")
	}
}
