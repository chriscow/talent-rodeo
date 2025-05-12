package internal

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/cheggaaa/pb"
	"github.com/pinecone-io/go-pinecone/v3/pinecone"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/protobuf/types/known/structpb"
)

type PineconeEmbedModel string

const (
	PineconeEmbedDense  PineconeEmbedModel = "llama-text-embed-v2"
	PineconeEmbedSparse PineconeEmbedModel = "pinecone-sparse-english-v0"
)

type Pinecone struct {
	*pinecone.Client
}

type PineconeQueryOptions struct {
	topK   int
	fields *[]string
	filter *map[string]any
	rerank *pinecone.SearchRecordsRerank
}

func WithTopK(topK int) func(*PineconeQueryOptions) {
	return func(q *PineconeQueryOptions) {
		q.topK = topK
	}
}

func WithFields(fields []string) func(*PineconeQueryOptions) {
	return func(q *PineconeQueryOptions) {
		q.fields = &fields
	}
}

/*
you can use metadata filtering to exclude specific keywords while including others. Here's an example from the documentation:

python
result = index.query(vector=xq, top_k=5, filter={'tokens': {
    '$nin': ['quora', 'quorans'],
    '$in': ['google', 'reddit', 'stackoverflow']
}})
*/
// WithFilter allows you to filter the results based on metadata.
func WithFilter(filter map[string]any) func(*PineconeQueryOptions) {
	return func(q *PineconeQueryOptions) {
		q.filter = &filter
	}
}

func WithRerank(topN int, rankFields []string) func(*PineconeQueryOptions) {
	return func(q *PineconeQueryOptions) {
		topNInt := int32(topN)

		q.rerank = &pinecone.SearchRecordsRerank{
			Model:      "bge-reranker-v2-m3",
			TopN:       &topNInt,
			RankFields: rankFields,
		}
	}
}

func NewPineconeClient() (*Pinecone, error) {
	pineconeKey := os.Getenv("PINECONE_API_KEY")
	if pineconeKey == "" {
		return nil, fmt.Errorf("PINECONE_API_KEY is not set")
	}

	pc, err := pinecone.NewClient(pinecone.NewClientParams{
		ApiKey: pineconeKey,
	})
	if err != nil {
		return nil, fmt.Errorf("create Client: %w", err)
	}

	return &Pinecone{
		Client: pc,
	}, nil
}

func (p *Pinecone) CreateEmbeddings(ctx context.Context, input []string, model PineconeEmbedModel) (*pinecone.EmbedResponse, error) {
	return p.Client.Inference.Embed(ctx, &pinecone.EmbedRequest{
		Model:      string(model),
		TextInputs: input,
		Parameters: map[string]any{
			"input_type": "query",
			"truncate":   "END",
		},
	})
}

func (p *Pinecone) CreateOpenAIEmbeddings(ctx context.Context, input []string, model openai.EmbeddingModel) (*pinecone.Vector, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is not set")
	}

	client := openai.NewClient(apiKey)

	res, err := client.CreateEmbeddings(context.Background(),
		openai.EmbeddingRequestStrings{
			Input: input,
			Model: model,
		})
	if err != nil {
		return nil, fmt.Errorf("create embeddings: %v", err)
	}

	if len(res.Data) != 1 {
		return nil, fmt.Errorf("expected 1 embedding, got %d", len(res.Data))
	}

	values := res.Data[0].Embedding
	vector := &pinecone.Vector{
		Values: &values,
	}

	return vector, nil
}

func (p *Pinecone) UpsertRecords(ctx context.Context, indexHost, namespace string, records []*pinecone.IntegratedRecord, dryRun bool) (int, error) {
	index, err := p.Index(pinecone.NewIndexConnParams{Host: indexHost, Namespace: namespace})
	if err != nil {
		return 0, fmt.Errorf("describe index: %v", err)
	}

	err = index.UpsertRecords(ctx, records)
	if err != nil {
		return 0, fmt.Errorf("upsert records: %v", err)
	}

	return len(records), nil
}

func (p *Pinecone) Upsert(ctx context.Context, indexHost, namespace string, vectors []*pinecone.Vector, dryRun bool) (int, error) {
	index, err := p.Index(pinecone.NewIndexConnParams{Host: indexHost, Namespace: namespace})
	if err != nil {
		return 0, fmt.Errorf("create IndexConnection for Host: %v: %v", indexHost, err)
	}

	chunkSize := 300 // this seems to be the right size for the current embeddings

	fmt.Printf("Upserting %d vectors...", len(vectors))
	bar := pb.StartNew(len(vectors))
	bar.SetRefreshRate(time.Second)

	for i := 0; i < len(vectors); i += chunkSize {
		end := min(i+chunkSize, len(vectors))

		if !dryRun {
			_, err := index.UpsertVectors(ctx, vectors[i:end])
			if err != nil {
				return 0, fmt.Errorf("upsert vectors: %v", err)
			}
		}

		bar.Add(chunkSize)
	}

	bar.Finish()
	return len(vectors), nil
}

