package linkedin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/gemini"
)

const remoteGEOID = "103644278"

type JobListing struct {
	Position       string `json:"job_position" db:"position"`
	Link           string `json:"job_link" db:"link"`
	ID             string `json:"job_id" db:"id"`
	CompanyName    string `json:"company_name" db:"company_name"`
	CompanyProfile string `json:"company_profile" db:"company_profile"`
	Location       string `json:"job_location" db:"location"`
	PostingDate    string `json:"job_posting_date" db:"posting_date"`
	CompanyLogo    string `json:"company_logo_url" db:"company_logo"`
}

type JobDetail struct {
	Position          string       `json:"job_position" db:"position"`
	Status            string       `json:"job_status" db:"status"`
	Location          string       `json:"job_location" db:"location"`
	CompanyName       string       `json:"company_name" db:"company_name"`
	CompanyLinkedInID string       `json:"company_linkedin_id" db:"company_linkedin_id"`
	PostingTime       string       `json:"job_posting_time" db:"posting_time"`
	BasePay           string       `json:"base_pay" db:"base_pay"`
	Description       string       `json:"job_description" db:"description"`
	SeniorityLevel    string       `json:"seniority_level" db:"seniority_level"`
	EmploymentType    string       `json:"employment_type" db:"employment_type"`
	JobFunction       string       `json:"job_function" db:"job_function"`
	Industries        string       `json:"industries" db:"industries"`
	Recruiters        []Recruiter  `json:"recruiter_details" db:"recruiter_details"`
	SimilarJobs       []RelatedJob `json:"similar_jobs" db:"similar_jobs"`
	PeopleAlsoViewed  []RelatedJob `json:"people_also_viewed" db:"people_also_viewed"`
}

type Recruiter struct {
	Photo      string  `json:"recruiter_photo" db:"photo_url"`
	Name       string  `json:"recruiter_name" db:"name"`
	Title      string  `json:"recruiter_title" db:"title"`
	ProfileURL *string `json:"recruiter_profile_url" db:"profile_url"` // Use a pointer to handle null values
}

type RelatedJob struct {
	Position    string `json:"job_position" db:"position"`
	Company     string `json:"job_company" db:"company"`
	Location    string `json:"job_location" db:"location"`
	PostingTime string `json:"job_posting_time" db:"posting_time"`
	Link        string `json:"job_link" db:"link"`
}

// For parsing the array of job listings
// type JobDetails []JobDetail

func GetTranscriptKeywords(ctx context.Context, transcript string) ([]string, error) {
	prompt := minds.Messages{{
		Role: minds.RoleUser,
		Content: `Given the complete transcript of a conversation between a software 
	development talent recruiter and a candidate, identify the most relevant 
	search keywords for a LinkedIn job search. Focus on the candidate's 
	preferences, skills, ideal job titles, industries, and any other relevant 
	factors. The output should include a mix of specific terms 
	(e.g., 'Full-Stack Developer') and broad categories (e.g., 'Web Development') 
	to optimize search results. Additionally, include alternative keywords or 
	synonyms where applicable to maximize match potential.
	\n\nTranscript:\n\n` + transcript}}

	type output struct {
		Keywords []string `json:"keywords"`
	}
	schema, err := minds.NewResponseSchema("search_keywords", "LinkedIn job search keywords", output{})
	if err != nil {
		return nil, fmt.Errorf("failed to create response schema: %w", err)
	}

	req := minds.NewRequest(prompt, minds.WithResponseSchema(*schema))

	llm, err := gemini.NewProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	resp, err := llm.GenerateContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	var o output
	if err := json.Unmarshal([]byte(resp.String()), &o); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return o.Keywords, nil
}

// func SearchJobs(ctx context.Context, geoID, keyword string, opts ...JobSearchOption) ([]JobListing, error) {
// 	options := JobSearchOptions{
// 		pageLimit: 1,
// 	}
// 	for _, opt := range opts {
// 		opt(&options)
// 	}

// 	jobListings := []JobListing{}
// 	for page := 1; page <= options.pageLimit; page++ {
// 		listings, err := searchJobs(ctx, geoID, keyword, fmt.Sprintf("%d", page), options)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to search jobs: %w", err)
// 		}

// 		jobListings = append(jobListings, listings...)
// 	}

// 	return jobListings, nil
// }

func searchJobs(ctx context.Context, geoID, keyword, page string, options JobSearchOptions) ([]JobListing, error) {

	if os.Getenv("SCRAPINGDOG_API_KEY") == "" {
		return nil, fmt.Errorf("SCRAPINGDOG_API_KEY is required")
	}

	req, err := http.NewRequest("GET", "https://api.scrapingdog.com/linkedinjobs/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("api_key", os.Getenv("SCRAPINGDOG_API_KEY"))
	q.Add("field", keyword)
	q.Add("geoid", geoID)
	q.Add("page", page)
	if options.sortBy != "" {
		q.Add("sort_by", options.sortBy)
	}
	if options.jobType != "" {
		q.Add("job_type", options.jobType)
	}
	if options.expLevel != "" {
		q.Add("exp_level", options.expLevel)
	}
	if options.workType != "" {
		q.Add("work_type", options.workType)
	}
	if options.filterByCompany != "" {
		q.Add("company_id", options.filterByCompany)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var jobListings []JobListing
	if err := json.NewDecoder(resp.Body).Decode(&jobListings); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return jobListings, nil
}
