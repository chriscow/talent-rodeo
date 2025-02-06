# Job Matching Package

The `jobmatch` package implements the core job matching functionality for Talent Rodeo. It provides a robust, extensible system for job search, analysis, and candidate matching.

## Package Structure

- `types/` - Core domain types and interfaces
- `processor/` - Job processing and matching logic
- `providers/` - Job listing provider implementations
- `handlers/` - HTTP/API request handlers
- `models/` - Database models and operations
- `store/` - Data persistence layer

## Key Features

### Job Processing
- Concurrent job search and analysis
- Provider-agnostic interface
- Robust error handling and logging
- State management and tracking
- Cancellation support

### Job Analysis
- LLM-powered job description analysis
- Keyword extraction and weighting
- Requirement parsing with experience detection
- Job data validation
- Traditional fallback analysis

### Job Matching
- Sophisticated matching algorithm
- Skills compatibility scoring
- Location and remote work preferences
- Experience level matching
- Company culture alignment

### Provider Integration
- Generic provider interface
- Multiple provider support
- Provider validation
- Extensible search criteria

## Implementation Status

The package is being implemented in phases:

1. Phase 1 (Current) - Core Infrastructure
   - Basic synchronous processing
   - In-memory state management
   - Single-node operation

2. Phase 5 (Planned) - Distributed Processing
   - Async processing via NATS/Watermill
   - Distributed state management
   - Message queue integration
   - Enhanced monitoring and metrics

## Usage Example

```go
// Create a job processor
processor := NewProcessor(providers...)

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
- Concurrent processing tests
- Mock implementations for testing
- LLM response mocking

## Configuration

Required environment variables:
```bash
OPENAI_API_KEY=your-key-here    # For LLM analysis
GEMINI_API_KEY=your-key-here    # Alternative LLM provider
REDIS_URL=redis://localhost:6379 # For caching (Phase 5)
LLM_CACHE_TTL=3600              # Cache TTL in seconds
LLM_PROVIDER=openai             # LLM provider selection
```

## Dependencies

- Go 1.21+
- github.com/chriscow/minds - LLM integration
- github.com/google/uuid - Unique identifiers
- github.com/mitchellh/mapstructure - Parameter decoding
- NATS/Watermill (Phase 5) - Message queue

## Best Practices

1. Always use context for cancellation
2. Implement proper error wrapping
3. Add debug logging for operations
4. Handle provider-specific errors
5. Validate all inputs
6. Use proper timeouts
7. Implement graceful fallbacks

## Future Enhancements

1. Distributed processing support
2. Enhanced caching layer
3. Improved matching algorithms
4. Additional provider integrations
5. Real-time updates
6. Performance optimizations 