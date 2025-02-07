package scrapingdog

// jobListing represents a job listing from the ScrapingDog API
type jobListing struct {
	Position       string `json:"job_position"`
	Link           string `json:"job_link"`
	ID             string `json:"job_id"`
	CompanyName    string `json:"company_name"`
	CompanyProfile string `json:"company_profile"`
	Location       string `json:"job_location"`
	PostingDate    string `json:"job_posting_date"`
	CompanyLogo    string `json:"company_logo_url"`
}

// jobListings is a slice of JobListing
type jobListings []jobListing

// jobDetail represents detailed job information from the ScrapingDog API
type jobDetail struct {
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
	Recruiters        []recruiter  `json:"recruiter_details"`
	SimilarJobs       []relatedJob `json:"similar_jobs"`
	PeopleAlsoViewed  []relatedJob `json:"people_also_viewed"`
}

// jobDetails is a slice of JobDetail
type jobDetails []jobDetail

// recruiter represents a recruiter from the ScrapingDog API
type recruiter struct {
	Photo      string `json:"recruiter_photo"`
	Name       string `json:"recruiter_name"`
	Title      string `json:"recruiter_title"`
	ProfileURL string `json:"recruiter_profile_url"`
}

// relatedJob represents a related or similar job from the ScrapingDog API
type relatedJob struct {
	Position    string `json:"job_position"`
	Company     string `json:"job_company"`
	Location    string `json:"job_location"`
	PostingTime string `json:"job_posting_time"`
	Link        string `json:"job_link"`
}