func (p *Pinecone) QueryByVector(ctx context.Context, indexHost, namespace string, queryVector []float32, opts ...func(*PineconeQueryOptions)) (*pinecone.QueryVectorsResponse, error) {
	index, err := p.Index(pinecone.NewIndexConnParams{Host: indexHost, Namespace: namespace})
	if err != nil {
		return nil, fmt.Errorf("create IndexConnection for Host: %v: %v", indexHost, err)
	}

	options := &PineconeQueryOptions{
		topK: 1000,
	}

	for _, opt := range opts {
		opt(options)
	}

	res, err := index.QueryByVectorValues(ctx, &pinecone.QueryByVectorValuesRequest{
		Vector:        queryVector,
		TopK:          uint32(options.topK),
		IncludeValues: false,
	})
	if err != nil {
		return nil, fmt.Errorf("querying by vector: %v", err)
	}

	return res, nil
}

func (p *Pinecone) SearchRecords(ctx context.Context, indexHost, namespace string, query string, opts ...func(*PineconeQueryOptions)) (*pinecone.SearchRecordsResponse, error) {
	index, err := p.Index(pinecone.NewIndexConnParams{Host: indexHost, Namespace: namespace})
	if err != nil {
		return nil, fmt.Errorf("create IndexConnection for Host: %v: %v", indexHost, err)
	}

	options := &PineconeQueryOptions{
		topK: 2,
	}

	for _, opt := range opts {
		opt(options)
	}

	res, err := index.SearchRecords(ctx, &pinecone.SearchRecordsRequest{
		Query: pinecone.SearchRecordsQuery{
			TopK:   int32(options.topK),
			Inputs: &map[string]any{"text": query},
			Filter: options.filter,
		},
		Fields: options.fields,
		Rerank: options.rerank,
	})
	if err != nil {
		return nil, fmt.Errorf("search records: %v", err)
	}

	return res, nil
}

func (p *Pinecone) FetchVectors(ctx context.Context, indexHost, namespace string, ids []string) (*pinecone.FetchVectorsResponse, error) {
	index, err := p.Index(pinecone.NewIndexConnParams{Host: indexHost, Namespace: namespace})
	if err != nil {
		return nil, fmt.Errorf("create IndexConnection for Host: %v: %v", indexHost, err)
	}

	res, err := index.FetchVectors(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("fetch vectors: %v", err)
	}

	return res, nil
}

// normalizeVector normalizes a vector to unit length
func normalizeVector(vector []float32) []float32 {
	// Calculate the magnitude (Euclidean norm)
	var sumSquares float64
	for _, v := range vector {
		sumSquares += float64(v * v)
	}
	magnitude := float32(math.Sqrt(sumSquares))

	// Avoid division by zero
	if magnitude == 0 {
		return vector
	}

	// Normalize each component
	normalized := make([]float32, len(vector))
	for i, v := range vector {
		normalized[i] = v / magnitude
	}

	return normalized
}

// CosineSimilarity calculates the cosine similarity between two vectors
func CosineSimilarity(v1, v2 *pinecone.Vector) (float64, error) {
	// Check if vectors have values
	if v1 == nil || v2 == nil {
		return 0, fmt.Errorf("vectors cannot be nil")
	}

	// Handle dense vectors
	if v1.Values != nil && v2.Values != nil {
		return CosineSimilarityDense(*v1.Values, *v2.Values)
	}

	// Handle sparse vectors - future implementation if needed
	if v1.SparseValues != nil && v2.SparseValues != nil {
		return 0, fmt.Errorf("sparse vector similarity not implemented")
	}

	return 0, fmt.Errorf("vectors must be both dense or both sparse")
}

// CosineSimilarityDense calculates cosine similarity between two dense vectors
func CosineSimilarityDense(v1, v2 []float32) (float64, error) {
	if len(v1) != len(v2) {
		return 0, fmt.Errorf("vectors must have the same dimension, got %d and %d", len(v1), len(v2))
	}

	var dotProduct float64
	var norm1 float64
	var norm2 float64

	for i := 0; i < len(v1); i++ {
		dotProduct += float64(v1[i]) * float64(v2[i])
		norm1 += float64(v1[i]) * float64(v1[i])
		norm2 += float64(v2[i]) * float64(v2[i])
	}

	// Avoid division by zero
	if norm1 == 0 || norm2 == 0 {
		return 0, nil
	}

	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2)), nil
}

func (p *Pinecone) UpdateMetadata(ctx context.Context, indexHost, namespace string, id string, metadata map[string]any) error {
	index, err := p.Client.Index(pinecone.NewIndexConnParams{Host: indexHost, Namespace: namespace})
	if err != nil {
		return fmt.Errorf("failed to create IndexConnection for Host: %v", err)
	}

	metadataFilter, err := structpb.NewStruct(metadata)
	if err != nil {
		return fmt.Errorf("failed to create metadata map: %v", err)
	}

	return index.UpdateVector(ctx, &pinecone.UpdateVectorRequest{
		Id:       id,
		Metadata: metadataFilter,
	})
}

func (p *Pinecone) DeleteRecords(ctx context.Context, indexHost, namespace string, ids []string) error {
	index, err := p.Client.Index(pinecone.NewIndexConnParams{Host: indexHost, Namespace: namespace})
	if err != nil {
		return fmt.Errorf("failed to create IndexConnection for Host: %v", err)
	}

	return index.DeleteVectorsById(ctx, ids)
}

func (p *Pinecone) DeleteAllRecords(ctx context.Context, indexHost, namespace string) error {
	index, err := p.Client.Index(pinecone.NewIndexConnParams{Host: indexHost, Namespace: namespace})
	if err != nil {
		return fmt.Errorf("failed to create IndexConnection for Host: %v", err)
	}

	return index.DeleteAllVectorsInNamespace(ctx)
}
