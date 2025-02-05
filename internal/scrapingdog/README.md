# Scrapingdog Package

This package provides integration with the ScrapingDog API to fetch and process LinkedIn job listings, enabling sophisticated job matching and analysis capabilities.

## Overview

The scrapingdog package, particularly its LinkedIn submodule, implements comprehensive job search and analysis functionality, including job listing retrieval, requirement extraction, and job matching capabilities.

## Directories

### linkedin/
LinkedIn-specific functionality including:
- Job search and retrieval
- Job detail processing
- Requirement analysis
- Job matching algorithms
- Search management

## Files

### linkedin/jobs.go
Core job search functionality:
- Job listing retrieval
- Data structure definitions
- API integration
- Keyword extraction
- Response parsing

### linkedin/job-match.go
Job matching functionality:
- Candidate-job matching
- Skill comparison
- Requirement analysis
- Score calculation
- Match ranking

### linkedin/job-description.go
Description processing:
- Content extraction
- HTML parsing
- Text summarization
- Requirement identification
- Format standardization

### linkedin/search-manager.go
Search orchestration:
- Search coordination
- Result pagination
- Query optimization
- Cache management
- Rate limiting

### linkedin/job-store.go
Data persistence:
- Job storage
- Cache management
- Data updates
- Query handling
- State management

### linkedin/options.go
Search configuration:
- Option definitions
- Parameter validation
- Default settings
- Configuration management
- Search customization

## Key Features

1. Job Search
   - Keyword-based search
   - Location filtering
   - Experience level filtering
   - Company filtering
   - Work type filtering

2. Data Processing
   - Job detail extraction
   - Requirement analysis
   - Company information
   - Recruiter details
   - Similar job suggestions

3. Job Matching
   - Skill matching
   - Requirement analysis
   - Location matching
   - Experience matching
   - Culture fit assessment

## Usage

Initialize job search:
```go
listings, err := linkedin.SearchJobs(ctx, geoID, keyword, linkedin.WithPageLimit(5))
```

Process job details:
```go
detail, err := linkedin.GetJobDetail(ctx, jobID)
```

Extract keywords from transcript:
```go
keywords, err := linkedin.GetTranscriptKeywords(ctx, transcript)
```

## Data Models

1. Job Listing
   - Basic job information
   - Company details
   - Location data
   - Posting metadata
   - Company branding

2. Job Detail
   - Comprehensive job information
   - Company details
   - Requirements
   - Recruiter information
   - Related jobs

3. Requirements
   - Must-have skills
   - Preferred qualifications
   - Work location
   - Company culture
   - Additional information

## API Integration

1. Configuration
   - API key management
   - Rate limiting
   - Error handling
   - Response validation
   - Cache control

2. Endpoints
   - Job search
   - Job details
   - Company information
   - Similar jobs
   - Recruiter details

## Best Practices

1. Search Optimization
   - Use specific keywords
   - Apply appropriate filters
   - Implement pagination
   - Cache results
   - Handle rate limits

2. Data Processing
   - Validate responses
   - Handle missing data
   - Process HTML safely
   - Extract key information
   - Maintain data integrity

3. Error Handling
   - API errors
   - Rate limiting
   - Invalid data
   - Network issues
   - Authentication failures 