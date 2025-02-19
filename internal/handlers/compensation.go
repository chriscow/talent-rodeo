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
	compensationPrompt = `You are an expert compensation analyst. Your task is to extract and structure compensation information from job descriptions.
		Focus on identifying:
		- Salary ranges with currency and payment period
		- Benefits packages
		- Equity compensation details
		- Additional compensation elements

		Provide structured output with clear categorization of all compensation components.`
)

// Compensation defines the compensation details for a job
type Compensation struct {
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
		generic: NewGenericHandler(js, logger, nil),
	}

	// Register the message handler
	if err := handler.generic.Register(ctx, rodeoStream, TopicJobDescriptionInput, "RODEO-JOBDESC-COMPENSATION-QUEUE", handler.handleMessage); err != nil {
		return nil, fmt.Errorf("failed to register compensation handler: %w", err)
	}

	return handler, nil
}

func (h *CompensationHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing compensation message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	var input JobDescriptionInput
	if err := json.Unmarshal(msg.Data(), &input); err != nil {
		h.logger.Error("Failed to unmarshal input", "error", err)
		return nil
	}

	comp, err := h.extract(ctx, input.Description)
	if err != nil {
		return fmt.Errorf("failed to extract compensation: %w", err)
	}

	compJSON, err := json.Marshal(comp)
	if err != nil {
		return fmt.Errorf("failed to marshal compensation: %w", err)
	}

	h.logger.Debug("Publishing compensation",
		"has_salary", comp.SalaryRange != nil,
		"benefits_count", len(comp.Benefits),
		"has_equity", comp.Equity != nil)

	_, err = h.generic.js.Publish(ctx, TopicCompensationExtracted, compJSON)
	if err != nil {
		return fmt.Errorf("failed to publish compensation: %w", err)
	}

	return nil
}

func (h *CompensationHandler) extract(ctx context.Context, description string) (*Compensation, error) {
	msgs := minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: compensationPrompt,
		},
		{
			Role:    minds.RoleUser,
			Content: fmt.Sprintf("Please analyze this job description and extract all compensation-related information:\n\n%s", description),
		},
	}

	schema, err := minds.NewResponseSchema("Compensation", "Compensation details", Compensation{})
	if err != nil {
		return nil, fmt.Errorf("failed to create compensation schema: %w", err)
	}

	request := minds.NewRequest(msgs, minds.WithResponseSchema(*schema))

	h.logger.Debug("Extracting compensation from job description")

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to extract compensation: %w", err)
	}

	var comp Compensation
	if err := json.Unmarshal([]byte(result.String()), &comp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal compensation: %w", err)
	}

	h.logger.Debug("Extracted compensation",
		"has_salary", comp.SalaryRange != nil,
		"benefits", len(comp.Benefits),
		"has_equity", comp.Equity != nil)

	return &comp, nil
}
