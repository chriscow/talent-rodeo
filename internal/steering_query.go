package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"talent-rodeo/assets"

	"github.com/chriscow/minds"
	"github.com/sashabaranov/go-openai"
	tools "talent-rodeo/internal/llmtools"
)

const (
	// The ForceAway value is intentionally kept relatively low (e.g. 0.3–0.5).
	//
	// In semantic vector space, subtracting too much in the direction of an undesired concept
	// can overly distort the query embedding, removing useful shared context or introducing noise.
	//
	// This is especially true when the MoveAway concept is only tangentially related
	// — subtracting a strong vector could inadvertently suppress desirable meaning.
	//
	// Empirically (e.g. in relevance feedback literature), negative signals are best applied
	// with lower weights than positive ones. A lighter touch avoids degrading the retrieval quality.
	//
	// So: steer clearly but gently away from unwanted concepts, and let MoveToward carry most of the intent.

	DefaultForceAway = 0.3 // Default strength to steer away from the avoid concept. Should be low.
)

// SteeringQuery encapsulates all parameters needed for semantic steering of
// queries. Why not just include "not X" in the text? Because embedding models
// don't reliably capture negation. As you found, simply phrasing "other than
// cats" or "except electrical" in the query still yields an embedding that is
// close to those concepts (the model sees the word "cats" and increases
// similarity to cat-related content). Therefore, we need to explicitly
// manipulate the vector to remove that semantic component, rather than relying
// on the embedding model to understand a negation in text.
type SteeringQuery struct {
	// Queries is the main concept or search term (required)
	Queries []string `description:"The canonical form of the one or more main query term(s), job title(s) to base the searches on."`

	// This parameter will help you filter out the jobs based on the experience required to apply for the job.
	// Possible Values - internship, entry_level, associate, mid_senior_level & director.
	ExperienceLevel ExperienceLevel `description:"The experience level to filter results by" enum:"internship, entry_level, associate, mid_senior_level, director"`

	// This parameter will help you filter out the jobs based on their working model.
	// Possible Values - at_work, remote,  & hybrid.
	WorkTypes []WorkType `description:"The work types to filter results by in order of preference" enum:"at_work, remote, hybrid"`

	// MoveToward is a concept as a complete sentence to move results toward
	MoveToward string `description:"The concept to steer the search results toward, expressed as a first person complete sentence and is required."`

	// MoveAway is a concept as a complete sentence to move results away from
	MoveAway string `description:"The concept to steer the search results away from, expressed as a first person complete sentence and is required."`

	// ForceAway determines how strongly to move away from the avoid concept (0.0-1.0)
	//
	// The ForceAway value is intentionally kept relatively low (e.g. 0.3–0.5).
	//
	// In semantic vector space, subtracting too much in the direction of an undesired concept
	// can overly distort the query embedding, removing useful shared context or introducing noise.
	//
	// This is especially true when the MoveAway concept is only tangentially related
	// — subtracting a strong vector could inadvertently suppress desirable meaning.
	//
	// Empirically (e.g. in relevance feedback literature), negative signals are best applied
	// with lower weights than positive ones. A lighter touch avoids degrading the retrieval quality.
	//
	// So: steer clearly but gently away from unwanted concepts, and let MoveToward carry most of the intent.

	ForceAway float64 `description:"A value between 0.0 and 1.0 indicating how strongly to steer the search results away from the MoveAway concept."`

	// RerankQuery is an all encompassing query expressed as one or more complete sentences
	// combining the original query, MoveToward, and MoveAway concepts.
	RerankQuery string `description:"An all-encompassing query that combines the original query, MoveToward, and MoveAway concepts into a single first person sentence for reranking results."`
}

