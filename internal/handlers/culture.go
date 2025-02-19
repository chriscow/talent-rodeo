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
	llm     minds.ContentGenerator
	logger  *slog.Logger
	generic *GenericHandler
}

func NewCultureHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*CultureHandler, error) {
	handler := &CultureHandler{
		llm:     llm,
		logger:  logger.With(slog.String("component", "culture_handler")),
		generic: NewGenericHandler(js, logger, llm),
	}

	if err := handler.generic.RegisterLLMHandler(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-JOBDESC-CULTURE-QUEUE", handler); err != nil {
		return nil, fmt.Errorf("failed to register culture handler: %w", err)
	}

	return handler, nil
}

// GetSystemPrompt implements LLMProcessor
func (h *CultureHandler) GetSystemPrompt() string {
	return culturePrompt
}

// GetResponseSchema implements LLMProcessor
func (h *CultureHandler) GetResponseSchema() (*minds.ResponseSchema, error) {
	return minds.NewResponseSchema("Culture", "Cultural aspects", Culture{})
}

// GetOutputTopic implements LLMProcessor
func (h *CultureHandler) GetOutputTopic() string {
	return TopicCultureExtracted
}

func (h *CultureHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing culture message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	var input JobDescriptionInput
	if err := json.Unmarshal(msg.Data(), &input); err != nil {
		h.logger.Error("Failed to unmarshal input", "error", err)
		return nil
	}

	msgs := minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: h.GetSystemPrompt(),
		},
		{
			Role:    minds.RoleUser,
			Content: fmt.Sprintf("Please analyze this job description and extract all culture-related information:\n\n%s", input.Description),
		},
	}

	schema, err := h.GetResponseSchema()
	if err != nil {
		return fmt.Errorf("failed to create culture schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Extracting culture details from job description")

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to extract culture details: %w", err)
	}

	var culture Culture
	if err := json.Unmarshal([]byte(result.String()), &culture); err != nil {
		h.logger.Error("Failed to unmarshal culture details", "error", err, "response", result.String())
		return fmt.Errorf("failed to unmarshal culture details: %w", err)
	}

	h.logger.Debug("Extracted culture details",
		"description_length", len(culture.Description),
		"values_count", len(culture.Values),
		"team_dynamics", culture.TeamDynamics,
		"work_style", culture.WorkStyle)

	// Publish the extracted culture
	cultureJSON, err := json.Marshal(culture)
	if err != nil {
		return fmt.Errorf("failed to marshal culture: %w", err)
	}

	_, err = h.generic.js.Publish(ctx, h.GetOutputTopic(), cultureJSON)
	if err != nil {
		return fmt.Errorf("failed to publish culture: %w", err)
	}

	return nil
}
