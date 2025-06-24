package scrapingdog

import (
	"context"
	"talent-rodeo/internal"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSalaryRange(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name             string
		salary           string
		expectedMin      float64
		expectedMax      float64
		expectedCurrency string
		expectError      bool
	}{
		{
			name:             "Simple range",
			salary:           "$50K-$70K",
			expectedMin:      50000,
			expectedMax:      70000,
			expectedCurrency: "USD",
			expectError:      false,
		},
		{
			name:             "Range with commas",
			salary:           "$50,000-$70,000",
			expectedMin:      50000,
			expectedMax:      70000,
			expectedCurrency: "USD",
			expectError:      false,
		},
		{
			name:             "Range with k lowercase",
			salary:           "$50k-$70k",
			expectedMin:      50000,
			expectedMax:      70000,
			expectedCurrency: "USD",
			expectError:      false,
		},
		{
			name:             "Range with thousand word",
			salary:           "$50 thousand-$70 thousand",
			expectedMin:      50000,
			expectedMax:      70000,
			expectedCurrency: "USD",
			expectError:      false,
		},
		{
			name:             "Single value",
			salary:           "$70K",
			expectedMin:      70000,
			expectedMax:      70000,
			expectedCurrency: "USD",
			expectError:      false,
		},
		{
			name:             "Empty string",
			salary:           "",
			expectedMin:      0,
			expectedMax:      0,
			expectedCurrency: "",
			expectError:      false,
		},
		{
			name:             "Invalid format",
			salary:           "not a salary",
			expectedMin:      0,
			expectedMax:      0,
			expectedCurrency: "",
			expectError:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseSalaryRange(ctx, nil, nil, nil, tc.salary)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			if tc.salary == "" {
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tc.expectedMin, result.Min)
			assert.Equal(t, tc.expectedMax, result.Max)
			assert.Equal(t, tc.expectedCurrency, result.Currency)
		})
	}
}

func TestConvertJobListings(t *testing.T) {
	now := time.Now()

	// Create test data
	jobListings := []LinkedInJobListing{
		{
			ID:             "1",
			LinkedInJobID:  "linkedin-1",
			Position:       "Go Developer",
			Link:           "https://example.com/jobs/1",
			CompanyName:    "Example Company",
			Location:       "Remote",
			PostingDate:    "1d ago",
			CompanyLogo:    "https://example.com/logo.png",
			CompanyProfile: "https://example.com/company/1",
		},
		{
			ID:             "2",
			LinkedInJobID:  "linkedin-2",
			Position:       "Senior Go Developer",
			Link:           "https://example.com/jobs/2",
			CompanyName:    "Another Company",
			Location:       "New York",
			PostingDate:    "3h ago",
			CompanyLogo:    "https://example.com/logo2.png",
			CompanyProfile: "https://example.com/company/2",
		},
	}

	// Create test search criteria
	criteria := &internal.SearchCriteria{
		Query: "go developer",
		GeoID: "remote",
	}

	// Convert job listings
	result, err := convertJobListings(JobProviderName, jobListings, criteria)
	require.NoError(t, err)
	require.Len(t, result, 2)

	// Verify first job
	assert.Equal(t, "scrapingdog:linkedin:linkedin-1", result[0].ID)
	assert.Equal(t, JobProviderName, result[0].Provider)
	assert.Equal(t, "linkedin-1", result[0].ProviderJobID)
	assert.Equal(t, "Go Developer", result[0].Title)
	assert.Equal(t, "Example Company", result[0].Company)
	assert.Equal(t, "Remote", result[0].Location)
	assert.True(t, result[0].PostedDate.Before(now) || result[0].PostedDate.Equal(now), "PostedDate should be in the past or now")
	assert.Equal(t, "https://example.com/jobs/1", result[0].URL)
	assert.Equal(t, "https://example.com/company/1", result[0].CompanyProfile)
	assert.Equal(t, "https://example.com/logo.png", result[0].CompanyLogo)
	assert.Equal(t, criteria.GetSearchHash(), result[0].SearchHash)

	// Verify second job
	assert.Equal(t, "scrapingdog:linkedin:linkedin-2", result[1].ID)
	assert.Equal(t, "Senior Go Developer", result[1].Title)
	assert.Equal(t, "Another Company", result[1].Company)
	assert.Equal(t, "New York", result[1].Location)
	assert.True(t, result[1].PostedDate.Before(now) || result[1].PostedDate.Equal(now), "PostedDate should be in the past or now")
	assert.Equal(t, criteria.GetSearchHash(), result[1].SearchHash)
}

