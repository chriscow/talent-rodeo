package handlers

import (
	"context"
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

	concurrentRequirementsProcessing = 10
)

// Requirements defines the requirements for a job
type Requirements struct {
	Summary                  string        `json:"summary" description:"A summary of the requirements for the job and details not covered in the other fields"`
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
		generic: NewGenericHandler(js, logger, llm),
	}

	if err := handler.generic.RegisterLLMHandler(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-JOBDESC-REQUIREMENTS-QUEUE", handler, concurrentRequirementsProcessing); err != nil {
		return nil, fmt.Errorf("failed to register requirements handler: %w", err)
	}

	return handler, nil
}

// GetSystemPrompt implements LLMProcessor
func (h *RequirementsHandler) GetSystemPrompt() string {
	return requirementsPrompt
}

// GetResponseSchema implements LLMProcessor
func (h *RequirementsHandler) GetResponseSchema() (*minds.ResponseSchema, error) {
	return minds.NewResponseSchema("Requirements", "Job requirements", Requirements{})
}

// GetOutputTopic implements LLMProcessor
func (h *RequirementsHandler) GetOutputTopic() string {
	return TopicRequirementsExtracted
}
