# Job Search Feature Overview

## Implementation Checklist

### 1. Database Updates (in 000002_create_users_and_onboarding_tables.up.sql)
- [ ] Add tier management to candidates table
  - [ ] Add tier enum type ('free', 'mid', 'top')
  - [ ] Add tier column with default 'free'
  - [ ] Add tier_updated_at timestamp for tracking last change
- [ ] Add last_search_at timestamp column to track periodic search timing

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
- [ ] Unit tests
  - [ ] Test tier management
  - [x] Test search criteria building
  - [x] Test result limiting
  - [ ] Test periodic search scheduling
- [ ] Integration tests
  - [x] Test search flow
  - [x] Test KV store updates
  - [x] Test error handling

### 6. Periodic Search Implementation (New)
- [ ] Integrate go-co-op/gocron
  - [ ] Create scheduler in application struct
  - [ ] Configure tier-based intervals:
    - Free: Every three days
    - Basic: Daily
    - Pro: Four times daily
  - [ ] Implement background worker for periodic searches
  - [ ] Add graceful shutdown handling
  - [ ] Add logging and error handling

### 7. Search Progress KV Store (New)
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
   - [ ] Added gocron scheduler for periodic searches

### Testing Requirements (New)

#### Available Test Utilities
1. `cmd/web/handlers_test.go`:
   - Provides `testServer` struct for HTTP handler testing
   - Includes utilities for creating test users and authenticated requests
   - Handles session management and CSRF tokens
   - Supports both GET and POST request testing

2. `internal/handlers/common_test.go`:
   - Provides `testEnv` struct for integration testing
   - Sets up test NATS server with JetStream
   - Includes mock LLM implementation
   - Manages test database connections
   - Handles cleanup of test resources

3. `internal/handlers/test_harness.go`:
   - Provides `SendAndReceiveMessage` generic function for NATS testing
   - Supports publishing and receiving messages in tests
   - Handles consumer creation and message acknowledgment
   - Includes debug logging for test troubleshooting

4. `internal/database/database_test.go`:
   - Provides `SetupTestDB` function for database testing
   - Manages test database lifecycle

#### Test Implementation Plan
- [ ] Unit Tests
  - [ ] Test `startJobSearch` handler
    - [ ] Test unauthenticated user redirect
    - [ ] Test invalid candidate ID error
    - [ ] Test missing skills error
    - [ ] Test successful message publishing
    - [ ] Test JSON response format
  - [ ] Test skill categorization
    - [ ] Test must-have skills extraction (rating = 5)
    - [ ] Test preferred skills extraction (rating = 4)
    - [ ] Test empty skills handling
  - [ ] Test NATS message publishing
    - [ ] Test message format
    - [ ] Test page number increments
    - [ ] Test error handling

- [ ] Integration Tests
  - [ ] Test full search flow
    - [ ] Test HTMX button interaction
    - [ ] Test URL push to progress page
    - [ ] Test loading indicator
  - [ ] Test NATS message handling
    - [ ] Test message receipt by search handler
    - [ ] Test message processing
    - [ ] Test error propagation

### Next Steps
1. Implement test suite for recent changes
2. Complete database tier management implementation
3. Proceed with search progress page implementation
4. Set up Pusher integration for real-time updates

## Current State Analysis

### Architecture Overview
- NATS-based message queue system for job search processing
  - Search input handler (`rodeo.search.input`)
  - Search results handler (`rodeo.search.result`)
- Provider-based architecture allowing multiple job search sources
- Comprehensive database schema for jobs, companies, and search-related data

### Existing Components
1. Search Infrastructure
   - Complete NATS-based search processing system
   - Provider-agnostic search criteria implementation
   - Search result handling and storage

2. UI Components
   - Implemented skill preference management
     - Add/remove skills
     - Preference sliders for each skill
     - HTMX-based dynamic updates
   - Salary filter implementation

### Search Implementation
- Provider-agnostic search criteria supporting:
  - Keyword-based search using candidate skills (from `candidate_skills` table)
    - All skills are used in search with their preference values (1-5)
    - Skills have preference values to weight search importance:
      * 5: Must Have skills
      * 4: Preferred skills
      * 3: No Preference skills
      * 2: Avoid skills
      * 1: Must Avoid skills

The following filters are available but we are not using them yet:
  - Location filtering
  - Date posted filtering
  - Job type filtering
  - Experience level filtering
  - Industry filtering
  - Skills matching
  - Salary range filtering

### Search Process Updates

