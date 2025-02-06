# Phase 1: Core Infrastructure Implementation (Week 1-2)

**IMPORTANT**: This document details Phase 1 implementation. For overall project context, see `JOB_MATCHING_OVERVIEW.md`.
**IMPORTANT**: DO NOT DELETE ANYTHING IN THIS FILE. Mark items as completed (e.g. via ~~strike through~~) or move them to a "Deprecated/Completed" section to maintain a full record.

## Phase Goals
- ~~Set up foundational database structure~~
- ~~Implement core data models~~
- ~~Create basic task handling infrastructure~~

## Implementation Notes and Clarifications

### Project Structure
- ~~Using `/internal/jobmatch` as the main package~~
- ~~Reference implementations:~~
  - ~~`/internal/scrapingdog` - Example job provider implementation (will be removed)~~
  - ~~`/internal/tasks` - Task processing patterns to follow~~

### Provider Strategy
- ~~Design for multiple job listing providers~~
- ~~ScrapingDog is first implementation but not the only one~~
- ~~Provider interface should be generic enough for future providers~~

### Current Progress (as of Phase 1)

#### Completed Items
1. ~~Implemented core processor infrastructure:~~
   - ~~Job processing request/response handling~~
   - ~~State management and transitions~~
   - ~~Concurrent processing support~~
   - ~~Cancellation handling~~
   - ~~Status tracking~~

2. ~~Provider Interface Implementation:~~
   - ~~Created `types.JobProvider` interface~~
   - ~~Implemented ScrapingDog provider~~
   - ~~Added provider validation~~
   - ~~Set up provider registration system~~

3. ~~Testing Infrastructure:~~
   - ~~Comprehensive test suite for processor~~
   - ~~Mock provider implementation~~
   - ~~Concurrent processing tests~~
   - ~~State transition tests~~

4. ~~Job Analysis (Phase 1):~~
   - ~~Keyword extraction with LLM and traditional fallback~~
   - ~~Requirement parsing with experience detection~~
   - ~~Job data validation~~
   - ~~Comprehensive test suite implemented~~
   - ~~LLM integration with error handling~~
   - ~~Parallel processing of analysis tasks~~

5. **Job Handler Setup:**
   - [x] Created base handler structure
   - [x] Implemented job search endpoints
   - [x] Created job ingestion pipeline
   - [x] Created analysis handler structure
   - [x] Implemented keyword extraction endpoint
   - [x] Created requirement parsing endpoint
   - [x] Set up match computation handler
   - [x] Implemented match status tracking
   - [ ] Created match update notifications

#### Pending Implementation
1. Job Matching (Phase 1):
   - [x] Match scoring system
   - [x] Candidate-job compatibility
   - [x] Location/remote preferences
   - [x] Test implementation needed

2. Job Validation (Phase 1):
   - [x] Data quality checks
   - [x] Requirement validation
   - [x] Duplicate detection
   - [x] Test implementation needed

3. Match Update Notifications:
   - [ ] WebSocket using Pusher notification system
   - [ ] Real-time status updates
   - [ ] Match score notifications
   - [ ] Test implementation needed

### Directory Structure
```
internal/jobmatch/
├── types/          # Common types and interfaces
├── providers/      # Job listing provider implementations
│   └── scrapingdog/ # ScrapingDog implementation
├── processor/      # Core job processing logic
│   ├── analyzer.go       # Traditional keyword/requirement analysis
│   ├── analyzer_llm.go   # LLM-based analysis implementation
│   ├── analyzer_test.go  # Traditional analyzer tests
│   ├── analyzer_llm_test.go # LLM analyzer tests
│   └── processor.go      # Core processing logic
└── handlers/       # HTTP/API handlers (pending)
```

### Implementation Notes

1. Debug Logging:
   - ~~Verbose logging implemented throughout processor~~
   - ~~Provider operations logged~~
   - ~~State transitions tracked~~
   - ~~Error contexts preserved~~
   - ~~Analysis stages logged with request IDs~~

