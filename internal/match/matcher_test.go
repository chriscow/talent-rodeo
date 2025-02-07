package match

import (
	"context"
	"fmt"
	"math"
	"talent-rodeo/internal/search"
	"testing"

	"github.com/joho/godotenv"
)

func init() {
	godotenv.Load("/Users/chris/dev/talent-rodeo/.env")
}

// MockJobProvider is a mock implementation of search.JobProvider
type MockJobProvider struct {
	name       string
	jobDetails map[string]*search.JobDetail
}

func NewMockJobProvider(name string) *MockJobProvider {
	return &MockJobProvider{
		name:       name,
		jobDetails: make(map[string]*search.JobDetail),
	}
}

func (m *MockJobProvider) SearchJobs(ctx context.Context, criteria search.Criteria) ([]search.Job, error) {
	// For testing, return empty slice
	return []search.Job{}, nil
}

func (m *MockJobProvider) GetJobDetails(ctx context.Context, jobID string) (*search.JobDetail, error) {
	// Return standard test job details
	return &search.JobDetail{
		Job: search.Job{
			ID:            jobID,
			Title:         "Test Job",
			Company:       "Test Company",
			Location:      "Test Location",
			Description:   "Test Description",
			URL:           "https://example.com",
			Provider:      m.name,
			ProviderJobID: jobID,
		},
		FullDescription: "Test full description with requirements for a senior software engineer with experience in Go, Docker, and Kubernetes.",
		Requirements:    []string{"Go programming", "Docker experience", "Kubernetes experience"},
		Qualifications:  []string{"Bachelor's degree", "5+ years experience"},
		Benefits:        []string{"Health insurance", "401k"},
		Skills:          []string{"Go", "Docker", "Kubernetes"},
		SalaryInfo: &search.SalaryRange{
			Min:      100000,
			Max:      150000,
			Currency: "USD",
			Period:   "yearly",
		},
		EmploymentType: "full-time",
		WorkplaceType:  "remote",
		Industry:       "Technology",
		CompanyInfo: &search.Company{
			Name:        "Test Company",
			Industry:    "Technology",
			Size:        "medium",
			Description: "Leading technology company",
		},
	}, nil
}

func (m *MockJobProvider) Name() string {
	return m.name
}

func (m *MockJobProvider) Validate(ctx context.Context) error {
	return nil
}

func setupMatcher(t *testing.T) *Matcher {
	ctx := context.Background()
	matcher, err := NewMatcher(ctx, nil)
	if err != nil {
		t.Fatalf("NewMatcher() failed: %v", err)
	}
	if matcher == nil {
		t.Fatal("NewMatcher() returned nil matcher")
	}

	// Create and register mock provider
	mockProvider := NewMockJobProvider("test-provider")
	err = matcher.RegisterProvider(mockProvider)
	if err != nil {
		t.Fatalf("RegisterProvider() failed: %v", err)
	}

	return matcher
}

func TestMatcher_PerfectMatch(t *testing.T) {
	matcher := setupMatcher(t)
	ctx := context.Background()

	job := &search.Job{
		ID:          "job1",
		Title:       "Senior Go Developer",
		Company:     "TechCorp",
		Location:    "Remote",
		Description: "Looking for a Senior Go Developer with 5+ years of experience in Go, Docker, and Kubernetes.",
		Provider:    "test-provider",
	}
	candidate := &Candidate{
		ID:                 "cand1",
		Title:              "Senior Software Engineer",
		Skills:             []string{"Go", "Docker", "Kubernetes", "AWS"},
		ExperienceYears:    7,
		PreferredLocations: []string{"Remote"},
	}

	got, err := matcher.Match(ctx, job, candidate)
	if err != nil {
		t.Fatalf("Match() error: %v", err)
	}
	if got == nil {
		t.Fatal("Match() returned nil details")
	}

	// Check overall score is within acceptable range
	if !withinDelta(got.Score.Overall, 0.9, 0.2) {
		t.Errorf("overall score = %.2f, want %.2f (±0.2)", got.Score.Overall, 0.9)
	}

	// Verify matched skills
	expectedSkills := []string{"Go", "Docker", "Kubernetes"}
	if !containsAll(got.MatchedSkills, expectedSkills) {
		t.Errorf("matched skills = %v, want to include %v", got.MatchedSkills, expectedSkills)
	}

	// Verify score components
	if err := validateScoreComponent("Skills", got.Score.Skills); err != nil {
		t.Error(err)
	}
	if err := validateScoreComponent("Experience", got.Score.Experience); err != nil {
		t.Error(err)
	}
	if err := validateScoreComponent("Location", got.Score.Location); err != nil {
		t.Error(err)
	}
	if err := validateScoreComponent("Preferences", got.Score.Preferences); err != nil {
		t.Error(err)
	}

	// Verify analysis is generated
	if got.Analysis == "" {
		t.Error("Match() analysis should not be empty")
	}

	// Log detailed results for debugging
	t.Log("Match Results for perfect_match:")
	t.Logf("Overall Score: %.2f", got.Score.Overall)
	t.Logf("Skills Score: %.2f", got.Score.Skills)
	t.Logf("Experience Score: %.2f", got.Score.Experience)
	t.Logf("Location Score: %.2f", got.Score.Location)
	t.Logf("Matched Skills: %v", got.MatchedSkills)
	t.Logf("Missing Skills: %v", got.MissingSkills)
	t.Logf("Analysis:\n%s", got.Analysis)
}

