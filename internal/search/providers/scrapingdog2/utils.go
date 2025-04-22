package scrapingdog2

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/search"
)

// CreateProviderJobID formats a provider job ID to ensure uniqueness across providers
func CreateProviderJobID(providerJobID string) string {
	return funcs.Slugify(fmt.Sprintf("%s %s", ProviderName, providerJobID))
}

// ParseSalaryRange parses a salary string into a structured SalaryRange object
func ParseSalaryRange(ctx context.Context, salary string) (*search.SalaryRange, error) {
	if salary == "" {
		return nil, nil
	}

	// Match patterns like "$50K-$70K", "$50,000-$70,000", etc.
	salaryRangeRegex := regexp.MustCompile(`\$?(\d+(?:,\d+)?(?:\.\d+)?)\s*(?:K|k|thousand)?\s*-\s*\$?(\d+(?:,\d+)?(?:\.\d+)?)\s*(?:K|k|thousand)?`)
	matches := salaryRangeRegex.FindStringSubmatch(salary)

	if len(matches) < 3 {
		// Try single value format like "$70K"
		singleValueRegex := regexp.MustCompile(`\$?(\d+(?:,\d+)?(?:\.\d+)?)\s*(?:K|k|thousand)?`)
		matches = singleValueRegex.FindStringSubmatch(salary)
		if len(matches) < 2 {
			return nil, fmt.Errorf("unable to parse salary: %s", salary)
		}
		// For single value, use the same value for min and max
		matches = append(matches, matches[1])
	}

	// Clean and convert to number
	cleanValue := func(val string) (float64, error) {
		// Remove commas
		val = strings.ReplaceAll(val, ",", "")

		// Check if the value is in thousands (K)
		isThousands := strings.Contains(strings.ToLower(val), "k") ||
			strings.Contains(strings.ToLower(val), "thousand")

		// Remove K or thousand
		val = regexp.MustCompile(`(?i)k|thousand`).ReplaceAllString(val, "")

		// Convert to float
		num, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			return 0, err
		}

		// Multiply by 1000 if value is in thousands
		if isThousands {
			num *= 1000
		}

		return num, nil
	}

	min, err := cleanValue(matches[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse min salary: %w", err)
	}

	max, err := cleanValue(matches[2])
	if err != nil {
		return nil, fmt.Errorf("failed to parse max salary: %w", err)
	}

	return &search.SalaryRange{
		Min:      min,
		Max:      max,
		Currency: "USD",    // Assuming USD as default
		Period:   "yearly", // Assuming yearly as default
	}, nil
}

// ConvertJobListings converts provider-specific job listings to a common format
func ConvertJobListings(jobListings []JobListing) ([]search.JobListing, error) {
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
func ConvertJobDetail(detail JobDetail) (search.JobDetail, error) {
	// Format provider job ID
	providerJobID := CreateProviderJobID(detail.ID)

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
		// Log the error but don't fail the conversion
		salaryRange = nil
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
		ID:               providerJobID,
		Provider:         ProviderName,
		ProviderJobID:    detail.ID,
		Title:            detail.Position,
		Company:          detail.CompanyName,
		Location:         detail.Location,
		PostedDate:       postedDate,
		URL:              "", // Not available in detail
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
	case "m": // Months
		return now.AddDate(0, -num, 0), nil
	case "y": // Years
		return now.AddDate(-num, 0, 0), nil
	default:
		return time.Time{}, fmt.Errorf("unknown time unit: %s", unit)
	}
}