func TestConvertJobDetail(t *testing.T) {
	now := time.Now()

	// Create test job detail
	jobDetail := LinkedInJobDetail{
		ID:                "detail-1",
		Position:          "Senior Go Developer",
		Status:            "active",
		Location:          "Remote",
		CompanyName:       "Example Company",
		CompanyLinkedInID: "company-1",
		PostingTime:       "2d ago",
		BasePay:           "$80K-$120K",
		Description:       "This is a test job description.",
		SeniorityLevel:    "Senior",
		EmploymentType:    "Full-time",
		JobFunction:       "Engineering",
		Industries:        "Software",
		Recruiters: []Recruiter{
			{
				Photo:      "https://example.com/photo.jpg",
				Name:       "John Recruiter",
				Title:      "Talent Acquisition",
				ProfileURL: "https://example.com/recruiter/1",
			},
		},
		SimilarJobs: []RelatedJob{
			{
				Position:    "Similar Job",
				Company:     "Similar Company",
				Location:    "Remote",
				PostingTime: "3d ago",
				Link:        "https://example.com/similar/1",
			},
		},
		PeopleAlsoViewed: []RelatedJob{
			{
				Position:    "Viewed Job",
				Company:     "Viewed Company",
				Location:    "Remote",
				PostingTime: "4d ago",
				Link:        "https://example.com/viewed/1",
			},
		},
	}

	// Convert job detail
	result, err := convertJobDetail(context.Background(), nil, nil, nil, jobDetail)
	require.NoError(t, err)

	// Verify basic fields
	assert.Equal(t, "scrapingdog:linkedin:detail-1", result.ID)
	assert.Equal(t, JobProviderName, result.Provider)
	assert.Equal(t, "detail-1", result.ProviderJobID)
	assert.Equal(t, "Senior Go Developer", result.Title)
	assert.Equal(t, "Example Company", result.Company)
	assert.Equal(t, "Remote", result.Location)
	assert.True(t, result.PostedDate.Before(now) || result.PostedDate.Equal(now), "PostedDate should be in the past or now")
	assert.Equal(t, "This is a test job description.", result.Description)
	assert.Equal(t, "Full-time", result.EmploymentType)
	assert.Equal(t, "active", result.Status)
	assert.Equal(t, "Senior", result.SeniorityLevel)
	assert.Equal(t, "Engineering", result.JobFunction)
	assert.Equal(t, "Software", result.Industry)

	// Verify salary
	require.NotNil(t, result.SalaryInfo, "SalaryInfo should not be nil")
	assert.Equal(t, 80000.0, result.SalaryInfo.Min)
	assert.Equal(t, 120000.0, result.SalaryInfo.Max)
	assert.Equal(t, "USD", result.SalaryInfo.Currency)

	// Verify recruiters
	require.Len(t, result.Recruiters, 1)
	assert.Equal(t, "John Recruiter", result.Recruiters[0].Name)
	assert.Equal(t, "Talent Acquisition", result.Recruiters[0].Title)
	assert.Equal(t, "https://example.com/photo.jpg", result.Recruiters[0].Photo)
	assert.Equal(t, "https://example.com/recruiter/1", result.Recruiters[0].ProfileURL)

	// Verify similar jobs
	require.Len(t, result.SimilarJobs, 1)
	assert.Equal(t, "Similar Job", result.SimilarJobs[0].Title)
	assert.Equal(t, "Similar Company", result.SimilarJobs[0].Company)
	assert.Equal(t, "Remote", result.SimilarJobs[0].Location)
	assert.Equal(t, "3d ago", result.SimilarJobs[0].PostingDate)
	assert.Equal(t, "https://example.com/similar/1", result.SimilarJobs[0].URL)

	// Verify people also viewed
	require.Len(t, result.PeopleAlsoViewed, 1)
	assert.Equal(t, "Viewed Job", result.PeopleAlsoViewed[0].Title)
	assert.Equal(t, "Viewed Company", result.PeopleAlsoViewed[0].Company)
	assert.Equal(t, "Remote", result.PeopleAlsoViewed[0].Location)
}

func TestParsePostedDate(t *testing.T) {
	now := time.Now()

	testCases := []struct {
		name             string
		dateStr          string
		expectedDuration time.Duration
		expectError      bool
		isMonth          bool // New field to indicate if this is a month test
	}{
		{
			name:             "Hours ago",
			dateStr:          "2h ago",
			expectedDuration: 2 * time.Hour,
			expectError:      false,
		},
		{
			name:             "Days ago",
			dateStr:          "3d ago",
			expectedDuration: 3 * 24 * time.Hour,
			expectError:      false,
		},
		{
			name:             "Weeks ago",
			dateStr:          "2w ago",
			expectedDuration: 2 * 7 * 24 * time.Hour,
			expectError:      false,
		},
		{
			name:             "Months ago",
			dateStr:          "1m ago",
			expectedDuration: 30 * 24 * time.Hour, // Approximate
			expectError:      false,
			isMonth:          true,
		},
		{
			name:             "Years ago",
			dateStr:          "1y ago",
			expectedDuration: 365 * 24 * time.Hour, // Approximate
			expectError:      false,
		},
		{
			name:             "Just now",
			dateStr:          "just now",
			expectedDuration: 0,
			expectError:      false,
		},
		{
			name:             "Now",
			dateStr:          "now",
			expectedDuration: 0,
			expectError:      false,
		},
		{
			name:             "Invalid format",
			dateStr:          "invalid",
			expectedDuration: 0,
			expectError:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parsePostedDate(tc.dateStr)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tc.expectedDuration == 0 {
				// "now" or "just now" cases
				timeDiff := now.Sub(result)
				assert.Less(t, timeDiff.Seconds(), 1.0, "Time difference should be less than 1 second")
				return
			}

			// For month tests, we need to be more flexible with our comparison
			if tc.isMonth {
				// For months, we'll accept a range between 28 and 31 days
				minDuration := 28 * 24 * time.Hour
				maxDuration := 31 * 24 * time.Hour
				timeDiff := now.Sub(result)
				assert.True(t, timeDiff >= minDuration && timeDiff <= maxDuration,
					"Month duration should be between 28 and 31 days, got %v", timeDiff)
				return
			}

			// Check that the result is approximately correct
			// Allow for a 1-minute tolerance to account for test execution time
			timeDiff := now.Sub(result)
			tolerance := 1 * time.Minute
			expectedDiff := tc.expectedDuration

			assert.InDelta(t, expectedDiff.Seconds(), timeDiff.Seconds(), tolerance.Seconds(),
				"Time difference should be approximately %v (actual: %v)", expectedDiff, timeDiff)
		})
	}
}