func TestMatcher_PartialMatch(t *testing.T) {
	matcher := setupMatcher(t)
	ctx := context.Background()

	job := &search.Job{
		ID:          "job2",
		Title:       "Senior Go Developer",
		Company:     "TechCorp",
		Location:    "San Francisco",
		Description: "Looking for a Senior Go Developer with 5+ years of experience in Go, Docker, and Kubernetes.",
		Provider:    "test-provider",
	}
	candidate := &Candidate{
		ID:                 "cand2",
		Title:              "Software Engineer",
		Skills:             []string{"Go", "Python", "AWS"},
		ExperienceYears:    3,
		PreferredLocations: []string{"San Francisco", "Remote"},
	}

	got, err := matcher.Match(ctx, job, candidate)
	if err != nil {
		t.Fatalf("Match() error: %v", err)
	}
	if got == nil {
		t.Fatal("Match() returned nil details")
	}

	// Check overall score is within acceptable range
	if !withinDelta(got.Score.Overall, 0.6, 0.2) {
		t.Errorf("overall score = %.2f, want %.2f (±0.2)", got.Score.Overall, 0.6)
	}

	// Verify matched skills
	expectedSkills := []string{"Go"}
	if !containsAll(got.MatchedSkills, expectedSkills) {
		t.Errorf("matched skills = %v, want to include %v", got.MatchedSkills, expectedSkills)
	}

	// Verify score components
	if err := validateScoreComponent("Skills", got.Score.Skills); err != nil {
		t.Error(err)
	}
	if err := validateScoreComponent("Experience", got.Score.Experience); err != nil {
		t.Error(err)
	}
	if err := validateScoreComponent("Location", got.Score.Location); err != nil {
		t.Error(err)
	}
	if err := validateScoreComponent("Preferences", got.Score.Preferences); err != nil {
		t.Error(err)
	}

	// Verify analysis is generated
	if got.Analysis == "" {
		t.Error("Match() analysis should not be empty")
	}

	// Log detailed results for debugging
	t.Log("Match Results for partial_match:")
	t.Logf("Overall Score: %.2f", got.Score.Overall)
	t.Logf("Skills Score: %.2f", got.Score.Skills)
	t.Logf("Experience Score: %.2f", got.Score.Experience)
	t.Logf("Location Score: %.2f", got.Score.Location)
	t.Logf("Matched Skills: %v", got.MatchedSkills)
	t.Logf("Missing Skills: %v", got.MissingSkills)
	t.Logf("Analysis:\n%s", got.Analysis)
}