// Validate checks if the steering query parameters are valid
func (sq SteeringQuery) Validate() error {
	if len(sq.Queries) == 0 {
		return fmt.Errorf("query cannot be empty")
	}

	if sq.ExperienceLevel == "" {
		return fmt.Errorf("experience level cannot be empty")
	}

	if len(sq.WorkTypes) == 0 {
		return fmt.Errorf("work types cannot be empty")
	}

	if sq.MoveToward == "" {
		return fmt.Errorf("MoveToward cannot be empty")
	}

	if sq.RerankQuery == "" {
		return fmt.Errorf("rerank query cannot be empty")
	}

	if sq.MoveAway == "" && sq.ForceAway != 0 {
		return fmt.Errorf("ForceAway must be zero when MoveAway is empty")
	}
	if sq.MoveAway != "" && (sq.ForceAway <= 0 || sq.ForceAway > 1) {
		return fmt.Errorf("ForceAway must be between 0.0 and 1.0, got %f", sq.ForceAway)
	}

	return nil
}

// SteeringOption is a function that modifies a SteeringQuery
type SteeringOption func(*SteeringQuery)

// NewSteeringQuery creates a new steering query with the given query and options
func NewSteeringQuery(queries []string, model PineconeEmbedModel, opts ...SteeringOption) (*SteeringQuery, error) {
	if model == "" {
		return nil, fmt.Errorf("model cannot be empty")
	}

	if len(queries) == 0 {
		return nil, fmt.Errorf("query cannot be empty")
	}

	// Ensure all queries are non-empty
	for _, query := range queries {
		if strings.TrimSpace(query) == "" {
			return nil, fmt.Errorf("all queries must be non-empty")
		}
	}

	sq := &SteeringQuery{
		Queries: queries,
	}

	for _, opt := range opts {
		opt(sq)
	}

	if err := sq.Validate(); err != nil {
		return nil, fmt.Errorf("invalid steering query: %w", err)
	}

	return sq, nil
}

// GetDenseVector generates a semantically steered dense vector from the SteeringQuery.
// It starts from the MoveToward embedding and adjusts it to steer away from MoveAway
// using vector projection, suitable for cosine similarity search.
//
// Subtract the projection of MoveToward onto MoveAway:
//
// This removes any semantic alignment between the MoveToward vector and the MoveAway concept.
// Mathematically, this implements the projection formula:
//
//	v' = v - (v · û) * û
//
// where:
//   - v  is the MoveToward vector,
//   - û  is the unit vector in the direction of MoveAway,
//   - v' is the adjusted vector with the unwanted dimension removed.
//
// This ensures we only steer away from the component of MoveToward that aligns with MoveAway,
// rather than blindly subtracting the entire MoveAway vector, which could introduce unrelated noise.
func (q *SteeringQuery) GetDenseVector(ctx context.Context, p *Pinecone, model PineconeEmbedModel) ([]float32, error) {
	// Ensure query parameters are valid
	if err := q.Validate(); err != nil {
		return nil, fmt.Errorf("invalid steering query: %w", err)
	}

	// Prepare input texts for embedding
	texts := []string{q.MoveToward}
	if q.MoveAway != "" {
		texts = append(texts, q.MoveAway)
	}

	// Request embeddings for MoveToward and MoveAway
	embedResp, err := p.CreateEmbeddings(ctx, texts, model, PineconeInputTypeQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to create embeddings: %w", err)
	}
	if len(embedResp.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(embedResp.Data))
	}

	// Extract vectors from response
	towardVec := *embedResp.Data[0].Values

	// Initialize the steering vector from MoveToward
	steeredVec := make([]float32, len(towardVec))
	copy(steeredVec, towardVec)
	if q.MoveAway == "" {
		return normalizeVector(steeredVec), nil
	}
	awayVec := *embedResp.Data[1].Values

	// Normalize the away vector to get a unit vector
	awayUnit := normalizeVector(awayVec)

	// Compute dot product between toward vector and away unit vector
	var dot float64
	for i := range towardVec {
		dot += float64(towardVec[i]) * float64(awayUnit[i])
	}

	// Subtract the component in the direction of MoveAway
	for i := range steeredVec {
		steeredVec[i] -= float32(q.ForceAway) * float32(dot) * awayUnit[i]
	}

	// Normalize the final vector to unit length for cosine similarity
	return normalizeVector(steeredVec), nil
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

