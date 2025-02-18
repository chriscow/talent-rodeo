package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// HandlerFunc defines a callback function signature for processing messages.
type HandlerFunc func(ctx context.Context, msg jetstream.Msg) error

// GenericHandler provides common functionality to register message handlers for specific subjects.
type GenericHandler struct {
	js     jetstream.JetStream
	logger *slog.Logger
}

// NewGenericHandler creates a new instance of GenericHandler.
func NewGenericHandler(js jetstream.JetStream, logger *slog.Logger) *GenericHandler {
	return &GenericHandler{
		js:     js,
		logger: logger,
	}
}

// Register registers a message subject along with a callback function.
// It creates (or updates) a consumer for the given stream and subject,
// subscribes to messages, and processes them using the callback.
func (h *GenericHandler) Register(ctx context.Context, streamName string, subject string, durable string, callback HandlerFunc) error {
	consumerConfig := jetstream.ConsumerConfig{
		Name:          durable,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckWait:       30 * time.Second,
	}

	// Create or update consumer for this subject.
	consumer, err := h.js.CreateOrUpdateConsumer(ctx, streamName, consumerConfig)
	if err != nil {
		h.logger.Error("failed to create/update consumer", "subject", subject, "error", err)
		return fmt.Errorf("failed to create consumer for subject %s: %w", subject, err)
	}

	// Start a goroutine to process incoming messages.
	go func() {
		for {
			select {
			case <-ctx.Done():
				h.logger.Debug("generic handler context cancelled", "subject", subject)
				return
			default:
				// Try fetching messages with a short timeout.
				msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(250*time.Millisecond))
				if err != nil {
					if strings.Contains(err.Error(), "timeout") {
						continue
					}
					if strings.Contains(err.Error(), "nats: connection closed") {
						// This is expected during cleanup/shutdown
						h.logger.Debug("connection closed (expected during cleanup)",
							"subject", subject,
							"error", err)
						return
					}
					h.logger.Error("failed to fetch messages",
						"subject", subject,
						"error", err)
					time.Sleep(100 * time.Millisecond) // Back off on error
					continue
				}

				// Process each message in the batch
				for msg := range msgs.Messages() {
					select {
					case <-ctx.Done():
						h.logger.Debug("context cancelled while processing message", "subject", subject)
						msg.Term() // Terminate message processing on context cancellation
						return
					default:
						h.logger.Debug("processing message", "subject", subject, "data", string(msg.Data()))
						if err := callback(ctx, msg); err != nil {
							h.logger.Error("callback processing failed", "subject", subject, "error", err)
							msg.Term()
						} else {
							msg.Ack()
						}
					}
				}
			}
		}
	}()

	h.logger.Debug("handler registered successfully", "subject", subject)
	return nil
}