2. Error Handling:
   - ~~Proper error wrapping with %w~~
   - ~~Context preservation~~
   - ~~Graceful failure handling~~
   - ~~Clear error messages~~
   - ~~LLM fallback mechanisms~~

3. Testing Strategy:
   - ~~Test-driven development~~
   - ~~Mock implementations~~
   - ~~Concurrent testing~~
   - ~~Edge case coverage~~
   - ~~LLM mock responses~~

4. Documentation:
   - [x] Package READMEs
   - [x] Interface documentation
   - [x] Implementation guides
   - [x] Testing documentation
   - [ ] API documentation

5. Code Organization:
   - [x] Clean package structure
   - [x] Clear interfaces
   - [x] Proper abstraction layers
   - [x] Dependency management
   - [x] Configuration handling

### Phase 5 Preparation
Current implementation includes comments and preparations for Phase 5:
- Message queue integration points identified
- Distributed processing considerations documented
- Async operation support planned
- Resource allocation framework ready

### LLM-Based Job Analysis Implementation

#### Overview
The job analysis system has been implemented with LLM capabilities providing sophisticated analysis of job postings:
1. ~~Extract meaningful insights beyond predefined keywords~~
2. ~~Better understand job requirements and their relative importance~~
3. ~~Normalize job descriptions for better matching~~

#### Implementation Details

1. ~~LLM Integration:~~
   ```go
   type LLMAnalyzer struct {
       provider interface {
           GenerateContent(ctx context.Context, req minds.Request) (minds.Response, error)
       }
       schemas struct {
           normalize    *minds.ResponseSchema
           requirements *minds.ResponseSchema
       }
       logger     *log.Logger
       fallback   Analyzer // Traditional analyzer for fallback
       retryCount int
   }
   ```

2. ~~Analysis Pipeline:~~
   - Stage 1: Job Description Normalization
     - Remove marketing fluff
     - Standardize format
     - Extract core job essence
   
   - Stage 2: Comprehensive Analysis
     - Job role classification
     - Key differentiators
     - Company culture indicators
     - Work environment details
   
   - Stage 3: Requirements Analysis
     - Must-have skills/experience
     - Nice-to-have skills
     - Equivalent skills/technologies
     - Experience level requirements

3. ~~Response Schemas:~~
   ```go
   type NormalizedJob struct {
       CoreDescription    string   `json:"core_description"`
       KeyDifferentiators []string `json:"key_differentiators"`
       WorkEnvironment    string   `json:"work_environment"`
   }

   type RequirementAnalysis struct {
       MustHave []struct {
           Skill        string   `json:"skill"`
           Weight       float64  `json:"weight"`
           Alternatives []string `json:"alternatives"`
       } `json:"must_have"`
       NiceToHave []struct {
           Skill  string  `json:"skill"`
           Weight float64 `json:"weight"`
       } `json:"nice_to_have"`
   }
   ```

4. ~~Parallel Processing:~~
   - Implemented manual parallelization using channels
   - Example workflow:
     ```go
     // Create parallel analysis tasks
     normalizedCh := make(chan result)
     requirementsCh := make(chan result)

     // Run analyses in parallel
     go func() {
         normalized, err := a.normalizeJob(ctx, text)
         normalizedCh <- result{data: normalized, err: err}
     }()

     go func() {
         requirements, err := a.analyzeRequirements(ctx, text)
         requirementsCh <- result{data: requirements, err: err}
     }()
     ```

5. ~~Error Handling and Validation:~~
   - Implemented retry mechanisms for LLM calls
   - Validate LLM outputs against schemas
   - Fall back to traditional keyword extraction if LLM fails
   - Proper error wrapping and context preservation

#### Testing Strategy

1. ~~Unit Tests:~~
   - ~~Mock LLM responses~~
   - ~~Test schema validation~~
   - ~~Test error handling~~
   - ~~Test fallback mechanisms~~

2. ~~Integration Tests:~~
   - ~~Test with mock LLM providers~~
   - ~~Verify parallel processing~~
   - ~~Validate error handling~~

3. ~~Performance Tests:~~
   - ~~Test concurrent processing~~
   - ~~Verify fallback performance~~

#### Next Steps