1. Search Initiation
   - ~~Triggered only by user clicking "Ready to Search" button~~ Now supports:
     * Manual trigger via "Ready to Search" button
     * Automatic periodic searches based on user tier
     * One search at a time per user enforced by KV store
   - Progress tracking via user-specific KV buckets
   - Automatic cleanup of progress data after completion
   - Tier-based search frequency:
     * Free: Every three days
     * Basic: Daily searches
     * Pro: Four times daily searches

2. Search Execution and Assessment
   - System uses a multi-phase assessment approach:
     a. Initial Keyword-Based Search:
        - Uses Provider.SearchJobs with tiered page limits
        - Filters jobs based on must-have skills
        - Performs initial scoring based on skill preferences:
          * +10 points: Must Have skill matches (rating 5)
          * +5 points: Preferred skill matches (rating 4)
          * +1 point: No Preference skill matches (rating 3)
          * -3 points: Avoid skill matches (rating 2)
          * Immediate exclusion: Must Avoid skills (rating 1)
     
     b. Detailed LLM Analysis (via assessment.go):
        - For each promising job:
          * Requirements Handler analyzes job description to extract:
            - Must-have skills with years/proficiency
            - Nice-to-have skills
            - Experience requirements
            - Education requirements
          * Assessment Handler performs deep matching:
            - Validates if must-have skills are truly primary
            - Calculates skill usage percentages
            - Generates MatchAssessment with:
              * Overall match score (0-1)
              * Confidence level
              * Detailed skills match analysis
              * Experience alignment
              * Compensation fit
              * Specific recommendations

3. Result Processing and Ranking
   - Jobs are categorized into:
     * Perfect Matches: High scores with validated primary skills
     * Strong Matches: Good scores with must-have skills present
     * Potential Matches: Lower scores but meeting basic criteria
   - Results stored in match_assessments table with:
     * Candidate ID
     * Job ID
     * Score
     * Confidence
     * Detailed analysis
   - Results limited by tier:
     * Free tier: First 3 pages analyzed
     * Mid tier: Up to 50 pages
     * Top tier: Up to 200 pages

### Implementation Notes

1. LLM Integration
   - Requirements Handler (requirements.go):
     * Extracts structured requirements from job descriptions
     * Identifies skill levels and years needed
     * Provides context for skill usage
   
   - Assessment Handler (assessment.go):
     * Performs comprehensive candidate-job matching
     * Generates detailed match assessments
     * Stores results in database
   
   - Experience Handler (experience.go):
     * Analyzes candidate experience
     * Maps skills to roles and duration
     * Validates experience claims

2. Scoring System Implementation
   - Initial score from keyword matching
   - Enhanced by LLM analysis:
     * Skill relevance validation
     * Experience alignment
     * Role suitability
   - Final score combines both metrics

### Questions Requiring Clarification

1. Search Implementation
   - How should we handle the case where a "must have" skill search returns no results?
   - Should we fall back to "preferred" skills search automatically?
   - Do we need to implement different search strategies for different providers?
   - How do we handle rate limiting from the job search provider?
   - Should we implement parallel fetching of job details or keep it sequential?

2. Performance Considerations
   - What is the optimal batch size for job detail fetching?
   - Should we implement a delay between API calls to avoid rate limiting?
   - How do we handle provider timeouts during multi-page searches?
   - Should we cache job details separately from search results?

3. Data Management
   - How long should we retain search results?
   - How should we handle stale job listings?
   - What metrics should we track for tier usage?
   - How do we handle failed job detail fetches?
   - Should we retry failed searches automatically?

4. LLM Assessment Integration
   - How should we handle conflicting assessments between keyword matching and LLM analysis?
   - What confidence threshold should trigger a secondary review?
   - Should we implement a feedback loop to improve LLM assessments?
   - How do we handle assessment timeouts or failures?
   - What is the optimal batch size for LLM analysis?

## Next Steps
1. Implement gocron scheduler
2. Implement tier tracking in candidates table
3. Add periodic search scheduling
4. Enhance results view with filtering
5. Integrate LLM-based assessment pipeline:
   - Configure NATS topics and queues
   - Set up assessment result storage
   - Implement result categorization
   - Add confidence-based filtering

## Notes
- Following Go coding guidelines for implementation `.cursor/go-coding-guidelines.mdc`
- Using HTMX for dynamic UI updates (see `skills.html` for examples and `handlers.skills.go` for the handler examples)
- Implementing proper error handling and logging
- Ensuring proper test coverage using only the standard library and the got/want pattern
- Following TDD principles as outlined in guidelines 

## References

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