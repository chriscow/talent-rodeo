# Phase 1: Core Infrastructure Implementation (Week 1-2)

**IMPORTANT**: This document details Phase 1 implementation. For overall project context, see `JOB_MATCHING_OVERVIEW.md`.
**IMPORTANT**: DO NOT DELETE ANYTHING IN THIS FILE. Mark items as completed (e.g. via ~~strike through~~) or move them to a "Deprecated/Completed" section to maintain a full record.

## Phase Goals
- Set up foundational database structure
- Implement core data models
- Create basic task handling infrastructure

## Database Schema Design

### Review & Planning
- [ ] Review existing migrations in:
  - `/assets/migrations`
  - `/internal/database`
- [ ] Document current schema structure
- [ ] Plan migration strategy

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
- [ ] Define `VectorGenerator` interface:
  ```go
  type VectorGenerator interface {
      JobToVector(ctx context.Context, job *LinkedinJob) ([]float64, error)
      CandidateToVector(ctx context.Context, candidate *Candidate) ([]float64, error)
      ComputeSimilarity(v1, v2 []float64) float64
  }
  ```

- [ ] Define `MatchScorer` interface:
  ```go
  type MatchScorer interface {
      ScoreMatch(ctx context.Context, job *LinkedinJob, candidate *Candidate) (*Match, error)
      ScoreRequirements(ctx context.Context, requirements []JobRequirement, candidate *Candidate) float64
      ScoreCultureFit(ctx context.Context, culture []CultureTrait, preferences []Preference) float64
  }
  ```

### Data Models
- [ ] Implement `LinkedinJob` model:
  ```go
  type LinkedinJob struct {
      ID              uuid.UUID
      Title           string
      Company         string
      Description     string
      Location        string
      SalaryRange     string
      Requirements    []string
      Qualifications  []string
      Source          string
      ExternalJobID   string
      PostingURL      string
      CreatedAt       time.Time
      UpdatedAt       time.Time
  }
  ```

- [ ] Implement `Candidate` model:
  ```go
  type Candidate struct {
      ID                uuid.UUID
      PublicName        string
      Headline          string
      Summary           string
      Skills           []string
      ExperienceYears   int
      PreferredLocations []string
      CreatedAt         time.Time
      UpdatedAt         time.Time
  }
  ```

- [ ] Implement supporting models:
  - [ ] `Match`
  - [ ] `Preference`
  - [ ] `Keyword`
  - [ ] `User`
  - [ ] `Role`

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

## Dependencies
- PostgreSQL
- Go 1.21+
- Required Go packages:
  ```
  github.com/lib/pq
  github.com/google/uuid
  github.com/gorilla/mux
  ```

## Progress Tracking

### Current Status
- [ ] Database Schema: Not Started
- [ ] Base Models: Not Started
- [ ] Task Handlers: Not Started

### Next Steps
1. [ ] Begin database schema implementation
2. [ ] Start model development
3. [ ] Initialize handler structure

## Updates
- Initial Document Creation: [Current Date] 