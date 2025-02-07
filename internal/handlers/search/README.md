# Search Package

The `search` package provides a flexible and extensible job search infrastructure for Talent Rodeo. It implements provider-agnostic interfaces for job search operations and manages various job search providers.

## Package Structure

```
/internal/pipeline/search/
├── search.go          # Core search functionality
├── provider.go        # Provider interface definition
├── company.go         # Company data structures
├── job.go            # Job data structures and methods
├── errors.go         # Package-specific errors
├── registry.go       # Provider registry management
└── providers/        # Provider implementations
    ├── README.md     # Provider documentation
    └── scrapingdog/  # ScrapingDog LinkedIn provider
        ├── provider.go
        ├── provider_test.go
        └── types.go
```

## Key Features

### Provider Management
- Generic provider interface
- Provider registry with dynamic registration
- Extensible provider configuration
- Error handling and validation
- Provider-specific type definitions

### Job Search
- Concurrent search operations
- Configurable search parameters
- Result aggregation and filtering
- Rate limiting and throttling
- Error recovery and retries

### Data Models
- Rich job data structures
- Company information handling
- Location data processing
- Salary range normalization
- Remote work classification

## Provider Implementation

### ScrapingDog Provider
- LinkedIn job search integration
- Robust error handling
- Rate limiting support
- Response parsing and validation
- Comprehensive test coverage

## Usage Example

```go
// Register providers
registry := search.NewRegistry()
registry.Register("linkedin", scrapingdog.NewProvider(cfg))

// Create search service
searcher := search.NewSearcher(registry)

// Perform search
results, err := searcher.Search(ctx, search.Request{
    Provider: "linkedin",
    Keywords: []string{"software engineer"},
    Location: "San Francisco",
    Remote:   true,
})
```

## Testing

The package includes:
- Unit tests for core functionality
- Integration tests for providers
- Mock implementations
- Error case coverage
- Rate limit testing

## Configuration

Required environment variables per provider:
```bash
# ScrapingDog Provider
SCRAPINGDOG_API_KEY=your-key-here
```

## Best Practices

1. Always implement proper error handling
2. Use rate limiting for external APIs
3. Validate all search parameters
4. Handle partial search results
5. Implement proper timeouts
6. Add comprehensive logging
7. Write thorough tests

## Future Enhancements

1. Additional provider implementations
2. Enhanced search filtering
3. Improved error recovery
4. Better rate limiting
5. Search result caching
6. Performance optimizations

## Provider Development

To implement a new provider:

1. Create a new directory under `providers/`
2. Implement the `Provider` interface
3. Add provider-specific types
4. Implement comprehensive tests
5. Add provider documentation
6. Register the provider in the registry

See the ScrapingDog provider implementation for a reference example. 