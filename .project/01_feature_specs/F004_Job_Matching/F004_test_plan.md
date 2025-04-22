# F004 Test Plan: Validating Search & Match Strategy

## Overview

Before committing to a full codebase refactor, we'll validate our search and matching strategy using existing job details from Firestore. This plan outlines incremental steps to build and test each component of the new architecture.

## Phase 1: Feature Extraction & Embedding

### 1.1 Sample Data Collection
1. Query existing job details from Firestore
2. Select a diverse sample set (different roles, locations, industries)
3. Create a test dataset with:
   - Raw job descriptions
   - Known skills and requirements (manually verified)
   - Expected search terms that should match

### 1.2 LLM Feature Extraction
1. Build a simple CLI tool to:
   - Take a job description as input
   - Use LLM to extract normalized features
   - Output structured features (skills, requirements, etc.)
2. Test with sample dataset
3. Evaluate accuracy and consistency
4. Iterate on prompt engineering if needed

### 1.3 Vector Embedding
1. Create a small Pinecone test index
2. Build embedding pipeline to:
   - Generate dense vectors from job descriptions
   - Generate sparse vectors from extracted features
3. Test with sample dataset
4. Measure embedding quality and clustering

## Phase 2: Search Strategy Validation

### 2.1 Basic Search Testing
1. Create test queries from known job titles/skills
2. Test vector similarity search
3. Test keyword/sparse vector search
4. Compare results with expected matches
5. Measure precision and recall

### 2.2 Hybrid Search Testing
1. Implement combined vector + keyword search
2. Test with more complex queries
3. Compare with baseline searches
4. Evaluate result quality

### 2.3 User Statement Testing
1. Create sample user statements
2. Convert to search-optimized format
3. Test matching against job vectors
4. Evaluate relevance of results

## Phase 3: Criteria Model Testing

### 3.1 Criteria Creation
1. Build tool to normalize and deduplicate search criteria
2. Test with various input formats
3. Verify slugification and uniqueness

### 3.2 Job-Criteria Linking
1. Create test criteria set
2. Build linking logic
3. Test many-to-many relationships
4. Verify efficient querying

## Phase 4: End-to-End Flow Testing

### 4.1 Mini Pipeline
1. Build simplified version of full pipeline:
   - Criteria creation
   - Feature extraction
   - Vector embedding
   - Search execution
   - Result ranking
2. Test with small dataset
3. Measure performance and quality

### 4.2 Scaling Test
1. Increase test dataset size
2. Measure performance impact
3. Identify bottlenecks
4. Test concurrent operations

## Success Criteria

### Feature Extraction
- [ ] 90%+ accuracy in identifying key skills
- [ ] Consistent normalization of similar terms
- [ ] Processing time < 5s per job

### Search Quality
- [ ] 80%+ precision for exact title matches
- [ ] 70%+ recall for related roles
- [ ] Search latency < 500ms

### Criteria Model
- [ ] Zero duplicate criteria created
- [ ] Efficient job-criteria linking
- [ ] Fast criteria enumeration

## Test Tools Needed

1. **Feature Extractor**
```go
// Simple CLI tool to test LLM feature extraction
type FeatureExtractor struct {
    llm    LLMClient
    logger Logger
}

func (f *FeatureExtractor) ExtractFeatures(jobDesc string) (*Features, error)
```

2. **Vector Tester**
```go
// Tool to test vector embeddings and search
type VectorTester struct {
    pinecone PineconeClient
    logger   Logger
}

func (v *VectorTester) TestSearch(query string, expectedMatches []string) TestResult
```

3. **Criteria Tester**
```go
// Tool to test criteria creation and deduplication
type CriteriaTester struct {
    store  FirestoreClient
    logger Logger
}

func (c *CriteriaTester) TestCriteriaCreation(queries []string) TestResult
```

## Implementation Plan

1. Build test tools one at a time
2. Start with small, controlled datasets
3. Document results and learnings
4. Iterate on problem areas
5. Only proceed with full refactor once quality metrics are met

## Metrics to Track

1. **Quality Metrics**
   - Feature extraction accuracy
   - Search precision and recall
   - Query response times

2. **System Metrics**
   - Processing time per job
   - Memory usage
   - API call frequency

3. **Cost Metrics**
   - LLM API usage
   - Vector DB operations
   - Storage requirements

## Next Steps

1. Create the Feature Extractor tool
2. Set up a test Pinecone index
3. Build initial test dataset
4. Begin Phase 1 testing

This incremental approach will help validate our strategy while minimizing risk and allowing for adjustments before committing to the full refactor. 