# ScrapingDog Provider Refactoring Plan

## Current Issues

1. **Unclear Method Naming**: Many method names don't follow a consistent pattern or clearly indicate their purpose.
2. **Mixed Responsibilities**: The Provider class handles API calls, job processing, data storage, and event notifications.
3. **Complex Worker Logic**: The worker pattern is implemented but with complex error handling scattered across methods.
4. **Repeated Code Patterns**: Common operations like error handling and job ID formatting are duplicated.
5. **Unclear Interface Implementation**: The package doesn't clearly implement the defined JobProvider interface.
6. **Excessive Error Handling**: Error handling is verbose and repeated across methods.
7. **Poor Separation of Concerns**: API calls, job processing, and data storage logic are intermingled.

## Refactoring Goals

1. **Improve Method Naming**: Ensure all methods follow a clear, consistent naming convention.
2. **Separate Responsibilities**: Divide the Provider class into smaller, focused components.
3. **Simplify Worker Logic**: Streamline the worker implementation with clear error handling patterns.
4. **Remove Duplication**: Extract common operations into reusable helper functions.
5. **Clear Interface Implementation**: Explicitly implement the required interfaces with well-defined methods.
6. **Centralize Error Handling**: Create a unified approach to error management.
7. **Better Separation of Concerns**: Separate API calls, job processing, and data storage.

## Proposed Structure

### Core Components

1. **API Client**: Handles all direct API interactions with ScrapingDog
   - Makes HTTP requests
   - Handles retries and rate limiting
   - Parses API responses

2. **Job Store**: Manages job data persistence
   - Caches job listings and details
   - Interacts with Firestore
   - Provides clear data access methods

3. **Provider**: Implements the JobProvider interface
   - Coordinates between API Client and JobStore 
   - Handles high-level search operations
   - Implements required interface methods

4. **Worker Manager**: Handles background processing
   - Manages worker goroutines
   - Controls queue and concurrency
   - Provides clean shutdown

### Files Structure

```
/internal/search/providers/scrapingdog/
├── client.go         # API client + interaction logic
├── types.go          # Common types and constants
├── errors.go         # Error definitions and handling
├── store.go          # Data storage implementation
├── provider.go       # Provider implementation
├── workers.go        # Worker management
└── utils.go          # Common utility functions
```

## Detailed Changes

### 1. Extract API Client

Create a dedicated `APIClient` to handle all direct interactions with the ScrapingDog API:

```go
// api_client.go
type APIClient struct {
    apiKey  string
    baseURL string
    client  *http.Client
    logger  *slog.Logger
}

func NewAPIClient(apiKey string, opts ...APIOption) *APIClient {...}

func (c *APIClient) SearchJobs(ctx context.Context, keywords []string, location string, page int) ([]jobListing, error) {...}
func (c *APIClient) GetJobDetails(ctx context.Context, jobID string) (*jobDetail, error) {...}
func (c *APIClient) doRequestWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {...}
```

### 2. Refactor Job Store

Simplify and clarify the JobStore interface and implementation:

```go
// store.go
type JobStore interface {
    GetCachedJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error)
    SaveJobs(ctx context.Context, criteria search.Criteria, jobs []search.JobListing) error
    GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error)
    SaveJobDetails(ctx context.Context, jobDetail *search.JobDetail) error
    CreateUserJob(ctx context.Context, userID string, searchHash string, job search.JobListing, details *search.JobDetail) error
}
```

### 3. Clarify Provider Interface Implementation

Explicitly implement the JobProvider interface with clear method boundaries:

```go
// provider.go
type Provider struct {
    api          *APIClient
    store        JobStore
    workerMgr    *WorkerManager
    eventHandler search.JobEventHandler
}

// Implement JobProvider interface
func (p *Provider) Name() string {...}
func (p *Provider) Validate(ctx context.Context) error {...}
func (p *Provider) SearchJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error) {...}
func (p *Provider) GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {...}

// Implement JobQueue interface
func (p *Provider) QueueSearch(ctx context.Context, userID string, criteria search.Criteria) error {...}
func (p *Provider) GetQueueDepth(ctx context.Context) (int, error) {...}
```

