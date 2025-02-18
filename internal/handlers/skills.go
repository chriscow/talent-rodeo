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
		generic: NewGenericHandler(js, logger),
	}

	if err := handler.generic.Register(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-TRANSCRIPT-SKILLS-QUEUE", handler.handleMessage); err != nil {
		return nil, fmt.Errorf("failed to register skills handler: %w", err)
	}

	return handler, nil
}

// handleMessage processes a single transcript to extract skills
func (h *SkillsHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing skills message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	transcript := string(msg.Data())

	skills, err := h.extract(ctx, transcript)
	if err != nil {
		return fmt.Errorf("failed to extract skills: %w", err)
	}

	skillsJSON, err := json.Marshal(skills)
	if err != nil {
		return fmt.Errorf("failed to marshal skills: %w", err)
	}

	h.logger.Debug("Publishing skills",
		"skills_count", len(skills.Skills))

	_, err = h.generic.js.Publish(ctx, TopicSkillsExtracted, skillsJSON)
	if err != nil {
		return fmt.Errorf("failed to publish skills: %w", err)
	}

	return nil
}

func (h *SkillsHandler) extract(ctx context.Context, transcript string) (*Skills, error) {
	msgs := minds.Messages{{
		Role:    minds.RoleSystem,
		Content: skillsPrompt,
	},
		{
			Role:    minds.RoleUser,
			Content: transcript,
		},
	}

	schema, err := minds.NewResponseSchema("Skills", "Array of extracted skills", Skills{})
	if err != nil {
		return nil, fmt.Errorf("failed to create skills schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Extracting skills from transcript")

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to extract skills: %w", err)
	}

	var output Skills
	if err := json.Unmarshal([]byte(result.String()), &output); err != nil {
		return nil, fmt.Errorf("failed to unmarshal skills: %w", err)
	}

	h.logger.Debug("Extracted skills",
		"skills_count", len(output.Skills))

	return &output, nil
}