func TestMatcher_LocationMismatch(t *testing.T) {
	matcher := setupMatcher(t)
	ctx := context.Background()

	job := &search.Job{
		ID:          "job3",
		Title:       "Senior Go Developer",
		Company:     "TechCorp",
		Location:    "New York",
		Description: "Looking for a Senior Go Developer with 5+ years of experience in Go, Docker, and Kubernetes.",
		Provider:    "test-provider",
	}
	candidate := &Candidate{
		ID:                 "cand3",
		Title:              "Senior Software Engineer",
		Skills:             []string{"Go", "Docker", "Kubernetes"},
		ExperienceYears:    7,
		PreferredLocations: []string{"San Francisco", "Remote"},
	}

	got, err := matcher.Match(ctx, job, candidate)
	if err != nil {
		t.Fatalf("Match() error: %v", err)
	}
	if got == nil {
		t.Fatal("Match() returned nil details")
	}

	// Check overall score is within acceptable range
	if !withinDelta(got.Score.Overall, 0.7, 0.2) {
		t.Errorf("overall score = %.2f, want %.2f (±0.2)", got.Score.Overall, 0.7)
	}

	// Verify matched skills
	expectedSkills := []string{"Go", "Docker", "Kubernetes"}
	if !containsAll(got.MatchedSkills, expectedSkills) {
		t.Errorf("matched skills = %v, want to include %v", got.MatchedSkills, expectedSkills)
	}

	// Verify score components
	if err := validateScoreComponent("Skills", got.Score.Skills); err != nil {
		t.Error(err)
	}
	if err := validateScoreComponent("Experience", got.Score.Experience); err != nil {
		t.Error(err)
	}
	if err := validateScoreComponent("Location", got.Score.Location); err != nil {
		t.Error(err)
	}
	if err := validateScoreComponent("Preferences", got.Score.Preferences); err != nil {
		t.Error(err)
	}

	// Verify analysis is generated
	if got.Analysis == "" {
		t.Error("Match() analysis should not be empty")
	}

	// Log detailed results for debugging
	t.Log("Match Results for location_mismatch:")
	t.Logf("Overall Score: %.2f", got.Score.Overall)
	t.Logf("Skills Score: %.2f", got.Score.Skills)
	t.Logf("Experience Score: %.2f", got.Score.Experience)
	t.Logf("Location Score: %.2f", got.Score.Location)
	t.Logf("Matched Skills: %v", got.MatchedSkills)
	t.Logf("Missing Skills: %v", got.MissingSkills)
	t.Logf("Analysis:\n%s", got.Analysis)
}

func TestMatcher_MissingRequiredData(t *testing.T) {
	matcher := setupMatcher(t)
	ctx := context.Background()

	job := &search.Job{
		ID:      "job4",
		Title:   "Senior Go Developer",
		Company: "TechCorp",
	}
	candidate := &Candidate{
		ID:    "cand4",
		Title: "Software Engineer",
	}

	got, err := matcher.Match(ctx, job, candidate)
	if err == nil {
		t.Error("Match() should return error for missing required data")
		t.Logf("Expected error received: %v", err)
	}
	if got != nil {
		t.Error("Match() should return nil details for error case")
	}

	// Log error for debugging
	t.Logf("Expected error received: %v", err)
}

func TestLocationMatching_ExactMatch(t *testing.T) {
	matcher := setupMatcher(t)

	job := &search.JobDetail{
		Job: search.Job{
			Location: "San Francisco",
		},
	}
	candidate := &Candidate{
		PreferredLocations: []string{"San Francisco", "Remote"},
	}

	score, match := matcher.computeLocationMatch(job, candidate)

	// Verify score and match details
	if !withinDelta(score, 1.0, 0.1) {
		t.Errorf("location score should be 1.0 for exact match, got: %.2f", score)
	}
	if match.IsRemote {
		t.Error("should not be marked as remote")
	}
	if !match.IsPreferred {
		t.Error("location should be marked as preferred")
	}

	// Log results for debugging
	t.Log("Location Match Results for exact_match:")
	t.Logf("Score: %.2f", score)
	t.Logf("Is Remote: %v", match.IsRemote)
	t.Logf("Is Preferred: %v", match.IsPreferred)
}

func TestLocationMatching_RemoteMatch(t *testing.T) {
	matcher := setupMatcher(t)

	job := &search.JobDetail{
		Job: search.Job{
			Location: "Remote",
		},
	}
	candidate := &Candidate{
		PreferredLocations: []string{"Remote", "San Francisco"},
	}

	score, match := matcher.computeLocationMatch(job, candidate)

	// Verify score and match details
	if !withinDelta(score, 1.0, 0.1) {
		t.Errorf("location score should be 1.0 for remote match, got: %.2f", score)
	}
	if !match.IsRemote {
		t.Error("should be marked as remote")
	}
	if !match.IsPreferred {
		t.Error("location should be marked as preferred")
	}

	// Log results for debugging
	t.Log("Location Match Results for remote_match:")
	t.Logf("Score: %.2f", score)
	t.Logf("Is Remote: %v", match.IsRemote)
	t.Logf("Is Preferred: %v", match.IsPreferred)
}

