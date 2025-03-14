package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chriscow/minds"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	compensationPrompt = `You are an expert compensation analyst. Your task is to extract and structure compensation information from job descriptions.
		Focus on identifying:
		- Salary ranges with currency and payment period
		- Benefits packages
		- Equity compensation details
		- Additional compensation elements

		Provide structured output with clear categorization of all compensation components.`

	concurrentCompensationProcessing = 10
)

// Compensation defines the compensation details for a job
type Compensation struct {
	Summary                string       `json:"summary" description:"Summary of compensation details or details not covered in the other fields"`
	SalaryRange            *SalaryRange `json:"salaryRange,omitempty" description:"Salary range"`
	Benefits               []string     `json:"benefits" description:"Benefits"`
	Equity                 *EquityRange `json:"equity,omitempty" description:"Equity"`
	AdditionalCompensation []string     `json:"additionalCompensation" description:"Additional compensation"`
}

// SalaryRange defines a salary range
type SalaryRange struct {
	Min      float64 `json:"min" description:"Minimum salary"`
	Max      float64 `json:"max" description:"Maximum salary"`
	Currency string  `json:"currency" description:"Currency"`
	Period   string  `json:"period" description:"Period"` // e.g., "yearly", "monthly", "hourly"
}

// EquityRange defines an equity range
type EquityRange struct {
	Min           float64 `json:"min" description:"Minimum equity"`
	Max           float64 `json:"max" description:"Maximum equity"`
	Type          string  `json:"type" description:"Type"` // e.g., "RSU", "Options"
	VestingPeriod string  `json:"vestingPeriod" description:"Vesting period"`
}

type CompensationHandler struct {
	llm     minds.ContentGenerator
	logger  *slog.Logger
	generic *GenericHandler
}

func NewCompensationHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*CompensationHandler, error) {
	handler := &CompensationHandler{
		llm:     llm,
		logger:  logger.With(slog.String("component", "compensation_handler")),
		generic: NewGenericHandler(js, logger, llm),
	}

	if err := handler.generic.RegisterLLMHandler(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-JOBDESC-COMPENSATION-QUEUE", handler, concurrentCompensationProcessing); err != nil {
		return nil, fmt.Errorf("failed to register compensation handler: %w", err)
	}

	return handler, nil
}

// GetSystemPrompt implements LLMProcessor
func (h *CompensationHandler) GetSystemPrompt() string {
	return compensationPrompt
}

// GetResponseSchema implements LLMProcessor
func (h *CompensationHandler) GetResponseSchema() (*minds.ResponseSchema, error) {
	return minds.NewResponseSchema("Compensation", "Compensation details", Compensation{})
}

// GetOutputTopic implements LLMProcessor
func (h *CompensationHandler) GetOutputTopic() string {
	return TopicCompensationExtracted
}
