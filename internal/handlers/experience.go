package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	ctx      context.Context
	js       jetstream.JetStream
	llm      minds.ContentGenerator
	logger   *slog.Logger
	consumer jetstream.Consumer
}

func NewExperienceHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*ExperienceHandler, error) {
	const experienceConsumerName = "RODEO-TRANSCRIPT-EXPERIENCE-QUEUE"

	cons, err := getConsumer(ctx, js, rodeoStream, experienceConsumerName, TopicTranscriptInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer: %w", err)
	}

	return &ExperienceHandler{
		ctx:      ctx,
		js:       js,
		llm:      llm,
		logger:   logger,
		consumer: cons,
	}, nil
}

func (h *ExperienceHandler) Start(ctx context.Context) error {
	h.logger.Info("Starting experience handler")
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Stopping experience handler", "reason", ctx.Err())
			return ctx.Err()
		default:
			if err := h.processNextMessage(ctx); err != nil {
				h.logger.Error("Error processing message", "error", err)
				continue
			}
		}
	}
}

func (h *ExperienceHandler) Close() error {
	return nil
}

func (h *ExperienceHandler) processNextMessage(ctx context.Context) error {
	msgs, err := h.consumer.Fetch(1, jetstream.FetchMaxWait(250*time.Millisecond))
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			return nil
		}
		return fmt.Errorf("failed to fetch message: %w", err)
	}

	for msg := range msgs.Messages() {
		if err := h.handleMessage(ctx, msg); err != nil {
			h.logger.Error("Failed to handle message", "error", err)
			msg.Term()
			return err
		}
	}
	return nil
}

func (h *ExperienceHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	transcript := string(msg.Data())

	schema, err := minds.NewResponseSchema("Experience", "Experience details", Experience{})
	if err != nil {
		return fmt.Errorf("failed to create experience schema: %w", err)
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

	result, err := h.llm.GenerateContent(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to generate content: %w", err)
	}

	var output Experience
	if err := json.Unmarshal([]byte(result.String()), &output); err != nil {
		return fmt.Errorf("failed to unmarshal experience: %w", err)
	}

	h.logger.Info("Experience", "experience", output)

	experienceJSON, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("failed to marshal experience: %w", err)
	}

	_, err = h.js.Publish(ctx, TopicExperienceExtracted, experienceJSON)
	if err != nil {
		return fmt.Errorf("failed to publish experience: %w", err)
	}

	msg.Ack()
	return nil
}
