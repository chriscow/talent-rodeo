package types

import "time"

// Job represents a basic job listing.
type Job struct {
	ID             string
	Title          string
	Company        string
	Location       string
	PostedDate     time.Time
	Description    string // Brief description or snippet
	URL            string // Provider-specific URL
	Provider       string // Name of the source provider
	ProviderJobID  string // Original ID from the provider
	Industry       string // Industry sector
	CompanySize    string // Company size range (small, medium, large, etc.)
	EmploymentType string // Full-time, part-time, contract, etc.
	WorkplaceType  string // Remote, on-site, hybrid
}

// Candidate represents a job candidate with their profile information
type Candidate struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	Title                string           `json:"title"`
	Skills               []string         `json:"skills"`
	ExperienceYears      int              `json:"experience_years"`
	PreferredLocations   []string         `json:"preferred_locations"`
	PreferredRoles       []string         `json:"preferred_roles"`
	PreferredIndustries  []string         `json:"preferred_industries"`
	PreferredSchedule    string           `json:"preferred_schedule"`     // Full-time, part-time, contract
	PreferredCompanySize string           `json:"preferred_company_size"` // Company size preference
	CulturePreferences   []string         `json:"culture_preferences"`    // Preferred company culture aspects
	RemotePreference     string           `json:"remote_preference"`      // "remote", "hybrid", "onsite"
	Summary              string           `json:"summary"`
	Education            []Education      `json:"education"`
	WorkHistory          []WorkExperience `json:"work_history"`
}

// Education represents educational background
type Education struct {
	Degree     string `json:"degree"`
	Field      string `json:"field"`
	School     string `json:"school"`
	GradYear   int    `json:"grad_year"`
	InProgress bool   `json:"in_progress"`
}

// WorkExperience represents work history
type WorkExperience struct {
	Title       string    `json:"title"`
	Company     string    `json:"company"`
	Location    string    `json:"location"`
	StartDate   time.Time `json:"start_date"`
	EndDate     time.Time `json:"end_date,omitempty"`
	IsCurrent   bool      `json:"is_current"`
	Description string    `json:"description"`
	Skills      []string  `json:"skills"`
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
	CompanyInfo     *CompanyInfo
}

// CompanyInfo contains information about the company posting the job.
// This information may be limited depending on the provider.
type CompanyInfo struct {
	Name        string
	Website     string
	Industry    string
	Size        string // Company size range
	Description string
	LogoURL     string
}
