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

func (h *SkillsHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing skills message",
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
			Content: input.Description,
		},
	}

	schema, err := h.GetResponseSchema()
	if err != nil {
		return fmt.Errorf("failed to create skills schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Extracting skills from job description")

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to extract skills: %w", err)
	}

	var skills Skills
	if err := json.Unmarshal([]byte(result.String()), &skills); err != nil {
		h.logger.Error("Failed to unmarshal skills", "error", err, "response", result.String())
		return fmt.Errorf("failed to unmarshal skills: %w", err)
	}

	h.logger.Debug("Extracted skills",
		"skills_count", len(skills.Skills))

	// Publish the extracted skills
	skillsJSON, err := json.Marshal(skills)
	if err != nil {
		return fmt.Errorf("failed to marshal skills: %w", err)
	}

	_, err = h.generic.js.Publish(ctx, h.GetOutputTopic(), skillsJSON)
	if err != nil {
		return fmt.Errorf("failed to publish skills: %w", err)
	}

	return nil
}
