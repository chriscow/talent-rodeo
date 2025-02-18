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
	requirementsPrompt = `You are an expert job requirements analyzer. Your task is to extract and categorize job requirements from job descriptions.
		Focus on identifying:
		- Must-have skills with years needed and proficiency level
		- Nice-to-have skills with years and level
		- Overall experience requirements
		- Education requirements

		Provide structured output with clear categorization and detailed context for each requirement.`
)

// Requirements defines the requirements for a job
type Requirements struct {
	MustHave                 []Requirement `json:"mustHave" description:"Must have skills"`
	NiceToHave               []Requirement `json:"niceToHave" description:"Nice to have skills"`
	MinYearsExperience       float64       `json:"minYearsExperience" description:"Minimum years of experience required"`
	PreferredYearsExperience float64       `json:"preferredYearsExperience" description:"Preferred years of experience"`
	Education                []string      `json:"education" description:"Education requirements"`
}

// Requirement defines a single skill requirement
type Requirement struct {
	Skill       string  `json:"skill" description:"Skill name"`
	YearsNeeded float64 `json:"yearsNeeded" description:"Years needed to have the skill"`
	Level       string  `json:"level" description:"Level of the skill"`                               // e.g., "Expert", "Intermediate", "Beginner"
	Context     string  `json:"context" description:"Additional context about how the skill is used"` // Additional context about how the skill is used
}

type RequirementsHandler struct {
	llm     minds.ContentGenerator
	logger  *slog.Logger
	generic *GenericHandler
}

func NewRequirementsHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*RequirementsHandler, error) {
	handler := &RequirementsHandler{
		llm:     llm,
		logger:  logger.With(slog.String("component", "requirements_handler")),
		generic: NewGenericHandler(js, logger),
	}

	if err := handler.generic.Register(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-JOBDESC-REQUIREMENTS-QUEUE", handler.handleMessage); err != nil {
		return nil, fmt.Errorf("failed to register requirements handler: %w", err)
	}

	return handler, nil
}

func (h *RequirementsHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing requirements message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	var input JobDescriptionInput
	if err := json.Unmarshal(msg.Data(), &input); err != nil {
		h.logger.Error("Failed to unmarshal input", "error", err)
		return nil
	}

	reqs, err := h.extract(ctx, input.Description)
	if err != nil {
		return fmt.Errorf("failed to extract requirements: %w", err)
	}

	reqJSON, err := json.Marshal(reqs)
	if err != nil {
		return fmt.Errorf("failed to marshal requirements: %w", err)
	}

	h.logger.Debug("Publishing requirements",
		"must_have_count", len(reqs.MustHave),
		"nice_to_have_count", len(reqs.NiceToHave),
		"min_years", reqs.MinYearsExperience)

	_, err = h.generic.js.Publish(ctx, TopicRequirementsExtracted, reqJSON)
	if err != nil {
		return fmt.Errorf("failed to publish requirements: %w", err)
	}

	return nil
}

func (h *RequirementsHandler) extract(ctx context.Context, description string) (*Requirements, error) {
	msgs := minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: requirementsPrompt,
		},
		{
			Role:    minds.RoleUser,
			Content: fmt.Sprintf("Please analyze this job description and extract all requirements:\n\n%s", description),
		},
	}

	schema, err := minds.NewResponseSchema("Requirements", "Job requirements", Requirements{})
	if err != nil {
		return nil, fmt.Errorf("failed to create requirements schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Extracting requirements from job description")

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to extract requirements: %w", err)
	}

	var reqs Requirements
	if err := json.Unmarshal([]byte(result.String()), &reqs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal requirements: %w", err)
	}

	h.logger.Debug("Extracted requirements",
		"mustHave", len(reqs.MustHave),
		"niceToHave", len(reqs.NiceToHave),
		"minYears", reqs.MinYearsExperience)

	return &reqs, nil
}