1. Job Matching Implementation:
   - Design match scoring system
   - Implement candidate-job compatibility
   - Add location/remote preference handling
   - Create comprehensive test suite

2. Job Validation Enhancement:
   - Implement data quality checks
   - Add requirement validation
   - Create duplicate detection system
   - Add validation test suite

3. Phase 5 Preparation:
   - Design caching system
   - Plan message queue integration
   - Prepare for distributed processing
   - Design metrics collection system

## Database Schema Design

### Review & Planning
- [ ] Review existing migrations in:
  - `/assets/migrations`
  - `/internal/database`
- [ ] Document current schema structure
- [ ] Migration Plan not necessary for this phase. Just update the schema files as needed to reflect the new changes.

### Core Tables Implementation
- [ ] Create `linkedin_jobs` table:
  ```sql
  CREATE TABLE linkedin_jobs (
      id UUID PRIMARY KEY,
      title TEXT NOT NULL,
      company TEXT NOT NULL,
      description TEXT,
      location TEXT,
      salary_range TEXT,
      requirements TEXT[],
      qualifications TEXT[],
      source TEXT DEFAULT 'linkedin',
      external_job_id TEXT,
      posting_url TEXT,
      created_at TIMESTAMPTZ DEFAULT current_timestamp,
      updated_at TIMESTAMPTZ DEFAULT current_timestamp,
      UNIQUE(external_job_id)
  );
  ```

- [ ] Create `candidates` table:
  ```sql
  CREATE TABLE candidates (
      id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
      public_name TEXT,
      headline TEXT,
      summary TEXT,
      skills TEXT[],
      experience_years INT,
      preferred_locations TEXT[],
      created_at TIMESTAMPTZ DEFAULT current_timestamp,
      updated_at TIMESTAMPTZ DEFAULT current_timestamp
  );
  ```

- [ ] Create `candidate_private_info` table:
  ```sql
  CREATE TABLE candidate_private_info (
      candidate_id UUID REFERENCES candidates(id) PRIMARY KEY,
      email TEXT,
      phone TEXT,
      full_name TEXT,
      address TEXT,
      created_at TIMESTAMPTZ DEFAULT current_timestamp,
      updated_at TIMESTAMPTZ DEFAULT current_timestamp
  );
  ```

- [ ] Create `users` and authentication tables:
  ```sql
  CREATE TABLE users (
      id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
      email TEXT UNIQUE NOT NULL,
      password_hash TEXT NOT NULL,
      role TEXT NOT NULL,
      created_at TIMESTAMPTZ DEFAULT current_timestamp,
      updated_at TIMESTAMPTZ DEFAULT current_timestamp
  );

  CREATE TABLE user_roles (
      id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
      name TEXT UNIQUE NOT NULL,
      permissions TEXT[],
      created_at TIMESTAMPTZ DEFAULT current_timestamp
  );
  ```

### Supporting Tables
- [ ] Create analysis-related tables:
  ```sql
  CREATE TABLE job_keywords (
      job_id UUID REFERENCES linkedin_jobs(id),
      keyword TEXT,
      weight FLOAT,
      category TEXT,
      created_at TIMESTAMPTZ DEFAULT current_timestamp,
      PRIMARY KEY (job_id, keyword)
  );

  CREATE TABLE job_requirements (
      job_id UUID REFERENCES linkedin_jobs(id),
      requirement TEXT,
      is_required BOOLEAN,
      category TEXT,
      created_at TIMESTAMPTZ DEFAULT current_timestamp,
      PRIMARY KEY (job_id, requirement)
  );
  ```

## Base Models Implementation

### Core Interfaces
- ~~[x] Define provider interfaces:~~
  ```go
  // JobProvider interface - Completed
  type JobProvider interface {
      SearchJobs(ctx context.Context, criteria SearchCriteria) ([]Job, error)
      GetJobDetails(ctx context.Context, jobID string) (*JobDetail, error)
      Name() string
      Validate(ctx context.Context) error
  }
  ```

