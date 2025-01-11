package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"

	// Twilio
	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"

	// OpenAI Realtime
	"github.com/WqyJh/go-openai-realtime"
	openairt "github.com/WqyJh/go-openai-realtime"

	// Gorilla
	"github.com/gorilla/websocket"

	// For environment variables
	"github.com/joho/godotenv"
)

// Utility function to ensure environment variables are set
func checkEnv() error {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found; continuing...")
	}
	required := []string{
		"TWILIO_ACCOUNT_SID",
		"TWILIO_AUTH_TOKEN",
		"TWILIO_PHONE_NUMBER",
		"TWIML_HANDLER",
		"APP_DOMAIN",
		"OPENAI_API_KEY",
		"ELEVENLABS_API_KEY",
		"ELEVENLABS_VOICE_ID",
	}
	for _, v := range required {
		if os.Getenv(v) == "" {
			return fmt.Errorf("%s is not set", v)
		}
	}
	return nil
}

func main() {
	if err := checkEnv(); err != nil {
		log.Fatal(err)
	}

	// Set up routes
	http.HandleFunc("/initiate-call", initiateCall)
	http.HandleFunc("/twiml", handleTwiML)          // TwiML that starts streaming
	http.HandleFunc("/handle-speech", handleSpeech) // TwiML speech gather

	// Route for real-time audio streaming from Twilio to ElevenLabs
	// This route is invoked in our TwiML <Stream> tag
	http.HandleFunc("/stream", handleWebSocket)

	port := ":2304"
	log.Printf("Listening on %s...", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

// initiateCall triggers an outbound Twilio call
func initiateCall(w http.ResponseWriter, r *http.Request) {
	toNumber := r.FormValue("phone")
	fromNumber := os.Getenv("TWILIO_PHONE_NUMBER")

	client := twilio.NewRestClient()
	params := &twilioApi.CreateCallParams{}
	params.SetTo(toNumber)
	params.SetFrom(fromNumber)
	params.SetUrl(os.Getenv("TWIML_HANDLER")) // Points to /twiml route
	resp, err := client.Api.CreateCall(params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Call initiated! Call SID: %s\n", *resp.Sid)
}

// handleTwiML returns initial TwiML that streams audio and captures user speech
func handleTwiML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	// <Stream> will send audio to our /stream endpoint via WebSocket
	// <Gather> listens for speech from user
	twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Stream url="wss://%s/stream" track="both_tracks"/>
    <Gather input="speech" action="/handle-speech" method="POST" timeout="5">
        <Say>Hello! Please say something after the beep.</Say>
    </Gather>
</Response>`, os.Getenv("APP_DOMAIN"))

	w.Write([]byte(twiml))
}

// handleSpeech receives recognized speech from Twilio <Gather> and can respond
func handleSpeech(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not parse form", http.StatusBadRequest)
		return
	}
	speechResult := r.FormValue("SpeechResult")

	w.Header().Set("Content-Type", "application/xml")
	response := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Say>You said: %s</Say>
    <Pause length="1"/>
    <Say>Goodbye!</Say>
    <Hangup/>
</Response>`, speechResult)

	w.Write([]byte(response))
}
