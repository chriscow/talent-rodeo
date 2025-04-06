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

### Search Service (`internal/search/service.go`)

The `Service` manages background job searches:

```go
type Service struct {
    fs *firestore.Client
    ticker *time.Ticker
}
```

Key features:
- Background processing using a ticker
- Tier-based search frequency (free: 3 days, pro: daily)
- Error handling with admin notifications via SMTP
- Integration with ScrapingDog provider

### User Search Data (`internal/user_search.go`)

The `UserSearch` struct tracks search state and preferences:

```go
type UserSearch struct {
    CompletedAt *time.Time
    Frequency    time.Duration
    Criteria     search.Criteria
    Error        string
}
```

### Job Storage Model

Jobs are stored as subcollections under each user:

```
users/
  {user_id}/
    job_details/  # Subcollection of detailed job information
      {job_id}/   # Individual job documents
        - title
        - description
        - company
        - search_phrase  # Search criteria that found this job
        - ...
```

## Search Flow

1. **Background Processing**:
   - Service runs on a ticker
   - Checks for users due for search based on tier frequency
   - Free tier: every 3 days
   - Pro tier: daily

2. **User Search State**:
   - Tracks CompletedAt timestamp
   - Manages search frequency by tier
   - Stores error state for admin review
   - Maintains search criteria

3. **Error Handling**:
   - Errors stored in UserSearch.Error
   - Admin notifications via SMTP include:
     - User information
     - Search criteria
     - Error details
     - Timestamp
   - Error recovery through background service

4. **Result Storage**:
   - Job details stored in user-specific subcollection
   - Each job detail includes search phrase
   - Efficient retrieval for user interface
   - Maintains search context

## Provider Implementations

### ScrapingDog Provider (`internal/services/search/providers/scrapingdog`)

This provider implements LinkedIn job search through the ScrapingDog API:
- API key configuration
- Rate limiting
- Error handling
- Result parsing
- Comprehensive test coverage

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

5. **Background Processing**:
   - Use ticker for consistent scheduling
   - Respect tier-based frequencies
   - Handle process shutdown gracefully
   - Monitor search completion rates

6. **Error Management**:
   - Store errors in UserSearch
   - Send admin notifications via SMTP
   - Include comprehensive error context
   - Track error resolution time

## Files

- Provider interface: `internal/services/search/provider.go`
- Registry implementation: `internal/services/search/registry.go`
- Job data structures: `internal/services/search/job.go`
- Core data types: `internal/services/search/types.go`
- Pagination utilities: `internal/services/search/page_range.go`
- ScrapingDog provider: `internal/services/search/providers/scrapingdog/`
- Search service: `internal/search/service.go`
- User search types: `internal/user_search.go`
- Admin mailer: `internal/smtp/mailer.go` 