func TestLocationMatching_RemoteJobNoPreference(t *testing.T) {
	matcher := setupMatcher(t)

	job := &search.JobDetail{
		Job: search.Job{
			Location: "Remote",
		},
	}
	candidate := &Candidate{
		PreferredLocations: []string{"San Francisco", "New York"},
	}

	score, match := matcher.computeLocationMatch(job, candidate)

	// Verify score and match details
	if !withinDelta(score, 0.7, 0.1) {
		t.Errorf("location score should be 0.7 for remote job without remote preference, got: %.2f", score)
	}
	if !match.IsRemote {
		t.Error("should be marked as remote")
	}
	if match.IsPreferred {
		t.Error("location should not be marked as preferred")
	}

	// Log results for debugging
	t.Log("Location Match Results for remote_job_no_preference:")
	t.Logf("Score: %.2f", score)
	t.Logf("Is Remote: %v", match.IsRemote)
	t.Logf("Is Preferred: %v", match.IsPreferred)
}

func TestLocationMatching_NoMatch(t *testing.T) {
	matcher := setupMatcher(t)

	job := &search.JobDetail{
		Job: search.Job{
			Location: "London",
		},
	}
	candidate := &Candidate{
		PreferredLocations: []string{"San Francisco", "New York"},
	}

	score, match := matcher.computeLocationMatch(job, candidate)

	// Verify score and match details
	if !withinDelta(score, 0.5, 0.1) {
		t.Errorf("location score should be 0.5 for no location match, got: %.2f", score)
	}
	if match.IsRemote {
		t.Error("should not be marked as remote")
	}
	if match.IsPreferred {
		t.Error("location should not be marked as preferred")
	}

	// Log results for debugging
	t.Log("Location Match Results for no_match:")
	t.Logf("Score: %.2f", score)
	t.Logf("Is Remote: %v", match.IsRemote)
	t.Logf("Is Preferred: %v", match.IsPreferred)
}

func TestExperienceMatching_ExceedsRequirements(t *testing.T) {
	matcher := setupMatcher(t)

	requirements := []Requirement{
		{Text: "5+ years of Go experience", Type: "must-have"},
		{Text: "3+ years of AWS", Type: "nice-to-have"},
	}
	candidate := &Candidate{
		ExperienceYears: 7,
	}

	score, match := matcher.computeExperienceMatch(requirements, candidate)

	// Verify score and match details
	if !withinDelta(score, 1.0, 0.1) {
		t.Errorf("experience score should be 1.0 for exceeding requirements, got: %.2f", score)
	}
	if match.YearsRequired != 5 {
		t.Errorf("should identify 5 years required, got: %d", match.YearsRequired)
	}
	if !match.LevelCompatible {
		t.Error("should be marked as level compatible")
	}

	// Log results for debugging
	t.Log("Experience Match Results for exceeds_requirements:")
	t.Logf("Score: %.2f", score)
	t.Logf("Years Required: %d", match.YearsRequired)
	t.Logf("Level Compatible: %v", match.LevelCompatible)
}

func TestExperienceMatching_MeetsRequirements(t *testing.T) {
	matcher := setupMatcher(t)

	requirements := []Requirement{
		{Text: "5+ years of Go experience", Type: "must-have"},
	}
	candidate := &Candidate{
		ExperienceYears: 5,
	}

	score, match := matcher.computeExperienceMatch(requirements, candidate)

	// Verify score and match details
	if !withinDelta(score, 1.0, 0.1) {
		t.Errorf("experience score should be 1.0 for meeting requirements, got: %.2f", score)
	}
	if match.YearsRequired != 5 {
		t.Errorf("should identify 5 years required, got: %d", match.YearsRequired)
	}
	if !match.LevelCompatible {
		t.Error("should be marked as level compatible")
	}

	// Log results for debugging
	t.Log("Experience Match Results for meets_requirements:")
	t.Logf("Score: %.2f", score)
	t.Logf("Years Required: %d", match.YearsRequired)
	t.Logf("Level Compatible: %v", match.LevelCompatible)
}

