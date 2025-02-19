package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"talent-rodeo/internal/handlers/search"

	"github.com/nats-io/nats.go/jetstream"
)

// Constants for message routing
const (
	TopicSearchInput  = "rodeo.search.input"
	TopicSearchResult = "rodeo.search.result"
)

// SearchHandler handles job search requests
type SearchHandler struct {
	logger   *slog.Logger
	provider search.JobProvider
	generic  *GenericHandler
}

// NewSearchHandler creates a new SearchHandler instance
func NewSearchHandler(ctx context.Context, js jetstream.JetStream, provider search.JobProvider, logger *slog.Logger) (*SearchHandler, error) {
	handler := &SearchHandler{
		logger:   logger.With(slog.String("component", "search_handler")),
		provider: provider,
		generic:  NewGenericHandler(js, logger, nil),
	}

	if err := handler.generic.Register(ctx, rodeoStream, TopicSearchInput, "RODEO-SEARCH-QUEUE", handler.handleMessage); err != nil {
		return nil, fmt.Errorf("failed to register search handler: %w", err)
	}

	return handler, nil
}

// handleMessage processes a single job search request message
func (h *SearchHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing search message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	var criteria search.Criteria
	if err := json.Unmarshal(msg.Data(), &criteria); err != nil {
		h.logger.Error("Failed to unmarshal criteria", "error", err)
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

	_, err = h.generic.js.Publish(ctx, TopicSearchResult, jobsJSON)
	if err != nil {
		return fmt.Errorf("failed to publish search result: %w", err)
	}

	return nil
}
