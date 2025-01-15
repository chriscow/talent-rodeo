package linkedin

import "time"

// JobRequirement represents a single required skill, qualification, or characteristic
type JobRequirement struct {
	Category    string  `json:"category"`     // Technical, Soft Skill, Education, Certification, Experience
	Name        string  `json:"name"`         // Specific requirement name
	Description string  `json:"description"`  // Detailed description
	Level       string  `json:"level"`        // Required proficiency level
	YearsNeeded float32 `json:"years_needed"` // Years of experience needed
	Importance  float32 `json:"importance"`   // 0-1 scale of importance
	Dealbreaker bool    `json:"dealbreaker"`  // If lacking this is an automatic no
}

// WorkLocation captures all aspects of where and how work is performed
type WorkLocation struct {
	Type           string   `json:"type"`            // Remote, Hybrid, On-site
	Frequency      string   `json:"frequency"`       // Days per week in office for hybrid
	Locations      []string `json:"locations"`       // Physical office locations
	TimeZones      []string `json:"time_zones"`      // Required working time zones
	TravelPercent  float32  `json:"travel_percent"`  // Expected travel percentage
	Relocate       bool     `json:"relocate"`        // Whether relocation is required
	FlexibleHours  bool     `json:"flexible_hours"`  // Whether hours are flexible
	CoreHours      string   `json:"core_hours"`      // Required core working hours if any
	RegionalPolicy string   `json:"regional_policy"` // Any regional work policy details
}

// CompanyCulture represents the company's working environment and values
type CompanyCulture struct {
	Values        []string `json:"values"`         // Core company values
	Environment   string   `json:"environment"`    // Description of work environment
	PaceOfWork    string   `json:"pace_of_work"`   // Fast-paced, moderate, etc.
	TeamSize      int      `json:"team_size"`      // Size of immediate team
	Communication string   `json:"communication"`  // Communication style and expectations
	Management    string   `json:"management"`     // Management style
	Growth        string   `json:"growth"`         // Growth and development opportunities
	WorkLifeInfo  string   `json:"work_life_info"` // Work-life balance details
}

// Compensation contains all compensation-related information
type Compensation struct {
	BaseSalaryRange struct {
		Min float64 `json:"min"`
		Max float64 `json:"max"`
	} `json:"base_salary_range"`
	EquityOffered    bool     `json:"equity_offered"`
	BenefitsSummary  []string `json:"benefits_summary"`
	BonusStructure   string   `json:"bonus_structure"`
	CompStructure    string   `json:"comp_structure"` // How compensation grows/changes
	AdditionalPerks  []string `json:"additional_perks"`
	ReviewFrequency  string   `json:"review_frequency"`
	CompensationType string   `json:"compensation_type"` // Hourly, Salary, Contract, etc.
}

// JobPosting represents the complete job posting information
type JobPosting struct {
	// Basic Information
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Department     string    `json:"department"`
	ReportsTo      string    `json:"reports_to"`
	Company        string    `json:"company"`
	Industry       string    `json:"industry"`
	PostedDate     time.Time `json:"posted_date"`
	StartDate      string    `json:"start_date"`
	ApplicationURL string    `json:"application_url"`
	ContractLength string    `json:"contract_length,omitempty"` // If contract position

	// Detailed Requirements
	RequiredQuals  []JobRequirement `json:"required_quals"`
	PreferredQuals []JobRequirement `json:"preferred_quals"`
	Location       WorkLocation     `json:"location"`
	Culture        CompanyCulture   `json:"culture"`
	Compensation   Compensation     `json:"compensation"`

	// Role Information
	MainResponsibilities []string `json:"main_responsibilities"`
	SuccessMetrics       []string `json:"success_metrics"`      // How success will be measured
	GrowthOpportunities  []string `json:"growth_opportunities"` // Career progression paths
	ProjectInfo          string   `json:"project_info"`         // Specific project details if any
	TechStack            []string `json:"tech_stack"`           // Technical environment details
	TeamStructure        string   `json:"team_structure"`       // Team composition and structure

	// Additional Context
	CompanyStage    string   `json:"company_stage"`    // Startup, Growth, Enterprise, etc.
	CompanySize     int      `json:"company_size"`     // Number of employees
	Challenges      []string `json:"challenges"`       // Current challenges or priorities
	InterviewSteps  []string `json:"interview_steps"`  // Interview process details
	AdditionalNotes string   `json:"additional_notes"` // Any other relevant information

	// LLM Analysis Fields
	Keywords       []string  `json:"keywords"`        // Key terms for matching
	RoleCategory   string    `json:"role_category"`   // General category of the role
	SeniorityLevel string    `json:"seniority_level"` // Junior, Mid, Senior, Lead, etc.
	UpdatedAt      time.Time `json:"updated_at"`      // Last time this was updated
}

// NewJobPosting creates a new JobPosting with initialized slices
func NewJobPosting() *JobPosting {
	return &JobPosting{
		RequiredQuals:        make([]JobRequirement, 0),
		PreferredQuals:       make([]JobRequirement, 0),
		MainResponsibilities: make([]string, 0),
		SuccessMetrics:       make([]string, 0),
		GrowthOpportunities:  make([]string, 0),
		TechStack:            make([]string, 0),
		Keywords:             make([]string, 0),
		PostedDate:           time.Now(),
		UpdatedAt:            time.Now(),
	}
}