func TestExperienceMatching_PartialExperience(t *testing.T) {
	matcher := setupMatcher(t)

	requirements := []Requirement{
		{Text: "5+ years of Go experience", Type: "must-have"},
	}
	candidate := &Candidate{
		ExperienceYears: 3,
	}

	score, match := matcher.computeExperienceMatch(requirements, candidate)

	// Verify score and match details
	if !withinDelta(score, 0.6, 0.1) {
		t.Errorf("experience score should be 0.6 for partial experience, got: %.2f", score)
	}
	if match.YearsRequired != 5 {
		t.Errorf("should identify 5 years required, got: %d", match.YearsRequired)
	}
	if match.LevelCompatible {
		t.Error("should not be marked as level compatible")
	}

	// Log results for debugging
	t.Log("Experience Match Results for partial_experience:")
	t.Logf("Score: %.2f", score)
	t.Logf("Years Required: %d", match.YearsRequired)
	t.Logf("Level Compatible: %v", match.LevelCompatible)
}

func TestExperienceMatching_NoRequirements(t *testing.T) {
	matcher := setupMatcher(t)

	requirements := []Requirement{}
	candidate := &Candidate{
		ExperienceYears: 3,
	}

	score, match := matcher.computeExperienceMatch(requirements, candidate)

	// Verify score and match details
	if !withinDelta(score, 0.8, 0.1) {
		t.Errorf("experience score should be 0.8 for no requirements, got: %.2f", score)
	}
	if match.YearsRequired != 0 {
		t.Errorf("should have 0 years required, got: %d", match.YearsRequired)
	}
	if !match.LevelCompatible {
		t.Error("should be marked as level compatible")
	}

	// Log results for debugging
	t.Log("Experience Match Results for no_requirements:")
	t.Logf("Score: %.2f", score)
	t.Logf("Years Required: %d", match.YearsRequired)
	t.Logf("Level Compatible: %v", match.LevelCompatible)
}

func TestSkillsMatching_AllSkillsMatch(t *testing.T) {
	matcher := setupMatcher(t)

	requirements := []Requirement{
		{Text: "Go programming", Type: "must-have", Keywords: []string{"Go"}},
		{Text: "Docker experience", Type: "must-have", Keywords: []string{"Docker"}},
	}
	skills := []string{"Go", "Docker", "Kubernetes"}

	gotScore, gotMatched, gotMissing := matcher.computeSkillsMatch(requirements, skills)

	// Verify results
	if !withinDelta(gotScore, 1.0, 0.1) {
		t.Errorf("score = %.2f, want %.2f (±0.1)", gotScore, 1.0)
	}

	wantMatched := []string{"Go", "Docker"}
	if !compareSlices(gotMatched, wantMatched) {
		t.Errorf("matched skills = %v, want %v", gotMatched, wantMatched)
	}

	if len(gotMissing) > 0 {
		t.Errorf("missing skills = %v, want empty", gotMissing)
	}

	// Log results for debugging
	t.Log("Skills Match Results for all_skills_match:")
	t.Logf("Score: %.2f", gotScore)
	t.Logf("Matched Skills: %v", gotMatched)
	t.Logf("Missing Skills: %v", gotMissing)
}

func TestSkillsMatching_PartialSkillsMatch(t *testing.T) {
	matcher := setupMatcher(t)

	requirements := []Requirement{
		{Text: "Go programming", Type: "must-have", Keywords: []string{"Go"}},
		{Text: "Docker experience", Type: "must-have", Keywords: []string{"Docker"}},
		{Text: "Kubernetes experience", Type: "nice-to-have", Keywords: []string{"Kubernetes"}},
	}
	skills := []string{"Go", "Python"}

	gotScore, gotMatched, gotMissing := matcher.computeSkillsMatch(requirements, skills)

	// Verify results
	if !withinDelta(gotScore, 0.4, 0.1) {
		t.Errorf("score = %.2f, want %.2f (±0.1)", gotScore, 0.4)
	}

	wantMatched := []string{"Go"}
	if !compareSlices(gotMatched, wantMatched) {
		t.Errorf("matched skills = %v, want %v", gotMatched, wantMatched)
	}

	wantMissing := []string{"Docker experience", "Kubernetes experience"}
	if !compareSlices(gotMissing, wantMissing) {
		t.Errorf("missing skills = %v, want %v", gotMissing, wantMissing)
	}

	// Log results for debugging
	t.Log("Skills Match Results for partial_skills_match:")
	t.Logf("Score: %.2f", gotScore)
	t.Logf("Matched Skills: %v", gotMatched)
	t.Logf("Missing Skills: %v", gotMissing)
}

