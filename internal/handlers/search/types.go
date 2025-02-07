package search

import (
	"time"

	"github.com/google/uuid"
)

// Result represents a job search result with metadata
type Result struct {
	Job       JobListing `json:"job"`
	Score     float64    `json:"score"`
	Keywords  []string   `json:"matched_keywords,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
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

	// ScrapingDog specific fields
	SortBy    string // day, week, month - for filtering by posting date
	WorkType  string // at_work, remote, hybrid
	CompanyID string // LinkedIn company ID for filtering by company
	// Future fields to consider:
	// - CompanySize
	// - Benefits
	// - Remote/Hybrid/Office preferences
}

// SortByPeriod defines the possible values for the SortBy field
type SortByPeriod string

const (
	SortByDay   SortByPeriod = "day"   // Last 24 hours
	SortByWeek  SortByPeriod = "week"  // Last 7 days
	SortByMonth SortByPeriod = "month" // Last 30 days
)

// WorkType defines the possible values for the WorkType field
type WorkType string

const (
	WorkTypeOffice WorkType = "at_work"
	WorkTypeRemote WorkType = "remote"
	WorkTypeHybrid WorkType = "hybrid"
)

// JobType defines the possible values for the JobType field
type JobType string

const (
	JobTypeFullTime  JobType = "full_time"
	JobTypePartTime  JobType = "part_time"
	JobTypeContract  JobType = "contract"
	JobTypeTemporary JobType = "temporary"
	JobTypeVolunteer JobType = "volunteer"
)

// ExperienceLevel defines the possible values for the Experience field
type ExperienceLevel string

const (
	ExperienceInternship ExperienceLevel = "internship"
	ExperienceEntryLevel ExperienceLevel = "entry_level"
	ExperienceAssociate  ExperienceLevel = "associate"
	ExperienceMidSenior  ExperienceLevel = "mid_senior_level"
	ExperienceDirector   ExperienceLevel = "director"
)
