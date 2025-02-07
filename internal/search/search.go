package search

import (
	"time"

	"github.com/google/uuid"
)

// Result represents a job search result with metadata
type Result struct {
	Job       Job       `json:"job"`
	Score     float64   `json:"score"`
	Keywords  []string  `json:"matched_keywords,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CacheEntry represents a cached search result
type CacheEntry struct {
	ID        uuid.UUID `json:"id"`
	QueryHash string    `json:"query_hash"`
	Provider  uuid.UUID `json:"provider_id"`
	Results   []Result  `json:"results"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Keyword represents an extracted and categorized keyword
type Keyword struct {
	ID         uuid.UUID `json:"id"`
	JobID      uuid.UUID `json:"job_id"`
	Text       string    `json:"text"`
	Category   string    `json:"category"` // skill, title, culture, etc.
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
}

// Criteria defines the parameters used to search for jobs.
// This struct is designed to be provider-agnostic, allowing each provider
// to map these fields to their specific API parameters.
type Criteria struct {
	Keywords   []string     // Required search terms
	Location   string       // Geographic location (can be remote)
	DatePosted time.Time    // Filter for jobs posted after this date
	JobType    []string     // Full-time, part-time, contract, etc.
	Experience []string     // Experience levels
	Industry   []string     // Industry sectors
	Skills     []string     // Required skills
	Salary     *SalaryRange // Desired salary range
	PageSize   int          // Number of results per page
	PageNumber int          // Page number for pagination
	// Future fields to consider:
	// - CompanySize
	// - Benefits
	// - Remote/Hybrid/Office preferences
}
