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
type JobProvider interface {
    // SearchJobs performs a search for jobs based on the provided criteria.
    SearchJobs(ctx context.Context, criteria Criteria) ([]JobListing, error)
    
    // GetJobDetails retrieves detailed information about a specific job.
    GetJobDetails(ctx context.Context, jobID string) (*JobDetail, error)
    
    // Name returns the unique identifier for this provider.
    Name() string
    
    // Validate verifies that the provider is properly configured and can make API calls.
    Validate(ctx context.Context) error
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

## Job Search Flow

The job search process follows these steps:

1. **Search Queueing**
   - User initiates a search with keywords and location
   - Search is queued with `QueueSearch()`
   - User's search state is updated with `queued_at` timestamp
   - Search request enters a buffered channel for processing

2. **Worker Processing**
   - Multiple workers (configured via `StartSearchWorkers()`) process queued searches
   - Workers use semaphores to control concurrent API requests
   - Each worker processes one search at a time from the queue

3. **Job Search and Caching**
   - Check for cached job listings using search hash (keywords + location)
   - If no cache hit, fetch new listings via `SearchJobListings()`
   - Save listings to Firestore with search hash for future cache hits

4. **Job Details Processing**
   - For each job listing found:
     - Check for cached job details
     - If not cached, fetch via `GetJobDetails()`
     - Store complete job details in Firestore
     - Create `UserJob` entries linking jobs to the searching user
     - Store search context (search hash) with each user job

5. **Status Updates**
   - Update user's search status throughout the process:
     - `queued_at`: When search enters queue
     - `started_at`: When processing begins
     - `completed_at`: When all jobs are processed
   - Clear `queued_at` after completion

6. **Error Handling**
   - Implement retries for rate limits and timeouts
   - Use exponential backoff for rate-limited requests
   - Gracefully handle API errors with specific error types
   - Maintain context cancellation support

7. **Deduplication and Efficiency**
   - Use search hash to cache and deduplicate job listings
   - Store job details separately from user associations
   - Reference job details from user jobs to avoid duplication
   - Maintain user-specific metadata (status, notes) separately

This flow ensures efficient job searching while maintaining data consistency and providing a good user experience through caching and proper error handling.

## Provider Structure

Each provider should follow this structure:
```
providers/
└── provider_name/
    ├── provider.go      # Main implementation
    ├── provider_test.go # Tests
    ├── store.go         # Data storage implementation
    ├── store_test.go    # Data storage tests
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
2. Implement the JobProvider interface
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