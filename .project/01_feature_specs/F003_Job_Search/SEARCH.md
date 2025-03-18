# Job Search Feature Overview

# Next Steps are intentionally kept at the top of this document for visibility and to avoid duplication

## Next Steps
1. Complete Test Suite Implementation (COMPLETED)
   - ~~Unit tests for tier management~~
   - ~~Tests for periodic scheduling~~
   - ~~Integration tests for search flow~~
   - ~~Add concurrent search handling tests~~
     * ~~Test one search at a time per user requirement~~
     * ~~Test database locking behavior~~
     * ~~Test task rejection for concurrent attempts~~
   - ~~Add error handling tests~~
     * ~~Test NATS timeouts~~
     * ~~Test invalid tier handling~~
     * ~~Test non-retryable errors~~
   - ~~Add tier change tests~~
     * ~~Test tier upgrade scenarios (maintaining page continuity)~~
     * ~~Add tier downgrade scenarios~~
       - ~~Test resetting search progress~~
       - ~~Test handling last_page > new tier limit~~
     * ~~Add tests for skipping tiers (e.g., free to top)~~
       - ~~Test page continuity~~
       - ~~Test intermediate page handling~~
   - ~~Add end-to-end tests with real NATS messages~~
     * ~~Test complete search flow with JetStream~~
     * ~~Test message publishing and consumption~~
     * ~~Test error handling and retries~~
   - ~~Add edge case tests~~
     * ~~Test provider API failures~~
     * ~~Test partial result scenarios~~
     * ~~Test invalid input handling~~

2. ~~Implement Tier Change Handling~~
   - ~~Add tier downgrade logic~~
     * ~~Handle cases where last_page exceeds new tier limit~~
     * ~~Implement progress reset policy~~
   - ~~Add tier skip handling~~
     * ~~Define behavior for processing intermediate pages~~
     * ~~Maintain page continuity across multiple tier jumps~~

3. Enhance Error Recovery
   - ✓ Implement fallback strategy for failed searches
   - ✓ Add retry mechanism with backoff
   - ✓ Add monitoring for failed searches
   - ✓ Implement partial result handling

4. Add Search Progress Monitoring
   - ✓ Add real-time progress updates via KV store
   - ✓ Implement progress polling endpoint
   - ✓ Add progress UI components
   - ✓ Handle search failures gracefully

5. Implement Results Management
   - Add filtering system
   - Implement result persistence
   - Handle stale results
   - Add result expiration handling

## Implementation Checklist

### 1. Database Updates (in 000002_create_users_and_onboarding_tables.up.sql)
- [x] Add tier management to candidates table
  - [x] Add tier enum type ('free', 'mid', 'top')
  - [x] Add tier column with default 'free'
  - [x] Add tier_updated_at timestamp for tracking last change
- [x] Add last_search_at timestamp column to track periodic search timing

### 2. Search Button Integration
- [x] Add "Ready to Search" button to profile-preferences.html
  - [x] Create button HTML with HTMX attributes
  - [x] Add loading state styles
  - [x] Add hx-push-url to navigate to progress page
- [x] Create endpoint handler for search initiation
  - [x] Validate user has completed profile
  - [x] Extract and categorize skills from candidate_skills
    - [x] Identify must-have skills (rating = 5)
    - [x] Collect preferred skills (rating = 4) for secondary search
  - [x] Generate search messages based on tier limits
    - [x] Create separate NATS message for each page to fetch
    - [x] Include page number in search criteria
  - [x] Redirect to search progress page

### 3. Search Progress Page
- [x] Create search-progress.html template
  - [x] Add progress indicators section
    - [x] Total jobs found counter
    - [x] Jobs analyzed counter
    - [x] Good fit jobs counter
  - [x] Add status message section
  - [x] Add loading animations
  - [x] Add placeholder for results view
- [x] ~~Set up Pusher integration~~ Implement KV Store Progress Tracking
  - [x] Create user-specific KV buckets for progress
  - [x] Store progress updates in KV store
  - [x] Client-side polling for progress updates
  - [x] Handle search completion
- [x] Implement progress tracking
  - [x] Track jobs found count
  - [x] Track analysis progress
  - [x] Track matching jobs count
