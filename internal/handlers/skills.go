package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chriscow/minds"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	skillsPrompt = `
		You analyze interview transcripts and extract all technical and professional skills.
		For each skill, determine the level of expertise and years of experience based on the context.

		Provide your response in JSON.`

	TopicSkillsExtracted = "rodeo.transcript.extracted.skills"
)

type Skills struct {
	Skills []Skill `json:"skills"`
}

type Skill struct {
	Summary    string  `json:"summary" description:"A summary of the skill as given in the transcript or details not covered in the other fields"`
	Name       string  `json:"name" description:"The name of the skill"`
	Level      string  `json:"level" description:"The level of expertise of the skill" enum:"expert,intermediate,beginner"`
	YearsUsed  float64 `json:"yearsUsed" description:"The number of years of experience with the skill"`
	Confidence string  `json:"confidence" description:"confidence in the extraction" enum:"low,medium,high"`
}

type SkillsHandler struct {
	llm     minds.ContentGenerator
	logger  *slog.Logger
	generic *GenericHandler
}

func NewSkillsHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*SkillsHandler, error) {
	handler := &SkillsHandler{
		llm:     llm,
		logger:  logger.With(slog.String("component", "skills_handler")),
		generic: NewGenericHandler(js, logger, llm),
	}

	if err := handler.generic.RegisterLLMHandler(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-TRANSCRIPT-SKILLS-QUEUE", handler); err != nil {
		return nil, fmt.Errorf("failed to register skills handler: %w", err)
	}

	return handler, nil
}

// GetSystemPrompt implements LLMProcessor
func (h *SkillsHandler) GetSystemPrompt() string {
	return skillsPrompt
}

// GetResponseSchema implements LLMProcessor
func (h *SkillsHandler) GetResponseSchema() (*minds.ResponseSchema, error) {
	return minds.NewResponseSchema("Skills", "Array of extracted skills", Skills{})
}

// GetOutputTopic implements LLMProcessor
func (h *SkillsHandler) GetOutputTopic() string {
	return TopicSkillsExtracted
}
