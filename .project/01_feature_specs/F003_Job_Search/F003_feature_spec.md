# F003: Job Search

## Feature Overview
The Job Search feature enables Talent Rodeo to find relevant job opportunities from external sources. It integrates with job listing platforms, processes and stores job data, and makes it available for the matching engine. This feature serves as the foundation for providing users with current, high-quality job opportunities.

## Implementation Status
- LinkedIn Job Data Integration: **Fully Implemented**
- Skill-Based Search: **Fully Implemented**  
- Search Result Storage: **Fully Implemented**

## Detailed Specifications

### SF003.1: LinkedIn Job Data Integration

#### Description
The system integrates with LinkedIn job listings through ScrapingDog to access a wide range of job opportunities. This integration ensures access to current, relevant job postings from one of the largest professional networks.

#### Implementation Details
- Integration with ScrapingDog API
- Pagination support for comprehensive results
- Cache management to minimize unnecessary API calls
- Error handling for API limitations and failures
- Rate limiting compliance
- Data normalization and cleaning

#### Current Implementation
- Complete implementation of ScrapingDog provider in `/workspaces/talent-rodeo/internal/services/search/providers/scrapingdog/provider.go`: Includes full API integration with features for job search and job details retrieval
- Comprehensive store implementation in `/workspaces/talent-rodeo/internal/services/search/providers/scrapingdog/store.go`: Manages caching and persistence of search results
- Complete types definition in `/workspaces/talent-rodeo/internal/services/search/providers/scrapingdog/types.go`
- Extensive test coverage in `/workspaces/talent-rodeo/internal/services/search/providers/scrapingdog/provider_test.go` and `store_test.go`
- Robust error handling for API failures and rate limiting as seen in the provider implementation
- Web handler implementation in `/workspaces/talent-rodeo/cmd/web/handlers.search.go`: Contains startJobSearch function to initiate searches and manage progress
- Integration with NATS JetStream for asynchronous processing

### SF003.2: Skill-Based Search

#### Description
The system conducts targeted job searches based on the skills identified during the onboarding and assessment processes. This ensures that search results align with the candidate's technical capabilities and experience.

#### Implementation Details
- Keyword construction from user's skill profile
- Prioritization of "must-have" skills
- Support for skill synonyms and variations
- Structured query building for optimal results
- Search caching for performance optimization

#### Current Implementation
- Complete implementation in `/workspaces/talent-rodeo/cmd/web/handlers.search.go`: Contains comprehensive logic for skill extraction and prioritization
- Complete search handler implementation in `/workspaces/talent-rodeo/internal/handlers/search.go`: Provides SearchHandler for processing search requests with complete scoring algorithm
- Advanced search criteria construction in the constructSearchCriteria function
- Comprehensive job scoring logic in the scoreJobs function
- Fully implemented search scheduler in `/workspaces/talent-rodeo/internal/tasks/handlers/search_scheduler.go`: Manages scheduled searches with tier-based frequency control
- Complete test cases in `/workspaces/talent-rodeo/internal/handlers/search_test.go` and other test files
- Integration with NATS JetStream for message processing

### SF003.3: Search Result Storage

#### Description
The system saves job search results in the database, allowing for efficient retrieval, caching, and historical tracking. This creates a foundation for the matching process and user dashboard views.

#### Implementation Details
- Database storage for job listings
- Structured format for consistent processing
- Timestamp tracking for result freshness
- Indexing for efficient retrieval
- Deduplication handling

#### Current Implementation
- Complete database schema in `/workspaces/talent-rodeo/assets/migrations/000004_linkedin_jobs.up.sql`: Defines comprehensive tables for companies, jobs, skills, searches, and results
- Fully implemented storage functionality in `/workspaces/talent-rodeo/internal/services/search/providers/scrapingdog/store.go`: Provides complete caching and persistence features
- Complete data structures defined in `/workspaces/talent-rodeo/internal/services/search/types.go`
- Comprehensive database operations in the internal/database/ package for storing and retrieving job data
- Robust caching mechanism with TTL and validation
- Complete indexing strategy implemented in the database schema

## Dependencies
- ScrapingDog API access (configured in environment)
- LinkedIn data source
- NATS JetStream for message processing
- Database schema for job listings:
  - `job_searches` table in `/workspaces/talent-rodeo/assets/migrations/000004_linkedin_jobs.up.sql` - Stores search queries and parameters
  - `job_listings` table in `/workspaces/talent-rodeo/assets/migrations/000004_linkedin_jobs.up.sql` - Stores job listings from LinkedIn
  - `search_results` table in `/workspaces/talent-rodeo/assets/migrations/000004_linkedin_jobs.up.sql` - Maps searches to job results
  - Multiple supporting tables (companies, jobs, job_skills, etc.)
- User profile with skill data:
  - `candidate_skills` table in `/workspaces/talent-rodeo/assets/migrations/000002_create_users_and_onboarding_tables.up.sql` - Stores candidate skills for search queries

## Feature Dependencies
- Depends on F001_User_Onboarding for user authentication and profile data
- Depends on F002_Initial_Assessment_Call for extracted skills and preferences
- Required for F004_Job_Matching to have job listings for matching algorithms

## Future Enhancements
- Add additional job data sources beyond LinkedIn (Indeed, Glassdoor, etc.)
- Implement automated periodic searches to keep listings fresh
- Add natural language processing for more semantic search capabilities
- Develop job categorization and tagging system
- Implement company and industry-specific search optimizations
- Add geographic intelligence for location-based searches
- Create API endpoint for external systems to access job data

## User Experience Considerations
- Job search interface should be intuitive and responsive
- Search parameters should be easily configurable
- Results should load quickly with appropriate pagination
- Job previews should provide sufficient information for users to determine interest
- UI should always maintain shippable, working state throughout implementation

## Testing Requirements
- Unit tests for all search functions
- Integration tests with mock API responses
- Performance testing under various load conditions
- Data validation tests for job listing integrity
- UI component tests for search interfaces

Each subfeature must be implemented with complete UI components, comprehensive tests, and a clear path forward for future enhancements. Code should always remain in a shippable state, following our agile practices. 