- [x] Handle state transitions
  - [x] Show progress view during search
  - [x] Switch to results view when complete
  - [x] Handle error states

### 4. Results View
- [x] Create basic results container
  - [x] Add results count summary
  - [x] Add job listings with match scores
  - [x] Add skill match indicators
- [x] Add styling using PicoCSS
- [ ] Add filtering capabilities (future enhancement)

### 5. Testing
- [x] Unit tests
  - [x] Test tier management
  - [x] Test search criteria building
  - [x] Test result limiting
  - [x] Test periodic search scheduling
- [ ] Integration tests
  - [x] Test search flow
  - [x] Test KV store updates
  - [x] Test error handling
  - [ ] Test end-to-end search process with real NATS messages
  - [ ] Test tier-based search limits
  - [ ] Test concurrent search handling

### 6. Periodic Search Implementation
- [x] Implement search scheduler
  - [x] Create scheduler in application struct
  - [x] Configure tier-based intervals:
    - Free: Every three days
    - Basic: Daily
    - Pro: Four times daily
  - [x] Implement background worker for periodic searches
  - [x] Add graceful shutdown handling
  - [x] Add logging and error handling
  - [x] Add test coverage for scheduler

### 7. Search Progress KV Store
- [x] Implement user-specific KV buckets
  - [x] Bucket naming: `search_progress_{user_id}`
  - [x] TTL: 5 minutes
  - [x] MaxBytes: 1MB
- [x] Progress tracking
  - [x] Store JSON-encoded progress updates
  - [x] Include total jobs, analyzed jobs, good fits
  - [x] Track completion status
- [x] Client integration
  - [x] Implement polling endpoint
  - [x] Handle progress updates
  - [x] Display results on completion

### Recent Updates
1. Code Organization
   - [x] Moved `SearchInput` struct to `internal/handlers/search/types.go`
   - [x] Added JetStream instance to `application` struct in `cmd/web/main.go`
   - [x] Updated `handlers.search.go` to use `app.jetstream` for publishing
   - [x] Added route for "/profile/search/start" in `routes.go`
   - [x] Implemented KV store for progress tracking
   - [x] Added user-specific search progress buckets
   - [x] Added search scheduler for periodic searches

## Search Flow Implementation Details (Simplified for MVP)

The search flow has been simplified to focus on core functionality for the MVP:

1. **Initial Trigger** (Profile Preferences Page):
   - User sets skill preferences with sliders (1-5) on profile-preferences.html
   - Skills with ratings 5 (Must Have) and 4 (Preferred) are used for search
   - User clicks "Ready to Search" button
   - POST request sent to "/profile/search/start" endpoint

2. **Search Scheduler** (internal/tasks/handlers/search_scheduler.go):
   - Receives candidate ID in search scheduler payload
   - Performs database lookup with lock to prevent concurrent searches
   - Determines user's tier (free, mid, top)
   - Calculates search timing and page ranges
   - Creates NATS message with search criteria for each page
   - Maintains search state in candidates table (last_page, last_search_at)
   - Schedules next search based on tier-specific intervals:
     * Free: 72 hours
     * Mid: 24 hours
     * Top: 6 hours

3. **Search Term Construction**:
   - Skills are extracted from candidate_skills table with their preference ratings
   - Simplified Search Strategy:
     * Primary search terms: Skills with preference = 5 ("Must Have")
     * Secondary search terms: Skills with preference = 4 ("Preferred")
     * If no primary terms exist, use secondary terms
     * All other skills are used only for scoring, not for search

4. **Search Execution** (internal/handlers/search.go):
   - Receives search input messages via NATS
   - Executes search using JobProvider interface
   - Collects job listings from each page
   - Stores listing results in database
   - Job Scoring System (Simplified):
     * +10 points for each "Must Have" skill match
     * +5 points for each "Preferred" skill match
     * +1 point for each "No Preference" skill match
     * -3 points for each "Avoid" skill that appears
     * Exclude jobs with "Must Avoid" skills

5. **Progress Tracking**:
   - Uses KV store to track search progress in real-time
   - Updates include:
     * Total jobs found
     * Jobs analyzed
     * Good matches found
   - Client polls progress endpoint until completion

