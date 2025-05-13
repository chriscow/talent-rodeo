package internal

import (
	"context"
	"fmt"
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

	// Model is the embedding model to use for vectorization
	Model PineconeEmbedModel
}

// Validate checks if the steering query parameters are valid
func (sq SteeringQuery) Validate() error {
	if sq.Model == "" {
		return fmt.Errorf("model cannot be empty")
	}
	if sq.Query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	// Validate ForceToward only if MoveToward is set
	if sq.MoveToward != nil && (sq.ForceToward < 0 || sq.ForceToward > 1) {
		return fmt.Errorf("ForceToward must be between 0.0 and 1.0, got %f", sq.ForceToward)
	}

	// Validate ForceAway only if MoveAway is set
	if sq.MoveAway != nil && (sq.ForceAway < 0 || sq.ForceAway > 1) {
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
		Model: model,
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

// WithModel sets the embedding model
func WithModel(model PineconeEmbedModel) SteeringOption {
	return func(sq *SteeringQuery) {
		sq.Model = model
	}
}

// Updated GetSteeringVector method to use SteeringQuery with optional fields
func (q *SteeringQuery) GetVector(ctx context.Context, p *Pinecone) ([]float32, error) {
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
	embedResp, err := p.CreateEmbeddings(ctx, texts, q.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to create embeddings: %w", err)
	}

	embeddings := embedResp.Data

	// Extract the query vector
	queryVector := *embeddings[0].Values
	modifiedVector := make([]float32, len(queryVector))
	copy(modifiedVector, queryVector)

	// Apply steering toward concept if specified
	if q.MoveToward != nil {
		idx := 1
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
		awayVector := *embeddings[idx].Values
		for i := range modifiedVector {
			// Subtract a portion of the difference between away vector and query vector
			modifiedVector[i] -= float32(q.ForceAway) * (awayVector[i] - queryVector[i])
		}
	}

	// Normalize the modified vector to unit length (important for cosine similarity)
	modifiedVector = normalizeVector(modifiedVector)

	return modifiedVector, nil
}

// Example usage:
/*
func example(p *Pinecone, ctx context.Context) {
	// Basic query without any steering
	query0 := NewSteeringQuery(
		"machine learning",
		PineconeEmbedModel3Small,
	)

	vector0, err := p.GetSteeringVector(ctx, query0)
	if err != nil {
		log.Fatal(err)
	}

	// Only moving toward
	query1 := NewSteeringQuery(
		"python programming",
		PineconeEmbedModel3Small,
		WithMoveToward("web development", 0.5),
	)

	vector1, err := p.GetSteeringVector(ctx, query1)
	if err != nil {
		log.Fatal(err)
	}

	// Only moving away
	query2 := NewSteeringQuery(
		"restaurants",
		PineconeEmbedModel3Small,
		WithMoveAway("fast food", 0.7),
	)

	vector2, err := p.GetSteeringVector(ctx, query2)
	if err != nil {
		log.Fatal(err)
	}

	// Both directions
	query3 := NewSteeringQuery(
		"programming tutorials",
		PineconeEmbedModel3Small,
		WithMoveToward("beginner friendly", 0.6),
		WithMoveAway("advanced topics", 0.4),
	)

	vector3, err := p.GetSteeringVector(ctx, query3)
	if err != nil {
		log.Fatal(err)
	}

	// Manual creation with only one direction
	query4 := &SteeringQuery{
		Query:       "cooking recipes",
		MoveToward:  StringPtr("healthy eating"),
		ForceToward: 0.8,
		Model:       PineconeEmbedModel3Small,
	}

	vector4, err := p.GetSteeringVector(ctx, query4)
	if err != nil {
		log.Fatal(err)
	}
}

// Helper function if you don't have one
func StringPtr(s string) *string {
	return &s
}
*/
