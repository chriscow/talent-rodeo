package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"

	openairt "github.com/WqyJh/go-openai-realtime"
	"github.com/gorilla/websocket"
)

// handleWebSocket is where we:
// 1. Receive audio from Twilio (WebSocket frames)
// 2. Forward user audio to OpenAI Realtime (in practice, you'd do speech-to-text first)
// 3. Send OpenAI's text output to ElevenLabs
// 4. Send ElevenLabs TTS audio back to Twilio

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	println("Handling WebSocket...")
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	twilioConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Failed to upgrade Twilio WS:", err)
		return
	}
	defer twilioConn.Close()

	// ----- Connect to ElevenLabs
	elConn, err := connectElevenLabs(os.Getenv("ELEVENLABS_API_KEY"), os.Getenv("ELEVENLABS_VOICE_ID"))
	if err != nil {
		log.Println("Failed to connect to ElevenLabs:", err)
		return
	}
	defer elConn.Close()

	// ----- Connect to OpenAI Realtime
	oaConn, err := connectOpenAIRealtime(os.Getenv("OPENAI_API_KEY"))
	if err != nil {
		log.Println("Failed to connect to OpenAI Realtime:", err)
		return
	}
	defer oaConn.Close()

	// For brevity, below is a *placeholder* flow:
	// Twilio -> (Audio) -> SpeechToText (some pipeline) -> OA Realtime -> TTS -> Twilio
	// In reality, you'd decode frames from Twilio, do STT, then feed text to OA, etc.

	// Example: Send an initial “system” message to OpenAI
	oaConn.SendMessage(context.Background(), openairt.ConversationItemCreateEvent{
		Item: openairt.MessageItem{
			Role:   openairt.MessageRoleSystem,
			Type:   openairt.MessageItemTypeMessage,
			Status: openairt.ItemStatusCompleted,
			Content: []openairt.MessageContentPart{
				{Type: openairt.MessageContentTypeInputText, Text: "You are a helpful assistant."},
			},
		},
	})

	// Read WS frames from Twilio
	go func() {
		for {
			_, data, readErr := twilioConn.ReadMessage()
			if readErr != nil {
				log.Println("Error reading from Twilio WS:", readErr)
				return
			}
			// data contains audio from Twilio
			// We would pass it to STT -> OpenAI input
			fmt.Println("Received Twilio audio chunk (bytes):", len(data))
		}
	}()

	// Read WS frames from ElevenLabs
	go func() {
		for {
			var out ElevenLabsResponse
			if err := elConn.ReadJSON(&out); err != nil {
				log.Println("Error reading from ElevenLabs WS:", err)
				return
			}
			if out.Audio != "" {
				audioBytes, decErr := base64.StdEncoding.DecodeString(out.Audio)
				if decErr != nil {
					log.Println("Decode audio error:", decErr)
					continue
				}
				// Send TTS audio chunks back to Twilio
				if wErr := twilioConn.WriteMessage(websocket.BinaryMessage, audioBytes); wErr != nil {
					log.Println("Error writing TTS audio to Twilio:", wErr)
					return
				}
			}
			if out.IsFinal {
				log.Println("ElevenLabs TTS final chunk received.")
				return
			}
		}
	}()

	// Example: send something to ElevenLabs TTS
	if err := elConn.WriteJSON(ElevenLabsMessage{
		Text: "Hello from ElevenLabs streaming!",
	}); err != nil {
		log.Println("Error sending initial TTS request:", err)
		return
	}
}

// connectElevenLabs sets up a WebSocket connection for streaming TTS
func connectElevenLabs(apiKey, voiceID string) (*websocket.Conn, error) {
	headers := http.Header{}
	headers.Add("xi-api-key", apiKey)

	url := fmt.Sprintf("wss://api.elevenlabs.io/v1/text-to-speech/%s/stream-input?output_format=mp3_44100", voiceID)
	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		return nil, err
	}

	// Send initial space to start connection
	initMsg := ElevenLabsMessage{
		Text: " ",
	}
	if err := conn.WriteJSON(initMsg); err != nil {
		log.Println("Error initializing ElevenLabs:", err)
		return nil, err
	}

	return conn, nil
}

// connectOpenAIRealtime sets up the Realtime WS to OpenAI
func connectOpenAIRealtime(apiKey string) (*openairt.Conn, error) {
	config := openairt.DefaultConfig(apiKey)
	client := openairt.NewClientWithConfig(config)
	conn, err := client.Connect(context.Background(),
		openairt.WithModel("gpt-4o-realtime-preview"),
		openairt.WithLogger(openairt.StdLogger{}),
	)
	return conn, err
}

// ElevenLabsMessage is a minimal message for TTS requests
type ElevenLabsMessage struct {
	Text                 string `json:"text,omitempty"`
	TryTriggerGeneration bool   `json:"try_trigger_generation,omitempty"`
	Flush                bool   `json:"flush,omitempty"`
}

// ElevenLabsResponse is the structure we receive from ElevenLabs
type ElevenLabsResponse struct {
	Audio   string `json:"audio"`
	IsFinal bool   `json:"isFinal"`
}
