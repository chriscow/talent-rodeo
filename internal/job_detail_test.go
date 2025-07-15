package internal

import (
	"encoding/json"
	"talent-rodeo/internal/utils"
	"testing"
)

type testHybridSearchParams struct {
	DenseWeight     float64
	SparseWeight    float64
	RecencyWeight   float64
	RecencyHalfLife float64
	ScoreThreshold  float64
	ScoreFusion     string
}

type testSteeringQuery struct {
	Queries         string
	RerankQuery     string
	ForceAway       float64
	MoveToward      string
	MoveAway        string
	WorkType        string
	ExperienceLevel string
}

func TestCacheKeyHashStability(t *testing.T) {
	params1 := testHybridSearchParams{
		DenseWeight:     1.0,
		SparseWeight:    2.0,
		RecencyWeight:   3.0,
		RecencyHalfLife: 4.0,
		ScoreThreshold:  5.0,
		ScoreFusion:     "sum",
	}
	params2 := testHybridSearchParams{
		DenseWeight:     1.0,
		SparseWeight:    2.0,
		RecencyWeight:   3.0,
		RecencyHalfLife: 4.0,
		ScoreThreshold:  5.0,
		ScoreFusion:     "sum",
	}

	b1, err := json.Marshal(params1)
	if err != nil {
		t.Fatalf("failed to marshal params1: %v", err)
	}
	b2, err := json.Marshal(params2)
	if err != nil {
		t.Fatalf("failed to marshal params2: %v", err)
	}
	hash1, err := utils.SHA256Hash(b1)
	if err != nil {
		t.Fatalf("failed to hash params1: %v", err)
	}
	hash2, err := utils.SHA256Hash(b2)
	if err != nil {
		t.Fatalf("failed to hash params2: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("expected identical params to produce same hash, got %s and %s", hash1, hash2)
	}

	steering1 := testSteeringQuery{
		Queries:         "foo",
		RerankQuery:     "bar",
		ForceAway:       1.23,
		MoveToward:      "baz",
		MoveAway:        "qux",
		WorkType:        "remote",
		ExperienceLevel: "senior",
	}
	steering2 := testSteeringQuery{
		Queries:         "foo",
		RerankQuery:     "bar",
		ForceAway:       1.23,
		MoveToward:      "baz",
		MoveAway:        "qux",
		WorkType:        "remote",
		ExperienceLevel: "senior",
	}

	b3, err := json.Marshal(steering1)
	if err != nil {
		t.Fatalf("failed to marshal steering1: %v", err)
	}
	b4, err := json.Marshal(steering2)
	if err != nil {
		t.Fatalf("failed to marshal steering2: %v", err)
	}
	hash3, err := utils.SHA256Hash(b3)
	if err != nil {
		t.Fatalf("failed to hash steering1: %v", err)
	}
	hash4, err := utils.SHA256Hash(b4)
	if err != nil {
		t.Fatalf("failed to hash steering2: %v", err)
	}

	if hash3 != hash4 {
		t.Errorf("expected identical steering queries to produce same hash, got %s and %s", hash3, hash4)
	}
}

func TestExtractLinkedInJobID(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectedID  string
		shouldError bool
	}{
		{
			name:        "valid LinkedIn job URL with query parameters",
			url:         "https://www.linkedin.com/jobs/view/staff-linux-devops-engineer-aws-remote-at-motion-recruitment-4249241958?refId=qKYEVK0CRSf%2Fg7%2BjxPLhGQ%3D%3D&trackingId=CSD7kLoSCliAfuqFK3e8uQ%3D%3D",
			expectedID:  "4249241958",
			shouldError: false,
		},
		{
			name:        "valid LinkedIn job URL without query parameters",
			url:         "https://www.linkedin.com/jobs/view/senior-software-engineer-3752841205",
			expectedID:  "3752841205",
			shouldError: false,
		},
		{
			name:        "LinkedIn job URL with complex job title",
			url:         "https://www.linkedin.com/jobs/view/senior-full-stack-engineer-remote-work-from-home-1234567890",
			expectedID:  "1234567890",
			shouldError: false,
		},
		{
			name:        "LinkedIn job URL with very long job ID",
			url:         "https://www.linkedin.com/jobs/view/software-engineer-12345678901234567890",
			expectedID:  "12345678901234567890",
			shouldError: false,
		},
		{
			name:        "empty URL",
			url:         "",
			expectedID:  "",
			shouldError: true,
		},
		{
			name:        "URL without job ID",
			url:         "https://www.linkedin.com/jobs/view/software-engineer",
			expectedID:  "",
			shouldError: true,
		},
		{
			name:        "URL with short number (not a job ID)",
			url:         "https://www.linkedin.com/jobs/view/engineer-123",
			expectedID:  "",
			shouldError: true,
		},
		{
			name:        "non-LinkedIn URL",
			url:         "https://www.indeed.com/viewjob?jk=1234567890",
			expectedID:  "",
			shouldError: true,
		},
		{
			name:        "malformed URL",
			url:         "not-a-url",
			expectedID:  "",
			shouldError: true,
		},
		{
			name:        "LinkedIn URL with job ID at end",
			url:         "https://www.linkedin.com/jobs/view/9876543210",
			expectedID:  "9876543210",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractLinkedInJobID(tt.url)

			if tt.shouldError {
				if err == nil {
					t.Errorf("ExtractLinkedInJobID() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("ExtractLinkedInJobID() unexpected error: %v", err)
				}
				if result != tt.expectedID {
					t.Errorf("ExtractLinkedInJobID() = %v, expected %v", result, tt.expectedID)
				}
			}
		})
	}
}

