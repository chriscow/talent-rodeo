package handlers

import (
	"context"
	"fmt"
	"log/slog"
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
	_, err := h.js.CreateOrUpdateConsumer(ctx, streamName, consumerConfig)
	if err != nil {
		h.logger.Error("failed to create/update consumer", "subject", subject, "error", err)
		return fmt.Errorf("failed to create consumer for subject %s: %w", subject, err)
	}

	// Subscribe to the subject.
	h.logger.Debug("registering handler", "subject", subject, "durable", durable)
	_, err = h.js.PullSubscribe(subject, durable)
	if err != nil {
		h.logger.Error("failed to subscribe to subject", "subject", subject, "error", err)
		return fmt.Errorf("failed to subscribe to subject %s: %w", subject, err)
	}

	// Start a goroutine to process incoming messages.
	go func() {
		sub, err := h.js.Consumer(ctx, streamName, durable)
		if err != nil {
			h.logger.Error("failed to get consumer", "subject", subject, "error", err)
			return
		}

		for {
			select {
			case <-ctx.Done():
				h.logger.Debug("generic handler context cancelled", "subject", subject)
				return
			default:
				// Try fetching one message with a short timeout.
				msgs, err := sub.Fetch(1, jetstream.FetchTimeout(1*time.Second))
				if err != nil {
					h.logger.Debug("no messages fetched", "subject", subject, "error", err)
					continue
				}

				for _, msg := range msgs {
					h.logger.Debug("processing message", "subject", subject, "data", string(msg.Data()))
					if err := callback(ctx, msg); err != nil {
						h.logger.Error("callback processing failed", "subject", subject, "error", err)
						// Optionally send a negative acknowledgement.
						_ = msg.Nak()
					} else {
						_ = msg.Ack()
					}
				}
			}
		}
	}()

	h.logger.Debug("handler registered successfully", "subject", subject)
	return nil
}
