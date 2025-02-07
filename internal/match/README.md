# Matching Package

The `match` package implements the core job matching functionality for Talent Rodeo. It provides a robust, extensible system for analysis, and candidate matching.

## Package Structure

```
/internal/match/
├── analyzer.go         # Job analysis and LLM integration
├── analyzer_test.go    # Core analyzer tests
├── analyzer_llm_test.go # LLM-specific analyzer tests
├── matcher.go         # Job matching implementation
├── matcher_test.go    # Matching logic tests
├── mock_test.go       # Test mocks and utilities
├── processor.go       # Job processing pipeline
├── processor_test.go  # Processing pipeline tests
├── provider.go        # Provider interface and registry
├── request.go         # Request handling and validation
├── types.go           # Shared types and interfaces
└── README.md          # Package documentation
```

## Key Features

### Job Processing
- Concurrent job search and analysis
- Provider-agnostic interface
- Robust error handling and logging
- State management and tracking
- Cancellation support via context
- LLM-powered analysis with fallbacks

### Job Analysis
- LLM-powered job description analysis
- Keyword extraction and weighting
- Requirement parsing with experience detection
- Job data validation
- Traditional fallback analysis when LLM is unavailable

### Job Matching
- Sophisticated matching algorithm
- Skills compatibility scoring
- Location and remote work preferences
- Experience level matching
- Company culture alignment
- Configurable matching weights

### Provider Integration
- Generic provider interface
- Multiple provider support
- Provider validation
- Extensible search criteria
- Built-in error handling

## Implementation Status

The package is currently in Phase 2:

1. ✅ Phase 1 - Core Infrastructure
   - Basic synchronous processing
   - In-memory state management
   - Single-node operation
   - LLM integration

2. 🚧 Phase 2 (Current) - Enhanced Features
   - Improved error handling
   - Better test coverage
   - Mock implementations
   - Performance optimizations

3. 📅 Phase 3 (Planned) - Distributed Processing
   - Async processing
   - Distributed state management
   - Enhanced monitoring
   - Scaling improvements

## Usage Example

```go
// Create a job processor with configuration
cfg := ProcessorConfig{
    MaxConcurrent: 10,
    Timeout:      time.Minute * 5,
}
processor := NewProcessor(cfg, providers...)

// Process a job search request
resp, err := processor.Process(ctx, ProcessRequest{
    ID:       uuid.New(),
    Type:     ProcessTypeSearch,
    Provider: "linkedin",
    Params: map[string]interface{}{
        "keywords": []string{"golang"},
        "location": "Remote",
    },
})

// Get job status
status, err := processor.Status(ctx, jobID)
```

## Testing

The package includes comprehensive test coverage:
- Unit tests for all components
- Integration tests for provider interactions
- LLM response mocking via mock_test.go
- Concurrent processing tests
- Error case coverage

## Configuration

Required environment variables:
```bash
OPENAI_API_KEY=your-key-here    # For LLM analysis
GEMINI_API_KEY=your-key-here    # Alternative LLM provider
```

## Dependencies

- Go 1.21+
- github.com/chriscow/minds - LLM integration
- github.com/google/uuid - Unique identifiers
- github.com/mitchellh/mapstructure - Parameter decoding

## Best Practices

1. Always use context for cancellation
2. Implement proper error wrapping
3. Add debug logging for operations
4. Handle provider-specific errors
5. Validate all inputs
6. Use proper timeouts
7. Implement graceful fallbacks
8. Write comprehensive tests

## Future Enhancements

1. Enhanced culture matching
2. Improved vector search
3. Better candidate scoring
4. Additional provider integrations
5. Real-time updates
6. Performance optimizations 