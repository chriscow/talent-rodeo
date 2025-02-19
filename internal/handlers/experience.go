package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chriscow/minds"
	"github.com/nats-io/nats.go/jetstream"
)

const experiencePrompt = `Analyze interview transcripts and extract work experience details.
Include total years of experience and details about each role mentioned.

Provide a JSON object with experience details including:
- totalYears: Total years of professional experience
- roles: Array of roles with title, company, years in role, and skills used`

type Experience struct {
	TotalYears float64 `json:"totalYears" description:"The total number of years of experience"`
	Roles      []Role  `json:"roles" description:"The roles held by the candidate"`
}

type Role struct {
	Title       string   `json:"title" description:"The title of the role"`
	Company     string   `json:"company" description:"The company the candidate has worked at"`
	YearsInRole float64  `json:"yearsInRole" description:"The number of years the candidate has held the role"`
	Skills      []string `json:"skills" description:"The skills the candidate has in the role"`
}

type ExperienceHandler struct {
	llm     minds.ContentGenerator
	logger  *slog.Logger
	generic *GenericHandler
}

func NewExperienceHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*ExperienceHandler, error) {
	handler := &ExperienceHandler{
		llm:     llm,
		logger:  logger.With(slog.String("component", "experience_handler")),
		generic: NewGenericHandler(js, logger, llm),
	}

	if err := handler.generic.RegisterLLMHandler(ctx, rodeoStream, TopicTranscriptInput, "RODEO-TRANSCRIPT-EXPERIENCE-QUEUE", handler); err != nil {
		return nil, fmt.Errorf("failed to register experience handler: %w", err)
	}

	return handler, nil
}

// GetSystemPrompt implements LLMProcessor
func (h *ExperienceHandler) GetSystemPrompt() string {
	return experiencePrompt
}

// GetResponseSchema implements LLMProcessor
func (h *ExperienceHandler) GetResponseSchema() (*minds.ResponseSchema, error) {
	return minds.NewResponseSchema("Experience", "Experience details", Experience{})
}

// GetOutputTopic implements LLMProcessor
func (h *ExperienceHandler) GetOutputTopic() string {
	return TopicExperienceExtracted
}