func GenerateSteeringQuery(ctx context.Context, resume Resume, transcript Transcript, opts ...tools.Option) (*SteeringQuery, error) {
	const promptFile = "prompts/transcript.steering-query.v002.txt"
	const exampleFile = "prompts/transcript.steering-query.examples.v002.txt"
	sysPrompt, err := assets.EmbeddedFiles.ReadFile(promptFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read steering query prompt: %w", err)
	}
	exampleData, err := assets.EmbeddedFiles.ReadFile(exampleFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read steering query examples: %w", err)
	}

	examples := minds.Messages{}
	if err := json.Unmarshal([]byte(exampleData), &examples); err != nil {
		return nil, fmt.Errorf("failed to unmarshal examples into messages: %w", err)
	}

	messages := minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: string(sysPrompt),
		},
	}

	messages = append(messages, examples...)

	resp := openai.ChatCompletionResponse{}
	options := []tools.Option{
		tools.WithModel(tools.DeepSeekFlash),
		tools.WithMaxTokens(8192),
		tools.WithMessages(messages),
		tools.WantCompletionResponse(&resp),
	}

	if len(opts) > 0 {
		options = append(options, opts...)
	}

	prompt := fmt.Sprintf(`<transcript>%s</transcript><resume>%s</resume>`, transcript.String(), resume.RawText)

	origResp, err := tools.Ask(ctx, prompt, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to ask LLM for steering query: %w", err)
	}

	// TODO: Record LLM cost for steering query generation - we don't have userID or fs here
	// This function is called from various contexts, would need to thread these through
	slog.Default().Debug("steering query generation LLM call", "promptTokens", resp.Usage.PromptTokens, "completionTokens", resp.Usage.CompletionTokens, "model", tools.DeepSeekFlash)

	sq, err := extractSteeringQuery(ctx, origResp)
	if err != nil {
		return nil, fmt.Errorf("failed to extract steering query after fix: %w", err)
	}

	if err := sq.Validate(); err != nil {
		return nil, fmt.Errorf("steering query validation failed: %w", err)
	}

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

		resp1 := openai.ChatCompletionResponse{}
		query, err = tools.StructuredAsk[SteeringQuery](ctx, fmt.Sprintf(prompt, err.Error(), string(jsonB), data), tools.WithModel(tools.GPT6Luna), tools.WantCompletionResponse(&resp1))
		if err != nil {
			return nil, fmt.Errorf("LLM failed to parse steering query: %w", err)
		}

		// TODO: Record LLM cost for steering query parsing - we don't have userID or fs here
		slog.Default().Debug("steering query parsing LLM call (first attempt)", "promptTokens", resp1.Usage.PromptTokens, "completionTokens", resp1.Usage.CompletionTokens, "model", tools.GPT6Luna)

		if err := query.Validate(); err != nil {
			resp2 := openai.ChatCompletionResponse{}
			query, err = tools.StructuredAsk[SteeringQuery](ctx, fmt.Sprintf(prompt, err.Error(), string(jsonB), data), tools.WithModel(tools.GPT6Luna), tools.WantCompletionResponse(&resp2))
			if err != nil {
				return nil, fmt.Errorf("LLM failed to parse steering query: %w", err)
			}

			// TODO: Record LLM cost for steering query parsing - we don't have userID or fs here
			slog.Default().Debug("steering query parsing LLM call (second attempt)", "promptTokens", resp2.Usage.PromptTokens, "completionTokens", resp2.Usage.CompletionTokens, "model", tools.GPT6Luna)
		}
	}

	// set this to the default value so Validate does not complain
	query.ForceAway = DefaultForceAway

	if err := query.Validate(); err != nil {
		return nil, err
	}

	return &query, nil
}
