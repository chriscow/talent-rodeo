package handlers

import (
	"context"
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
	Summary     string   `json:"summary" description:"A summary of the workplace details or details not covered in the other fields"`
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
