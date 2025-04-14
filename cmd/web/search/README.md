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
   - Any job with `JobStatusUnprocessed` is requeued for processing
   - This ensures no jobs are left unprocessed after a service interruption

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