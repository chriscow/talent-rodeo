# Search Queue and Processing Service

The search queue and processing service is responsible for managing the enqueing
of job searches to search providers for users and processing job matches. It
periodically checks for users that are due for a new search, and enqueues a
search for them. It also processes job matches as they are found.

## Architecture Overview

### Components

1. **Service (`service.go`)**
   - Manages the enqueing of job searches for users
   - Handles periodic user search scheduling based on tier (Free, Pro, Enterprise)
   - Implements graceful startup, shutdown, and recovery
   - Manages job processing workers

2. **Events (`events.go`)**
   - Handles callbacks from job providers
   - Manages real-time user notifications via Pusher
   - Creates initial job records in Firestore

3. **Worker (`worker.go`)**
   - Processes queued search results from search providers through various stages
   - Performs job analysis using LLM
   - Updates job records with analysis results

## Job Processing Flow

1. **Job Discovery**
   ```
   Job Provider (e.g., ScrapingDog) 
   → OnNewJob callback 
   → Create bare-bones record (Status: Unprocessed) 
   → Queue for processing
   ```

2. **Job Processing**
   ```
   Worker picks up job search result
   → Load job details from provider
   → Summarize with LLM 
   → Analyze match 
   → Save results 
   → Notify user
   ```

## ScrapingDog Provider Search Process

The ScrapingDog provider's search process happens in two distinct phases:

1. **Phase 1: Job Listings Retrieval**
   - When a search is queued via `QueueSearch`, the user's `QueuedAt` field is set
   - The provider's worker calls `processJobListings` to download the list of job IDs matching the search criteria
   - This is the first API call to get the basic job listings

2. **Phase 2: Job Details Processing**
   - For each job in the listings, `processJobDetailsAndSave` is called which:
     - Downloads the full job details via another API call
     - Caches the job details
     - Creates a `UserJob` record with `"unprocessed"` status
     - This is when the `JobStatusUnprocessed` record is created
     
This two-phase process is important for understanding the recovery mechanism, as interruptions can occur at different points.

## Status Flow Between Components

The job status transitions between components as follows:

1. **ScrapingDog Worker (in `internal/search/providers/scrapingdog/workers.go`)**
   - When a job is found by the search provider, it processes the job details from the API
   - After successfully retrieving and caching job details, it creates a `UserJob` record
   - The full sequence in `processJobDetailsAndSave` is:
     1. Process job details via API call using `processJobDetails`
     2. Retrieve the cached job details
     3. Create UserJob with `"unprocessed"` status
     4. Save the UserJob to Firestore
     5. Notify about the new job
   - This means when an "unprocessed" job is found during recovery, all the underlying job details are already available

2. **Search Worker (in `cmd/web/search/worker.go`)**
   - Picks up jobs with `JobStatusUnprocessed` status
   - Performs analysis with LLM in `analyzeJobMatch` and `extractJobMatch`
   - When processing completes successfully, sets status to `JobStatusNew`

This status transition is crucial for the recovery mechanism, as it allows the system to identify jobs that were discovered but not yet processed.

## Recovery Mechanism

The service implements a robust recovery mechanism to handle service interruptions (crashes, restarts, etc.).

### How It Works

1. **Initial Job Recording**
   - When a new job is discovered, a bare-bones record is immediately created in Firestore
   - Status is set to `JobStatusUnprocessed`
   - This ensures no jobs are lost even if the service crashes before processing

2. **Service Startup Recovery**
   ```go
   Start Service
   → Recover unprocessed jobs
   → Requeue incomplete searches
   → Start normal processing
   ```

3. **Recovery Process**
   - On startup, the service scans all users' jobs
   - Any job with status `JobStatusUnprocessed` is requeued for processing
   - This ensures no jobs are left unprocessed after a service interruption

### Detailed Recovery Implementation

The recovery process is implemented in two main functions in `service.go`:

1. **recoverUnprocessedJobs()**
   - Retrieves all users via `internal.ListUsers()`
   - For each user, gets all their jobs via `internal.GetUserJobs()`
   - Scans for jobs with status `JobStatusUnprocessed`
   - Pushes these jobs onto the jobQueue for processing by the search worker
   - This handles cases where the service crashed after job details were retrieved (Phase 2) but before LLM analysis

2. **requeueIncompleteSearches()**
   - Scans all users to find any with a non-nil `QueuedAt` field in their search configuration
   - This indicates a search was queued but never completed (e.g., due to a service crash during Phase 1 or early in Phase 2)
   - Requeues these searches by calling the queue provider's `QueueSearch` method
   - This ensures searches interrupted before or during job listings retrieval are retried from the beginning

These two recovery mechanisms handle different failure scenarios:
   - `recoverUnprocessedJobs`: Recovers individual jobs that were found but not yet analyzed
   - `requeueIncompleteSearches`: Recovers entire search operations that may not have completed job discovery

