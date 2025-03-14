package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"talent-rodeo/internal/services/search"
	"time"

	"talent-rodeo/internal/database"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	maxAckPending = 5 // Limit concurrent messages to 5 - Scrapingdog API limit
)

// SearchHandler handles job search requests
type SearchHandler struct {
	logger         *slog.Logger
	provider       search.JobProvider
	generic        *GenericHandler
	publishSubject string // Subject to publish results to
	db             *database.DB
}

// NewSearchHandler creates a new SearchHandler instance
func NewSearchHandler(ctx context.Context, js jetstream.JetStream, provider search.JobProvider, logger *slog.Logger, db *database.DB) (*SearchHandler, error) {
	handler := &SearchHandler{
		logger:         logger.With(slog.String("component", "search_handler")),
		provider:       provider,
		generic:        NewGenericHandler(js, logger, nil),
		publishSubject: search.TopicSearchResult,
		db:             db,
	}

	logger.Info("Registering search handler",
		"streamName", rodeoStream,
		"subscribe_subject", search.TopicSearchInput,
		"publish_subject", search.TopicSearchResult,
		"queue", "RODEO-SEARCH-QUEUE",
		"maxAckPending", maxAckPending)

	if err := handler.generic.Register(ctx, rodeoStream, search.TopicSearchInput, "RODEO-SEARCH-QUEUE", maxAckPending, handler.handleMessage); err != nil {
		return nil, fmt.Errorf("failed to register search handler: %w", err)
	}

	return handler, nil
}

// Register registers the handler with a new stream and subjects
func (h *SearchHandler) Register(ctx context.Context, streamName string, inputSubject string, queue string, maxAckPending int) error {
	h.publishSubject = fmt.Sprintf("%s.%s", streamName, strings.TrimPrefix(search.TopicSearchResult, rodeoStream+"."))
	h.logger.Info("Re-registering search handler",
		"streamName", streamName,
		"subscribe_subject", inputSubject,
		"publish_subject", h.publishSubject,
		"queue", queue,
		"maxAckPending", maxAckPending)
	return h.generic.Register(ctx, streamName, inputSubject, queue, maxAckPending, h.handleMessage)
}

// constructSearchCriteria converts a SearchInput into provider-agnostic search criteria
func (h *SearchHandler) constructSearchCriteria(input search.SearchInput) search.Criteria {
	// If we have must-have skills, use those as keywords
	keywords := input.MustHaveSkills

	// If no must-have skills, use preferred skills as keywords
	if len(keywords) == 0 {
		keywords = input.PreferredSkills
	}

	// Combine all skills for filtering
	skills := make([]string, 0, len(input.MustHaveSkills)+len(input.PreferredSkills))
	skills = append(skills, input.MustHaveSkills...)
	skills = append(skills, input.PreferredSkills...)

	return search.Criteria{
		Keywords:   keywords,
		Skills:     skills,
		PageNumber: input.Page,
	}
}

// scoreJobs scores jobs based on skill matches
func (h *SearchHandler) scoreJobs(jobs []search.JobListing, input search.SearchInput) []search.Result {
	var results []search.Result

	for _, job := range jobs {
		score := 0.0
		var matchedKeywords []string
		desc := strings.ToLower(job.Description)

		// +10 points for each Must Have skill match
		for _, skill := range input.MustHaveSkills {
			skillLower := strings.ToLower(skill)
			// Check for word boundaries using regex-like pattern
			if strings.Contains(desc, " "+skillLower+" ") ||
				strings.HasPrefix(desc, skillLower+" ") ||
				strings.HasSuffix(desc, " "+skillLower) ||
				desc == skillLower {
				score += 10
				matchedKeywords = append(matchedKeywords, skill)
			}
		}

		// +5 points for each Preferred skill match
		for _, skill := range input.PreferredSkills {
			skillLower := strings.ToLower(skill)
			// Check for word boundaries using regex-like pattern
			if strings.Contains(desc, " "+skillLower+" ") ||
				strings.HasPrefix(desc, skillLower+" ") ||
				strings.HasSuffix(desc, " "+skillLower) ||
				desc == skillLower {
				score += 5
				matchedKeywords = append(matchedKeywords, skill)
			}
		}

		results = append(results, search.Result{
			Job:       job,
			Score:     score,
			Keywords:  matchedKeywords,
			CreatedAt: time.Now().UTC(),
		})
	}

	// Sort results by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// handleMessage processes a single job search request message
func (h *SearchHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	h.logger.Debug("Processing search message",
		"subject", msg.Subject(),
		"data_size", len(msg.Data()))

	var input search.SearchInput
	if err := json.Unmarshal(msg.Data(), &input); err != nil {
		h.logger.Error("Failed to unmarshal criteria", "error", err)
		return nil
	}

	// Convert SearchInput to provider-agnostic criteria
	criteria := h.constructSearchCriteria(input)

	h.logger.Debug("Constructed search criteria",
		"must_have_skills", input.MustHaveSkills,
		"preferred_skills", input.PreferredSkills,
		"search_keywords", criteria.Keywords)

	jobs, err := h.provider.SearchJobs(ctx, criteria)
	if err != nil {
		return fmt.Errorf("failed to perform job search: %w", err)
	}

	// Score and filter results
	results := h.scoreJobs(jobs, input)

	// Publish results
	jobsJSON, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("failed to marshal jobs: %w", err)
	}

	h.logger.Debug("Publishing search results",
		"criteria", criteria,
		"num_results", len(results),
		"publish_subject", h.publishSubject)

	_, err = h.generic.js.Publish(ctx, h.publishSubject, jobsJSON)
	if err != nil {
		return fmt.Errorf("failed to publish search result: %w", err)
	}

	// Update the candidate's last_page field, only if this page is higher than the current one
	_, err = h.db.ExecContext(ctx,
		`UPDATE candidates 
		SET last_page = $1 
		WHERE id = $2 
		  AND (last_page IS NULL OR last_page < $1)`,
		input.Page,
		input.CandidateID)
	if err != nil {
		return fmt.Errorf("failed to update candidate last_page: %w", err)
	}

	return nil
}
