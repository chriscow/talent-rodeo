package processor

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"talent-rodeo/internal/jobmatch/providers"
	"talent-rodeo/internal/jobmatch/types"
)

// MatchScore represents the detailed scoring of a job-candidate match
type MatchScore struct {
	Overall     float64 `json:"overall"`     // 0-1 overall match score
	Skills      float64 `json:"skills"`      // 0-1 skills match score
	Experience  float64 `json:"experience"`  // 0-1 experience match score
	Location    float64 `json:"location"`    // 0-1 location match score
	Preferences float64 `json:"preferences"` // 0-1 work preferences match score
	Confidence  float64 `json:"confidence"`  // 0-1 confidence in the match
}

// MatchDetails provides detailed information about the match
type MatchDetails struct {
	Score           MatchScore      `json:"score"`
	MatchedSkills   []string        `json:"matched_skills"`
	MissingSkills   []string        `json:"missing_skills"`
	LocationMatch   LocationMatch   `json:"location_match"`
	ExperienceMatch ExperienceMatch `json:"experience_match"`
	Analysis        string          `json:"analysis"`
}

// LocationMatch represents location compatibility details
type LocationMatch struct {
	IsRemote      bool    `json:"is_remote"`
	Distance      float64 `json:"distance,omitempty"` // Distance in km if not remote
	IsPreferred   bool    `json:"is_preferred"`
	Compatibility float64 `json:"compatibility"` // 0-1 score
}

// ExperienceMatch represents experience compatibility details
type ExperienceMatch struct {
	YearsRequired   int     `json:"years_required"`
	YearsMatched    int     `json:"years_matched"`
	LevelCompatible bool    `json:"level_compatible"`
	IndustryMatch   float64 `json:"industry_match"` // 0-1 score
	RoleMatch       float64 `json:"role_match"`     // 0-1 score
}

// Matcher handles job-candidate matching logic
type Matcher struct {
	logger   *log.Logger
	analyzer Analyzer
	registry *providers.Registry
}

// NewMatcher creates a new job matcher instance
func NewMatcher(ctx context.Context, logger *log.Logger) (*Matcher, error) {
	if logger == nil {
		logger = log.New(os.Stdout, "[MATCHER] ", log.LstdFlags|log.Lmicroseconds)
	}

	analyzer, err := NewLLMAnalyzer(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create analyzer: %w", err)
	}

	registry := providers.NewRegistry()

	return &Matcher{
		logger:   logger,
		analyzer: analyzer,
		registry: registry,
	}, nil
}

// RegisterProvider registers a job provider with the matcher
func (m *Matcher) RegisterProvider(provider types.JobProvider) error {
	return m.registry.Register(provider)
}

