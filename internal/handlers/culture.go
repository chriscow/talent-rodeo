package handlers

import (
	"context"
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
