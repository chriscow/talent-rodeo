package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"talent-rodeo/internal/handlers/search"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Constants for message routing
const (
	TopicSearchInput  = "rodeo.search.input"
	TopicSearchResult = "rodeo.search.result"
	StreamNameSearch  = "rodeo.search"
)

// SearchHandler handles job search requests
type SearchHandler struct {
	ctx      context.Context
	js       jetstream.JetStream
	logger   *slog.Logger
	provider search.JobProvider
	consumer jetstream.Consumer
}

// NewSearchHandler creates a new SearchHandler instance
func NewSearchHandler(ctx context.Context, js jetstream.JetStream, provider search.JobProvider, logger *slog.Logger) (*SearchHandler, error) {
	const searchConsumerName = "RODEO-SEARCH-QUEUE"

	cons, err := getConsumer(ctx, js, rodeoStream, searchConsumerName, TopicSearchInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer: %w", err)
	}

	return &SearchHandler{
		ctx:      ctx,
		js:       js,
		logger:   logger.With(slog.String("component", "search_handler")),
		provider: provider,
		consumer: cons,
	}, nil
}

// Start begins processing messages for job search requests
func (h *SearchHandler) Start(ctx context.Context) error {
	h.logger.Info("Starting search handler")
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Stopping search handler", "reason", ctx.Err())
			return ctx.Err()
		default:
			if err := h.processNextMessage(ctx); err != nil {
				h.logger.Error("Error processing message", "error", err)
				continue
			}
		}
	}
}

// processNextMessage fetches and processes the next message
func (h *SearchHandler) processNextMessage(ctx context.Context) error {
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

// handleMessage processes a single job search request message
func (h *SearchHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing search message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	var criteria search.Criteria
	if err := json.Unmarshal(msg.Data(), &criteria); err != nil {
		h.logger.Error("Failed to unmarshal criteria", "error", err)
		msg.Ack()
		return nil
	}

	jobs, err := h.provider.SearchJobs(ctx, criteria)
	if err != nil {
		return fmt.Errorf("failed to perform job search: %w", err)
	}

	jobsJSON, err := json.Marshal(jobs)
	if err != nil {
		return fmt.Errorf("failed to marshal jobs: %w", err)
	}

	h.logger.Debug("Publishing search results",
		"criteria", criteria)

	_, err = h.js.Publish(ctx, TopicSearchResult, jobsJSON)
	if err != nil {
		return fmt.Errorf("failed to publish search result: %w", err)
	}

	msg.Ack()
	return nil
}

// Close closes the handler and releases any resources
func (h *SearchHandler) Close() error {
	h.logger.Info("Closing search handler")
	return nil
}
