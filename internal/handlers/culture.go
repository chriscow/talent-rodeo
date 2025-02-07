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
	culturePrompt = `You are an expert culture analyst. Your task is to extract and structure cultural information from job descriptions.
		Focus on identifying:
		- Overall company culture and work environment
		- Core company values
		- Team dynamics and collaboration style
		- Work style and expectations

		Provide structured output with clear categorization of all cultural aspects.`
)

// Culture defines the cultural aspects of a job
type Culture struct {
	Description  string   `json:"description" description:"Detailed description of the company culture and work environment"`
	Values       []string `json:"values" description:"Core company values extracted from the job description"`
	TeamDynamics string   `json:"teamDynamics" description:"Description of team dynamics and collaboration style"`
	WorkStyle    string   `json:"workStyle" description:"Description of work style and expectations"`
}

type CultureHandler struct {
	ctx      context.Context
	js       jetstream.JetStream
	llm      minds.ContentGenerator
	logger   *slog.Logger
	consumer jetstream.Consumer
}

func NewCultureHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*CultureHandler, error) {
	const cultureConsumerName = "RODEO-JOBDESC-CULTURE-QUEUE"

	cons, err := getConsumer(ctx, js, rodeoStream, cultureConsumerName, TopicJobDescriptionInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer: %w", err)
	}

	return &CultureHandler{
		ctx:      ctx,
		js:       js,
		llm:      llm,
		logger:   logger,
		consumer: cons,
	}, nil
}

func (h *CultureHandler) Start(ctx context.Context) error {
	h.logger.Info("Starting culture handler")
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Stopping culture handler", "reason", ctx.Err())
			return ctx.Err()
		default:
			if err := h.processNextMessage(ctx); err != nil {
				h.logger.Error("Error processing message", "error", err)
				continue
			}
		}
	}
}

func (h *CultureHandler) processNextMessage(ctx context.Context) error {
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

func (h *CultureHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing culture message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	var input JobDescriptionInput
	if err := json.Unmarshal(msg.Data(), &input); err != nil {
		h.logger.Error("Failed to unmarshal input", "error", err)
		msg.Ack()
		return nil
	}

	culture, err := h.extract(ctx, input.Description)
	if err != nil {
		return fmt.Errorf("failed to extract culture: %w", err)
	}

	cultureJSON, err := json.Marshal(culture)
	if err != nil {
		return fmt.Errorf("failed to marshal culture: %w", err)
	}

	h.logger.Debug("Publishing culture",
		"description_length", len(culture.Description),
		"values_count", len(culture.Values),
		"team_dynamics", culture.TeamDynamics)

	_, err = h.js.Publish(ctx, TopicCultureExtracted, cultureJSON)
	if err != nil {
		return fmt.Errorf("failed to publish culture: %w", err)
	}

	msg.Ack()
	return nil
}

func (h *CultureHandler) extract(ctx context.Context, description string) (*Culture, error) {
	msgs := minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: culturePrompt,
		},
		{
			Role:    minds.RoleUser,
			Content: fmt.Sprintf("Please analyze this job description and extract all culture-related information:\n\n%s", description),
		},
	}

	schema, err := minds.NewResponseSchema("Culture", "Cultural aspects", Culture{})
	if err != nil {
		return nil, fmt.Errorf("failed to create culture schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Extracting culture details from job description")

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to extract culture details: %w", err)
	}

	var culture Culture
	if err := json.Unmarshal([]byte(result.String()), &culture); err != nil {
		return nil, fmt.Errorf("failed to unmarshal culture details: %w", err)
	}

	h.logger.Debug("Extracted culture details",
		"description_length", len(culture.Description),
		"values_count", len(culture.Values),
		"team_dynamics", culture.TeamDynamics,
		"work_style", culture.WorkStyle)

	return &culture, nil
}
