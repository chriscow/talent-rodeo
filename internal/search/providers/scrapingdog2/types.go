package scrapingdog2

import (
	"context"
	"talent-rodeo/internal/search"
	"time"
)

const (
	ProviderName       = "scrapingdog"
	DefaultBaseURL     = "https://api.scrapingdog.com/linkedinjobs"
	MaxRetries         = 3
	RetryDelay         = 5 * time.Second
	DefaultConcurrency = 5
	DefaultQueueSize   = 100
)

// GeoID represents a LinkedIn location ID
type GeoID string

const (
	RemoteGeoID GeoID = "103644278"
)

// QueuedSearch represents a search job in the queue
type QueuedSearch struct {
	UserID   string
	Criteria search.Criteria
}

// JobProvider interface defines methods that must be implemented by all job providers
type JobProvider interface {
	Name() string
	Validate(ctx context.Context) error
	SearchJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error)
	GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error)
}

// JobQueue interface defines methods for queuing and managing job searches
type JobQueue interface {
	QueueSearch(ctx context.Context, userID string, criteria search.Criteria) error
	GetQueueDepth(ctx context.Context) (int, error)
}

// JobEventHandler interface defines methods for handling job-related events
type JobEventHandler interface {
	OnNewJob(userID string, criteria search.Criteria, jobID string, details search.JobDetail) error
	OnSearchCompleted(userID string, criteria search.Criteria, totalJobs int, errors []error) error
}

// JobStore interface defines methods for job data persistence
type JobStore interface {
	GetCachedJobs(ctx context.Context, criteria search.Criteria) ([]search.JobListing, error)
	SaveJobs(ctx context.Context, criteria search.Criteria, jobs []search.JobListing) error
	GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error)
	SaveJobDetails(ctx context.Context, jobDetail search.JobDetail) error
	// CreateUserJob(ctx context.Context, userID string, searchHash string, job search.JobListing, details *search.JobDetail) error
}

// API Response Types

// JobListing represents a job listing from the ScrapingDog API
type JobListing struct {
	ID             string `json:"id"`
	LinkedInJobID  string `json:"job_id"`
	Position       string `json:"job_position"`
	Link           string `json:"job_link"`
	CompanyName    string `json:"company_name"`
	CompanyProfile string `json:"company_profile"`
	Location       string `json:"job_location"`
	PostingDate    string `json:"job_posting_date"`
	CompanyLogo    string `json:"company_logo_url"`
}

// JobListings is a slice of JobListing
type JobListings []JobListing

// JobDetail represents detailed job information from the ScrapingDog API
type JobDetail struct {
	ID                string       `json:"id,omitempty"`
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