- [ ] Define analysis interfaces:
  ```go
  // Analyzer interface - To be implemented
  type Analyzer interface {
      ExtractKeywords(ctx context.Context, text string) ([]Keyword, error)
      ParseRequirements(ctx context.Context, description string) ([]Requirement, error)
      ValidateJobData(ctx context.Context, job *Job) (*ValidationResult, error)
  }
  ```

### Data Models
- ~~[x] Implement core job models:~~
  ```go
  // Job model - Completed
  type Job struct {
      ID            string
      Title         string
      Company       string
      Location      string
      PostedDate    time.Time
      Description   string
      URL           string
      Provider      string
      ProviderJobID string
  }

  // JobDetail model - Completed
  type JobDetail struct {
      Job
      FullDescription string
      Requirements    []string
      Qualifications  []string
      Benefits        []string
      Skills          []string
      SalaryInfo      *SalaryRange
      EmploymentType  string
      WorkplaceType   string
      Industry        string
      CompanyInfo     *CompanyInfo
  }

  // SearchCriteria model - Completed
  type SearchCriteria struct {
      Keywords    []string
      Location    string
      DatePosted  time.Time
      JobType     []string
      Experience  []string
      Industry    []string
      Skills      []string
      Salary      *SalaryRange
      PageSize    int
      PageNumber  int
  }
  ```

- [ ] Implement analysis models:
  ```go
  // Analysis models - To be implemented
  type Keyword struct {
      Text       string
      Weight     float64
      Category   string
      Confidence float64
  }

  type Requirement struct {
      Text        string
      IsRequired  bool
      Category    string
      Priority    int
      Skills      []string
  }

  type ValidationResult struct {
      IsValid      bool
      Score        float64
      Issues       []string
      Suggestions  []string
  }
  ```

### Implementation Status

1. Core Infrastructure:
   - [x] Database schema and migrations
   - [x] Core data models
   - [x] Task handling infrastructure
   - [x] Provider interface and registry
   - [x] Job processing pipeline

2. Job Analysis:
   - [x] LLM integration with Gemini
   - [x] Keyword extraction
   - [x] Requirement parsing
   - [x] Job validation
   - [x] Test coverage

3. Job Matching:
   - [x] Match scoring system
   - [x] Candidate compatibility
   - [x] Location preferences
   - [x] Test implementation

4. API Handlers:
   - [x] Job search endpoints
   - [x] Job analysis endpoints
   - [x] Match computation
   - [ ] Real-time notifications

### Next Steps

1. Real-time Updates:
   - Implement WebSocket using Pusher
   - Add status update notifications
   - Create match score notifications
   - Add test coverage

2. Documentation:
   - Complete API documentation
   - Update schema documentation
   - Add usage examples

3. Phase 5 Preparation:
   - Message queue integration points
   - Distributed processing design
   - Async operation support
   - Resource allocation framework

### Dependencies Update
```go
require (
    github.com/chriscow/minds v0.1.0
    github.com/dgraph-io/ristretto v0.1.1  // for caching
    github.com/alexedwards/flow v1.0.0     // for routing
    github.com/fatih/color v1.13.0         // for debug logging
)
```

## Task Handler Setup

### Job Search & Ingestion
- [ ] Create base handler structure:
  ```go
  type JobHandler struct {
      store     *store.Store
      processor *processor.Processor
  }
  ```
- [ ] Implement job search endpoints
- [ ] Create job ingestion pipeline

### Job Analysis
- [ ] Create analysis handler structure
- [ ] Implement keyword extraction endpoint
- [ ] Create requirement parsing endpoint

### Match Processing
- [ ] Set up match computation handler
- [ ] Implement match status tracking
- [ ] Create match update notifications

## Testing Plan

### Unit Tests
- [ ] Database models
- [ ] Interface implementations
- [ ] Handler functions

### Integration Tests
- [ ] Database operations
- [ ] Handler endpoints
- [ ] Analysis pipeline

## Documentation

### API Documentation
- [ ] Document all endpoints
- [ ] Include request/response examples
- [ ] Document error cases

### Implementation Notes
- [ ] Document design decisions
- [ ] Include setup instructions
- [ ] Add configuration details
