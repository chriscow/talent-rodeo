# Phase 2: Search and Analysis Implementation (Week 2-3)

**IMPORTANT**: This document details Phase 2 implementation. For overall project context, see `JOB_MATCHING_OVERVIEW.md`.
**IMPORTANT**: DO NOT DELETE ANYTHING IN THIS FILE. Mark items as completed (e.g. via ~~strike through~~) or move them to a "Deprecated/Completed" section to maintain a full record.

## Phase Goals
- Integrate LinkedIn job data
- Build robust search capabilities
- Implement analysis systems

## LinkedIn Jobs Integration

### ScrapingDog Code Refactor
- [ ] Review existing code:
  - `/internal/scrapingdog`
  - `/cmd/try/scrapingdog`
- [ ] Plan refactoring strategy:
  ```go
  // Current structure
  internal/scrapingdog/
  └── linkedin/
      └── jobs.go

  // Target structure
  internal/jobmatch/
  └── sources/
      └── linkedin/
          ├── client.go    // API client
          ├── models.go    // Data models
          └── scraper.go   // Scraping logic
  ```

### Job Data Pipeline
- [ ] Implement job data extraction:
  ```go
  type LinkedInScraper struct {
      client    *http.Client
      apiKey    string
      rateLimit *rate.Limiter
  }

  func (s *LinkedInScraper) ScrapeJob(ctx context.Context, url string) (*LinkedinJob, error)
  func (s *LinkedInScraper) ScrapeJobs(ctx context.Context, query string) ([]LinkedinJob, error)
  ```

- [ ] Create data transformation pipeline:
  ```go
  type Pipeline struct {
      extractor  JobExtractor
      transform  JobTransformer
      loader     JobLoader
      errorChan  chan error
  }

  func (p *Pipeline) Process(ctx context.Context, jobs <-chan RawJob) <-chan ProcessedJob
  ```

### Data Indexing
- [ ] Set up search indexing:
  ```go
  type JobIndex struct {
      store     *store.Store
      vectors   *VectorStore
      analyzer  *TextAnalyzer
  }

  func (idx *JobIndex) Index(ctx context.Context, job *LinkedinJob) error
  func (idx *JobIndex) Search(ctx context.Context, query string) ([]SearchResult, error)
  ```

## Keyword & Requirement Analysis

### ~~Keyword Extraction System~~
- [x] Implement text analysis:
  ```go
  type TextAnalyzer struct {
      tokenizer    Tokenizer
      stopWords    map[string]struct{}
      weightConfig WeightConfig
  }

  func (a *TextAnalyzer) ExtractKeywords(text string) []Keyword
  func (a *TextAnalyzer) AnalyzeFrequency(keywords []string) map[string]float64
  ```

- [x] Create keyword categorization:
  ```go
  type KeywordCategorizer struct {
      categories map[string][]string
      matcher   *PhraseMatcher
  }

  func (c *KeywordCategorizer) Categorize(keyword string) string
  ```

### ~~Requirement Analysis~~
- [x] Build requirement parser:
  ```go
  type RequirementParser struct {
      patterns    []*regexp.Regexp
      classifier  *RequirementClassifier
  }

  func (p *RequirementParser) Parse(text string) []Requirement
  func (p *RequirementParser) Classify(req string) RequirementType
  ```

- [x] Implement requirement matcher:
  ```go
  type RequirementMatcher struct {
      store    *store.Store
      scorer   *SkillScorer
  }

  func (m *RequirementMatcher) Match(ctx context.Context, reqs []Requirement, candidate *Candidate) float64
  ```

### Culture Trait Mapping
- [ ] Create culture trait extractor:
  ```go
  type CultureTraitExtractor struct {
      llm       *minds.ContentGenerator
      traits    map[string][]string
      analyzer  *TextAnalyzer
  }

  func (e *CultureTraitExtractor) Extract(description string) ([]CultureTrait, error)
  ```

- [ ] Implement preference mapper:
  ```go
  type PreferenceMapper struct {
      store     *store.Store
      matcher   *TraitMatcher
  }

  func (m *PreferenceMapper) MapPreferences(ctx context.Context, traits []CultureTrait) []Preference
  ```

### Missing Items
- [ ] Add support for soft skills analysis
- [ ] Implement industry-specific keyword weighting
- [ ] Add context-aware requirement parsing
- [ ] Create comprehensive test suite for culture matching
- [ ] Add validation for culture trait extraction
- [ ] Implement feedback loop for preference mapping accuracy

## Search Infrastructure

### Vector Generation
- [ ] Implement job vectorization using OpenAI embeddings:
  ```go
  type JobVectorizer struct {
      client    *openai.Client  // Using github.com/sashabaranov/go-openai
      analyzer  *TextAnalyzer
  }

  func (v *JobVectorizer) Vectorize(ctx context.Context, job *LinkedinJob) ([]float64, error)
  ```

- [ ] Create Pinecone vector store integration:
  ```go
  type VectorStore struct {
      pinecone  *pinecone.Client
      dimension int
  }

  func (vs *VectorStore) Store(ctx context.Context, id uuid.UUID, vector []float64) error
  func (vs *VectorStore) Search(ctx context.Context, vector []float64, limit int) ([]SearchResult, error)
  ```

Note: We will use Pinecone as the vector database for semantic search and the github.com/sashabaranov/go-openai package for generating embeddings. Additional vectors for matching amounts and degrees of preferences will be generated internally.

### Search System
- [ ] Build search service:
  ```go
  type SearchService struct {
      store     *store.Store
      index     *JobIndex
      ranker    *SearchRanker
  }

  func (s *SearchService) Search(ctx context.Context, query string, filters SearchFilters) ([]SearchResult, error)
  ```

- [ ] Implement result ranking:
  ```go
  type SearchRanker struct {
      weights   RankingWeights
      scorer    *RelevanceScorer
  }

  func (r *SearchRanker) Rank(results []SearchResult) []RankedResult
  ```

## Testing Plan

### Unit Tests
- [ ] LinkedIn integration
- [ ] Text analysis components
- [ ] Vector operations
- [ ] Search functionality

### Integration Tests
- [ ] Data pipeline
- [ ] Search system
- [ ] Analysis pipeline

### Performance Tests
- [ ] Vector operations
- [ ] Search latency
- [ ] Pipeline throughput

## Documentation

### API Documentation
- [ ] Document search endpoints
- [ ] Detail analysis APIs
- [ ] Include usage examples

### Implementation Notes
- [ ] Document design decisions
- [ ] Add configuration guide
- [ ] Include performance notes

## Dependencies
- Text analysis libraries
- Vector computation packages
- Required Go packages:
  ```
  github.com/PullRequestInc/go-gpt3
  github.com/golang/protobuf
  github.com/jackc/pgx/v4
  ```

## Progress Tracking

### Current Status
- [ ] LinkedIn Integration: Not Started
- [ ] Analysis Systems: Not Started
- [ ] Search Infrastructure: Not Started

### Next Steps
1. [ ] Begin LinkedIn integration
2. [ ] Set up text analysis
3. [ ] Implement search system

## Updates
- Initial Document Creation: [Current Date] 