package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	TopicVoiceInput  = "rodeo.voice.input"
	TopicVoiceResult = "rodeo.voice.result"
)

// CallRequest represents the input data for making a phone call
type CallRequest struct {
	PhoneNumber string `json:"phone_number" description:"The phone number to call"`
}

// CallResult represents the result of a phone call attempt
type CallResult struct {
	CallSID     string `json:"call_sid" description:"The Twilio call SID"`
	PhoneNumber string `json:"phone_number" description:"The phone number that was called"`
	Status      string `json:"status" description:"The status of the call" enum:"initiated,failed"`
}

// TwilioClient defines the interface for making phone calls
type TwilioClient interface {
	MakeCall(ctx context.Context, fromNumber, toNumber, twiml string) (string, error)
}

// CallConfig holds configuration for the voice handler
type CallConfig struct {
	FromNumber string // The Twilio number to call from
	Twiml      string // TwiML template for the call
}

// VoiceHandler handles voice calls
type VoiceHandler struct {
	ctx      context.Context
	logger   *slog.Logger
	js       jetstream.JetStream
	consumer jetstream.Consumer
	twilio   TwilioClient
	config   CallConfig
}

// NewVoiceHandler creates a new VoiceHandler instance
func NewVoiceHandler(ctx context.Context, js jetstream.JetStream, twilio TwilioClient, logger *slog.Logger, config CallConfig) (*VoiceHandler, error) {
	const voiceConsumerName = "RODEO-VOICE-CALLER-QUEUE"

	consumer, err := getConsumer(ctx, js, rodeoStream, voiceConsumerName, TopicVoiceInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer: %w", err)
	}

	return &VoiceHandler{
		ctx:      ctx,
		logger:   logger,
		js:       js,
		consumer: consumer,
		twilio:   twilio,
		config:   config,
	}, nil
}

// Start begins processing messages from the NATS stream.
// It will continue processing until the context is cancelled.
func (h *VoiceHandler) Start(ctx context.Context) error {
	h.logger.Info("Starting voice call handler")
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Stopping voice call handler", "reason", ctx.Err())
			return ctx.Err()
		default:
			if err := h.processNextMessage(ctx); err != nil {
				h.logger.Error("Error processing message", "error", err)
				continue
			}
		}
	}
}

// processNextMessage fetches and processes the next message from the stream
func (h *VoiceHandler) processNextMessage(ctx context.Context) error {
	msgs, err := h.consumer.Fetch(1)
	if err != nil {
		return fmt.Errorf("failed to fetch messages: %w", err)
	}

	msg := <-msgs.Messages()
	if err := h.handleMessage(ctx, msg); err != nil {
		h.logger.Error("Failed to handle message", "error", err)
		msg.Term()
		return err
	}

	return nil
}

// handleMessage processes a single message containing a call request
func (h *VoiceHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing voice call message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	// Parse the call request
	var req CallRequest
	if err := json.Unmarshal(msg.Data(), &req); err != nil {
		msg.Term()
		return fmt.Errorf("failed to unmarshal call request: %w", err)
	}

	// Make the call
	result, err := h.makeCall(ctx, req)
	if err != nil {
		h.logger.Error("Failed to make call",
			"error", err,
			"phone_number", req.PhoneNumber)

		// Even on error, we publish a result
		result = &CallResult{
			PhoneNumber: req.PhoneNumber,
			Status:      "failed",
		}
	}

	// Publish the result
	if err := h.publishResult(ctx, result); err != nil {
		msg.Term()
		return fmt.Errorf("failed to publish result: %w", err)
	}

	// Acknowledge message only after successful processing
	msg.DoubleAck(ctx)
	return nil
}

// makeCall attempts to make a phone call using the Twilio client
func (h *VoiceHandler) makeCall(ctx context.Context, req CallRequest) (*CallResult, error) {
	h.logger.Debug("Making call with Twilio",
		"from_number", h.config.FromNumber,
		"to_number", req.PhoneNumber,
		"twiml", h.config.Twiml)

	callSID, err := h.twilio.MakeCall(ctx, h.config.FromNumber, req.PhoneNumber, h.config.Twiml)
	if err != nil {
		return nil, fmt.Errorf("failed to make twilio call: %w", err)
	}

	h.logger.Debug("Call initiated successfully",
		"call_sid", callSID,
		"phone_number", req.PhoneNumber)

	return &CallResult{
		CallSID:     callSID,
		PhoneNumber: req.PhoneNumber,
		Status:      "initiated",
	}, nil
}

// publishResult publishes the call result to NATS
func (h *VoiceHandler) publishResult(ctx context.Context, result *CallResult) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal call result: %w", err)
	}

	h.logger.Debug("Publishing call result",
		"status", result.Status,
		"phone_number", result.PhoneNumber,
		"call_sid", result.CallSID)

	if _, err := h.js.Publish(ctx, TopicVoiceResult, resultJSON); err != nil {
		return fmt.Errorf("failed to publish result: %w", err)
	}

	return nil
}

func (h *VoiceHandler) Close() error {
	return nil
}
