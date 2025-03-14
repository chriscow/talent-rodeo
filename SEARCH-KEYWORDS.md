# Search Keywords Implementation Strategy (Simplified MVP Approach)

## Current Implementation Gaps

### 1. Search Term Construction (`internal/services/search/types.go`)
- **Gap**: The `SearchInput` struct only has basic fields for `MustHaveSkills` and `PreferredSkills`
- **Missing**: 
  - Basic preference-based search implementation (1-5 rating system)
  - Simple approach for generating search terms based on skill preferences

### 2. Search Execution (`internal/handlers/search.go`)
- **Gap**: The handler doesn't implement our preference-based search strategy
- **Missing**:
  - Logic to determine primary search terms based on preference ratings
  - Implementation of basic scoring system based on preferences
  - Simple filtering based on "Must Avoid" skills

### 3. Provider Implementation (`internal/services/search/providers/scrapingdog/provider.go`)
- **Gap**: The provider doesn't support our preference-based search strategy
- **Missing**:
  - Basic preference-based keyword handling
  - Simple term construction from skill preferences

## Implementation Strategy (Simplified for MVP)

### 1. Search Result Storage and Retrieval

The search process will be split into two simple phases:
1. Raw job search and caching (shared across candidates)
2. Candidate-specific scoring and filtering

#### Phase 1: Raw Job Search
- Execute search using provider API with skills based on preference
- Cache raw results in `job_listings` table
- Link search criteria to results

#### Phase 2: Candidate-Specific Processing
- Score and filter jobs based on candidate's skills and preferences
- Store candidate-specific results

### 2. Enhanced Search Types

```go
// SearchInput represents the input message for job searches
type SearchInput struct {
    CandidateID     int64    `json:"candidate_id"`
    MustHaveSkills  []string `json:"must_have_skills"`   // Preference = 5
    PreferredSkills []string `json:"preferred_skills"`   // Preference = 4
    Page            int      `json:"page"`
}
```

### 3. Search Logic Implementation

```go
// handleMessage processes a single job search request message
func (h *SearchHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
    var input search.SearchInput
    if err := json.Unmarshal(msg.Data(), &input); err != nil {
        return fmt.Errorf("failed to unmarshal input: %w", err)
    }

    // Prepare search criteria based on skill preferences
    criteria := search.Criteria{
        Keywords: h.prepareSearchKeywords(input.MustHaveSkills, input.PreferredSkills),
        Page:     input.Page,
        // Other criteria fields as needed
    }
    
    // Execute search
    jobs, err := h.provider.SearchJobs(ctx, criteria)
    if err != nil {
        return fmt.Errorf("failed to execute search: %w", err)
    }

    // Score and filter results
    scoredJobs := h.scoreJobs(jobs, input.MustHaveSkills, input.PreferredSkills)

    // Publish results
    return h.publishResults(ctx, scoredJobs)
}

// prepareSearchKeywords prepares keywords based on preference priority
func (h *SearchHandler) prepareSearchKeywords(mustHaveSkills, preferredSkills []string) []string {
    // If we have must-have skills, use those for search
    if len(mustHaveSkills) > 0 {
        return mustHaveSkills
    }
    
    // Otherwise, use preferred skills
    return preferredSkills
}

// scoreJobs scores jobs based on skill matches and preferences
func (h *SearchHandler) scoreJobs(jobs []search.JobListing, mustHaveSkills, preferredSkills []string) []search.Result {
    var results []search.Result
    
    for _, job := range jobs {
        score := 0
        
        // +10 points for each Must Have skill match
        for _, skill := range mustHaveSkills {
            if strings.Contains(strings.ToLower(job.Description), strings.ToLower(skill)) {
                score += 10
            }
        }
        
        // +5 points for each Preferred skill match
        for _, skill := range preferredSkills {
            if strings.Contains(strings.ToLower(job.Description), strings.ToLower(skill)) {
                score += 5
            }
        }
        
        // Add to results
        results = append(results, search.Result{
            Job:   job,
            Score: score,
        })
    }
    
    // Sort by score descending
    sort.Slice(results, func(i, j int) bool {
        return results[i].Score > results[j].Score
    })
    
    return results
}
```

