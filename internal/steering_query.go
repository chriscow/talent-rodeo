package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"talent-rodeo/assets"

	"github.com/chriscow/minds/tools"
	"github.com/getsentry/sentry-go"
)

// SteeringQuery encapsulates all parameters needed for semantic steering of queries
type SteeringQuery struct {
	// Query is the main concept or search term (required)
	Query string

	// MoveToward is an optional concept to move results toward
	MoveToward *string

	// MoveAway is an optional concept to move results away from
	MoveAway *string

	// ForceToward determines how strongly to move toward the target concept (0.0-1.0)
	// Only used if MoveToward is set
	ForceToward float64

	// ForceAway determines how strongly to move away from the avoid concept (0.0-1.0)
	// Only used if MoveAway is set
	ForceAway float64

	RerankQuery string
}

// Validate checks if the steering query parameters are valid
func (sq SteeringQuery) Validate() error {
	if sq.Query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	// Validate ForceToward only if MoveToward is set
	if sq.MoveToward != nil && (sq.ForceToward == 0 || sq.ForceToward > 1) {
		return fmt.Errorf("ForceToward must be between 0.0 and 1.0, got %f", sq.ForceToward)
	}

	// Validate ForceAway only if MoveAway is set
	if sq.MoveAway != nil && (sq.ForceAway == 0 || sq.ForceAway > 1) {
		return fmt.Errorf("ForceAway must be between 0.0 and 1.0, got %f", sq.ForceAway)
	}

	return nil
}

// SteeringOption is a function that modifies a SteeringQuery
type SteeringOption func(*SteeringQuery)

// NewSteeringQuery creates a new steering query with the given query and options
func NewSteeringQuery(query string, model PineconeEmbedModel, opts ...SteeringOption) (*SteeringQuery, error) {
	if model == "" {
		return nil, fmt.Errorf("model cannot be empty")
	}

	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	sq := &SteeringQuery{
		Query: query,
	}

	for _, opt := range opts {
		opt(sq)
	}

	return sq, nil
}

// WithMoveToward sets the concept to move toward
func WithMoveToward(concept string, force float64) SteeringOption {
	return func(sq *SteeringQuery) {
		sq.MoveToward = &concept
		sq.ForceToward = force
	}
}

// WithMoveAway sets the concept to move away from
func WithMoveAway(concept string, force float64) SteeringOption {
	return func(sq *SteeringQuery) {
		sq.MoveAway = &concept
		sq.ForceAway = force
	}
}

// Updated GetSteeringVector method to use SteeringQuery with optional fields
func (q *SteeringQuery) GetDenseVector(ctx context.Context, p *Pinecone, model PineconeEmbedModel) ([]float32, error) {
	// Validate the query
	if err := q.Validate(); err != nil {
		return nil, fmt.Errorf("invalid steering query: %w", err)
	}

	// Build the list of texts to embed
	texts := []string{q.Query}
	if q.MoveToward != nil {
		texts = append(texts, *q.MoveToward)
	}
	if q.MoveAway != nil {
		texts = append(texts, *q.MoveAway)
	}

	// Create embeddings for all concepts in one call
	embedResp, err := p.CreateEmbeddings(ctx, texts, model, PineconeInputTypeQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to create embeddings: %w", err)
	}

	if len(embedResp.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	// Check if the number of embeddings matches the number of texts
	if len(embedResp.Data) != len(texts) {
		return nil, fmt.Errorf("number of embeddings (%d) does not match number of texts (%d)", len(embedResp.Data), len(texts))
	}

	embeddings := embedResp.Data

	if embeddings[0].Values == nil {
		return nil, fmt.Errorf("embedding for query is nil")
	}

	// Extract the query vector
	queryVector := *embeddings[0].Values
	modifiedVector := make([]float32, len(queryVector))
	copy(modifiedVector, queryVector)

	// Apply steering toward concept if specified
	if q.MoveToward != nil {
		idx := 1
		if embeddings[idx].Values == nil {
			return nil, fmt.Errorf("embedding for MoveToward is nil")
		}

		towardVector := *embeddings[idx].Values
		for i := range modifiedVector {
			// Add a portion of the difference between toward vector and query vector
			modifiedVector[i] += float32(q.ForceToward) * (towardVector[i] - queryVector[i])
		}
	}

	// Apply steering away from concept if specified
	if q.MoveAway != nil {
		idx := 1
		if q.MoveToward != nil {
			idx = 2
		}
		if embeddings[idx].Values == nil {
			return nil, fmt.Errorf("embedding for MoveAway is nil")
		}
		awayVector := *embeddings[idx].Values
		for i := range modifiedVector {
			// Subtract a portion of the difference between away vector and query vector
			modifiedVector[i] -= float32(q.ForceAway) * (awayVector[i] - queryVector[i])
		}
	}

	modifiedVector = normalizeVector(modifiedVector)

	return modifiedVector, nil
}

// NOTE: I guess you cannot currently generate sparse vectors with Pinecone. You can only query them.
//
// GetSparseVector generates a sparse vector from a steering query
// This works specifically with sparse embedding models that use dot product similarity
func (q *SteeringQuery) GetSparseVector(ctx context.Context, p *Pinecone, model PineconeEmbedModel) ([]float32, error) {
	// Validate the query
	if err := q.Validate(); err != nil {
		return nil, fmt.Errorf("invalid steering query: %w", err)
	}

	return nil, fmt.Errorf("sparse vector generation is not supported")
}

func GenerateSteeringQuery(ctx context.Context, transcript *Transcript, opts ...tools.Option) (*SteeringQuery, error) {
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-010-steering-query.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to read steering query prompt: %w", err)
	}

	prompt := string(tmpl) + `\n\n<transcript>` + transcript.String() + `</transcript>`

	options := []tools.Option{
		tools.WithModel(tools.DeepSeekChat),
		tools.WithMaxTokens(8192),
	}

	if len(opts) > 0 {
		options = append(options, opts...)
	}

	data, err := tools.Ask(ctx, prompt, options...)
	if err != nil {
		SentryReportError(ctx, err, sentry.LevelError, nil)
		return nil, fmt.Errorf("failed to ask LLM: %w", err)
	}

	return extractSteeringQuery(ctx, data)
}

func extractSteeringQuery(ctx context.Context, data string) (*SteeringQuery, error) {
	if strings.Contains(data, "<answer>") && strings.Contains(data, "</answer>") {
		// Extract the string between the <answer> tags
		// and remove the tags
		start := strings.Index(data, "<answer>")
		if start == -1 {
			return nil, fmt.Errorf("failed to generate steering query: <answer> tag not found")
		}
		end := strings.Index(data, "</answer>")
		if end == -1 {
			return nil, fmt.Errorf("failed to generate steering query: </answer> tag not found")
		}
		if end < start {
			return nil, fmt.Errorf("failed to generate steering query: </answer> tag is before <answer> tag")
		}
		// Extract the JSON data between the tags
		data = data[start+len("<answer>") : end]
	}

	var query SteeringQuery
	err := json.Unmarshal([]byte(data), &query)
	if err != nil {
		query, err = tools.StructuredAsk[SteeringQuery](ctx, "steering-query", "extract the data into this JSON structure\r\n"+string(data), tools.WithModel(tools.GPT41Nano))
		if err != nil {
			SentryReportError(ctx, err, sentry.LevelError, nil)
			return nil, fmt.Errorf("failed to parse steering query: %w", err)
		}
	}

	query.ForceToward = 0.8
	query.ForceAway = 0.5

	return &query, nil
}
