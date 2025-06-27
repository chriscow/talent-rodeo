package scrapingdog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"talent-rodeo/assets"
	"talent-rodeo/internal"

	"github.com/chriscow/minds/tools"
	"github.com/sashabaranov/go-openai"
)

// parseSalaryRange uses the LLM to extract a salary range from
// arbitrary text from the job listing
func parseSalaryRange(ctx context.Context, fs *internal.FirestoreClient, _ *slog.Logger, userID *string, basePay string) (*internal.SalaryRange, error) {
	if basePay == "" {
		return nil, fmt.Errorf("no base pay provided")
	}

	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/salary.parse-range.v001.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to read salary prompt: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"BasePay": basePay,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate prompt: %w", err)
	}

	resp := openai.ChatCompletionResponse{}
	salary, err := tools.StructuredAsk[internal.SalaryRange](ctx, prompt,
		tools.WithModel(tools.GPT41Nano), tools.WantCompletionResponse(&resp))
	if err != nil {
		return nil, fmt.Errorf("failed to ask: %w", err)
	}

	// Record LLM cost for salary parsing
	if err := internal.RecordLLMCost(ctx, fs, userID, string(tools.GPT41Nano), resp, "salary_parsing", nil); err != nil {
		return nil, fmt.Errorf("failed to record LLM cost: %w", err)
	}

	return &salary, nil
}

// convertJobListings converts provider-specific job listings to a common format
func convertJobListings(jobProviderName string, jobListings []LinkedInJobListing, criteria *internal.SearchCriteria) ([]internal.JobListing, error) {
	result := make([]internal.JobListing, len(jobListings))

	for i, job := range jobListings {
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

		result[i] = internal.JobListing{
			Provider:       jobProviderName, // linkedin, etc.
			ProviderJobID:  job.LinkedInJobID,
			Title:          job.Position,
			Company:        job.CompanyName,
			Location:       job.Location,
			PostedDate:     postedDate,
			URL:            job.Link,
			CompanyProfile: job.CompanyProfile,
			CompanyLogo:    job.CompanyLogo,
			CreatedAt:      time.Now(),
		}

	}

	return result, nil
}

// convertJobDetail converts provider-specific job details to a common format
func convertJobDetail(ctx context.Context, fs *internal.FirestoreClient, logger *slog.Logger, userID *string, detail LinkedInJobDetail) (*internal.JobDetail, error) {
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
	salaryRange, err := parseSalaryRange(ctx, fs, logger, userID, detail.BasePay)
	if err != nil {
		logger.Warn("failed to parse salary range", "error", err)
	}

	// Convert recruiters
	recruiters := make([]internal.Recruiter, len(detail.Recruiters))
	for i, r := range detail.Recruiters {
		recruiters[i] = internal.Recruiter{
			Photo:      r.Photo,
			Name:       r.Name,
			Title:      r.Title,
			ProfileURL: r.ProfileURL,
		}
	}

	// Convert similar jobs
	similarJobs := make([]internal.RelatedJob, len(detail.SimilarJobs))
	for i, j := range detail.SimilarJobs {
		similarJobs[i] = internal.RelatedJob{
			Title:       j.Position,
			Company:     j.Company,
			Location:    j.Location,
			PostingDate: j.PostingTime,
			URL:         j.Link,
		}
	}

	// Convert people also viewed
	peopleAlsoViewed := make([]internal.RelatedJob, len(detail.PeopleAlsoViewed))
	for i, j := range detail.PeopleAlsoViewed {
		peopleAlsoViewed[i] = internal.RelatedJob{
			Title:       j.Position,
			Company:     j.Company,
			Location:    j.Location,
			PostingDate: j.PostingTime,
			URL:         j.Link,
		}
	}

	result := internal.JobDetail{
		Provider:      JobProviderName,
		ProviderJobID: detail.ID,
		Title:         detail.Position,
		Company:       detail.CompanyName,
		Location:      detail.Location,
		PostedDate:    postedDate,
		// URL is set by the worker because the URL is part of the job listing
		ApplyURL:         detail.JobApplyLink,
		Description:      detail.Description,
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

	return &result, nil
}

// SummarizeJobDetails uses LLM to create a job summary directly on the JobDetail
func SummarizeJobDetails(ctx context.Context, fs *internal.FirestoreClient, logger *slog.Logger, userID *string, jobDetail *internal.JobDetail) error {

	jobJSON, err := json.MarshalIndent(jobDetail, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal job details: %w", err)
	}

	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/job-summary.001.txt")
	if err != nil {
		return fmt.Errorf("failed to read job summary prompt: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"JobDetail": string(jobJSON),
	})
	if err != nil {
		return fmt.Errorf("failed to generate prompt: %w", err)
	}

	resp := openai.ChatCompletionResponse{}
	summary, err := tools.StructuredAsk[internal.JobDetailExtractedData](ctx, prompt, tools.WantCompletionResponse(&resp))
	if err != nil {
		return fmt.Errorf("failed to ask: %w", err)
	}

	// Record LLM cost for job summary
	if err := internal.RecordLLMCost(ctx, fs, userID, string(tools.DeepSeekChat), resp, "job_summary", nil); err != nil {
		logger.Error("failed to record LLM cost for job summary", "error", err)
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
