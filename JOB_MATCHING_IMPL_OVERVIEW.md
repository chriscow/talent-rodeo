# Job Matching System - Implementation Overview

**IMPORTANT**: This document provides a high-level overview of the implementation plan. Detailed phase implementations can be found in their respective documents:
**IMPORTANT**: DO NOT DELETE ANYTHING IN THIS FILE. Mark items as completed (e.g. via ~~strike through~~) or move them to a "Deprecated/Completed" section to maintain a full record.

- `JOB_MATCHING_IMPL_PHASE_1.md` - Core Infrastructure & Distribution
- `JOB_MATCHING_IMPL_PHASE_2.md` - Job Ingestion & Search
- `JOB_MATCHING_IMPL_PHASE_3.md` - Job Analysis & Matching
- `JOB_MATCHING_IMPL_PHASE_4.md` - Optimization and Testing
- `JOB_MATCHING_IMPL_PHASE_5.md` - Load Distribution

## System Purpose
The system automates the job search process that a human recruiter or job seeker would perform:

1. Search for jobs using relevant keywords (titles, skills, preferences)
2. Analyze job descriptions for compatibility
3. Refine searches based on results
4. Save and track promising matches
5. Leverage AI to make intelligent matching decisions

## Implementation Guidelines
- Keep it simple - this is an MVP
- Avoid premature optimization
- Minimize dependencies
- Focus on user value
- Add complexity only when needed
- Design for future distributed processing for ingestion and matching

## Project Structure
```
internal/
├── types/              # Shared core domain types
│   utils/              # Shared utilities
│
├── ingestion/          # Job listing ingestion and search
│   ├── providers/      # Job listing provider implementations
│   │   ├── linkedin/   # LinkedIn specific implementation
│   │   └── registry/   # Provider registry and factory
│   ├── search/         # Search functionality
│   │   ├── keywords/   # Keyword management
│   │   └── cache/      # Search result caching
│   └── store/          # Ingestion data persistence
│
└── matching/           # Job analysis and candidate matching
    ├── analysis/       # Job description analysis
    │   ├── skills/     # Skills extraction and matching
    │   ├── culture/    # Culture and preferences analysis
    │   └── requirements/ # Job requirements parsing
    ├── candidates/     # Candidate profile management
    ├── scoring/        # Match scoring and ranking
    └── store/          # Matching data persistence
```

## Implementation Phases Overview

### Phase 1: Core Infrastructure (Week 1–2)
- Database Schema Design
- Message Queue Setup (Watermill + NATS) (we are using embedded NATS for now but will use a more robust solution in the future)
- Base Models Implementation
- Distribution Framework

### Phase 2: Job Ingestion (Week 2–3)
- Provider Integration Framework
- Keyword-based Search
- Search Result Caching from job listing providers
- Intelligent Search Refinement

### Phase 3: Job Analysis & Matching (Week 3–4)
- Job Description Analysis via LLM
- Candidate Profile Matching
- Match Scoring System
- Match Result Management

### Phase 4: Optimization and Testing (Week 4–5)
- Performance Optimization
- Testing
- Documentation

### Phase 5: Load Distribution (Week 5–6)
- Distributed Search Processing
- Distributed Match Processing
- Enhanced Monitoring

## Database Structure Overview

### Core Tables
- `users`: Basic account information and roles (candidate, recruiter, admin)
- `job_searches`: Search queries and metadata
- `providers`: Registered job listing providers (linkedin, indeed, etc.)
- `job_listings`: Core job data common across all providers
  ```sql
  CREATE TABLE job_listings (
    id UUID PRIMARY KEY,
    provider_id UUID REFERENCES providers(id),
    provider_job_id TEXT,           -- Original ID from provider
    title TEXT NOT NULL,
    company TEXT NOT NULL,
    location TEXT,
    is_remote BOOLEAN,
    description TEXT,
    url TEXT NOT NULL,
    posted_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(provider_id, provider_job_id)
  );
  ```

### Provider-Specific Tables
- `linkedin_jobs`: LinkedIn-specific fields
  ```sql
  CREATE TABLE linkedin_jobs (
    job_id UUID PRIMARY KEY REFERENCES job_listings(id),
    experience_level TEXT,
    workplace_type TEXT,
    employment_type TEXT,
    company_linkedin_id TEXT,
    applicant_count INT,
    raw_data JSONB,  -- Store complete raw response
    FOREIGN KEY (job_id) REFERENCES job_listings(id)
  );
  ```
- `indeed_jobs`: Indeed-specific fields
  ```sql
  CREATE TABLE indeed_jobs (
    job_id UUID PRIMARY KEY REFERENCES job_listings(id),
    job_type TEXT,
    salary_range TEXT,
    indeed_company_rating FLOAT,
    raw_data JSONB,
    FOREIGN KEY (job_id) REFERENCES job_listings(id)
  );
  ```

### Search & Cache Tables
- `search_cache`: Cached search results
  ```sql
  CREATE TABLE search_cache (
    id UUID PRIMARY KEY,
    query_hash TEXT NOT NULL,       -- Hash of search parameters
    provider_id UUID,
    results JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP,
    UNIQUE(query_hash, provider_id)
  );
  ```
- `job_keywords`: Extracted and normalized keywords
  ```sql
  CREATE TABLE job_keywords (
    id UUID PRIMARY KEY,
    job_id UUID REFERENCES job_listings(id),
    keyword TEXT NOT NULL,
    category TEXT,    -- skill, title, culture, etc.
    confidence FLOAT, -- AI confidence in extraction
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(job_id, keyword, category)
  );
  ```

### Candidate & Matching Tables
- `candidates`: Candidate profiles
- `candidate_preferences`: Search and job preferences
- `job_matches`: Computed match results
- `job_analysis`: Analyzed job data

### Benefits of this Structure:
1. Clean separation of common vs provider-specific data
2. Easy to add new providers without schema changes to core tables
3. Efficient querying of common fields
4. Raw provider data preserved for future analysis
5. Normalized keywords for cross-provider searching
6. Provider-agnostic matching system

## Security and Privacy Overview
- HTTPS for all connections
- Password security with bcrypt
- PostgreSQL role-based access control
- Separated sensitive data storage
- Future encryption enhancements planned

## Progress Tracking

### Current Status
- [ ] Phase 1: Not Started
- [ ] Phase 2: Not Started
- [ ] Phase 3: Not Started
- [ ] Phase 4: Not Started
- [ ] Phase 5: Not Started

### Completed
- [x] Initial design document
- [x] Project structure definition
- [x] Phase implementation plans

### Next Steps
1. [ ] Begin Phase 1 implementation
2. [ ] Review and refine phase details
3. [ ] Set up development environment

## Dependencies Overview
- Go 1.21+
- PostgreSQL 14+
- Redis 6+
- NATS with Jetstream
- Required Go packages (see individual phase documents for details)

## Updates
- Initial Document Creation: [Current Date]
- Split implementation details into phase-specific documents 