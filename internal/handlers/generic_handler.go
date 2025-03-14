package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/chriscow/minds"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	DefaultMaxAckPending = 0
)

// HandlerFunc defines a callback function signature for processing messages.
type HandlerFunc func(ctx context.Context, msg jetstream.Msg) error

// LLMProcessor defines the interface for handlers that use LLM processing
type LLMProcessor interface {
	// GetSystemPrompt returns the system prompt for LLM processing
	GetSystemPrompt() string
	// GetResponseSchema returns the schema for LLM response
	GetResponseSchema() (*minds.ResponseSchema, error)
	// GetOutputTopic returns the topic to publish LLM processed results
	GetOutputTopic() string
}

// GenericHandler provides common functionality to register message handlers for specific subjects.
type GenericHandler struct {
	js     jetstream.JetStream
	logger *slog.Logger
	llm    minds.ContentGenerator
}

// NewGenericHandler creates a new instance of GenericHandler.
func NewGenericHandler(js jetstream.JetStream, logger *slog.Logger, llm minds.ContentGenerator) *GenericHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &GenericHandler{
		js:     js,
		logger: logger,
		llm:    llm, // llm can be nil for non-LLM handlers
	}
}

// Register registers a message subject along with a callback function.
// It creates (or updates) a consumer for the given stream and subject,
// subscribes to messages, and processes them using the callback.
// maxAckPending is the maximum number of messages that can be processed concurrently.
// If maxAckPending is 0, the default value (1000) will be used. Set to -1 for unlimited.
func (h *GenericHandler) Register(ctx context.Context, streamName string, subject string, durable string, maxAckPending int, callback HandlerFunc) error {
	consumerConfig := jetstream.ConsumerConfig{
		Name:          durable,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckWait:       30 * time.Second,
		MaxAckPending: maxAckPending,
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

// RegisterLLMHandler registers a handler that uses LLM processing.
// It creates a consumer for the given stream and subject, and processes messages using LLM.
func (h *GenericHandler) RegisterLLMHandler(ctx context.Context, streamName string, subject string, durable string, processor LLMProcessor, maxAckPending int) error {
	if h.llm == nil {
		return fmt.Errorf("LLM content generator is required for LLM handlers")
	}

	// Create the callback function that will use the LLM processor
	callback := func(ctx context.Context, msg jetstream.Msg) error {
		input := string(msg.Data())
		h.logger.Debug("processing message with LLM",
			"subject", subject,
			"data_size", len(input))

		// Get schema from processor
		schema, err := processor.GetResponseSchema()
		if err != nil {
			return fmt.Errorf("failed to get response schema: %w", err)
		}

		// Create LLM request
		request := minds.NewRequest(minds.Messages{
			{Role: minds.RoleSystem, Content: processor.GetSystemPrompt()},
			{Role: minds.RoleUser, Content: input},
		}, minds.WithResponseSchema(*schema))

		// Generate content using LLM
		result, err := h.llm.GenerateContent(ctx, request)
		if err != nil {
			return fmt.Errorf("failed to generate content: %w", err)
		}

		// Parse result into schema object
		var output interface{}
		if err := json.Unmarshal([]byte(result.String()), &output); err != nil {
			return fmt.Errorf("failed to unmarshal LLM response: %w", err)
		}

		// Marshal the output for publishing
		outputJSON, err := json.Marshal(output)
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}

		// Publish the processed result
		_, err = h.js.Publish(ctx, processor.GetOutputTopic(), outputJSON)
		if err != nil {
			return fmt.Errorf("failed to publish processed result: %w", err)
		}

		h.logger.Debug("published LLM processed result",
			"output_topic", processor.GetOutputTopic(),
			"output_size", len(outputJSON))

		return nil
	}

	// Register using the standard registration method
	return h.Register(ctx, streamName, subject, durable, maxAckPending, callback)
}
