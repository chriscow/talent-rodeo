package scrapingdog

import (
	"time"
)

const (
	SearchProviderName = "scrapingdog"
	JobProviderName    = "linkedin"
	DefaultBaseURL     = "https://api.scrapingdog.com/jobs"
	MaxRetries         = 10
	RetryDelay         = 5 * time.Second
	DefaultConcurrency = 20
	DefaultQueueSize   = 100
)

// GeoID represents a LinkedIn location ID
type GeoID string

const (
	RemoteGeoID GeoID = "103644278"
)

// LinkedInJobListing represents a job listing from the ScrapingDog API
type LinkedInJobListing struct {
	ID             string `json:"-"`
	LinkedInJobID  string `json:"job_id"`
	Position       string `json:"job_position"`
	Link           string `json:"job_link"`
	CompanyName    string `json:"company_name"`
	CompanyProfile string `json:"company_profile"`
	Location       string `json:"job_location"`
	PostingDate    string `json:"job_posting_date"`
	CompanyLogo    string `json:"company_logo_url"`
}

// LinkedInJobListings is a slice of JobListing
type LinkedInJobListings []LinkedInJobListing

// LinkedInJobDetail represents detailed job information from the ScrapingDog API
type LinkedInJobDetail struct {
	ID                string       `json:"-"`
	Position          string       `json:"job_position"`
	Status            string       `json:"jobs_status"`
	Location          string       `json:"job_location"`
	CompanyName       string       `json:"company_name"`
	CompanyLinkedInID string       `json:"company_linkedin_id"`
	PostingTime       string       `json:"job_posting_time"`
	BasePay           string       `json:"base_pay"`
	Description       string       `json:"job_description"`
	SeniorityLevel    string       `json:"Seniority_level"`
	EmploymentType    string       `json:"Employment_type"`
	JobFunction       string       `json:"Job_function"`
	Industries        string       `json:"Industries"`
	JobApplyLink      string       `json:"job_apply_link"`
	Recruiters        []Recruiter  `json:"recruiter_details"`
	SimilarJobs       []RelatedJob `json:"similar_jobs"`
	PeopleAlsoViewed  []RelatedJob `json:"people_also_viewed"`
}

// Recruiter represents a recruiter from the ScrapingDog API
type Recruiter struct {
	Photo      string `json:"recruiter_photo"`
	Name       string `json:"recruiter_name"`
	Title      string `json:"recruiter_title"`
	ProfileURL string `json:"recruiter_profile_url"`
}

// RelatedJob represents a related or similar job from the ScrapingDog API
type RelatedJob struct {
	Position    string `json:"job_position"`
	Company     string `json:"job_company"`
	Location    string `json:"job_location"`
	PostingTime string `json:"job_posting_time"`
	Link        string `json:"job_link"`
}
