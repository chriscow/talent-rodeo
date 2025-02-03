package voice

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

// Session manages the lifecycle of a voice chat interaction between Twilio and OpenAI.
// It handles WebSocket connections, message routing, and coordination of the audio
// streaming pipeline. Session is responsible for the high-level chat workflow,
// including initialization, cleanup, and error handling of the WebSocket layer.
//
// Session delegates the complexity of managing bidirectional audio streams to a
// Coordinator, allowing the Session to focus on connection lifecycle and protocol
// handling without mixing concerns.
type Session struct {
	coordinator *Coordinator
	openAI      *OpenAIClient
	twilio      *TwilioAudioWriter
	logger      *slog.Logger
	cleanup     []io.Closer
}

func NewSession(twilioWS *websocket.Conn, logger *slog.Logger) (*Session, error) {
	// Initialize OpenAI client
	openAI := NewOpenAIClient(logger, nil, nil)
	twilio := NewTwilioAudioWriter(twilioWS, logger)

	// Create recording directory if it doesn't exist
	recordingDir := "recordings"
	if err := os.MkdirAll(recordingDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create recordings directory: %w", err)
	}

	// Create the conversation recorder
	recorder, err := NewConversationRecorder(
		fmt.Sprintf("recordings/conv_%s.ulaw", time.Now().Format("20060102_150405")),
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("create recorder: %w", err)
	}

	// Create audio processors
	inputProcessors := []AudioProcessor{
		recorder,
		&SilenceDetector{
			threshold: 0.1,
			onSilence: func(d time.Duration) {
				if d > 2*time.Second {
					logger.Debug("input silence detected", "duration", d)
				}
			},
		},
	}

	outputProcessors := []AudioProcessor{
		recorder,
		&SilenceDetector{
			threshold: 0.1,
			onSilence: func(d time.Duration) {
				if d > 2*time.Second {
					logger.Debug("output silence detected", "duration", d)
				}
			},
		},
	}

	// Create coordinator with processing pipelines
	coordinator := NewCoordinator(
		logger,
		openAI,
		twilio,
		inputProcessors,
		outputProcessors,
		WithShutdownTimeout(10*time.Second),
	)

	return &Session{
		coordinator: coordinator,
		openAI:      openAI,
		twilio:      twilio,
		logger:      logger,
		cleanup:     []io.Closer{recorder},
	}, nil
}

func (s *Session) Start(ctx context.Context) error {
	defer s.Close()

	if err := s.openAI.Connect(); err != nil {
		return fmt.Errorf("error connecting to OpenAI: %w", err)
	}

	if err := s.openAI.StartSession(ctx); err != nil {
		return fmt.Errorf("error starting OpenAI session: %w", err)
	}

	// Handle incoming Twilio messages
	errChan := make(chan error, 2)
	go s.handleTwilioMessages(ctx, errChan)

	// Start the coordinator
	if err := s.coordinator.Start(ctx); err != nil {
		return fmt.Errorf("coordinator error: %w", err)
	}

	select {
	case err := <-errChan:
		return fmt.Errorf("error during chat session: %w", err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) Close() {
	s.logger.Info("cleaning up session")
	if s.coordinator != nil {
		s.coordinator = nil
	}
	for _, c := range s.cleanup {
		c.Close()
	}
}

// handleTwilioMessages processes incoming Twilio WebSocket messages
func (cs *Session) handleTwilioMessages(ctx context.Context, errChan chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			var msg TwilioMessage
			if err := cs.twilio.conn.ReadJSON(&msg); err != nil {
				cs.logger.Error("Error reading Twilio message", "error", err)
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
					errChan <- fmt.Errorf("unexpected Twilio connection close: %w", err)
				}
				return
			}

			// Handle different message types
			switch msg.Event {
			case "start":
				cs.twilio.streamID = msg.Start.StreamSid
				cs.logger.Info("Twilio stream started", "streamId", msg.Start.StreamSid)
			case "stop":
				cs.logger.Info(greenBk("Twilio stream ended"), "streamId", msg.StreamSid)
				cs.Close()
				return
			case "media":
				if err := cs.handleTwilioAudio(ctx, msg.Media.Payload); err != nil {
					cs.logger.Error("Error handling Twilio audio", "error", err)
					errChan <- fmt.Errorf("error handling Twilio audio: %w", err)
					return
				}
			}
		}
	}
}

// handleTwilioAudio processes audio data from Twilio
func (cs *Session) handleTwilioAudio(ctx context.Context, payload string) error {
	if payload == "" {
		return nil
	}

	// Decode base64 audio from Twilio
	audio, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return fmt.Errorf("error decoding audio data: %w", err)
	}

	chunk := AudioChunk{
		Data:    audio,
		Speaker: "human",
	}

	processedChunk, err := cs.coordinator.inputPipeline.Process(ctx, chunk)
	if err != nil {
		return fmt.Errorf("input pipeline error: %w", err)
	}

	// Send the audio data through our interface
	if err := cs.openAI.WriteAudio(ctx, processedChunk.Data); err != nil {
		cs.logger.Error("Error sending audio to OpenAI", "error", err)
		return fmt.Errorf("error sending audio to text reader: %w", err)
	}

	return nil
}
