package linkedin

import (
	"fmt"
	"time"
)

// Qualification represents a single required or preferred qualification
// which could be a skill, certification, or educational requirement
type Qualification struct {
	// "Skill", "Experience", "Education", "Certification"
	Category string `json:"category" enum:"Skill,Experience,Education,Certification" description:"Category of the qualification"`

	// Name of the qualification
	Name string `json:"name" description:"Name of the qualification"`

	// Required level or years of experience
	Level string `json:"level" description:"Required level or years of experience"`

	// Whether this is a must-have
	Required bool `json:"required" description:"Whether this is a must-have qualification"`

	// Whether candidate meets this qualification
	HasMatch bool `json:"has_match" description:"Whether candidate meets this qualification"`

	// Context about the match or transferable experience
	MatchDetails string `json:"match_details" description:"Context about the match or transferable experience"`

	// Confidence in the match assessment (0-1)
	Confidence float32 `json:"confidence" description:"Confidence in the match assessment"`

	// When this qualification was last verified
	LastVerified time.Time `json:"last_verified" jsonschema:"-"`
}

// WorkPreference captures a specific preference about the work environment
// or conditions, with context about importance and flexibility
type WorkPreference struct {
	// e.g., "Remote", "Culture", "Schedule"
	Category string `json:"category" description:"Category of the preference"`

	// The actual preference
	Preference string `json:"preference" description:"The actual preference"`

	// How important this is (0-1)
	Importance float32 `json:"importance" description:"How important this is"`

	// How flexible this preference is (0-1)
	Flexibility float32 `json:"flexibility" description:"How flexible this preference is"`

	// Whether the job matches this preference
	HasMatch bool `json:"has_match" description:"Whether the job matches this preference"`

	// Additional context about the preference
	Context string `json:"context" description:"Additional context about the preference"`
}

// MatchMetrics holds quantifiable aspects of the job match
type MatchMetrics struct {
	SkillMatch struct {
		Required  float32 // Percentage match on required skills
		Preferred float32 // Percentage match on preferred skills
		Overall   float32 // Weighted overall match
	}
	ExperienceMatch struct {
		YearsRequired float32
		YearsActual   float32
		Relevance     float32 // How relevant the experience is (0-1)
	}
	PreferenceMatch struct {
		Essential float32 // Match on must-have preferences
		Desired   float32 // Match on nice-to-have preferences
	}
	ConfidenceScore float32 // Overall confidence in the analysis (0-1)
}

// VisualMetrics contains data structured for visualization
type VisualMetrics struct {
	SkillRadar []struct {
		Category string
		Required float32
		Actual   float32
	}
	TimelineMatch struct {
		RequiredYears float32
		ActualYears   float32
		Overlap       float32
	}
	PreferenceAlignment []struct {
		Factor   string
		Match    float32
		Priority float32
	}
}

// Summary provides different levels of match summary
type Summary struct {
	// Executive summary
	OneLine string `json:"one_line" description:"One-line executive summary"`

	// 3-5 key points
	KeyPoints []string `json:"key_points" description:"3-5 key points"`

	// Full analysis
	Detailed string `json:"detailed" description:"Detailed analysis"`

	// Confidence in the summary (0-1)
	Confidence float32 `json:"confidence" description:"Confidence in the summary"`
}

// JobMatch is the primary struct that holds all match analysis data
type JobMatch struct {
	// Position details
	Position struct {
		Title        string
		Company      string
		Location     string
		Type         string // Full-time, Contract, etc.
		RemotePolicy string
		Department   string
		Level        string
		PostedDate   time.Time
	}

	// Core match data
	Qualifications  []Qualification
	WorkPreferences []WorkPreference
	Metrics         MatchMetrics
	VisualData      VisualMetrics

	// Analysis results
	Analysis struct {
		Pros          []string
		Cons          []string
		RiskFactors   []string
		Opportunities []string
		RedFlags      []string
		Summary       Summary
		LastUpdated   time.Time
	}

	// Source tracking
	DataSources struct {
		HasResume        bool
		HasLinkedIn      bool
		HasInterview     bool
		HasVerification  bool
		SourcesUsed      []string
		DataCompleteness float32
	}
}

