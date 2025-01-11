// internal/handlers/handlers.go
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"talent-rodeo/internal/response"
	"time"

	openairt "github.com/WqyJh/go-openai-realtime"
)

type PageData struct {
	Title               string
	RequireConfirmation bool
}

type InterviewResponse struct {
	Transcript string `json:"transcript"`
	Response   string `json:"response"`
}

type Handler struct {
	client *openairt.Client
}

func NewHandler() (*Handler, error) {
	client := openairt.NewClient(os.Getenv("OPENAI_API_KEY"))

	return &Handler{
		client: client,
	}, nil
}

func (h *Handler) InterviewPage(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		Title:               "Job Interview - JobMatcher AI",
		RequireConfirmation: true,
	}

	patterns := []string{"layout.tmpl", "partials/*.tmpl", "pages/interview.tmpl"}
	if err := response.NamedTemplate(w, http.StatusOK, data, "layout", patterns...); err != nil {
		log.Printf("Error rendering template: %v", err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}

}

func (h *Handler) HandleAudio(w http.ResponseWriter, r *http.Request) {
	log.Println("Received audio request")
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the multipart form data
	err := r.ParseMultipartForm(10 << 20) // 10 MB max
	if err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "Error retrieving audio file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read the audio data
	audioData, err := io.ReadAll(file)
	if err != nil {
		log.Printf("Error reading audio data: %v", err)
		http.Error(w, "Error reading audio data", http.StatusInternalServerError)
		return
	}

	// Log audio data information
	log.Printf("Received audio data size: %d bytes", len(audioData))

	// Save the audio file for debugging (temporary)
	tempFile, err := os.CreateTemp("", "audio-*.wav")
	if err != nil {
		log.Printf("Error creating temp file: %v", err)
	} else {
		defer tempFile.Close()
		if _, err := tempFile.Write(audioData); err != nil {
			log.Printf("Error writing temp file: %v", err)
		}
		log.Printf("Saved audio file for debugging: %s", tempFile.Name())
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to OpenAI
	conn, err := h.client.Connect(ctx)
	if err != nil {
		http.Error(w, "Error connecting to OpenAI", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Initialize conversation with audio capabilities
	err = conn.SendMessage(ctx, openairt.SessionUpdateEvent{
		Session: openairt.ClientSession{
			Modalities: []openairt.Modality{openairt.ModalityText, openairt.ModalityAudio},
			InputAudioTranscription: &openairt.InputAudioTranscription{
				Model: "whisper-1",
			},
		},
	})
	if err != nil {
		http.Error(w, "Error initializing session", http.StatusInternalServerError)
		return
	}

	// Send the audio data
	base64Audio := base64.StdEncoding.EncodeToString(audioData)
	err = conn.SendMessage(ctx, openairt.InputAudioBufferAppendEvent{
		Audio: base64Audio,
	})
	if err != nil {
		http.Error(w, "Error sending audio", http.StatusInternalServerError)
		return
	}

	err = conn.SendMessage(ctx, openairt.InputAudioBufferCommitEvent{})
	if err != nil {
		http.Error(w, "Error committing audio", http.StatusInternalServerError)
		return
	}

	// Request response
	err = conn.SendMessage(ctx, openairt.ResponseCreateEvent{
		Response: openairt.ResponseCreateParams{
			Modalities: []openairt.Modality{openairt.ModalityText},
		},
	})
	if err != nil {
		http.Error(w, "Error requesting response", http.StatusInternalServerError)
		return
	}

	// Process the response
	var transcript, response string
	for {
		event, err := conn.ReadMessage(ctx)
		if err != nil {
			log.Printf("Error reading OpenAI response: %v", err)
			http.Error(w, "Error reading response", http.StatusInternalServerError)
			return
		}

		// Log each event type for debugging
		log.Printf("Received event type: %s", event.ServerEventType())

		switch evt := event.(type) {
		case openairt.ConversationItemInputAudioTranscriptionCompletedEvent:
			transcript = evt.Transcript
		case openairt.ResponseTextDoneEvent:
			response = evt.Text
			// Send the response back to the client
			json.NewEncoder(w).Encode(InterviewResponse{
				Transcript: transcript,
				Response:   response,
			})
			return
		}
	}
}
