# Processor Package

The `processor` package implements the core job processing and matching logic for the job matching system. It handles job analysis, requirement parsing, and candidate matching using both traditional and LLM-based approaches.

## Components

### Job Processor
```go
type JobProcessor interface {
    Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error)
    Status(ctx context.Context, id uuid.UUID) (*ProcessStatus, error)
    Cancel(ctx context.Context, id uuid.UUID) error
}
```

The `JobProcessor` interface defines the main entry point for job processing operations. It supports:
- Synchronous job processing (Phase 1)
- Status tracking
- Cancellation handling
- Future async processing (Phase 5)

### Analyzer
```go
type Analyzer interface {
    ExtractKeywords(ctx context.Context, text string) ([]Keyword, error)
    ParseRequirements(ctx context.Context, text string) ([]Requirement, error)
    ValidateJobData(ctx context.Context, job *types.JobDetail) (*ValidationResult, error)
}
```

The `Analyzer` interface provides job analysis capabilities:
- Keyword extraction
- Requirement parsing
- Data validation
- LLM-powered analysis
- Traditional fallback analysis

### Matcher
```go
type Matcher struct {
    logger   *log.Logger
    analyzer Analyzer
    registry *providers.Registry
}
```

The `Matcher` handles job-candidate matching:
- Skills compatibility scoring
- Experience matching
- Location preference matching
- Work environment alignment
- Culture fit analysis

## Key Features

### Job Analysis
- LLM-powered job description analysis
- Intelligent keyword extraction
- Requirement categorization
- Experience level detection
- Validation and quality checks

### Matching Algorithm
- Multi-factor scoring system
- Weighted skill matching
- Location compatibility
- Experience alignment
- Preference matching

### Processing Pipeline
1. Job data validation
2. Description analysis
3. Requirement extraction
4. Candidate matching
5. Score computation
6. Result generation

## Usage Example

```go
// Create a new processor
processor := NewProcessor(providers...)

// Process a job search
resp, err := processor.Process(ctx, ProcessRequest{
    ID:       uuid.New(),
    Type:     ProcessTypeSearch,
    Provider: "linkedin",
    Params: map[string]interface{}{
        "keywords": []string{"golang"},
    },
})

// Create a matcher
matcher, err := NewMatcher(ctx, logger)
if err != nil {
    log.Fatal(err)
}

// Compute match
details, err := matcher.Match(ctx, job, candidate)
```

## Testing

The package includes comprehensive tests:
- Unit tests for all components
- Integration tests
- Concurrent processing tests
- Mock implementations
- LLM response mocking

## Configuration

Required environment variables:
```bash
OPENAI_API_KEY=your-key-here    # For LLM analysis
GEMINI_API_KEY=your-key-here    # Alternative LLM provider
LLM_PROVIDER=openai             # Provider selection
```

## Dependencies

- github.com/chriscow/minds - LLM integration
- github.com/google/uuid - Request IDs
- github.com/mitchellh/mapstructure - Parameter handling

## Best Practices

1. Always use context for cancellation
2. Implement proper error wrapping
3. Add debug logging
4. Handle provider-specific errors
5. Validate all inputs
6. Use proper timeouts
7. Implement graceful fallbacks

## Future Enhancements (Phase 5)

1. Async Processing
   - Message queue integration
   - Distributed state management
   - Enhanced monitoring

2. Performance Optimizations
   - Caching layer
   - Batch processing
   - Parallel analysis

3. Enhanced Matching
   - Machine learning models
   - Historical data analysis
   - Feedback incorporation 