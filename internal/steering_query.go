package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"talent-rodeo/assets"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/tools"
	"github.com/sashabaranov/go-openai"
)

const (
	DefaultForceToward = 0.9 // Default strength to steer toward the target concept
	DefaultForceAway   = 0.8 // Default strength to steer away from the avoid concept
)

// SteeringQuery encapsulates all parameters needed for semantic steering of queries
type SteeringQuery struct {
	// Query is the main concept or search term (required)
	Query string `description:"The canonical form of the one main query term, job title or concept to base the search on."`

	// This parameter will help you filter out the jobs based on the experience required to apply for the job.
	// Possible Values - internship, entry_level, associate, mid_senior_level & director.
	ExperienceLevel ExperienceLevel `description:"The experience level to filter results by" enum:"internship, entry_level, associate, mid_senior_level, director"`

	// This parameter will help you filter out the jobs based on their working model.
	// Possible Values - at_work, remote,  & hybrid.
	WorkType WorkType `description:"The work type to filter results by" enum:"at_work, remote, hybrid"`

	// MoveToward is a concept as a complete sentence to move results toward
	MoveToward string `description:"The concept to steer the search results toward, expressed as a first person complete sentence and is required."`

	// MoveAway is a concept as a complete sentence to move results away from
	MoveAway string `description:"The concept to steer the search results away from, expressed as a first person complete sentence and is required."`

	// ForceToward determines how strongly to move toward the target concept (0.0-1.0)
	// Only used if MoveToward is set
	ForceToward float64 `description:"A value between 0.0 and 1.0 indicating how strongly to steer the search results toward the MoveToward concept."`

	// ForceAway determines how strongly to move away from the avoid concept (0.0-1.0)
	// Only used if MoveAway is set
	ForceAway float64 `description:"A value between 0.0 and 1.0 indicating how strongly to steer the search results away from the MoveAway concept."`

	// RerankQuery is an all encompassing query expressed as one or more complete sentences
	// combining the original query, MoveToward, and MoveAway concepts.
	RerankQuery string `description:"An all-encompassing query that combines the original query, MoveToward, and MoveAway concepts into a single first person sentence for reranking results."`
}

// Validate checks if the steering query parameters are valid
func (sq SteeringQuery) Validate() error {
	if sq.Query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	if sq.ExperienceLevel == "" {
		return fmt.Errorf("experience level cannot be empty")
	}

	if sq.WorkType == "" {
		return fmt.Errorf("work type cannot be empty")
	}

	if sq.MoveToward == "" {
		return fmt.Errorf("MoveToward cannot be empty")
	}

	if sq.MoveAway == "" {
		return fmt.Errorf("MoveAway cannot be empty")
	}

	if sq.RerankQuery == "" {
		return fmt.Errorf("rerank query cannot be empty")
	}

	// Validate ForceToward only if MoveToward is set
	if sq.ForceToward == 0 || sq.ForceToward > 1 {
		return fmt.Errorf("ForceToward must be between 0.0 and 1.0, got %f", sq.ForceToward)
	}

	// Validate ForceAway only if MoveAway is set
	if sq.ForceAway == 0 || sq.ForceAway > 1 {
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
		sq.MoveToward = concept
		sq.ForceToward = force
	}
}

// WithMoveAway sets the concept to move away from
func WithMoveAway(concept string, force float64) SteeringOption {
	return func(sq *SteeringQuery) {
		sq.MoveAway = concept
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
	texts := []string{q.Query, q.MoveToward, q.MoveAway}

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
	if embeddings[1].Values == nil {
		return nil, fmt.Errorf("embedding for MoveToward is nil")
	}

	towardVector := *embeddings[1].Values
	for i := range modifiedVector {
		// Add a portion of the difference between toward vector and query vector
		modifiedVector[i] += float32(q.ForceToward) * (towardVector[i] - queryVector[i])
	}

	// Apply steering away from concept if specified
	if embeddings[2].Values == nil {
		return nil, fmt.Errorf("embedding for MoveAway is nil")
	}
	awayVector := *embeddings[2].Values
	for i := range modifiedVector {
		// Subtract a portion of the difference between away vector and query vector
		modifiedVector[i] -= float32(q.ForceAway) * (awayVector[i] - queryVector[i])
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
	const promptFile = "prompts/transcript-010-steering-query.txt"
	const exampleFile = "prompts/transcript-010-steering-query-examples.txt"
	prompt, err := assets.EmbeddedFiles.ReadFile(promptFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read steering query prompt: %w", err)
	}
	examples, err := assets.EmbeddedFiles.ReadFile(exampleFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read steering query examples: %w", err)
	}

	messages := minds.Messages{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: string(prompt) + "\n\n" + string(examples),
		},
	}
	options := []tools.Option{
		tools.WithModel(tools.DeepSeekChat),
		tools.WithMaxTokens(8192),
		tools.WithMessages(messages),
	}

	if len(opts) > 0 {
		options = append(options, opts...)
	}

	origResp, err := tools.Ask(ctx, `<transcript>\n\n`+transcript.String()+`</transcript>`, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to ask LLM for steering query: %w", err)
	}

	sq, err := extractSteeringQuery(ctx, origResp)
	if err != nil {
		return nil, fmt.Errorf("failed to extract steering query after fix: %w", err)
	}

	if err := sq.Validate(); err != nil {
		return nil, fmt.Errorf("steering query validation failed: %w", err)
	}

	sq.ForceToward = DefaultForceToward
	sq.ForceAway = DefaultForceAway

	return sq, nil
}

// cleanJSONFromMarkdown removes markdown code block markers and trims whitespace
func cleanJSONFromMarkdown(data string) string {
	data = strings.TrimSpace(data)

	// Remove opening markdown code block
	data = strings.TrimPrefix(data, "```json")
	data = strings.TrimPrefix(data, "```")

	data = strings.TrimSpace(data)

	// Remove closing markdown code block
	data = strings.TrimSuffix(data, "```")

	return strings.TrimSpace(data)
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

	data = cleanJSONFromMarkdown(data)

	var query SteeringQuery
	err := json.Unmarshal([]byte(data), &query)
	if err != nil {
		schema, _ := tools.GenerateJSONSchema(query)
		jsonB, _ := json.MarshalIndent(schema, "", "  ")
		prompt := `
The steering query produced in this data is invalid. Error: "%s"

Remember that all fields are requred. Please fix it and return a valid JSON object in this example format:

%s

Extract the data below into a JSON structure:

%s`

		query, err = tools.StructuredAsk[SteeringQuery](ctx, "steering-query", fmt.Sprintf(prompt, err.Error(), string(jsonB), data), tools.WithModel(tools.GPT41Mini))
		if err != nil {
			return nil, fmt.Errorf("LLM failed to parse steering query: %w", err)
		}

		if err := query.Validate(); err != nil {
			query, err = tools.StructuredAsk[SteeringQuery](ctx, "steering-query", fmt.Sprintf(prompt, err.Error(), string(jsonB), data), tools.WithModel(tools.GPT41Mini))
			if err != nil {
				return nil, fmt.Errorf("LLM failed to parse steering query: %w", err)
			}
		}
	}

	query.ForceToward = 0.8
	query.ForceAway = 0.5

	if err := query.Validate(); err != nil {
		return nil, err
	}

	return &query, nil
}
