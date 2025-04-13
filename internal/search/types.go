package search

import (
	"time"
)

// SortByPeriod defines the possible values for the SortBy field
type SortByPeriod string

// WorkType defines the possible values for the WorkType field
type WorkType string

// JobType defines the possible values for the JobType field
type JobType string

// ExperienceLevel defines the possible values for the Experience field
type ExperienceLevel string

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

// Result represents a job search result with metadata
// type Result struct {
// 	Job       JobListing `json:"job"`
// 	Score     float64    `json:"score"`
// 	Keywords  []string   `json:"matched_keywords,omitempty"`
// 	CreatedAt time.Time  `json:"created_at"`
// }

// CacheEntry represents a cached search result
// type CacheEntry struct {
// 	ID        uuid.UUID `json:"id"`
// 	QueryHash string    `json:"query_hash"`
// 	Provider  uuid.UUID `json:"provider_id"`
// 	Results   []Result  `json:"results"`
// 	CreatedAt time.Time `json:"created_at"`
// 	ExpiresAt time.Time `json:"expires_at"`
// }

// Keyword represents an extracted and categorized keyword
// type Keyword struct {
// 	ID         uuid.UUID `json:"id"`
// 	JobID      uuid.UUID `json:"job_id"`
// 	Text       string    `json:"text"`
// 	Category   string    `json:"category"` // skill, title, culture, etc.
// 	Confidence float64   `json:"confidence"`
// 	CreatedAt  time.Time `json:"created_at"`
// }

// Criteria defines the parameters used to search for jobs.
// This struct is designed to be provider-agnostic, allowing each provider
// to map these fields to their specific API parameters.
type Criteria struct {
	Keywords   []string `json:"keywords" firestore:"keywords" description:"Search phrases and keywords to find in job titles, descriptions, and requirements"`                                                                                    // Required search terms
	Location   string   `json:"location" firestore:"location" description:"Nearest major metropolitan area in the United States (can be 'Remote')"`                                                                                               // Geographic location (can be remote)
	PageNumber int      `json:"-" firestore:"-" jsonschema:"-"`                                                                                                                                                                                   // Page number for pagination
	SortBy     string   `json:"-" firestore:"-" jsonschema:"-"`                                                                                                                                                                                   // day, week, month - for filtering by posting date
	JobType    []string `json:"-" firestore:"job-type" jsonschema:"job-type" enum:"temporary,contract,volunteer,full_time,part_time" description:"A value here will filter on job type.  Leave blank if you ant all jobs."`                       // temporary, contract, volunteer, full_time & part_time                                                                                                                                                                    // Full-time, part-time, contract, etc.
	Experience []string `json:"-" firestore:"experience" jsonschema:"experience" enum:"internship,entry_level,associate,mid_senior_level,director" description:"A value here will filter on experience level.  Leave blank if you ant all jobs."` // internship, entry_level, associate, mid_senior_level & director                                                                                                                                                                         // Experience levels
	WorkType   string   `json:"-" firestore:"work-type" jsonschema:"work-type" enum:"at_work,remote,hybrid" description:"This will FILTER OUT jobs. If the user was explicit about the work type, include a value here otherwise leave it blank"` // at_work, remote, hybrid
	CompanyID  string   `json:"-" firestore:"-" jsonschema:"-"`                                                                                                                                                                                   // LinkedIn company ID for filtering by company
}

// ProgressUpdate represents a search progress update message
// type ProgressUpdate struct {
// 	TotalJobs       int         `json:"totalJobs"`
// 	AnalyzedJobs    int         `json:"analyzedJobs"`
// 	GoodFitJobs     int         `json:"goodFitJobs"`
// 	Status          string      `json:"status"`
// 	IsComplete      bool        `json:"isComplete"`
// 	PercentComplete float64     `json:"percentComplete"`
// 	Results         []JobResult `json:"results,omitempty"`
// }

// JobResult represents a job search result
// type JobResult struct {
// 	Title      string          `json:"title"`
// 	Company    string          `json:"company"`
// 	Location   string          `json:"location"`
// 	MatchScore int             `json:"matchScore"`
// 	SkillMatch map[string]bool `json:"skillMatch"`
// }

// JobListing represents a basic job listing.
// This is the minimal set of fields returned from a search operation.
// More detailed information is available via GetJobDetails.
type JobListing struct {
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
type JobDetail struct {
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
// type SearchResults struct {
// 	Jobs []JobListing `json:"jobs"`
// }

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