// Match computes the match score between a job and candidate
func (m *Matcher) Match(ctx context.Context, job *types.Job, candidate *types.Candidate) (*MatchDetails, error) {
	m.logger.Printf("Starting match computation for job=%s candidate=%s", job.ID, candidate.ID)

	// Get detailed job information
	provider, err := m.getProvider(job.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	jobDetail, err := provider.GetJobDetails(ctx, job.ID)
	if err != nil {
		m.logger.Printf("Warning: failed to get job details: %v, falling back to basic job info", err)
		// Create a basic JobDetail from the Job
		jobDetail = &types.JobDetail{
			Job:             *job,
			FullDescription: job.Description,
			Requirements:    make([]string, 0),
			Qualifications:  make([]string, 0),
			Benefits:        make([]string, 0),
			Skills:          make([]string, 0),
			SalaryInfo:      nil,
			EmploymentType:  "",
			WorkplaceType:   "",
			Industry:        "",
			CompanyInfo:     nil,
		}
	}

	// Extract job requirements and keywords
	requirements, err := m.analyzer.ParseRequirements(ctx, jobDetail.FullDescription)
	if err != nil {
		// If LLM fails, create basic requirements from job description
		m.logger.Printf("Warning: failed to parse requirements: %v, using basic extraction", err)
		return nil, fmt.Errorf("failed to parse requirements: %w", err)
	}

	// Extract keywords for logging
	keywords, err := m.analyzer.ExtractKeywords(ctx, jobDetail.FullDescription)
	if err != nil {
		m.logger.Printf("Warning: failed to extract keywords: %v", err)
	} else {
		m.logger.Printf("Extracted keywords: %v", keywords)
	}

	// Compute individual scores
	skillsScore, matchedSkills, missingSkills := m.computeSkillsMatch(requirements, candidate.Skills)
	locationScore, locationMatch := m.computeLocationMatch(jobDetail, candidate)
	experienceScore, experienceMatch := m.computeExperienceMatch(requirements, candidate)
	preferencesScore := m.computePreferencesMatch(jobDetail, candidate)

	// Calculate overall score with weighted average
	weights := map[string]float64{
		"skills":      0.35,
		"experience":  0.25,
		"location":    0.20,
		"preferences": 0.20,
	}

	overallScore := (skillsScore*weights["skills"] +
		experienceScore*weights["experience"] +
		locationScore*weights["location"] +
		preferencesScore*weights["preferences"]) / (weights["skills"] + weights["experience"] + weights["location"] + weights["preferences"])

	// Calculate confidence in the match
	confidence := m.calculateConfidence(jobDetail, candidate)

	score := MatchScore{
		Overall:     overallScore,
		Skills:      skillsScore,
		Experience:  experienceScore,
		Location:    locationScore,
		Preferences: preferencesScore,
		Confidence:  confidence,
	}

	// Generate analysis
	analysis := m.generateMatchAnalysis(score, matchedSkills, missingSkills, locationMatch, experienceMatch)

	details := &MatchDetails{
		Score:           score,
		MatchedSkills:   matchedSkills,
		MissingSkills:   missingSkills,
		LocationMatch:   locationMatch,
		ExperienceMatch: experienceMatch,
		Analysis:        analysis,
	}

	m.logger.Printf("Match computation complete: score=%.2f confidence=%.2f", overallScore, confidence)
	return details, nil
}

// getProvider returns the JobProvider for the given provider name
func (m *Matcher) getProvider(providerName string) (types.JobProvider, error) {
	return m.registry.Get(providerName)
}

// computeSkillsMatch calculates the skills match score and returns matched/missing skills
func (m *Matcher) computeSkillsMatch(requirements []Requirement, candidateSkills []string) (float64, []string, []string) {
	if len(requirements) == 0 || len(candidateSkills) == 0 {
		return 0.0, nil, nil
	}

	// Create maps for both normalized and original skills
	normalizedCandidateSkills := make(map[string]string) // maps lowercase skill to original case
	for _, skill := range candidateSkills {
		normalizedCandidateSkills[strings.ToLower(strings.TrimSpace(skill))] = skill
	}

	var matchedSkills, missingSkills []string
	var totalWeight, matchedWeight float64

	// Track matched requirements to avoid duplicates
	matchedReqs := make(map[string]bool)

	// Check each requirement
	for _, req := range requirements {
		// Only consider skill requirements
		if !isSkillRequirement(req.Text) {
			continue
		}

		weight := 1.0
		if req.Type == "nice-to-have" {
			weight = 0.5
		}

		totalWeight += weight
		matched := false

		// Check each keyword associated with the requirement
		for _, keyword := range req.Keywords {
			normalizedKeyword := strings.ToLower(strings.TrimSpace(keyword))
			// Try different case variations of the candidate's skills
			for normalizedSkill, origSkill := range normalizedCandidateSkills {
				if normalizedSkill == normalizedKeyword {
					if !matchedReqs[normalizedSkill] {
						matchedSkills = append(matchedSkills, origSkill)
						matchedReqs[normalizedSkill] = true
					}
					matchedWeight += weight
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}

		if !matched {
			missingSkills = append(missingSkills, req.Text)
		}
	}

	if totalWeight == 0 {
		return 0.0, matchedSkills, missingSkills
	}

	// Log matching details
	m.logger.Printf("Skills match: total_weight=%.2f matched_weight=%.2f matched=%v missing=%v",
		totalWeight, matchedWeight, matchedSkills, missingSkills)

	return matchedWeight / totalWeight, matchedSkills, missingSkills
}

// computeLocationMatch calculates location compatibility
func (m *Matcher) computeLocationMatch(job *types.JobDetail, candidate *types.Candidate) (float64, LocationMatch) {
	m.logger.Printf("Computing location match for job=%s candidate=%s", job.ID, candidate.ID)

	// Check if job is remote
	isRemote := job.WorkplaceType == "remote" || strings.Contains(strings.ToLower(job.Location), "remote")

	// Initialize match details
	match := LocationMatch{
		IsRemote:      isRemote,
		Distance:      0, // TODO: Implement distance calculation
		IsPreferred:   false,
		Compatibility: 0,
	}

	// If job is remote, check if candidate prefers remote
	if isRemote {
		// Check if "Remote" is in preferred locations (case-insensitive)
		for _, loc := range candidate.PreferredLocations {
			if strings.EqualFold(loc, "remote") {
				match.IsPreferred = true
				match.Compatibility = 1.0
				m.logger.Printf("Remote job match: preferred=%v", match.IsPreferred)
				return match.Compatibility, match
			}
		}
		// Remote job but not preferred - partial match
		match.Compatibility = 0.7
		m.logger.Printf("Remote job match: preferred=%v", match.IsPreferred)
		return match.Compatibility, match
	}

	// Check if job location matches any preferred locations
	jobLoc := strings.ToLower(job.Location)
	for _, prefLoc := range candidate.PreferredLocations {
		if strings.Contains(jobLoc, strings.ToLower(prefLoc)) {
			match.IsPreferred = true
			match.Compatibility = 1.0
			m.logger.Printf("Location match found: job=%s preferred=%s", jobLoc, prefLoc)
			return match.Compatibility, match
		}
	}

	// No location match - return base compatibility score
	match.Compatibility = 0.5
	m.logger.Printf("No location match found")
	return match.Compatibility, match
}

// computeExperienceMatch calculates the experience match score
func (m *Matcher) computeExperienceMatch(requirements []Requirement, candidate *types.Candidate) (float64, ExperienceMatch) {
	var maxYearsRequired int
	var match ExperienceMatch

	// Find the maximum years required from requirements
	for _, req := range requirements {
		years := extractYearsFromText(req.Text)
		if years > maxYearsRequired {
			maxYearsRequired = years
		}
	}

	match.YearsRequired = maxYearsRequired
	match.YearsMatched = candidate.ExperienceYears

	// Calculate experience score
	if maxYearsRequired == 0 {
		// No specific experience requirement
		match.LevelCompatible = true
		match.RoleMatch = 0.8 // Assume good match if no specific requirement
		return 0.8, match
	}

	if candidate.ExperienceYears >= maxYearsRequired {
		match.LevelCompatible = true
		match.RoleMatch = 1.0
		return 1.0, match
	}

	// Partial match
	match.LevelCompatible = false
	match.RoleMatch = float64(candidate.ExperienceYears) / float64(maxYearsRequired)
	return match.RoleMatch, match
}

// computePreferencesMatch calculates a match score based on various preferences
// like work type, schedule, industry, company size, and culture
func (m *Matcher) computePreferencesMatch(job *types.JobDetail, candidate *types.Candidate) float64 {
	m.logger.Printf("Computing preferences match between job %s and candidate %s", job.ID, candidate.ID)

	// Define preference categories and their weights
	const (
		workTypeWeight    = 0.3
		scheduleWeight    = 0.2
		industryWeight    = 0.2
		companySizeWeight = 0.15
		roleWeight        = 0.15
	)

	var totalScore float64

	// Work type match (remote/hybrid/onsite)
	if job.WorkplaceType != "" && candidate.RemotePreference != "" {
		m.logger.Printf("Comparing work type: job=%s, candidate preference=%s", job.WorkplaceType, candidate.RemotePreference)
		if strings.EqualFold(job.WorkplaceType, candidate.RemotePreference) {
			totalScore += workTypeWeight
		} else if (job.WorkplaceType == "hybrid" && candidate.RemotePreference == "onsite") ||
			(job.WorkplaceType == "onsite" && candidate.RemotePreference == "hybrid") {
			totalScore += workTypeWeight * 0.5 // Partial match
		}
	}

	// Schedule/Employment type match
	if job.EmploymentType != "" && candidate.PreferredSchedule != "" {
		m.logger.Printf("Comparing schedule: job=%s, candidate preference=%s", job.EmploymentType, candidate.PreferredSchedule)
		if strings.EqualFold(job.EmploymentType, candidate.PreferredSchedule) {
			totalScore += scheduleWeight
		}
	}

	// Industry match
	if job.Industry != "" && len(candidate.PreferredIndustries) > 0 {
		m.logger.Printf("Comparing industry: job=%s, candidate preferences=%v", job.Industry, candidate.PreferredIndustries)
		for _, prefIndustry := range candidate.PreferredIndustries {
			if strings.EqualFold(job.Industry, prefIndustry) {
				totalScore += industryWeight
				break
			}
		}
	}

	// Company size match
	if job.CompanyInfo != nil && job.CompanyInfo.Size != "" && candidate.PreferredCompanySize != "" {
		m.logger.Printf("Comparing company size: job=%s, candidate preference=%s", job.CompanyInfo.Size, candidate.PreferredCompanySize)
		if strings.EqualFold(job.CompanyInfo.Size, candidate.PreferredCompanySize) {
			totalScore += companySizeWeight
		}
	}

	// Role match
	if job.Title != "" && len(candidate.PreferredRoles) > 0 {
		m.logger.Printf("Comparing role: job=%s, candidate preferences=%v", job.Title, candidate.PreferredRoles)
		for _, prefRole := range candidate.PreferredRoles {
			if strings.Contains(strings.ToLower(job.Title), strings.ToLower(prefRole)) {
				totalScore += roleWeight
				break
			}
		}
	}

	m.logger.Printf("Final preferences match score: %.2f", totalScore)
	return totalScore
}

// calculateConfidence determines the confidence level in the match computation
func (m *Matcher) calculateConfidence(job *types.JobDetail, candidate *types.Candidate) float64 {
	// Base confidence starts at 0.8
	confidence := 0.8

	// Reduce confidence if job description is too short
	if len(job.FullDescription) < 100 {
		confidence *= 0.7
	}

	// Reduce confidence if candidate has very few skills listed
	if len(candidate.Skills) < 3 {
		confidence *= 0.8
	}

	// Reduce confidence if job requirements couldn't be properly parsed
	if len(job.Requirements) < 2 {
		confidence *= 0.9
	}

	// Ensure confidence stays within 0-1 range
	return math.Max(0.0, math.Min(1.0, confidence))
}

// generateMatchAnalysis creates a human-readable analysis of the match
func (m *Matcher) generateMatchAnalysis(score MatchScore, matchedSkills, missingSkills []string, locationMatch LocationMatch, experienceMatch ExperienceMatch) string {
	var analysis strings.Builder

	// Overall assessment
	analysis.WriteString(fmt.Sprintf("Overall Match Score: %.1f%%\n\n", score.Overall*100))

	// Skills analysis
	analysis.WriteString("Skills Analysis:\n")
	analysis.WriteString(fmt.Sprintf("- Matched %d required skills\n", len(matchedSkills)))
	if len(matchedSkills) > 0 {
		analysis.WriteString(fmt.Sprintf("- Key matches: %s\n", strings.Join(matchedSkills[:min(3, len(matchedSkills))], ", ")))
	}
	if len(missingSkills) > 0 {
		analysis.WriteString(fmt.Sprintf("- Missing skills: %s\n", strings.Join(missingSkills[:min(3, len(missingSkills))], ", ")))
	}

	// Experience analysis
	analysis.WriteString("\nExperience Analysis:\n")
	if experienceMatch.YearsRequired > 0 {
		analysis.WriteString(fmt.Sprintf("- Required: %d years, Candidate has: %d years\n",
			experienceMatch.YearsRequired, experienceMatch.YearsMatched))
	}
	analysis.WriteString(fmt.Sprintf("- Level compatibility: %v\n", experienceMatch.LevelCompatible))

	// Location analysis
	analysis.WriteString("\nLocation Analysis:\n")
	if locationMatch.IsRemote {
		analysis.WriteString("- Remote position")
		if locationMatch.IsPreferred {
			analysis.WriteString(" (matches candidate preference)")
		}
		analysis.WriteString("\n")
	} else {
		analysis.WriteString(fmt.Sprintf("- Location compatibility: %.1f%%\n", locationMatch.Compatibility*100))
	}

	// Confidence assessment
	analysis.WriteString(fmt.Sprintf("\nConfidence in Match: %.1f%%\n", score.Confidence*100))

	return analysis.String()
}

// Helper functions

func isSkillRequirement(text string) bool {
	text = strings.ToLower(text)
	return !strings.Contains(text, "year") && !strings.Contains(text, "degree") &&
		!strings.Contains(text, "education") && !strings.Contains(text, "certification")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
