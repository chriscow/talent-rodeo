# Search Architecture

The search system in Talent Rodeo provides a flexible and extensible framework for job search operations across multiple providers. It is implemented in the `internal/services/search` package.

## Overview

The search architecture follows a provider-based design pattern:
- A generic `JobProvider` interface
- Provider registry for dynamic registration
- Rich job data structures
- Provider-specific implementations

## Core Components

### Job Provider Interface (`internal/services/search/provider.go`)

The `JobProvider` interface defines the contract for all search providers:

```go
type JobProvider interface {
    Name() string
    SearchJobs(ctx context.Context, criteria Criteria) (*Results, error)
    GetJobDetails(ctx context.Context, jobID string) (*Job, error)
}
```

### Provider Registry (`internal/services/search/registry.go`)

The `Registry` manages provider registration and selection:

```go
type Registry struct {
    providers map[string]JobProvider
    mu        sync.RWMutex
}
```

Key methods:
- `Register(provider JobProvider)`: Registers a provider
- `Get(name string)`: Retrieves a provider by name
- `GetAll()`: Retrieves all registered providers

### Job Data Structure (`internal/services/search/job.go`)

The `Job` struct represents a job listing:

```go
type Job struct {
    ID          string         `json:"id"`
    Title       string         `json:"title"`
    Company     Company        `json:"company"`
    Location    string         `json:"location"`
    Description string         `json:"description"`
    URL         string         `json:"url"`
    Salary      *SalaryRange   `json:"salary,omitempty"`
    PostedAt    time.Time      `json:"posted_at"`
    WorkType    WorkType       `json:"work_type"`
    Source      string         `json:"source"`
    Keywords    []string       `json:"keywords,omitempty"`
    ExternalID  string         `json:"external_id"`
}
```

### Search Criteria (`internal/services/search/types.go`)

The `Criteria` struct defines search parameters:

```go
type Criteria struct {
    Keywords    []string `json:"keywords"`
    Location    string   `json:"location"`
    Remote      bool     `json:"remote"`
    WorkType    WorkType `json:"work_type"`
    DatePosted  string   `json:"date_posted"`
    JobTypes    []string `json:"job_types"`
    Experience  string   `json:"experience"`
    PageSize    int      `json:"page_size"`
    PageNumber  int      `json:"page_number"`
    MaxPages    int      `json:"max_pages"`
}
```

### Pagination Support (`internal/services/search/page_range.go`)

The `PageRange` utility provides pagination functionality with different strategies based on user tiers.

## Provider Implementations

### ScrapingDog Provider (`internal/services/search/providers/scrapingdog`)

This provider implements LinkedIn job search through the ScrapingDog API:
- API key configuration
- Rate limiting
- Error handling
- Result parsing
- Comprehensive test coverage

## Search Flow

1. **Provider Selection**:
   - The application selects an appropriate provider from the registry
   - Provider configuration is loaded

2. **Search Execution**:
   - Search criteria are validated
   - Provider executes the search
   - Results are parsed and validated
   - Errors are handled

3. **Result Processing**:
   - Results are normalized
   - Job data is enriched
   - Results are filtered by relevance
   - Pagination information is applied

## Provider Development

To implement a new provider:
1. Create a new directory under `providers/`
2. Implement the `JobProvider` interface
3. Add provider-specific types and configuration
4. Implement comprehensive tests
5. Register the provider in the registry

## Best Practices

1. **Rate Limiting**:
   - Respect API rate limits
   - Implement exponential backoff
   - Cache results when appropriate
   - Monitor usage metrics

2. **Error Handling**:
   - Classify errors appropriately
   - Retry transient failures
   - Provide detailed error information
   - Log errors with context

3. **Result Quality**:
   - Normalize job titles
   - Standardize company information
   - Validate salary ranges
   - Enrich job descriptions

4. **Performance**:
   - Implement proper caching
   - Use concurrent requests when appropriate
   - Limit result set sizes based on user tier
   - Monitor response times

## Files

- Provider interface: `internal/services/search/provider.go`
- Registry implementation: `internal/services/search/registry.go`
- Job data structures: `internal/services/search/job.go`
- Core data types: `internal/services/search/types.go`
- Pagination utilities: `internal/services/search/page_range.go`
- ScrapingDog provider: `internal/services/search/providers/scrapingdog/` 