func TestSkillsMatching_NoSkillsMatch(t *testing.T) {
	matcher := setupMatcher(t)

	requirements := []Requirement{
		{Text: "Go programming", Type: "must-have", Keywords: []string{"Go"}},
		{Text: "Docker experience", Type: "must-have", Keywords: []string{"Docker"}},
	}
	skills := []string{"Python", "JavaScript"}

	gotScore, gotMatched, gotMissing := matcher.computeSkillsMatch(requirements, skills)

	// Verify results
	if !withinDelta(gotScore, 0.0, 0.1) {
		t.Errorf("score = %.2f, want %.2f (±0.1)", gotScore, 0.0)
	}

	if len(gotMatched) > 0 {
		t.Errorf("matched skills = %v, want empty", gotMatched)
	}

	wantMissing := []string{"Go programming", "Docker experience"}
	if !compareSlices(gotMissing, wantMissing) {
		t.Errorf("missing skills = %v, want %v", gotMissing, wantMissing)
	}

	// Log results for debugging
	t.Log("Skills Match Results for no_skills_match:")
	t.Logf("Score: %.2f", gotScore)
	t.Logf("Matched Skills: %v", gotMatched)
	t.Logf("Missing Skills: %v", gotMissing)
}

func TestSkillsMatching_NoRequirements(t *testing.T) {
	matcher := setupMatcher(t)

	requirements := []Requirement{}
	skills := []string{"Go", "Docker"}

	gotScore, gotMatched, gotMissing := matcher.computeSkillsMatch(requirements, skills)

	// Verify results
	if !withinDelta(gotScore, 0.0, 0.1) {
		t.Errorf("score = %.2f, want %.2f (±0.1)", gotScore, 0.0)
	}

	if len(gotMatched) > 0 {
		t.Errorf("matched skills = %v, want empty", gotMatched)
	}

	if len(gotMissing) > 0 {
		t.Errorf("missing skills = %v, want empty", gotMissing)
	}

	// Log results for debugging
	t.Log("Skills Match Results for no_requirements:")
	t.Logf("Score: %.2f", gotScore)
	t.Logf("Matched Skills: %v", gotMatched)
	t.Logf("Missing Skills: %v", gotMissing)
}

// Helper functions for testing

func withinDelta(got, want, delta float64) bool {
	return math.Abs(got-want) <= delta
}

func validateScoreComponent(name string, score float64) error {
	if score < 0.0 || score > 1.0 {
		return &scoreError{name: name, score: score}
	}
	return nil
}

type scoreError struct {
	name  string
	score float64
}

func (e *scoreError) Error() string {
	return fmt.Sprintf("%s score %.2f is not between 0.0 and 1.0", e.name, e.score)
}

func sliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// containsAll checks if a slice contains all elements from another slice
func containsAll(got, want []string) bool {
	if len(want) == 0 {
		return true
	}
	if len(got) == 0 {
		return false
	}

	// Create a map of the got skills for O(1) lookup
	gotMap := make(map[string]bool)
	for _, s := range got {
		gotMap[s] = true
	}

	// Check if all wanted skills are in the got map
	for _, w := range want {
		if !gotMap[w] {
			return false
		}
	}
	return true
}

// compareSlices compares two slices for equality regardless of order
func compareSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	// Create maps for both slices
	gotMap := make(map[string]bool)
	wantMap := make(map[string]bool)

	for _, s := range got {
		gotMap[s] = true
	}
	for _, s := range want {
		wantMap[s] = true
	}

	// Compare maps
	for k := range gotMap {
		if !wantMap[k] {
			return false
		}
	}
	for k := range wantMap {
		if !gotMap[k] {
			return false
		}
	}
	return true
}
