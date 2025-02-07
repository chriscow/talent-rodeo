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
	workplacePrompt = `You are an expert workplace analyst. Your task is to extract and structure workplace information from job descriptions.
		Focus on identifying:
		- Work location details
		- Work arrangement type (remote/hybrid/on-site)
		- Schedule and time requirements
		- Time zone requirements
		- Travel requirements
		
		Provide structured output with clear categorization of all workplace arrangements.`
)

// Workplace defines workplace details
type Workplace struct {
	Location       string `json:"location" description:"Location"`
	Type           string `json:"type" description:"Type"`         // e.g., "Remote", "Hybrid", "On-site"
	Schedule       string `json:"schedule" description:"Schedule"` // e.g., "Full-time", "Part-time", "Contract"
	TimeZone       string `json:"timeZone,omitempty" description:"Time zone"`
	TravelRequired string `json:"travelRequired,omitempty" description:"Travel required"`
}

type WorkplaceHandler struct {
	ctx      context.Context
	js       jetstream.JetStream
	llm      minds.ContentGenerator
	logger   *slog.Logger
	consumer jetstream.Consumer
}

func NewWorkplaceHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*WorkplaceHandler, error) {
	const workplaceConsumerName = "RODEO-JOBDESC-WORKPLACE-QUEUE"

	cons, err := getConsumer(ctx, js, rodeoStream, workplaceConsumerName, TopicJobDescriptionInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer: %w", err)
	}

	return &WorkplaceHandler{
		ctx:      ctx,
		js:       js,
		llm:      llm,
		logger:   logger,
		consumer: cons,
	}, nil
}

func (h *WorkplaceHandler) Start(ctx context.Context) error {
	h.logger.Info("Starting workplace handler")
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Stopping workplace handler", "reason", ctx.Err())
			return ctx.Err()
		default:
			if err := h.processNextMessage(ctx); err != nil {
				h.logger.Error("Error processing message", "error", err)
				continue
			}
		}
	}
}

func (h *WorkplaceHandler) processNextMessage(ctx context.Context) error {
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

func (h *WorkplaceHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing workplace message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	var input JobDescriptionInput
	if err := json.Unmarshal(msg.Data(), &input); err != nil {
		h.logger.Error("Failed to unmarshal input", "error", err)
		msg.Ack()
		return nil
	}

	workplace, err := h.extract(ctx, input.Description)
	if err != nil {
		return fmt.Errorf("failed to extract workplace: %w", err)
	}

	workplaceJSON, err := json.Marshal(workplace)
	if err != nil {
		return fmt.Errorf("failed to marshal workplace: %w", err)
	}

	h.logger.Debug("Publishing workplace",
		"location", workplace.Location,
		"type", workplace.Type,
		"schedule", workplace.Schedule)

	_, err = h.js.Publish(ctx, TopicWorkplaceExtracted, workplaceJSON)
	if err != nil {
		return fmt.Errorf("failed to publish workplace: %w", err)
	}

	msg.Ack()
	return nil
}

func (h *WorkplaceHandler) extract(ctx context.Context, description string) (*Workplace, error) {
	msgs := minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: workplacePrompt,
		},
		{
			Role:    minds.RoleUser,
			Content: fmt.Sprintf("Please analyze this job description and extract all workplace-related information:\n\n%s", description),
		},
	}

	schema, err := minds.NewResponseSchema("Workplace", "Workplace details", Workplace{})
	if err != nil {
		return nil, fmt.Errorf("failed to create workplace schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Extracting workplace details from job description")

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to extract workplace details: %w", err)
	}

	var workplace Workplace
	if err := json.Unmarshal([]byte(result.String()), &workplace); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workplace details: %w", err)
	}

	h.logger.Debug("Extracted workplace details",
		"location", workplace.Location,
		"type", workplace.Type,
		"schedule", workplace.Schedule,
		"timezone", workplace.TimeZone,
		"travel", workplace.TravelRequired)

	return &workplace, nil
}
