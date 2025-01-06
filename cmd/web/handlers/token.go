package handlers

/*
WebRTC experiment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// RealtimeSessionRequest represents the request body we send to OpenAI
type RealtimeSessionRequest struct {
	Model string `json:"model"`
	Voice string `json:"voice"`
}

// RealtimeSessionResponse represents the expected response from OpenAI
type RealtimeSessionResponse struct {
	ClientSecret struct {
		Value string `json:"value"`
	} `json:"client_secret"`
}

// GetRealtimeToken handles requests for OpenAI realtime session tokens
func (h *Handler) GetRealtimeToken(w http.ResponseWriter, r *http.Request) {
	// Prepare the request to OpenAI's sessions endpoint
	sessionReq := RealtimeSessionRequest{
		Model: "gpt-4o-realtime-preview-2024-12-17",
		Voice: "verse",
	}

	// Convert request to JSON
	reqBody, err := json.Marshal(sessionReq)
	if err != nil {
		h.log.Printf("error marshaling session request: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Create the HTTP request to OpenAI
	req, err := http.NewRequestWithContext(
		r.Context(),
		"POST",
		"https://api.openai.com/v1/realtime/sessions",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		h.log.Printf("error creating request: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set required headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("OPENAI_API_KEY")))
	req.Header.Set("Content-Type", "application/json")

	// Make the request to OpenAI
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		h.log.Printf("error making request to OpenAI: %v", err)
		http.Error(w, "Failed to get token", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Check if the response was successful
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		h.log.Printf("OpenAI error: status=%d body=%s", resp.StatusCode, string(body))
		http.Error(w, "Failed to get token from OpenAI", resp.StatusCode)
		return
	}

	// Copy the response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Stream the response body directly to the client
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		h.log.Printf("error copying response: %v", err)
		// Cannot write error to response here as headers are already sent
	}
}
*/
