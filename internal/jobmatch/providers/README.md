# Providers Package

The `providers` package manages job listing provider implementations and their registration. It provides a flexible system for integrating multiple job data sources while maintaining a consistent interface.

## Components

### Provider Registry
```go
type Registry struct {
    providers map[string]types.JobProvider
    mu        sync.RWMutex
}
```

The `Registry` manages provider registration and access:
- Thread-safe provider management
- Provider lookup by name
- Registration validation
- Provider lifecycle management

### Core Functions

```go
func NewRegistry() *Registry
func (r *Registry) Register(provider types.JobProvider) error
func (r *Registry) Get(name string) (types.JobProvider, error)
func (r *Registry) List() []string
func (r *Registry) Unregister(name string) error
```

## Provider Implementation

Each provider must implement the `types.JobProvider` interface:

```go
type JobProvider interface {
    SearchJobs(ctx context.Context, criteria SearchCriteria) ([]Job, error)
    GetJobDetails(ctx context.Context, jobID string) (*JobDetail, error)
    Name() string
    Validate(ctx context.Context) error
}
```

### Implementation Guidelines

1. Error Handling
   - Use proper error wrapping
   - Provider-specific error types
   - Graceful failure handling
   - Rate limit handling

2. Data Mapping
   - Map provider data to common types
   - Handle missing fields gracefully
   - Normalize data formats
   - Validate mapped data

3. API Integration
   - Implement proper retries
   - Handle rate limits
   - Cache responses
   - Monitor API health

## Usage Example

```go
// Create a new registry
registry := NewRegistry()

// Register a provider
provider := &MyProvider{
    name: "my-provider",
    client: api.NewClient(),
}
err := registry.Register(provider)

// Get a provider
provider, err := registry.Get("my-provider")

// Use the provider
jobs, err := provider.SearchJobs(ctx, criteria)
```

## Included Providers

1. LinkedIn Provider
   - Job search functionality
   - Detailed job information
   - Company data integration
   - Location handling

2. ScrapingDog Provider (Reference Implementation)
   - Basic job search
   - Job detail retrieval
   - Example implementation patterns

## Testing

The package includes:
- Unit tests for registry
- Provider implementation tests
- Integration tests
- Mock providers
- API response mocking

## Best Practices

1. Provider Implementation
   - Implement proper validation
   - Add comprehensive logging
   - Handle API errors gracefully
   - Cache when appropriate
   - Monitor performance

2. Data Quality
   - Validate all fields
   - Handle missing data
   - Normalize formats
   - Maintain data consistency

3. Error Handling
   - Use proper error types
   - Implement retries
   - Handle rate limits
   - Log errors appropriately

## Future Enhancements

1. Phase 5 Features
   - Async provider operations
   - Enhanced caching
   - Distributed rate limiting
   - Health monitoring

2. Provider Improvements
   - Additional providers
   - Enhanced data mapping
   - Better error handling
   - Performance optimization

3. Monitoring
   - Provider metrics
   - Error tracking
   - Performance monitoring
   - Usage analytics

## Dependencies

- Standard library
- Provider-specific API clients
- Context for cancellation
- Sync for thread safety 