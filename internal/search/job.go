package search

import "time"

// SalaryRange represents a salary range for job listings.
// Some providers may not support all fields, so implementations
// should handle partial data appropriately.
type SalaryRange struct {
	Min      float64
	Max      float64
	Currency string // ISO 4217 currency code
	Period   string // yearly, monthly, hourly
	// Future considerations:
	// - Equity compensation
	// - Bonus structure
	// - Benefits value
}

/*

// Job represents a job listing with common fields across all providers
type Job struct {
	ID            int       `json:"id"`
	ProviderID    int       `json:"provider_id"`
	ProviderJobID string    `json:"provider_job_id"`
	Title         string    `json:"title"`
	Company       string    `json:"company"`
	Location      string    `json:"location"`
	IsRemote      bool      `json:"is_remote"`
	Description   string    `json:"description"`
	URL           string    `json:"url"`
	PostedAt      time.Time `json:"posted_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

*/

// Job represents a basic job listing.
// This is the minimal set of fields returned from a search operation.
// More detailed information is available via GetJobDetails.
type Job struct {
	ID            string
	Title         string
	Company       string
	Location      string
	PostedDate    time.Time
	Description   string // Brief description or snippet
	URL           string // Provider-specific URL
	Provider      string // Name of the source provider
	ProviderJobID string // Original ID from the provider
	// Future considerations:
	// - Matching score
	// - Application deadline
	// - Number of applicants
}

// JobDetail contains comprehensive information about a specific job.
// This is returned by GetJobDetails and includes all available
// information about the position.
type JobDetail struct {
	Job                      // Embed basic job information
	FullDescription string   // Complete job description
	Requirements    []string // List of job requirements
	Qualifications  []string // Required qualifications
	Benefits        []string // List of benefits
	Skills          []string // Required skills
	SalaryInfo      *SalaryRange
	EmploymentType  string // Full-time, part-time, contract, etc.
	WorkplaceType   string // Remote, on-site, hybrid
	Industry        string
	CompanyInfo     *Company
	// Future considerations:
	// - Application process details
	// - Required certifications
	// - Travel requirements
	// - Team size/structure
}
