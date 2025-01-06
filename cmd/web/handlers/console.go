package handlers

/*
import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

type Session struct {
	ID             string
	PeerConnection *webrtc.PeerConnection
	DataChannel    *webrtc.DataChannel
}

type Handler struct {
	log      *log.Logger
	api      *webrtc.API
	sessions sync.Map // store WebRTC sessions
}

func New(log *log.Logger) *Handler {
	s := webrtc.SettingEngine{}
	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))

	return &Handler{
		log: log,
		api: api,
	}
}

func (h *Handler) StartSession(w http.ResponseWriter, r *http.Request) {
	// Create a new PeerConnection
	peerConnection, err := h.api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create a data channel for events
	dataChannel, err := peerConnection.CreateDataChannel("oai-events", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Handle data channel messages
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		var event map[string]interface{}
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			h.log.Printf("error parsing event: %v", err)
			return
		}
		h.handleEvent(dataChannel, event)
	})

	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Read the SDP offer from the request body
	// var offer webrtc.SessionDescription
	// if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
	// 	http.Error(w, err.Error(), http.StatusBadRequest)
	// 	return
	// }

	// Set the remote description
	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update the ICE connection state handling to properly handle closed states
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		if connectionState == webrtc.ICEConnectionStateFailed ||
			connectionState == webrtc.ICEConnectionStateClosed {
			h.log.Printf("ICE Connection State: %s", connectionState)
			// Handle cleanup
		}
	})

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set local description
	if err := peerConnection.SetLocalDescription(answer); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionID := generateSessionID()
	session := &Session{
		ID:             sessionID,
		PeerConnection: peerConnection,
		DataChannel:    dataChannel,
	}
	h.sessions.Store(sessionID, session)

	// Send answer back to client
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(answer)
}

func (h *Handler) handleEvent(dc *webrtc.DataChannel, event map[string]interface{}) {
	eventType, _ := event["type"].(string)

	switch eventType {
	case "conversation.item.create":
		if item, ok := event["item"].(map[string]interface{}); ok {
			if content, ok := item["content"].([]interface{}); ok {
				if len(content) > 0 {
					if text, ok := content[0].(map[string]interface{}); ok {
						message := text["text"].(string)
						if containsColorPaletteRequest(message) {
							palette := generateColorPalette("bright and modern")
							response, _ := json.Marshal(palette)
							dc.Send(response)
						}
					}
				}
			}
		}
	}
}

func (h *Handler) StopSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	if s, ok := h.sessions.LoadAndDelete(sessionID); ok {
		session := s.(*Session)
		if session.PeerConnection != nil {
			session.PeerConnection.Close()
		}
	}
	w.WriteHeader(http.StatusOK)
}

func generateSessionID() string {
	return time.Now().Format("20060102150405")
}
*/
