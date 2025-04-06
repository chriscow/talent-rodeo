package search

import (
	"time"

	"github.com/google/uuid"
)

// Constants for message routing
const (
	TopicSearchInput  = "rodeo.search.input"
	TopicSearchResult = "rodeo.search.result"

	SortByDay   SortByPeriod = "day"   // Last 24 hours
	SortByWeek  SortByPeriod = "week"  // Last 7 days
	SortByMonth SortByPeriod = "month" // Last 30 days

	WorkTypeOffice WorkType = "at_work"
	WorkTypeRemote WorkType = "remote"
	WorkTypeHybrid WorkType = "hybrid"

	JobTypeFullTime  JobType = "full_time"
	JobTypePartTime  JobType = "part_time"
	JobTypeContract  JobType = "contract"
	JobTypeTemporary JobType = "temporary"
	JobTypeVolunteer JobType = "volunteer"

	ExperienceInternship ExperienceLevel = "internship"
	ExperienceEntryLevel ExperienceLevel = "entry_level"
	ExperienceAssociate  ExperienceLevel = "associate"
	ExperienceMidSenior  ExperienceLevel = "mid_senior_level"
	ExperienceDirector   ExperienceLevel = "director"
)

type (
	// SearchInput represents the input message for job searches
	SearchInput struct {
		CandidateID     string   `json:"candidate_id"`
		MustHaveSkills  []string `json:"must_have_skills"`
		PreferredSkills []string `json:"preferred_skills"`
		Page            int      `json:"page"`
	}

	// Result represents a job search result with metadata
	Result struct {
		Job       JobListing `json:"job"`
		Score     float64    `json:"score"`
		Keywords  []string   `json:"matched_keywords,omitempty"`
		CreatedAt time.Time  `json:"created_at"`
	}

	// CacheEntry represents a cached search result
	CacheEntry struct {
		ID        uuid.UUID `json:"id"`
		QueryHash string    `json:"query_hash"`
		Provider  uuid.UUID `json:"provider_id"`
		Results   []Result  `json:"results"`
		CreatedAt time.Time `json:"created_at"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	// Keyword represents an extracted and categorized keyword
	Keyword struct {
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
	Criteria struct {
		Keywords   []string `json:"keywords" firestore:"keywords" description:"Search phrases and keywords to find in job titles, descriptions, and requirements"` // Required search terms
		Location   string   `json:"location" firestore:"location" description:"General geographic location (can be remote)"`                                       // Geographic location (can be remote)
		PageNumber int      `json:"-" firestore:"-" jsonschema:"-"`                                                                                                // Page number for pagination
		SortBy     string   `json:"-" firestore:"-" jsonschema:"-"`                                                                                                // day, week, month - for filtering by posting date
		JobType    []string `json:"-" firestore:"-" jsonschema:"-"`                                                                                                // Full-time, part-time, contract, etc.
		Experience []string `json:"-" firestore:"-" jsonschema:"-"`                                                                                                // Experience levels
		WorkType   string   `json:"-" firestore:"-" jsonschema:"-"`                                                                                                // at_work, remote, hybrid
		CompanyID  string   `json:"-" firestore:"-" jsonschema:"-"`                                                                                                // LinkedIn company ID for filtering by company
	}

	// ProgressUpdate represents a search progress update message
	ProgressUpdate struct {
		TotalJobs       int         `json:"totalJobs"`
		AnalyzedJobs    int         `json:"analyzedJobs"`
		GoodFitJobs     int         `json:"goodFitJobs"`
		Status          string      `json:"status"`
		IsComplete      bool        `json:"isComplete"`
		PercentComplete float64     `json:"percentComplete"`
		Results         []JobResult `json:"results,omitempty"`
	}

	// JobResult represents a job search result
	JobResult struct {
		Title      string          `json:"title"`
		Company    string          `json:"company"`
		Location   string          `json:"location"`
		MatchScore int             `json:"matchScore"`
		SkillMatch map[string]bool `json:"skillMatch"`
	}

	// SortByPeriod defines the possible values for the SortBy field
	SortByPeriod string

	// WorkType defines the possible values for the WorkType field
	WorkType string

	// JobType defines the possible values for the JobType field
	JobType string

	// ExperienceLevel defines the possible values for the Experience field
	ExperienceLevel string

	// JobListing represents a basic job listing.
	// This is the minimal set of fields returned from a search operation.
	// More detailed information is available via GetJobDetails.
	JobListing struct {
		ID             string    `json:"id" firestore:"id"`
		Provider       string    `json:"provider" firestore:"provider"`
		ProviderJobID  string    `json:"provider_job_id" firestore:"provider_job_id"` // Original ID from the provider
		Title          string    `json:"title" firestore:"title"`                     // maps to Position in ScrapingDog
		Company        string    `json:"company" firestore:"company"`                 // maps to CompanyName in ScrapingDog
		Location       string    `json:"location" firestore:"location"`
		PostedDate     time.Time `json:"posted_date" firestore:"posted_date"`
		Description    string    `json:"description" firestore:"description"` // Brief description or snippet if available. ScrapingDog does not return this.
		URL            string    `json:"url" firestore:"url"`                 // Provider-specific URL, maps to Link in ScrapingDog
		CompanyProfile string    `json:"company_profile,omitempty" firestore:"company_profile"`
		CompanyLogo    string    `json:"company_logo,omitempty" firestore:"company_logo"`

		SearchHash  string    `json:"-" firestore:"search_hash"`
		FirstSeenAt time.Time `json:"-" firestore:"first_seen_at"`
	}

	// JobDetail contains comprehensive information about a specific job.
	// This is returned by GetJobDetails and includes all available
	// information about the position.
	JobDetail struct {
		// Basic job information (duplicated from JobListing for clarity)
		ID            string    `json:"id" firestore:"id"`
		Provider      string    `json:"provider" firestore:"provider"`
		ProviderJobID string    `json:"provider_job_id" firestore:"provider_job_id"`
		Title         string    `json:"title" firestore:"title"`
		Company       string    `json:"company" firestore:"company"`
		Location      string    `json:"location" firestore:"location"`
		PostedDate    time.Time `json:"posted_date" firestore:"posted_date"`
		URL           string    `json:"url" firestore:"url"`

		// Detailed information
		FullDescription string       `json:"full_description" firestore:"full_description"` // Complete job description
		Requirements    []string     `json:"requirements" firestore:"requirements"`         // List of job requirements
		Qualifications  []string     `json:"qualifications" firestore:"qualifications"`     // Required qualifications
		Benefits        []string     `json:"benefits" firestore:"benefits"`                 // List of benefits
		Skills          []string     `json:"skills" firestore:"skills"`                     // Required skills
		SalaryInfo      *SalaryRange `json:"salary_info,omitempty" firestore:"salary_info"`
		EmploymentType  string       `json:"employment_type" firestore:"employment_type"` // Full-time, part-time, contract, etc.
		WorkplaceType   string       `json:"workplace_type" firestore:"workplace_type"`   // Remote, on-site, hybrid
		Industry        string       `json:"industry" firestore:"industry"`
		CompanyInfo     *Company     `json:"company_info,omitempty" firestore:"company_info"`

		// Additional fields from ScrapingDog
		Status           string       `json:"status,omitempty" firestore:"status"`
		SeniorityLevel   string       `json:"seniority_level,omitempty" firestore:"seniority_level"`
		JobFunction      string       `json:"job_function,omitempty" firestore:"job_function"`
		Recruiters       []Recruiter  `json:"recruiters,omitempty" firestore:"recruiters"`
		SimilarJobs      []RelatedJob `json:"similar_jobs,omitempty" firestore:"similar_jobs"`
		PeopleAlsoViewed []RelatedJob `json:"people_also_viewed,omitempty" firestore:"people_also_viewed"`

		CreatedAt    time.Time `firestore:"created_at"`
		SearchPhrase []string  `firestore:"search_phrase"`
	}

	// SearchResults represents the results of a job search
	SearchResults struct {
		Jobs []JobListing `json:"jobs"`
	}
)
