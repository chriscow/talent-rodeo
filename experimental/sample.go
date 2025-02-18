package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

type Config struct {
	AgentID          string
	ElevenLabsAPIKey string
}

type VoiceResponse struct {
	XMLName xml.Name `xml:"Response"`
	Connect Connect  `xml:"Connect"`
}

type Connect struct {
	Stream Stream `xml:"Stream"`
}

type Stream struct {
	URL string `xml:"url,attr"`
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	config Config
	mu     sync.RWMutex
)

func loadConfig() error {
	if err := godotenv.Load(); err != nil {
		return err
	}

	config = Config{
		AgentID:          os.Getenv("AGENT_ID"),
		ElevenLabsAPIKey: os.Getenv("ELEVENLABS_API_KEY"),
	}
	return nil
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Twilio-ElevenLabs Integration Server",
	})
}

func handleIncomingCall(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	callSid := r.FormValue("CallSid")
	fromNumber := r.FormValue("From")
	if callSid == "" {
		callSid = "Unknown"
	}
	if fromNumber == "" {
		fromNumber = "Unknown"
	}

	log.Printf("Incoming call: CallSid=%s, From=%s", callSid, fromNumber)

	streamURL := fmt.Sprintf("wss://%s/media-stream-eleven", r.Host)
	response := VoiceResponse{
		Connect: Connect{
			Stream: Stream{
				URL: streamURL,
			},
		},
	}

	w.Header().Set("Content-Type", "application/xml")
	if err := xml.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func handleMediaStream(w http.ResponseWriter, r *http.Request) {
	/*
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}
		defer ws.Close()

		log.Println("WebSocket connection opened")

		mu.RLock()
		audioInterface := NewTwilioAudioInterface(ws)
		elevenLabsClient := NewElevenLabsClient(config.ElevenLabsAPIKey)
		mu.RUnlock()

		conversation, err := NewConversation(ConversationConfig{
			Client:         elevenLabsClient,
			AgentID:        config.AgentID,
			RequiresAuth:   true,
			AudioInterface: audioInterface,
			AgentCallback: func(text string) {
				log.Printf("Agent: %s", text)
			},
			UserCallback: func(text string) {
				log.Printf("User: %s", text)
			},
		})
		if err != nil {
			log.Printf("Error creating conversation: %v", err)
			return
		}

		conversation.StartSession()
		log.Println("Conversation started")

		defer func() {
			if err := conversation.EndSession(); err != nil {
				log.Printf("Error ending conversation: %v", err)
			}
			log.Println("Conversation ended")
		}()

		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.Printf("WebSocket read error: %v", err)
				}
				break
			}

			if len(message) == 0 {
				continue
			}

			var data map[string]any
			if err := json.Unmarshal(message, &data); err != nil {
				log.Printf("Error parsing message: %v", err)
				continue
			}

			audioInterface.HandleTwilioMessage(data)
		}
	*/
}

/*
func main() {
	if err := loadConfig(); err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/twilio/inbound_call", handleIncomingCall)
	http.HandleFunc("/media-stream-eleven", handleMediaStream)

	port := ":8000"
	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
*/
