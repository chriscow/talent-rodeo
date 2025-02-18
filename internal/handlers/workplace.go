package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/chriscow/minds"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	workplacePrompt = `You are an expert workplace analyst. Your task is to extract and structure workplace information from job descriptions.
		Focus on identifying:
		- Location details (remote, hybrid, onsite)
		- Work schedule and hours
		- Office environment
		- Travel requirements

		Provide structured output with clear categorization of all workplace aspects.`
)

// Workplace defines the workplace details for a job
type Workplace struct {
	Location    string   `json:"location" description:"Location details (remote, hybrid, onsite)"`
	Type        string   `json:"type" description:"Type of workplace (office, remote, hybrid)"`
	Schedule    string   `json:"schedule" description:"Work schedule and hours"`
	Environment string   `json:"environment" description:"Office environment description"`
	Travel      []string `json:"travel" description:"Travel requirements"`
}

type WorkplaceHandler struct {
	llm     minds.ContentGenerator
	logger  *slog.Logger
	generic *GenericHandler
}

func NewWorkplaceHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*WorkplaceHandler, error) {
	handler := &WorkplaceHandler{
		llm:     llm,
		logger:  logger.With(slog.String("component", "workplace_handler")),
		generic: NewGenericHandler(js, logger),
	}

	if err := handler.generic.Register(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-JOBDESC-WORKPLACE-QUEUE", handler.handleMessage); err != nil {
		return nil, fmt.Errorf("failed to register workplace handler: %w", err)
	}

	return handler, nil
}

func (h *WorkplaceHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing workplace message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	var input JobDescriptionInput
	if err := json.Unmarshal(msg.Data(), &input); err != nil {
		h.logger.Error("Failed to unmarshal input", "error", err)
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

	_, err = h.generic.js.Publish(ctx, TopicWorkplaceExtracted, workplaceJSON)
	if err != nil {
		return fmt.Errorf("failed to publish workplace: %w", err)
	}

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
		"environment", workplace.Environment,
		"travel_requirements", len(workplace.Travel))

	return &workplace, nil
}