6. **Result Management**:
   - Results are limited based on tier:
     * Free: 3 pages (approx. 30 jobs)
     * Mid: 50 pages (approx. 500 jobs)
     * Top: 200 pages (approx. 2000 jobs)
   - Results are sorted by score descending

### Implementation Notes

For the MVP, we've eliminated the following complexities:
1. Complex skill relationship detection
2. Multiple separate search queries
3. Advanced skill centrality verification
4. Skill categories and grouping
5. Complex LLM analysis for job requirements

This simplified approach focuses on core functionality while still respecting user preferences. It aligns well with the existing JobProvider interface and provides a solid foundation for future enhancements.

## Notes
- Following Go coding guidelines for implementation `.cursor/go-coding-guidelines.mdc`
- Using HTMX for dynamic UI updates (see `skills.html` for examples and `handlers.skills.go` for the handler examples)
- Implementing proper error handling and logging
- Ensuring proper test coverage using only the standard library and the got/want pattern
- Following TDD principles as outlined in guidelines 

## References

### Testing Framework
- `/internal/handlers/common_test.go` - Common test utilities and mock implementations
  * `testEnv` struct for NATS testing environment
  * Mock LLM implementation
  * NATS server setup and teardown
- `/internal/handlers/test_harness.go` - Generic test harness for NATS messaging
  * `SendAndReceiveMessage` for end-to-end message testing
  * JSON marshaling/unmarshaling helpers
- `/cmd/web/handlers_test.go` - HTTP handler testing utilities
  * Test server setup
  * Authentication helpers
  * Form submission helpers

### Search Implementation
- `/internal/handlers/search.go` - Main search handler, NATS message processing
- `/internal/handlers/search/` - Search package directory
  - `README.md` - Search provider implementation details. Always read this first.
  - `types.go` - Search criteria and result types
  - `provider.go` - Job provider interface
  - `job.go` - Job-related functionality
  - `registry.go` - Provider registry
  - `store.go` - Data storage interface
  - `errors.go` - Error definitions
  - `company.go` - Company-related functionality

### Database Schema
- `/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` 
  - Users table
  - Candidates table
  - Candidate skills table
  - Candidate preferences table
  - Onboarding progress tracking
- `/assets/migrations/000004_linkedin_jobs.up.sql`
  - Jobs table
  - Companies table
  - Job applications table
  - Job skills table
  - Search-related tables

### UI Templates
- `/assets/templates/pages/profile-preferences.html` - Main preferences page
- `/assets/templates/partials/skills.html` - Skills management UI
  - Add/remove skills
  - Preference sliders
  - HTMX integration

### Key Tables for Search Feature
- `candidates` - User profile and tier information
- `candidate_skills` - User skills with preference values
- `jobs` - Job listings
- `job_skills` - Job required skills
- `job_searches` - Search history
- `search_results` - Search result storage 

## Question Priority Analysis

### MVP Blockers
1. Search Trigger
   - When to trigger searches during onboarding (ANSWERED: Button on profile-preferences.html)
   - How to handle search failures during onboarding
   - Basic error recovery strategy needed

2. Tier Management
   - Need tier tracking in candidates table (CRITICAL: affects core functionality)
   - Basic usage monitoring for tier limits

### Can Be Addressed Post-MVP

1. Search Behavior
   - How often should searches be refreshed? (Can start with manual refresh)
   - Advanced error handling strategies

2. Caching Questions
   - Cache duration/expiration policy (Can use default expiration initially)
   - Cache size limits (Can monitor and adjust)
   - Cache invalidation strategy (Can use time-based initially)
   - Partial results caching (Can cache complete results only)
   - Query hash generation (Can use simple hash of criteria)

3. Data Management
   - Long-term retention policy (Can retain all initially)
   - Stale listing handling (Can keep all listings initially)
   - Advanced tier usage metrics (Can track basic usage initially)

4. Result Management
   - Advanced sorting options (ANSWERED: Initial sort by assessment)
   - Advanced filtering options (Can add post-MVP)
   - Saved searches (Can implement later) 


