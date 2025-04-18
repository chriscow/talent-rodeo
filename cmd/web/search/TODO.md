# Search Service Improvements

This document outlines identified issues and proposed improvements for the search service's recovery mechanism.

## Critical Issues

### 1. Duplicate Job Processing Risk
**Problem**: When a search is partially completed, both recovery mechanisms can trigger simultaneously
- `recoverUnprocessedJobs()` processes existing "unprocessed" jobs
- `requeueIncompleteSearches()` requeues the entire search
- Only basic de-duplication via Analysis field check

**Impact**: 
- Unnecessary API calls to ScrapingDog
- Potential duplicate job processing
- Increased costs and system load

### 2. Stuck Searches
**Problem**: Searches can remain in "queued" state indefinitely
- No timeout mechanism for long-running searches
- `QueuedAt` field never cleared after errors
- No maximum retry attempts for failed searches

**Impact**:
- Resources tied up in stuck searches
- Unnecessary requeuing on service restart
- Difficult to diagnose failed searches

### 3. Incomplete Progress Tracking
**Problem**: No way to track partial progress of multi-page searches
- Crashes during page processing restart from beginning
- No record of which jobs were successfully processed
- No tracking of API errors or retry attempts

**Impact**:
- Redundant processing of already completed pages
- Increased API costs
- Poor visibility into search progress

## Proposed Solutions

### Phase 1: Search State Management
1. Implement proper search state machine:
   ```go
   type SearchState string
   
   const (
       SearchStateQueued     SearchState = "QUEUED"
       SearchStateProcessing SearchState = "PROCESSING"
       SearchStateCompleted  SearchState = "COMPLETED"
       SearchStateError      SearchState = "ERROR"
   )
   
   type SearchStatus struct {
       State           SearchState
       QueuedAt       *time.Time
       StartedAt      *time.Time
       CompletedAt    *time.Time
       LastUpdatedAt  *time.Time
       ErrorCount     int
       LastError      string
       CurrentPage    int
       TotalPages     int
   }
   ```

2. Add timeout and health check mechanisms:
   - Maximum time limits for each state
   - Heartbeat updates during processing
   - Automatic cleanup of stale searches

### Phase 2: Progress Tracking
1. Implement search transaction log:
   ```go
   type SearchTransaction struct {
       SearchID    string
       PageNumber  int
       StartTime   time.Time
       EndTime     *time.Time
       Status      string
       JobsFound   int
       Error       string
   }
   ```

2. Add resume capability:
   - Store progress at page level
   - Track successfully processed jobs
   - Enable restart from last successful page

### Phase 3: Idempotency and Deduplication
1. Improve job uniqueness tracking:
   ```go
   type JobIdentifier struct {
       SearchHash    string    // Hash of search criteria
       JobID        string    // Provider's job ID
       ContentHash  string    // Hash of job content
       DiscoveredAt time.Time
   }
   ```

2. Strengthen duplicate detection:
   - Check existing jobs before processing
   - Use content-based deduplication
   - Track job history across searches

### Phase 4: Error Handling
1. Implement graduated retry with backoff:
   ```go
   type RetryConfig struct {
       MaxAttempts     int
       InitialDelay    time.Duration
       MaxDelay        time.Duration
       BackoffFactor   float64
   }
   ```

2. Add error classification:
   - Categorize errors (temporary vs permanent)
   - Track error patterns
   - Implement different strategies per error type

## Implementation Priority

1. **High Priority** (Immediate Improvements)
   - [ ] Search state machine implementation
   - [ ] Basic timeout mechanism
   - [ ] Error state handling

2. **Medium Priority** (Reliability Improvements)
   - [ ] Progress tracking system
   - [ ] Basic retry mechanism
   - [ ] Duplicate detection improvements

3. **Lower Priority** (Optimization)
   - [ ] Advanced error handling
   - [ ] Detailed analytics
   - [ ] Performance optimizations

## Notes for Implementation

- Keep changes backward compatible
- Add proper logging for debugging
- Include metrics for monitoring
- Add configuration options for timeouts and retry policies
- Consider adding admin dashboard for search status visibility

## Questions to Resolve

1. What is an acceptable timeout period for searches?
2. How many retry attempts should be allowed?
3. Should we implement different timeout policies for different user tiers?
4. How do we handle partial successes in multi-page searches?
5. What metrics should we track for monitoring search health?

## Related Files

- `cmd/web/search/service.go`
- `cmd/web/search/worker.go`
- `internal/search/providers/scrapingdog/workers.go`
- `internal/search/providers/scrapingdog/provider.go` 