package providers

import (
	"context"
	"talent-rodeo/internal/jobmatch/types"
	"time"
)

// JobProvider defines the interface that all job listing providers must implement.
// This interface is designed to be generic enough to support multiple providers
// while maintaining consistency in how we interact with them.
//
// Future Considerations:
// - Phase 5 will introduce async processing via NATS/Watermill
// - Rate limiting and throttling will be needed per provider
// - Caching layer may be added for frequently accessed data
// - Error handling will need to account for provider-specific error types
type JobProvider interface {
	// SearchJobs performs a search for jobs based on the provided criteria.
	// This is a synchronous operation for Phase 1, but will be converted to
	// async in Phase 5 using NATS/Watermill for better scalability.
	SearchJobs(ctx context.Context, criteria SearchCriteria) ([]types.Job, error)

	// GetJobDetails retrieves detailed information about a specific job.
	// The jobID format may vary by provider, so implementations should handle
	// provider-specific ID formats internally.
	GetJobDetails(ctx context.Context, jobID string) (*types.JobDetail, error)

	// Name returns the unique identifier for this provider.
	// This is used for logging, metrics, and provider-specific configurations.
	Name() string

	// Validate verifies that the provider is properly configured and can make
	// API calls. This should check API keys, connection settings, etc.
	Validate(ctx context.Context) error
}

// SearchCriteria defines the parameters used to search for jobs.
// This struct is designed to be provider-agnostic, allowing each provider
// to map these fields to their specific API parameters.
type SearchCriteria struct {
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

// // Job represents a basic job listing.
// // This is the minimal set of fields returned from a search operation.
// // More detailed information is available via GetJobDetails.
// type Job struct {
// 	ID            string
// 	Title         string
// 	Company       string
// 	Location      string
// 	PostedDate    time.Time
// 	Description   string // Brief description or snippet
// 	URL           string // Provider-specific URL
// 	Provider      string // Name of the source provider
// 	ProviderJobID string // Original ID from the provider
// 	// Future considerations:
// 	// - Matching score
// 	// - Application deadline
// 	// - Number of applicants
// }

// // JobDetail contains comprehensive information about a specific job.
// // This is returned by GetJobDetails and includes all available
// // information about the position.
// type JobDetail struct {
// 	Job                      // Embed basic job information
// 	FullDescription string   // Complete job description
// 	Requirements    []string // List of job requirements
// 	Qualifications  []string // Required qualifications
// 	Benefits        []string // List of benefits
// 	Skills          []string // Required skills
// 	SalaryInfo      *SalaryRange
// 	EmploymentType  string // Full-time, part-time, contract, etc.
// 	WorkplaceType   string // Remote, on-site, hybrid
// 	Industry        string
// 	CompanyInfo     *CompanyInfo
// 	// Future considerations:
// 	// - Application process details
// 	// - Required certifications
// 	// - Travel requirements
// 	// - Team size/structure
// }

// // CompanyInfo contains information about the company posting the job.
// // This information may be limited depending on the provider.
// type CompanyInfo struct {
// 	Name        string
// 	Website     string
// 	Industry    string
// 	Size        string // Company size range
// 	Description string
// 	LogoURL     string
// 	// Future considerations:
// 	// - Company culture
// 	// - Tech stack
// 	// - Funding status
// 	// - Growth metrics
// }
