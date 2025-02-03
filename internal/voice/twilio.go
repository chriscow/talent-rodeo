package voice

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"
)

// TwilioMessage represents messages received from Twilio WebSocket
type TwilioMessage struct {
	Event string `json:"event"`
	Media struct {
		Payload string `json:"payload"`
	} `json:"media"`
	StreamSid string `json:"streamSid"`
	Start     *struct {
		StreamSid string `json:"streamSid"`
		CallSid   string `json:"callSid"`
	} `json:"start,omitempty"`
}

// TwilioAudioWriter implements the AudioStreamWriter interface for Twilio
type TwilioAudioWriter struct {
	conn     *websocket.Conn
	streamID string
	mu       sync.Mutex // Protects the connection
	logger   *slog.Logger
}

func NewTwilioAudioWriter(conn *websocket.Conn, logger *slog.Logger) *TwilioAudioWriter {
	return &TwilioAudioWriter{
		conn:   conn,
		logger: logger,
	}
}

/*
	type AudioSink interface {
		Write(ctx context.Context, chunk AudioChunk) error
		Close() error
	}
*/
func (w *TwilioAudioWriter) Write(ctx context.Context, chunk AudioChunk) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Convert raw audio data to base64
		payload := base64.StdEncoding.EncodeToString(chunk.Data)

		msg := TwilioMessage{
			Event:     "media",
			StreamSid: w.streamID,
			Media: struct {
				Payload string `json:"payload"`
			}{
				Payload: payload,
			},
		}

		if err := w.conn.WriteJSON(msg); err != nil {
			return fmt.Errorf("error writing to Twilio: %w", err)
		}
		return nil
	}
}

func (w *TwilioAudioWriter) Close() error {
	w.logger.Debug("closing Twilio audio writer")
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn != nil {
		return w.conn.Close()
	}
	return nil
}
