package match

import "time"

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
