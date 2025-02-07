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
	ctx      context.Context
	js       jetstream.JetStream
	llm      minds.ContentGenerator
	logger   *slog.Logger
	consumer jetstream.Consumer
}

func NewSkillsHandler(ctx context.Context, js jetstream.JetStream, llm minds.ContentGenerator, logger *slog.Logger) (*SkillsHandler, error) {
	const skillsConsumerName = "RODEO-TRANSCRIPT-SKILLS-QUEUE"

	cons, err := getConsumer(ctx, js, rodeoStream, skillsConsumerName, TopicJobDescriptionInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer: %w", err)
	}

	return &SkillsHandler{
		ctx:      ctx,
		js:       js,
		llm:      llm,
		logger:   logger,
		consumer: cons,
	}, nil
}

// Start begins processing messages from the NATS stream.
// It will continue processing until the context is cancelled.
func (h *SkillsHandler) Start(ctx context.Context) error {
	h.logger.Info("Starting compensation handler")
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Stopping compensation handler", "reason", ctx.Err())
			return ctx.Err()
		default:
			if err := h.processNextMessage(ctx); err != nil {
				h.logger.Error("Error processing message", "error", err)
				continue
			}
		}
	}
}

func (h *SkillsHandler) Close() error {
	return nil
}

func (h *SkillsHandler) processNextMessage(ctx context.Context) error {
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

// ExtractSkills analyzes the transcript to identify skills and expertise levels.
func (h *SkillsHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {

	transcript := string(msg.Data())

	skills, err := h.extract(ctx, transcript)
	if err != nil {
		return fmt.Errorf("failed to extract skills: %w", err)
	}

	skillsJSON, err := json.Marshal(skills)
	if err != nil {
		return fmt.Errorf("failed to marshal skills: %w", err)
	}

	_, err = h.js.Publish(ctx, TopicSkillsExtracted, skillsJSON)
	if err != nil {
		return fmt.Errorf("failed to publish skills: %w", err)
	}

	msg.Ack()
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

	return &output, nil
}
