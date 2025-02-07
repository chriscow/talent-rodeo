# Search Providers

This directory contains implementations of various job search providers for the Talent Rodeo platform. Each provider implements the common search interface while handling provider-specific details.

## Current Providers

### ScrapingDog (LinkedIn)
- Implementation: `/scrapingdog`
- Purpose: LinkedIn job search integration
- Features:
  - Job search by keywords
  - Location-based filtering
  - Remote work detection
  - Salary information extraction
  - Company details retrieval

## Provider Interface

All providers must implement the following interface:

```go
type Provider interface {
    Search(ctx context.Context, req Request) ([]Job, error)
    Name() string
    Validate() error
}
```

## Provider Requirements

Each provider implementation must:

1. Handle rate limiting appropriately
2. Implement proper error handling
3. Validate search parameters
4. Parse and normalize responses
5. Include comprehensive tests
6. Document configuration requirements
7. Handle API-specific edge cases

## Provider Structure

Each provider should follow this structure:
```
providers/
└── provider_name/
    ├── provider.go      # Main implementation
    ├── provider_test.go # Tests
    ├── types.go         # Provider-specific types
    └── README.md        # Provider documentation
```

## Testing Guidelines

Provider tests should cover:
1. Successful search scenarios
2. Error handling
3. Rate limiting
4. Parameter validation
5. Response parsing
6. Edge cases
7. Integration tests

## Adding New Providers

To add a new provider:

1. Create a new directory under `providers/`
2. Implement the Provider interface
3. Add necessary configuration
4. Write comprehensive tests
5. Document usage and requirements
6. Update this README

## Best Practices

1. Use proper error wrapping
2. Implement graceful degradation
3. Add detailed logging
4. Handle timeouts appropriately
5. Validate all external data
6. Use proper rate limiting
7. Document all assumptions

## Configuration

Each provider should document its configuration requirements in its README. Common configuration patterns:

```go
type ProviderConfig struct {
    APIKey      string
    BaseURL     string
    RateLimit   int
    MaxRetries  int
    Timeout     time.Duration
}
```

## Error Handling

Providers should use package-specific errors:

```go
var (
    ErrInvalidConfig = errors.New("invalid provider configuration")
    ErrRateLimited   = errors.New("rate limit exceeded")
    ErrTimeout       = errors.New("request timeout")
)
```

## Future Providers

Planned integrations:
1. Indeed API
2. Glassdoor API
3. ZipRecruiter API
4. Direct company APIs 