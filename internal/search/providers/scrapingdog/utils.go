package scrapingdog

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/search"
)

// CreateProviderJobID formats a provider job ID to ensure uniqueness across providers
func CreateProviderJobID(providerJobID string) string {
	if providerJobID == "" {
		return ProviderName
	}
	// First slugify the ID to handle spaces and basic special characters
	id := funcs.Slugify(providerJobID)
	// Then replace any remaining underscores with hyphens
	id = strings.ReplaceAll(id, "_", "-")
	return fmt.Sprintf("%s-%s", ProviderName, id)
}

// ParseSalaryRange parses a salary string into a structured SalaryRange object
func ParseSalaryRange(ctx context.Context, basePay string) (*search.SalaryRange, error) {
	if basePay == "" {
		return nil, nil
	}

	// Basic validation for salary format
	if !strings.Contains(basePay, "$") && !strings.Contains(strings.ToLower(basePay), "k") &&
		!strings.Contains(basePay, ",") && !strings.Contains(strings.ToLower(basePay), "thousand") {
		return nil, fmt.Errorf("invalid salary format: %s", basePay)
	}

	prompt, err := internal.GeneratePrompt("prompts/match-000.salary.txt", map[string]any{
		"BasePay": basePay,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate prompt: %w", err)
	}

	salary, err := internal.StructuredAsk[search.SalaryRange](ctx, "salary_data", prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to ask: %w", err)
	}

	return &salary, nil
}

// ConvertJobListings converts provider-specific job listings to a common format
func ConvertJobListings(jobListings []LinkedInJobListing) ([]search.JobListing, error) {
	result := make([]search.JobListing, len(jobListings))

	for i, job := range jobListings {
		// Format provider job ID
		providerJobID := CreateProviderJobID(job.LinkedInJobID)

		// Parse posting date
		var postedDate time.Time
		var err error
		if job.PostingDate != "" {
			// Convert strings like "1h ago", "2d ago", etc. to time.Time
			postedDate, err = parsePostedDate(job.PostingDate)
			if err != nil {
				postedDate = time.Now() // Default to current time if parsing fails
			}
		} else {
			postedDate = time.Now()
		}

		result[i] = search.JobListing{
			ID:             providerJobID,
			Provider:       ProviderName,
			ProviderJobID:  job.LinkedInJobID,
			Title:          job.Position,
			Company:        job.CompanyName,
			Location:       job.Location,
			PostedDate:     postedDate,
			URL:            job.Link,
			CompanyProfile: job.CompanyProfile,
			CompanyLogo:    job.CompanyLogo,
			FirstSeenAt:    time.Now(),
		}
	}

	return result, nil
}

// ConvertJobDetail converts provider-specific job details to a common format
func ConvertJobDetail(detail LinkedInJobDetail) (search.JobDetail, error) {
	// Use the raw ID from the detail
	rawID := detail.ID

	// Parse posting date
	var postedDate time.Time
	var err error
	if detail.PostingTime != "" {
		postedDate, err = parsePostedDate(detail.PostingTime)
		if err != nil {
			postedDate = time.Now() // Default to current time if parsing fails
		}
	} else {
		postedDate = time.Now()
	}

	// Parse salary range
	salaryRange, err := ParseSalaryRange(context.Background(), detail.BasePay)
	if err != nil {
		// noop - we don't want to fail the conversion
	}

	// Convert recruiters
	recruiters := make([]search.Recruiter, len(detail.Recruiters))
	for i, r := range detail.Recruiters {
		recruiters[i] = search.Recruiter{
			Photo:      r.Photo,
			Name:       r.Name,
			Title:      r.Title,
			ProfileURL: r.ProfileURL,
		}
	}

	// Convert similar jobs
	similarJobs := make([]search.RelatedJob, len(detail.SimilarJobs))
	for i, j := range detail.SimilarJobs {
		similarJobs[i] = search.RelatedJob{
			Title:       j.Position,
			Company:     j.Company,
			Location:    j.Location,
			PostingDate: j.PostingTime,
			URL:         j.Link,
		}
	}

	// Convert people also viewed
	peopleAlsoViewed := make([]search.RelatedJob, len(detail.PeopleAlsoViewed))
	for i, j := range detail.PeopleAlsoViewed {
		peopleAlsoViewed[i] = search.RelatedJob{
			Title:       j.Position,
			Company:     j.Company,
			Location:    j.Location,
			PostingDate: j.PostingTime,
			URL:         j.Link,
		}
	}

	result := search.JobDetail{
		ID:               CreateProviderJobID(rawID),
		Provider:         ProviderName,
		ProviderJobID:    rawID,
		Title:            detail.Position,
		Company:          detail.CompanyName,
		Location:         detail.Location,
		PostedDate:       postedDate,
		URL:              "",
		FullDescription:  detail.Description,
		SalaryInfo:       salaryRange,
		EmploymentType:   detail.EmploymentType,
		Status:           detail.Status,
		SeniorityLevel:   detail.SeniorityLevel,
		JobFunction:      detail.JobFunction,
		Industry:         detail.Industries,
		Recruiters:       recruiters,
		SimilarJobs:      similarJobs,
		PeopleAlsoViewed: peopleAlsoViewed,
		CreatedAt:        time.Now(),
	}

	return result, nil
}

// SummarizeJobDetails uses LLM to create a job summary directly on the JobDetail
func SummarizeJobDetails(ctx context.Context, jobDetail *search.JobDetail) error {

	jobJSON, err := json.MarshalIndent(jobDetail, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal job details: %w", err)
	}

	prompt, err := internal.GeneratePrompt("prompts/job-summary.001.txt", map[string]any{
		"JobDetail": string(jobJSON),
	})
	if err != nil {
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	summary, err := internal.StructuredAsk[search.ExtractedData](ctx, "job_summary", prompt)
	if err != nil {
		return fmt.Errorf("failed to ask: %w", err)
	}

	jobDetail.ExtractedData = &summary

	return nil
}

// parsePostedDate converts a string like "1h ago", "2d ago" to a time.Time
func parsePostedDate(dateStr string) (time.Time, error) {
	now := time.Now()

	// Clean up the string
	dateStr = strings.ToLower(strings.TrimSpace(dateStr))

	// Check for common patterns
	if dateStr == "now" || dateStr == "just now" {
		return now, nil
	}

	// Match patterns like "2h ago", "3d ago", "1w ago"
	re := regexp.MustCompile(`(\d+)\s*([hdwmy])\w*\s+ago`)
	matches := re.FindStringSubmatch(dateStr)

	if len(matches) < 3 {
		return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
	}

	// Extract number and unit
	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Time{}, err
	}

	unit := matches[2]

	// Calculate the time
	switch unit {
	case "h": // Hours
		return now.Add(-time.Duration(num) * time.Hour), nil
	case "d": // Days
		return now.AddDate(0, 0, -num), nil
	case "w": // Weeks
		return now.AddDate(0, 0, -num*7), nil
	case "m": // Months (using exact month calculation)
		return now.AddDate(0, -num, 0), nil
	case "y": // Years
		return now.AddDate(-num, 0, 0), nil
	default:
		return time.Time{}, fmt.Errorf("unknown time unit: %s", unit)
	}
}