func TestExtractRelatedJobIDs(t *testing.T) {
	tests := []struct {
		name             string
		similarJobs      []RelatedJob
		peopleAlsoViewed []RelatedJob
		expectedIDs      []string
		expectedCount    int
	}{
		{
			name: "extract IDs from both similar jobs and people also viewed",
			similarJobs: []RelatedJob{
				{URL: "https://www.linkedin.com/jobs/view/software-engineer-1111111111"},
				{URL: "https://www.linkedin.com/jobs/view/data-scientist-2222222222"},
			},
			peopleAlsoViewed: []RelatedJob{
				{URL: "https://www.linkedin.com/jobs/view/product-manager-3333333333"},
				{URL: "https://www.linkedin.com/jobs/view/ux-designer-4444444444"},
			},
			expectedIDs:   []string{"1111111111", "2222222222", "3333333333", "4444444444"},
			expectedCount: 4,
		},
		{
			name: "handle duplicate job IDs",
			similarJobs: []RelatedJob{
				{URL: "https://www.linkedin.com/jobs/view/software-engineer-1111111111"},
				{URL: "https://www.linkedin.com/jobs/view/data-scientist-2222222222"},
			},
			peopleAlsoViewed: []RelatedJob{
				{URL: "https://www.linkedin.com/jobs/view/software-engineer-duplicate-1111111111"}, // Same ID as first
				{URL: "https://www.linkedin.com/jobs/view/ux-designer-3333333333"},
			},
			expectedIDs:   []string{"1111111111", "2222222222", "3333333333"},
			expectedCount: 3,
		},
		{
			name: "ignore invalid URLs",
			similarJobs: []RelatedJob{
				{URL: "https://www.linkedin.com/jobs/view/software-engineer-1111111111"},
				{URL: "invalid-url"},
				{URL: "https://www.indeed.com/job/123"}, // Non-LinkedIn URL
			},
			peopleAlsoViewed: []RelatedJob{
				{URL: "https://www.linkedin.com/jobs/view/data-scientist-2222222222"},
				{URL: ""}, // Empty URL
			},
			expectedIDs:   []string{"1111111111", "2222222222"},
			expectedCount: 2,
		},
		{
			name:             "empty job arrays",
			similarJobs:      []RelatedJob{},
			peopleAlsoViewed: []RelatedJob{},
			expectedIDs:      []string{},
			expectedCount:    0,
		},
		{
			name: "only similar jobs",
			similarJobs: []RelatedJob{
				{URL: "https://www.linkedin.com/jobs/view/backend-engineer-5555555555"},
			},
			peopleAlsoViewed: []RelatedJob{},
			expectedIDs:      []string{"5555555555"},
			expectedCount:    1,
		},
		{
			name:        "only people also viewed",
			similarJobs: []RelatedJob{},
			peopleAlsoViewed: []RelatedJob{
				{URL: "https://www.linkedin.com/jobs/view/frontend-engineer-6666666666"},
				{URL: "https://www.linkedin.com/jobs/view/devops-engineer-7777777777"},
			},
			expectedIDs:   []string{"6666666666", "7777777777"},
			expectedCount: 2,
		},
		{
			name: "mixed valid and invalid URLs with query parameters",
			similarJobs: []RelatedJob{
				{URL: "https://www.linkedin.com/jobs/view/staff-engineer-8888888888?refId=test&trackingId=test"},
			},
			peopleAlsoViewed: []RelatedJob{
				{URL: "https://www.linkedin.com/jobs/view/invalid"}, // Too short
				{URL: "https://www.linkedin.com/jobs/view/senior-manager-9999999999?utm_source=linkedin"},
			},
			expectedIDs:   []string{"8888888888", "9999999999"},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractRelatedJobIDs(tt.similarJobs, tt.peopleAlsoViewed)

			if len(result) != tt.expectedCount {
				t.Errorf("ExtractRelatedJobIDs() returned %d IDs, expected %d", len(result), tt.expectedCount)
			}

			// Check that all expected IDs are present
			resultMap := make(map[string]bool)
			for _, id := range result {
				resultMap[id] = true
			}

			for _, expectedID := range tt.expectedIDs {
				if !resultMap[expectedID] {
					t.Errorf("ExtractRelatedJobIDs() missing expected ID: %s", expectedID)
				}
			}

			// Check for unexpected IDs
			expectedMap := make(map[string]bool)
			for _, id := range tt.expectedIDs {
				expectedMap[id] = true
			}

			for _, resultID := range result {
				if !expectedMap[resultID] {
					t.Errorf("ExtractRelatedJobIDs() returned unexpected ID: %s", resultID)
				}
			}
		})
	}
}

func TestMinRatingForRelatedJobsConstant(t *testing.T) {
	// Test that the constant is properly defined
	if MinRatingForRelatedJobs != 4 {
		t.Errorf("MinRatingForRelatedJobs = %d, expected 3", MinRatingForRelatedJobs)
	}
}
