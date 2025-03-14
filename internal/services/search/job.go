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

// Recruiter represents a recruiter associated with a job posting
type Recruiter struct {
	Photo      string `json:"photo"`
	Name       string `json:"name"`
	Title      string `json:"title"`
	ProfileURL string `json:"profile_url"`
}

// RelatedJob represents a related or similar job posting
type RelatedJob struct {
	Title       string `json:"title"`
	Company     string `json:"company"`
	Location    string `json:"location"`
	PostingDate string `json:"posting_date"`
	URL         string `json:"url"`
}

// JobListing represents a basic job listing.
// This is the minimal set of fields returned from a search operation.
// More detailed information is available via GetJobDetails.
type JobListing struct {
	ID             string    `json:"id"`
	Provider       string    `json:"provider"`
	ProviderJobID  string    `json:"provider_job_id"` // Original ID from the provider
	Title          string    `json:"title"`           // maps to Position in ScrapingDog
	Company        string    `json:"company"`         // maps to CompanyName in ScrapingDog
	Location       string    `json:"location"`
	PostedDate     time.Time `json:"posted_date"`
	Description    string    `json:"description"` // Brief description or snippet if available. ScrapingDog does not return this.
	URL            string    `json:"url"`         // Provider-specific URL, maps to Link in ScrapingDog
	CompanyProfile string    `json:"company_profile,omitempty"`
	CompanyLogo    string    `json:"company_logo,omitempty"`
}

// JobDetail contains comprehensive information about a specific job.
// This is returned by GetJobDetails and includes all available
// information about the position.
type JobDetail struct {
	// Basic job information (duplicated from JobListing for clarity)
	ID            string    `json:"id"`
	Provider      string    `json:"provider"`
	ProviderJobID string    `json:"provider_job_id"`
	Title         string    `json:"title"`
	Company       string    `json:"company"`
	Location      string    `json:"location"`
	PostedDate    time.Time `json:"posted_date"`
	URL           string    `json:"url"`

	// Detailed information
	FullDescription string       `json:"full_description"` // Complete job description
	Requirements    []string     `json:"requirements"`     // List of job requirements
	Qualifications  []string     `json:"qualifications"`   // Required qualifications
	Benefits        []string     `json:"benefits"`         // List of benefits
	Skills          []string     `json:"skills"`           // Required skills
	SalaryInfo      *SalaryRange `json:"salary_info,omitempty"`
	EmploymentType  string       `json:"employment_type"` // Full-time, part-time, contract, etc.
	WorkplaceType   string       `json:"workplace_type"`  // Remote, on-site, hybrid
	Industry        string       `json:"industry"`
	CompanyInfo     *Company     `json:"company_info,omitempty"`

	// Additional fields from ScrapingDog
	Status           string       `json:"status,omitempty"`
	SeniorityLevel   string       `json:"seniority_level,omitempty"`
	JobFunction      string       `json:"job_function,omitempty"`
	Recruiters       []Recruiter  `json:"recruiters,omitempty"`
	SimilarJobs      []RelatedJob `json:"similar_jobs,omitempty"`
	PeopleAlsoViewed []RelatedJob `json:"people_also_viewed,omitempty"`
}
