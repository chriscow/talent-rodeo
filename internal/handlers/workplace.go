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
		generic: NewGenericHandler(js, logger, llm),
	}

	if err := handler.generic.RegisterLLMHandler(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-JOBDESC-WORKPLACE-QUEUE", handler); err != nil {
		return nil, fmt.Errorf("failed to register workplace handler: %w", err)
	}

	return handler, nil
}

// GetSystemPrompt implements LLMProcessor
func (h *WorkplaceHandler) GetSystemPrompt() string {
	return workplacePrompt
}

// GetResponseSchema implements LLMProcessor
func (h *WorkplaceHandler) GetResponseSchema() (*minds.ResponseSchema, error) {
	return minds.NewResponseSchema("Workplace", "Workplace details", Workplace{})
}

// GetOutputTopic implements LLMProcessor
func (h *WorkplaceHandler) GetOutputTopic() string {
	return TopicWorkplaceExtracted
}

func (h *WorkplaceHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing workplace message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	var input JobDescriptionInput
	if err := json.Unmarshal(msg.Data(), &input); err != nil {
		h.logger.Error("Failed to unmarshal input", "error", err)
		return fmt.Errorf("failed to unmarshal job description input: %w", err)
	}

	msgs := minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: h.GetSystemPrompt(),
		},
		{
			Role:    minds.RoleUser,
			Content: fmt.Sprintf("Please analyze this job description and extract all workplace-related information:\n\n%s", input.Description),
		},
	}

	schema, err := h.GetResponseSchema()
	if err != nil {
		return fmt.Errorf("failed to create workplace schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Extracting workplace details from job description")

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to extract workplace details: %w", err)
	}

	var workplace Workplace
	if err := json.Unmarshal([]byte(result.String()), &workplace); err != nil {
		h.logger.Error("Failed to unmarshal workplace details", "error", err, "response", result.String())
		return fmt.Errorf("failed to unmarshal workplace details: %w", err)
	}

	h.logger.Debug("Extracted workplace details",
		"location", workplace.Location,
		"type", workplace.Type,
		"schedule", workplace.Schedule,
		"environment", workplace.Environment,
		"travel_requirements", len(workplace.Travel))

	// Publish the extracted workplace details
	workplaceJSON, err := json.Marshal(workplace)
	if err != nil {
		return fmt.Errorf("failed to marshal workplace details: %w", err)
	}

	_, err = h.generic.js.Publish(ctx, h.GetOutputTopic(), workplaceJSON)
	if err != nil {
		return fmt.Errorf("failed to publish workplace details: %w", err)
	}

	return nil
}
