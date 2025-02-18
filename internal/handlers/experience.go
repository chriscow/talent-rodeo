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
	experiencePrompt = `Analyze interview transcripts and extract work experience details.
Include total years of experience and details about each role mentioned.

Provide a JSON object with experience details including:
- totalYears: Total years of professional experience
- roles: Array of roles with title, company, years in role, and skills used`

	TopicExperienceExtracted = "rodeo.transcript.extracted.experience"
)

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
		generic: NewGenericHandler(js, logger),
	}

	if err := handler.generic.Register(ctx, rodeoStream, TopicTranscriptInput, "RODEO-TRANSCRIPT-EXPERIENCE-QUEUE", handler.handleMessage); err != nil {
		return nil, fmt.Errorf("failed to register experience handler: %w", err)
	}

	return handler, nil
}

func (h *ExperienceHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing experience message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	transcript := string(msg.Data())

	experience, err := h.extract(ctx, transcript)
	if err != nil {
		return fmt.Errorf("failed to extract experience: %w", err)
	}

	experienceJSON, err := json.Marshal(experience)
	if err != nil {
		return fmt.Errorf("failed to marshal experience: %w", err)
	}

	h.logger.Debug("Publishing experience",
		"total_years", experience.TotalYears,
		"roles_count", len(experience.Roles))

	_, err = h.generic.js.Publish(ctx, TopicExperienceExtracted, experienceJSON)
	if err != nil {
		return fmt.Errorf("failed to publish experience: %w", err)
	}

	return nil
}

func (h *ExperienceHandler) extract(ctx context.Context, transcript string) (*Experience, error) {
	schema, err := minds.NewResponseSchema("Experience", "Experience details", Experience{})
	if err != nil {
		return nil, fmt.Errorf("failed to create experience schema: %w", err)
	}

	request := minds.NewRequest(minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: experiencePrompt,
		},
		{
			Role:    minds.RoleUser,
			Content: transcript,
		},
	}, minds.WithResponseSchema(*schema))

	h.logger.Debug("Extracting experience from transcript")

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	var output Experience
	if err := json.Unmarshal([]byte(result.String()), &output); err != nil {
		return nil, fmt.Errorf("failed to unmarshal experience: %w", err)
	}

	h.logger.Debug("Extracted experience",
		"total_years", output.TotalYears,
		"roles_count", len(output.Roles))

	return &output, nil
}