// MatchConfig controls how the match analysis is performed
type MatchConfig struct {
	DetailLevel        string  // "brief", "standard", "detailed"
	RequiredConfidence float32 // Minimum confidence required for inclusion
	WeightRequired     float32 // Weight for required qualifications
	WeightPreferred    float32 // Weight for preferred qualifications
	IncludeVisuals     bool    // Whether to generate visualization data
	VerificationAge    float32 // Maximum age of verification data in days
}

// NewJobMatch creates a new JobMatch instance with default configuration
// func NewJobMatch() *JobMatch {
// 	match := &JobMatch{}
// 	match.initializeDefaults()
// 	return match
// }

// AnalyzeMatch performs the complete match analysis
func (j *JobMatch) AnalyzeMatch(config MatchConfig) error {
	if err := j.validateData(); err != nil {
		return fmt.Errorf("data validation failed: %w", err)
	}

	// j.calculateMetrics(config)
	// j.generateVisualData()
	// j.performAnalysis(config)
	// j.generateSummaries(config)

	return nil
}

// Helper methods

func (j *JobMatch) validateData() error {
	// Implement validation logic
	return nil
}

func (j *JobMatch) calculateMetrics(config MatchConfig) {
	// Calculate all metrics based on configuration
	j.calculateSkillMatch(config)
	// j.calculateExperienceMatch(config)
	// j.calculatePreferenceMatch(config)
	// j.calculateConfidenceScore(config)
}

func (j *JobMatch) calculateSkillMatch(config MatchConfig) {
	var reqTotal, reqMatched, prefTotal, prefMatched float32

	for _, qual := range j.Qualifications {
		if qual.Category != "Skill" {
			continue
		}

		if qual.Required {
			reqTotal++
			if qual.HasMatch {
				reqMatched++
			}
		} else {
			prefTotal++
			if qual.HasMatch {
				prefMatched++
			}
		}
	}

	j.Metrics.SkillMatch.Required = safeDiv(reqMatched, reqTotal) * 100
	j.Metrics.SkillMatch.Preferred = safeDiv(prefMatched, prefTotal) * 100
	j.Metrics.SkillMatch.Overall = weightedAverage(
		[]float32{j.Metrics.SkillMatch.Required, j.Metrics.SkillMatch.Preferred},
		[]float32{config.WeightRequired, config.WeightPreferred},
	)
}

// generatePrompt creates the LLM prompt based on available data
func (j *JobMatch) generatePrompt() string {
	return fmt.Sprintf(`Analyze this job match using the following structured approach:

Core Match Analysis
------------------
1. Essential Requirements
   - Focus only on must-have qualifications
   - Consider recent evidence and verification
   - Note any missing critical data

2. Experience Alignment
   - Evaluate depth and relevance
   - Consider industry context
   - Map career progression

3. Preference Compatibility
   - Work environment alignment
   - Career trajectory match
   - Technical environment fit

Scoring Guidelines
-----------------
- Use concrete evidence only
- Score each factor 0-100
- Document scoring rationale
- Flag uncertain assessments
- Note verification status

Output Format
------------
1. Core Metrics (JSON)
   - Individual scores
   - Confidence levels
   - Data completeness

2. Analysis Summary
   - One-line executive summary
   - Three key decision factors
   - Critical considerations

3. Risk Assessment
   - Data gaps
   - Verification needs
   - Potential mismatches

Available Data:
Position: %s
Qualifications: %s
Preferences: %s
Sources: %s`,
		j.formatPositionData(),
		j.formatQualificationData(),
		j.formatPreferenceData(),
		j.formatSourceData())
}

// Utility functions

func safeDiv(num, denom float32) float32 {
	if denom == 0 {
		return 0
	}
	return num / denom
}

func weightedAverage(values, weights []float32) float32 {
	if len(values) != len(weights) {
		return 0
	}

	var sum, weightSum float32
	for i := range values {
		sum += values[i] * weights[i]
		weightSum += weights[i]
	}

	return safeDiv(sum, weightSum)
}

// formatPositionData formats position data for the LLM prompt
func (j *JobMatch) formatPositionData() string {
	// Implementation here
	return ""
}

// formatQualificationData formats qualification data for the LLM prompt
func (j *JobMatch) formatQualificationData() string {
	// Implementation here
	return ""
}

// formatPreferenceData formats preference data for the LLM prompt
func (j *JobMatch) formatPreferenceData() string {
	// Implementation here
	return ""
}

// formatSourceData formats source data for the LLM prompt
func (j *JobMatch) formatSourceData() string {
	// Implementation here
	return ""
}
