# Job Search Feature Overview

## Implementation Checklist

### 1. Database Updates (in 000002_create_users_and_onboarding_tables.up.sql)
- [ ] Add tier management to candidates table
  - [ ] Add tier enum type ('free', 'mid', 'top')
  - [ ] Add tier column with default 'free'
  - [ ] Add tier_updated_at timestamp for tracking last change

### 2. Search Button Integration
- [ ] Add "Ready to Search" button to profile-preferences.html
  - [ ] Create button HTML with HTMX attributes
  - [ ] Add loading state styles
  - [ ] Add hx-push-url to navigate to progress page
- [ ] Create endpoint handler for search initiation
  - [ ] Validate user has completed profile
  - [ ] Extract and categorize skills from candidate_skills
    - [ ] Identify must-have skills (rating = 5)
    - [ ] Collect preferred skills (rating = 4) for secondary search
  - [ ] Generate search messages based on tier limits
    - [ ] Create separate NATS message for each page to fetch
    - [ ] Include page number in search criteria
  - [ ] Redirect to search progress page

### 3. Search Progress Page
- [ ] Create search-progress.html template
  - [ ] Add progress indicators section
    - [ ] Total jobs found counter
    - [ ] Jobs analyzed counter
    - [ ] Good fit jobs counter
  - [ ] Add status message section
  - [ ] Add loading animations
  - [ ] Add placeholder for results view
- [ ] Set up Pusher integration (pusher is already initialized on /cmd/web/main.go)
  - [ ] Subscribe to search status channel
  - [ ] Subscribe to job analysis channel
  - [ ] Handle counter updates
  - [ ] Handle search completion event
- [ ] Implement progress tracking
  - [ ] Track jobs found count
  - [ ] Track analysis progress
  - [ ] Track matching jobs count
- [ ] Handle state transitions
  - [ ] Show progress view during search
  - [ ] Switch to results view when complete
  - [ ] Handle error states

### 4. Results View (Placeholder)
- [ ] Create basic results container
  - [ ] Add results count summary
  - [ ] Add placeholder for job listings
  - [ ] Add placeholder for filters
- [ ] Add minimal styling
- [ ] Prepare for future enhancement

### 5. Testing
- [ ] Unit tests
  - [ ] Test tier management
  - [ ] Test search criteria building
  - [ ] Test result limiting
- [ ] Integration tests
  - [ ] Test search flow
  - [ ] Test Pusher updates
  - [ ] Test error handling


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

### Search Process

Please refer to `SEARCH-ALGORITHM.md` for the search algorithm.

1. Search Initiation
   - Triggered by user clicking "Ready to Search" button on profile-preferences.html
   - Asynchronous processing via NATS messaging
   - UI updates via Pusher to show search progress (total jobs found, jobs analyzed, good fit jobs)
   - System extracts user's skills from candidate_skills table
     - Search algorithm is below
   - System generates multiple NATS messages based on tier limits
     - Each message contains search criteria for a specific page
     - Free tier: 3 pages
     - Mid tier: 50 pages
     - Top tier: 200 pages

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
1. Review and clarify questions about search triggers and timing
2. Implement tier tracking in candidates table
3. Add search trigger to appropriate onboarding step
4. Design and implement results view
5. Integrate LLM-based assessment pipeline:
   - Configure NATS topics and queues
   - Set up assessment result storage
   - Implement result categorization
   - Add confidence-based filtering

## Notes
- Following Go coding guidelines for implementation
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