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
	preferencesPrompt = `
		Analyze interview transcripts and extract job preferences.
		Focus on location preferences, work style, company preferences, and salary expectations.

		Provide your response in JSON.`
	TopicPreferencesExtracted = "rodeo.transcript.extracted.preferences"
)

type Preferences struct {
	PreferredLocations []string     `json:"preferredLocations" description:"The preferred locations of the candidate"`
	RemotePreference   string       `json:"remotePreference" description:"The preferred remote preference of the candidate" enum:"remote,hybrid,onsite"`
	WorkStyle          string       `json:"workStyle" description:"The preferred work style of the candidate" enum:"collaborative,independent"`
	CompanySize        string       `json:"companySize" description:"The preferred company size of the candidate" enum:"startup,mid-size,enterprise"`
	Industry           []string     `json:"industry" description:"The preferred industry of the candidate"`
	SalaryRange        *SalaryRange `json:"salaryRange,omitempty" description:"The preferred salary range of the candidate"`
}

type PreferencesHandler struct {
	ctx      context.Context
	js       jetstream.JetStream
	llm      minds.ContentGenerator
	logger   *slog.Logger
	consumer jetstream.Consumer
}

func NewPreferencesHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*PreferencesHandler, error) {
	const preferencesConsumerName = "RODEO-TRANSCRIPT-PREFERENCES-QUEUE"

	cons, err := getConsumer(ctx, js, rodeoStream, preferencesConsumerName, TopicJobDescriptionInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer: %w", err)
	}

	return &PreferencesHandler{
		ctx:      ctx,
		js:       js,
		llm:      llm,
		logger:   logger,
		consumer: cons,
	}, nil
}

// Start begins processing messages from the NATS stream.
// It will continue processing until the context is cancelled.
func (h *PreferencesHandler) Start(ctx context.Context) error {
	h.logger.Info("Starting preferences handler")
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Stopping preferences handler", "reason", ctx.Err())
			return ctx.Err()
		default:
			if err := h.processNextMessage(ctx); err != nil {
				h.logger.Error("Error processing message", "error", err)
				continue
			}
		}
	}
}

func (h *PreferencesHandler) Close() error {
	return nil
}

func (h *PreferencesHandler) processNextMessage(ctx context.Context) error {
	msgs, err := h.consumer.Fetch(1, jetstream.FetchMaxWait(250*time.Millisecond))
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			return nil
		}
		return fmt.Errorf("failed to fetch message: %w", err)
	}

	for msg := range msgs.Messages() {
		if err := h.handleMessage(ctx, msg); err != nil {
			h.logger.Error("Failed to handle message", "error", err)
			msg.Term()
			return err
		}
	}
	return nil
}

// ExtractSkills analyzes the transcript to identify skills and expertise levels.
func (h *PreferencesHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {

	transcript := string(msg.Data())

	preferences, err := h.extract(ctx, transcript)
	if err != nil {
		return fmt.Errorf("failed to extract preferences: %w", err)
	}

	preferencesJSON, err := json.Marshal(preferences)
	if err != nil {
		return fmt.Errorf("failed to marshal preferences: %w", err)
	}

	_, err = h.js.Publish(ctx, TopicPreferencesExtracted, preferencesJSON)
	if err != nil {
		return fmt.Errorf("failed to publish preferences: %w", err)
	}

	msg.Ack()
	return nil
}

func (h *PreferencesHandler) extract(ctx context.Context, transcript string) (*Preferences, error) {
	msgs := minds.Messages{{
		Role:    minds.RoleSystem,
		Content: preferencesPrompt,
	},
		{
			Role:    minds.RoleUser,
			Content: transcript,
		},
	}

	schema, err := minds.NewResponseSchema("Preferences", "Job preferences", Preferences{})
	if err != nil {
		return nil, fmt.Errorf("failed to create preferences schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Extracting preferences from transcript")

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to extract preferences: %w", err)
	}

	var output Preferences
	if err := json.Unmarshal([]byte(result.String()), &output); err != nil {
		return nil, fmt.Errorf("failed to unmarshal preferences: %w", err)
	}

	return &output, nil
}
