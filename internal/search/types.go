package search

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

// Criteria defines the parameters used to search for jobs.
// This struct is designed to be provider-agnostic, allowing each provider
// to map these fields to their specific API parameters.
type Criteria struct {
	Query      string   `json:"query"  description:"Search query to find in job titles, descriptions, and requirements"`                                                                                      // Required search terms
	Location   string   `json:"location"  description:"Nearest major metropolitan area in the United States (can be 'Remote')"`                                                                               // Geographic location (can be remote)
	JobType    []string `json:"job-type"  enum:"temporary,contract,volunteer,full_time,part_time" description:"A value here will filter on job type.  Leave blank if you ant all jobs."`                      // temporary, contract, volunteer, full_time & part_time                                                                                                                                                                    // Full-time, part-time, contract, etc.
	Experience []string `json:"experience"  enum:"internship,entry_level,associate,mid_senior_level,director" description:"A value here will filter on experience level.  Leave blank if you ant all jobs."`  // internship, entry_level, associate, mid_senior_level & director                                                                                                                                                                         // Experience levels
	WorkType   string   `json:"work-type"  enum:"at_work,remote,hybrid" description:"This will FILTER OUT jobs. If the user was explicit about the work type, include a value here otherwise leave it blank"` // at_work, remote, hybrid

	MaxPages  int    `json:"-" jsonschema:"-" firestore:"-" ` // Page number for pagination
	SortBy    string `json:"-" jsonschema:"-" firestore:"-"`  // day, week, month - for filtering by posting date
	CompanyID string `json:"-" jsonschema:"-" firestore:"-"`
}

func (c Criteria) GetSearchHash() string {
	// serialize the criteria to a json string and get the SHA256 hash
	json, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(json))
}

// JobListing represents a basic job listing.
// This is the minimal set of fields returned from a search operation.
// More detailed information is available via GetJobDetails.
type JobListing struct {
	ID             string
	Provider       string
	ProviderJobID  string // Original ID from the provider
	Title          string // maps to Position in ScrapingDog
	Company        string // maps to CompanyName in ScrapingDog
	Location       string
	PostedDate     time.Time
	Description    string // Brief description or snippet if available. ScrapingDog does not return this.
	URL            string // Provider-specific URL, maps to Link in ScrapingDog
	CompanyProfile string
	CompanyLogo    string

	SearchHash  string    `json:"-"`
	FirstSeenAt time.Time `json:"-"`
}

// JobDetail contains comprehensive information about a specific job.
// This is returned by GetJobDetails and includes all available
// information about the position.
type JobDetail struct {
	// Basic job information (duplicated from JobListing for clarity)
	ID            string
	Provider      string
	ProviderJobID string
	Title         string
	Company       string
	Location      string
	PostedDate    time.Time
	URL           string

	// Detailed information
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

	// Summary information
	ExtractedData *ExtractedData // Summarized job description

	// Additional fields from ScrapingDog
	Status           string
	SeniorityLevel   string
	JobFunction      string
	Recruiters       []Recruiter
	SimilarJobs      []RelatedJob
	PeopleAlsoViewed []RelatedJob

	CreatedAt    time.Time
	SearchPhrase []string
}

type ExtractedData struct {
	Summary        string   `description:"a concise summary of the job listing"`
	Requirements   []string `description:"the essential criteria that employers consider non-negotiable for a candidate to perform the job successfully"`
	Qualifications []string `description:"the broader set of skills, experiences, and attributes that make someone a good fit for the role"`
}

// SalaryRange represents a salary range for job listings.
// Some providers may not support all fields, so implementations
// should handle partial data appropriately.
type SalaryRange struct {
	Min      float64 `json:"min" description:"The minimum salary in the range"`
	Max      float64 `json:"max" description:"The maximum salary in the range"`
	Currency string  `json:"currency" description:"The currency of the salary range"`
	Period   string  `json:"period" description:"The period of the salary range" enum:"yearly,monthly,hourly"`
	// Future considerations:
	// - Equity compensation
	// - Bonus structure
	// - Benefits value
}

// Recruiter represents a recruiter associated with a job posting
type Recruiter struct {
	Photo      string
	Name       string
	Title      string
	ProfileURL string
}

// RelatedJob represents a related or similar job posting
type RelatedJob struct {
	Title       string
	Company     string
	Location    string
	PostingDate string
	URL         string
}