## Testing Strategy

### 1. Unit Tests for Search Term Construction

```go
func TestSearchTermConstruction(t *testing.T) {
    tests := []struct {
        name             string
        mustHaveSkills   []string
        preferredSkills  []string
        wantKeywords     []string
    }{
        {
            name:            "must have skills only",
            mustHaveSkills:  []string{"Go", "Docker"},
            preferredSkills: []string{"Python", "JavaScript"},
            wantKeywords:    []string{"Go", "Docker"},
        },
        {
            name:            "no must have, only preferred",
            mustHaveSkills:  []string{},
            preferredSkills: []string{"Python", "JavaScript"},
            wantKeywords:    []string{"Python", "JavaScript"},
        },
        {
            name:            "no skills",
            mustHaveSkills:  []string{},
            preferredSkills: []string{},
            wantKeywords:    []string{},
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            handler := NewSearchHandler(nil, nil, nil, nil, nil)
            keywords := handler.prepareSearchKeywords(tt.mustHaveSkills, tt.preferredSkills)
            
            if !reflect.DeepEqual(keywords, tt.wantKeywords) {
                t.Errorf("prepareSearchKeywords() = %v, want %v", keywords, tt.wantKeywords)
            }
        })
    }
}
```

### 2. Unit Tests for Scoring System

```go
func TestJobScoring(t *testing.T) {
    tests := []struct {
        name            string
        mustHaveSkills  []string
        preferredSkills []string
        jobDescription  string
        wantScore       int
    }{
        {
            name:            "job with all must have skills",
            mustHaveSkills:  []string{"Go", "Docker"},
            preferredSkills: []string{"Kubernetes"},
            jobDescription:  "This job requires Go programming and Docker experience.",
            wantScore:       20, // 10 points per must have skill
        },
        {
            name:            "job with some preferred skills",
            mustHaveSkills:  []string{"Go"},
            preferredSkills: []string{"Docker", "Kubernetes"},
            jobDescription:  "This job requires Go programming and Docker experience.",
            wantScore:       15, // 10 for must have + 5 for preferred
        },
        {
            name:            "job with no matching skills",
            mustHaveSkills:  []string{"Go", "Docker"},
            preferredSkills: []string{"Kubernetes"},
            jobDescription:  "This job requires Python programming and AWS experience.",
            wantScore:       0,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            handler := NewSearchHandler(nil, nil, nil, nil, nil)
            
            job := search.JobListing{
                Title:       "Test Job",
                Company:     "Test Company",
                Location:    "Remote",
                Description: tt.jobDescription,
            }
            
            results := handler.scoreJobs([]search.JobListing{job}, tt.mustHaveSkills, tt.preferredSkills)
            
            if len(results) == 0 {
                t.Fatalf("Expected results but got none")
            }
            
            if results[0].Score != tt.wantScore {
                t.Errorf("Job score = %d, want %d", results[0].Score, tt.wantScore)
            }
        })
    }
}
```

## Implementation Order

1. Update `types.go` with simplified SearchInput structure
   - Focus on MustHaveSkills and PreferredSkills only 

2. Implement basic search logic
   - Simple skill preference-based search term selection
   - Basic scoring system implementation

3. Update provider implementation
   - Support for basic keyword search with preferences

4. Add unit tests
   - Tests for search term selection
   - Tests for scoring system

5. Update documentation
   - Document the simplified search logic

## Notes

- This simplified approach focuses on core functionality for MVP
- No implementation of complex skill relationships or multiple searches
- Scoring system is straightforward and easy to understand
- Future enhancements can build on this foundation if needed
- The approach aligns well with the existing JobProvider interface 