### 4. Extract Worker Manager

Create a dedicated WorkerManager for handling background processing:

```go
// workers.go
type WorkerManager struct {
    provider      *Provider
    searchQueue   chan QueuedSearch
    semaphore     chan struct{}
    logger        *slog.Logger
    shutdownCtx   context.Context
    shutdownFunc  context.CancelFunc
    shutdownFlag  int32
}

func NewWorkerManager(provider *Provider, maxConcurrent int, queueSize int) *WorkerManager {...}

func (wm *WorkerManager) Start(ctx context.Context, numWorkers int) {...}
func (wm *WorkerManager) Shutdown(ctx context.Context) error {...}
func (wm *WorkerManager) QueueSearch(ctx context.Context, userID string, criteria search.Criteria) error {...}
func (wm *WorkerManager) GetQueueDepth() int {...}
func (wm *WorkerManager) startWorker(ctx context.Context, workerID int) {...}
func (wm *WorkerManager) processQueuedSearch(ctx context.Context, qs QueuedSearch) error {...}
```

### 5. Centralize Error Handling

Create a dedicated errors.go file for error types and handling functions:

```go
// errors.go
type (
    ErrRequestTimeout   struct{ error }
    ErrInvalidURL       struct{ error }
    ErrRequestPending   struct{ error }
    ErrPlanLimitReached struct{ error }
    ErrConcurrentLimit  struct{ error }
    ErrInvalidAPIKey    struct{ error }
    ErrRequestFailed    struct{ error }
)

func NewRequestTimeoutError(msg string) *ErrRequestTimeout {...}
func NewInvalidURLError(msg string) *ErrInvalidURL {...}
// ... etc

func IsRetryableError(err error) bool {...}
func HandleAPIError(statusCode int) error {...}
```

### 6. Extract Utility Functions

Create a utils.go file for common utility functions:

```go
// utils.go
func CreateProviderJobID(providerJobID string) string {
    return funcs.Slugify(fmt.Sprintf("%s %s", providerName, providerJobID))
}

func ParseSalaryRange(ctx context.Context, salary string) (*search.SalaryRange, error) {...}
func ConvertJobListings(jobListings []jobListing) ([]search.JobListing, error) {...}
func ConvertJobDetail(detail jobDetail) (*search.JobDetail, error) {...}
```

## Implementation Phases

### Phase 1: Structural Reorganization
1. Create the new file structure
2. Move existing code into the appropriate files
3. Update imports and dependencies

### Phase 2: Interface Definition
1. Clearly define interfaces for each component
2. Update method signatures to match interfaces
3. Fix compilation errors from structure changes

### Phase 3: Functionality Refactoring
1. Extract duplicated code into utility functions
2. Improve error handling patterns
3. Simplify complex methods

### Phase 4: Worker System Refactoring
1. Refactor the worker manager implementation
2. Improve queue management
3. Enhance shutdown handling

### Phase 5: Testing
1. Update tests to match new structure
2. Add tests for new components
3. Verify functionality matches original implementation

## Benefits

1. **Improved Readability**: Clear component responsibilities and consistent method naming
2. **Better Testability**: Smaller components with focused responsibilities are easier to test
3. **Enhanced Maintainability**: Clearer separation of concerns makes future changes easier
4. **Better Error Handling**: Centralized error management improves reliability
5. **Improved Performance**: Potential optimizations from clearer code structure
6. **Easier Onboarding**: New developers can understand the codebase more quickly

## Risks and Mitigations

1. **Risk**: Introducing bugs during refactoring
   **Mitigation**: Comprehensive test coverage before and after changes

2. **Risk**: Breaking existing functionality
   **Mitigation**: Incremental changes with continuous testing

3. **Risk**: Implementation complexity during transition
   **Mitigation**: Clear phasing of changes and documentation

4. **Risk**: Performance regression
   **Mitigation**: Benchmark key operations before and after 