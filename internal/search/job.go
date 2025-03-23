package search

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