Additionally, the worker implements a safeguard in the `processQueuedJob` function:
   - If a job is detected as already processed (non-nil `Analysis` field)
   - The system logs a warning and resets the job status to `JobStatusNew`
   - This prevents duplication of work while ensuring the job is in a consistent state

This multi-layered approach ensures that all types of interruptions are handled:
   - Interruptions during job discovery (Phase 1)
   - Interruptions during job details retrieval (Phase 2)
   - Interruptions during job analysis (in the search worker)

## Search Recovery Edge Cases

### Current Behavior for Different Failure Scenarios

The following table describes how the current recovery mechanisms handle various failure scenarios:

| Scenario | Description | Current Recovery Behavior |
|----------|-------------|---------------------------|
| Search queued but never starts | `QueuedAt` set but worker never picks it up | `requeueIncompleteSearches()` will requeue the search |
| Search starts but never reaches job listing download | Worker picks up search but crashes before API call | `requeueIncompleteSearches()` will requeue the search |
| Job listings retrieved but job details never start downloading | Listings retrieved but crashes before processing any | `requeueIncompleteSearches()` will requeue the entire search |
| Job details downloading starts but doesn't complete all listings | Some UserJobs created with "unprocessed" status | Both recovery mechanisms trigger: <br>- `recoverUnprocessedJobs()` processes created jobs<br>- `requeueIncompleteSearches()` requeues the search (potential duplicates) |
| All job listings processed but search worker never notified | All UserJobs created with "unprocessed" status | `recoverUnprocessedJobs()` will process the jobs |
| Search completed but `CompletedAt` never set | Jobs processed but `updateUserSearchCompletion()` fails | `QueuedAt` remains set, causing unnecessary requeuing |
| Provider returns error during search | API errors during retrieval | Search stays in "queued" state indefinitely |
| Partial overlap with already processed jobs | Rerun finds same jobs as before | Only basic de-duplication via Analysis check |

### Potential Improvement Areas

The current recovery mechanism has several areas that could be improved:

1. **Completion Status Tracking**
   - Problem: If a search completes all job processing but fails to set `CompletedAt`, it will be incorrectly requeued
   - Current behavior relies on both setting `CompletedAt` and clearing `QueuedAt` in a single transaction

2. **Duplicate Job Prevention**
   - Problem: Requeuing an incomplete search may rediscover already processed jobs
   - The only protection is the Analysis field check in `processQueuedJob`

3. **Error State Handling**
   - Problem: Searches that encounter errors may remain in "queued" state indefinitely
   - No mechanism to clear `QueuedAt` after repeated failures

4. **Search Progress Tracking**
   - Problem: No way to track which parts of a multi-page search have completed
   - A crash during page 3 of 5 would restart from page 1

5. **Stale Search Detection**
   - Problem: No timeout mechanism for searches that remain queued for an extended period
   - `QueuedAt` timestamp isn't used to determine if a search has been queued for too long

### Proposed Solutions

Several improvements could be implemented to make the recovery mechanism more robust:

1. **Search State Machine**
   - Implement explicit search states (QUEUED, PROCESSING, COMPLETED, ERROR)
   - Include timestamps for state transitions
   - Add a maximum time limit for searches to remain in QUEUED or PROCESSING states

2. **Search Transaction Log**
   - Maintain a log of search progress (pages processed, jobs discovered)
   - Allow for resume-from-point-of-failure rather than restarting
   - Include error records with retry counts

3. **Idempotent Job Processing**
   - Use deterministic job IDs based on content hash
   - Implement stronger de-duplication checks when requeuing searches
   - Use the search hash + job ID combination for uniqueness

4. **Health Check Mechanism**
   - Periodically check for "stuck" searches (in QUEUED state for too long)
   - Implement a heartbeat mechanism for long-running search operations
   - Auto-reset searches that exceed maximum processing time

5. **Error Backoff Strategy**
   - Implement graduated retry with exponential backoff for failed searches
   - Set maximum retry attempts before marking a search as failed
   - Store error information to help diagnose issues

These improvements would enhance the reliability of the search process and minimize the risk of lost jobs or redundant processing.

### Job States

- `JobStatusUnprocessed`: Initial state, job found but not yet processed
- `JobStatusNew`: Processing complete, ready for user interaction
- `JobStatusViewed`: User has viewed the job
- `JobStatusSaved`: User has saved the job
- `JobStatusApplied`: User has applied to the job
- `JobStatusRejected`: User has rejected the job

## Configuration

- `defaultTickInterval`: How often to check for new searches (default: 5s)
- `maxConcurrentJobs`: Maximum number of jobs processed simultaneously (default: 5)
- Search frequency by tier:
  - Free: Every 72 hours
  - Pro: Every 24 hours
  - Enterprise: Every 4 hours

## Error Handling

- Failed jobs are logged and reported to administrators
- Errors during processing don't affect other jobs
- Admin notifications are sent via email for critical errors

## Testing

The service includes comprehensive tests in:
- `service_test.go`
- `worker_test.go`

Test mode can be enabled by setting `TESTING_MODE=true` 