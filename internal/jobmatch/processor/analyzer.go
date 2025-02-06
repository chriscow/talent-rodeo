package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"talent-rodeo/internal/jobmatch/types"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/gemini"
)

// Keyword represents an extracted keyword with metadata
type Keyword struct {
	Text       string  // The actual keyword text
	Category   string  // Category of the keyword (technical, soft skill, etc)
	Weight     float64 // Importance weight (0-1)
	Confidence float64 // Confidence score (0-1)
}

// Requirement represents a parsed job requirement
type Requirement struct {
	Text        string   // The requirement text
	Type        string   // Type of requirement (must-have, nice-to-have, etc)
	Keywords    []string // Associated keywords
	YearsNeeded int      // Years of experience needed, if applicable
}

// ValidationResult represents the outcome of job data validation
type ValidationResult struct {
	Valid  bool     // Whether the job data is valid
	Score  float64  // Validation score (0-1)
	Errors []string // List of validation errors if any
}

// Analyzer interface defines methods for analyzing job data
type Analyzer interface {
	ExtractKeywords(ctx context.Context, text string) ([]Keyword, error)
	ParseRequirements(ctx context.Context, text string) ([]Requirement, error)
	ValidateJobData(ctx context.Context, job *types.JobDetail) (*ValidationResult, error)
}

// Analysis result types
type NormalizedJob struct {
	CoreDescription    string   `json:"core_description"`
	KeyDifferentiators []string `json:"key_differentiators"`
	WorkEnvironment    string   `json:"work_environment"`
}

type RequirementAnalysis struct {
	MustHave []struct {
		Skill        string   `json:"skill"`
		Weight       float64  `json:"weight"`
		Alternatives []string `json:"alternatives"`
	} `json:"must_have"`
	NiceToHave []struct {
		Skill  string  `json:"skill"`
		Weight float64 `json:"weight"`
	} `json:"nice_to_have"`
}

// LLMAnalyzer implements the Analyzer interface using LLM capabilities
type LLMAnalyzer struct {
	provider minds.ContentGenerator
	schemas  struct {
		normalize    *minds.ResponseSchema
		requirements *minds.ResponseSchema
	}
	logger *log.Logger
	// fallback   Analyzer // Don't use a fallback, we want to fail if the LLM fails
	retryCount int
}

// NewLLMAnalyzer creates a new LLM-based analyzer
func NewLLMAnalyzer(ctx context.Context, logger *log.Logger) (*LLMAnalyzer, error) {
	// Create response schemas
	normalizeSchema, err := minds.NewResponseSchema("NormalizedJob", "Normalized job description", NormalizedJob{})
	if err != nil {
		return nil, fmt.Errorf("failed to create normalize schema: %w", err)
	}

	requirementsSchema, err := minds.NewResponseSchema("RequirementAnalysis", "Job requirements analysis", RequirementAnalysis{})
	if err != nil {
		return nil, fmt.Errorf("failed to create requirements schema: %w", err)
	}

	return &LLMAnalyzer{
		schemas: struct {
			normalize    *minds.ResponseSchema
			requirements *minds.ResponseSchema
		}{
			normalize:    normalizeSchema,
			requirements: requirementsSchema,
		},
		logger: logger,
		// fallback:   NewTraditionalAnalyzer(), // don't use a fallback, we want to fail if the LLM fails
		retryCount: 3,
	}, nil
}

// Analysis prompts
const (
	normalizePrompt = `Analyze this job description and:
1. Rewrite it removing any references to the company and removing marketing language and fluff
2. Extract the core job responsibilities
3. Identify key differentiators
Output in the specified JSON format.`

	requirementsPrompt = `You are a job requirements extractor. 
Analyze the job requirements and:

1. Identify must-have skills/experience
2. Determine relative importance (0-1)
3. List alternative equivalent skills

Output in the specified JSON format.`
)

// ExtractKeywords implements the Analyzer interface using LLM
func (a *LLMAnalyzer) ExtractKeywords(ctx context.Context, text string) ([]Keyword, error) {
	if text == "" {
		return nil, errors.New("empty text provided")
	}

	a.logger.Printf("Extracting keywords using LLM from text: %s", text)

	// Create parallel analysis tasks using channels for manual parallelization
	type result struct {
		data interface{}
		err  error
	}

	normalizedCh := make(chan result)
	requirementsCh := make(chan result)

	// Run analyses in parallel
	go func() {
		normalized, err := a.normalizeJob(ctx, text)
		normalizedCh <- result{data: normalized, err: err}
	}()

	go func() {
		requirements, err := a.analyzeRequirements(ctx, text)
		requirementsCh <- result{data: requirements, err: err}
	}()

	// Wait for results
	normalizedResult := <-normalizedCh
	requirementsResult := <-requirementsCh

	// Check for errors
	if normalizedResult.err != nil || requirementsResult.err != nil {
		return nil, fmt.Errorf("LLM analysis failed: %w", normalizedResult.err)
	}

	normalized := normalizedResult.data.(*NormalizedJob)
	requirements := requirementsResult.data.(*RequirementAnalysis)

	keywords := make([]Keyword, 0)

	// Add must-have requirements as keywords
	for _, req := range requirements.MustHave {
		keywords = append(keywords, Keyword{
			Text:       req.Skill,
			Category:   "technical",
			Weight:     req.Weight,
			Confidence: 0.9,
		})
	}

	// Add nice-to-have requirements as keywords
	for _, req := range requirements.NiceToHave {
		keywords = append(keywords, Keyword{
			Text:       req.Skill,
			Category:   "technical",
			Weight:     req.Weight,
			Confidence: 0.7,
		})
	}

	// Add key differentiators as keywords
	for _, diff := range normalized.KeyDifferentiators {
		keywords = append(keywords, Keyword{
			Text:       diff,
			Category:   "differentiator",
			Weight:     0.8,
			Confidence: 0.8,
		})
	}

	a.logger.Printf("Extracted %d keywords using LLM", len(keywords))
	return keywords, nil
}

// ParseRequirements implements the Analyzer interface using LLM
func (a *LLMAnalyzer) ParseRequirements(ctx context.Context, text string) ([]Requirement, error) {
	if text == "" {
		return nil, errors.New("empty text provided")
	}

	a.logger.Printf("Parsing requirements using LLM from text: %s", text)

	analysis, err := a.analyzeRequirements(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("LLM analysis failed: %w", err)
	}

	requirements := make([]Requirement, 0)

	// Convert must-have requirements
	for _, req := range analysis.MustHave {
		requirements = append(requirements, Requirement{
			Text:        req.Skill,
			Type:        "must-have",
			Keywords:    append([]string{req.Skill}, req.Alternatives...),
			YearsNeeded: extractYearsFromText(req.Skill),
		})
	}

	// Convert nice-to-have requirements
	for _, req := range analysis.NiceToHave {
		requirements = append(requirements, Requirement{
			Text:     req.Skill,
			Type:     "nice-to-have",
			Keywords: []string{req.Skill},
		})
	}

	a.logger.Printf("Parsed %d requirements using LLM", len(requirements))
	return requirements, nil
}

// ValidateJobData implements the Analyzer interface
func (a *LLMAnalyzer) ValidateJobData(ctx context.Context, job *types.JobDetail) (*ValidationResult, error) {
	if job == nil {
		return nil, errors.New("job is nil")
	}

	result := &ValidationResult{
		Valid:  true,
		Score:  1.0,
		Errors: make([]string, 0),
	}

	// Check required fields
	if job.Title == "" {
		result.Valid = false
		result.Score *= 0.5
		result.Errors = append(result.Errors, "missing job title")
	}

	if job.FullDescription == "" {
		result.Valid = false
		result.Score *= 0.5
		result.Errors = append(result.Errors, "missing job description")
	}

	if job.Company == "" {
		result.Valid = false
		result.Score *= 0.5
		result.Errors = append(result.Errors, "missing company name")
	}

	if job.Location == "" {
		result.Valid = false
		result.Score *= 0.7
		result.Errors = append(result.Errors, "missing location")
	}

	// Check for required fields that enhance matching
	if job.WorkplaceType == "" {
		result.Score *= 0.8
		result.Errors = append(result.Errors, "missing workplace type")
	}

	if job.EmploymentType == "" {
		result.Score *= 0.8
		result.Errors = append(result.Errors, "missing employment type")
	}

	if job.Industry == "" {
		result.Score *= 0.8
		result.Errors = append(result.Errors, "missing industry")
	}

	if len(job.Requirements) == 0 {
		result.Score *= 0.7
		result.Errors = append(result.Errors, "no requirements specified")
	}

	if len(job.Skills) == 0 {
		result.Score *= 0.7
		result.Errors = append(result.Errors, "no skills specified")
	}

	// If any required fields are missing, return error
	if !result.Valid {
		return nil, fmt.Errorf("invalid job data: %s", strings.Join(result.Errors, ", "))
	}

	return result, nil
}

// Helper methods for LLM analysis

func (a *LLMAnalyzer) normalizeJob(ctx context.Context, text string) (*NormalizedJob, error) {
	schema, err := minds.NewResponseSchema("NormalizedJob", "Normalized job description", NormalizedJob{})
	if err != nil {
		return nil, fmt.Errorf("failed to create normalize schema: %w", err)
	}

	req := minds.NewRequest(
		minds.Messages{
			{Role: minds.RoleSystem, Content: normalizePrompt},
			{Role: minds.RoleUser, Content: "Job Description:\n" + text},
		},
		minds.WithResponseSchema(*schema),
	)

	provider := a.provider
	if provider == nil {
		// just use a new provider to be sure there are no concurrency issues
		provider, err = gemini.NewProvider(ctx)
		if err != nil {
			log.Println("GEMINI_API_KEY:", os.Getenv("GEMINI_API_KEY"))
			return nil, fmt.Errorf("failed to create LLM provider: %w", err)
		}
	}

	resp, err := provider.GenerateContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	var result NormalizedJob
	if err := json.Unmarshal([]byte(resp.String()), &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return &result, nil
}

func (a *LLMAnalyzer) analyzeRequirements(ctx context.Context, text string) (*RequirementAnalysis, error) {
	schema, err := minds.NewResponseSchema("RequirementAnalysis", "Job requirements analysis", RequirementAnalysis{})
	if err != nil {
		return nil, fmt.Errorf("failed to create requirements schema: %w", err)
	}

	req := minds.NewRequest(
		minds.Messages{
			{Role: minds.RoleSystem, Content: requirementsPrompt},
			{Role: minds.RoleUser, Content: "Job Description:\n" + text},
		},
		minds.WithResponseSchema(*schema),
	)

	// just use a new provider to be sure there are no concurrency issues
	provider := a.provider
	if provider == nil {
		provider, err = gemini.NewProvider(ctx)
		if err != nil {
			log.Println("GEMINI_API_KEY:", os.Getenv("GEMINI_API_KEY"))
			return nil, fmt.Errorf("failed to create LLM provider: %w", err)
		}
	}

	resp, err := provider.GenerateContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	var result RequirementAnalysis
	if err := json.Unmarshal([]byte(resp.String()), &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return &result, nil
}

// Keep existing helper functions but make them package-private
func extractYearsFromText(text string) int {
	text = strings.ToLower(text)
	patterns := []string{
		`(\d+)\+?\s*(?:years?|yrs?)`,
		`(\d+)\+?\s*(?:years?|yrs?)\s*(?:of)?\s*(?:experience|exp)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			years, err := strconv.Atoi(matches[1])
			if err == nil {
				return years
			}
		}
	}

	return 0
}

/*

Do not use this analyzer, it is not used in the new code.
We only want to use the LLM analyzers.

// Rename existing analyzer to TraditionalAnalyzer
type TraditionalAnalyzer struct {
	technicalKeywords map[string]bool
	toolKeywords      map[string]bool
	levelKeywords     map[string]bool
	stopWords         map[string]bool
}


// NewTraditionalAnalyzer creates a new instance of the traditional analyzer
func NewTraditionalAnalyzer() *TraditionalAnalyzer {
	return &TraditionalAnalyzer{
		technicalKeywords: map[string]bool{
			"Go":            true,
			"golang":        true,
			"Python":        true,
			"Java":          true,
			"JavaScript":    true,
			"AWS":           true,
			"Kubernetes":    true,
			"Docker":        true,
			"React":         true,
			"Node.js":       true,
			"SQL":           true,
			"ML":            true,
			"Cloud":         true,
			"Frameworks":    true,
			"Platforms":     true,
			"Certification": true,
		},
		toolKeywords: map[string]bool{
			"Git":    true,
			"Jira":   true,
			"GitLab": true,
			"VSCode": true,
			"Docker": true,
		},
		levelKeywords: map[string]bool{
			"Senior":    true,
			"Junior":    true,
			"Lead":      true,
			"Manager":   true,
			"Staff":     true,
			"Principal": true,
		},
		stopWords: map[string]bool{
			"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
			"be": true, "by": true, "for": true, "from": true, "has": true, "he": true,
			"in": true, "is": true, "it": true, "its": true, "of": true, "on": true,
			"that": true, "the": true, "to": true, "was": true, "were": true, "will": true,
			"with": true, "experience": true, "skills": true, "developer": true,
			"development": true, "looking": true, "must": true, "have": true, "needed": true,
			"required": true, "using": true, "knowledge": true, "understanding": true,
			"strong": true, "excellent": true, "good": true, "great": true, "ability": true,
			"years": true, "work": true, "working": true, "team": true, "position": true,
			"role": true, "job": true, "candidate": true, "candidates": true, "qualified": true,
			"requirements": true, "qualifications": true, "responsibilities": true,
			"opportunity": true, "company": true, "business": true, "client": true,
			"project": true, "projects": true, "solutions": true, "environment": true,
			"environments": true, "system": true, "systems": true, "application": true,
			"applications": true, "software": true, "code": true, "coding": true,
			"programming": true, "written": true, "verbal": true, "communication": true,
			"problem": true, "solving": true, "analytical": true, "analysis": true,
			"design": true, "implementation": true, "developing": true, "building": true,
			"maintaining": true, "supporting": true, "testing": true, "deployment": true,
			"production": true, "agile": true, "scrum": true, "methodology": true,
			"methodologies": true, "practices": true, "processes": true, "standards": true,
			"tools": true, "libraries": true, "apis": true, "services": true,
			"infrastructure": true, "architectures": true, "patterns": true,
			"principles": true, "concepts": true,
		},
	}
}

// Common word patterns to identify categories
var (
	technicalPattern = regexp.MustCompile(`(?i)(golang|python|java|aws|kubernetes|docker|sql|react|node\.js|typescript|javascript|rust|c\+\+|ruby|php)`)
	toolPattern      = regexp.MustCompile(`(?i)(git|jenkins|terraform|ansible|kubernetes|docker|jira|confluence)`)
	levelPattern     = regexp.MustCompile(`(?i)(senior|junior|lead|principal|staff|architect)`)
	skillPattern     = regexp.MustCompile(`(?i)(problem.solving|communication|leadership|teamwork|analytical|collaboration)`)
)

// categoryPatterns maps categories to their detection patterns
var categoryPatterns = map[string]*regexp.Regexp{
	"technical": technicalPattern,
	"tool":      toolPattern,
	"level":     levelPattern,
	"skill":     skillPattern,
}

// ExtractKeywords implements keyword extraction from text
func (a *TraditionalAnalyzer) ExtractKeywords(ctx context.Context, text string) ([]Keyword, error) {
	if text == "" {
		return nil, errors.New("empty text provided")
	}

	log.Printf("Extracting keywords from text: %s", text)

	// Split into words while preserving case
	words := strings.FieldsFunc(text, func(r rune) bool {
		return strings.ContainsRune(" \t\n\r\f.,:;!?()[]{}\"'", r)
	})

	// Track keyword frequencies with original case
	frequencies := make(map[string]int)
	for _, word := range words {
		if !a.stopWords[strings.ToLower(word)] { // Check stop words in lowercase
			frequencies[word]++
		}
	}

	// Map to deduplicate keywords
	keywordMap := make(map[string]Keyword)

	for word, freq := range frequencies {
		var category string
		var baseWeight float64
		var baseConfidence float64

		// Check if the word or its lowercase version matches any technical keywords
		isTechnical := a.technicalKeywords[word]
		if !isTechnical {
			// Try common variations
			variations := []string{
				word,                                 // Original case
				strings.ToLower(word),                // lowercase
				strings.ToUpper(word),                // UPPERCASE
				strings.Title(strings.ToLower(word)), // Title Case
			}
			for _, v := range variations {
				if a.technicalKeywords[v] {
					isTechnical = true
					word = v // Use the matched variation
					break
				}
			}
		}

		switch {
		case isTechnical:
			category = "technical"
			baseWeight = 0.9
			baseConfidence = 0.8
		case a.toolKeywords[word]:
			category = "tool"
			baseWeight = 0.7
			baseConfidence = 0.7
		case a.levelKeywords[word]:
			category = "level"
			baseWeight = 0.7
			baseConfidence = 0.8
		default:
			continue // Skip non-categorized words
		}

		// Adjust confidence based on frequency
		confidence := baseConfidence
		if freq > 1 {
			confidence += 0.1 // Boost confidence for repeated keywords
		}
		if confidence > 1.0 {
			confidence = 1.0
		}

		keywordMap[word] = Keyword{
			Text:       word,
			Category:   category,
			Weight:     baseWeight,
			Confidence: confidence,
		}
	}

	// Convert map to slice
	keywords := make([]Keyword, 0, len(keywordMap))
	for _, k := range keywordMap {
		keywords = append(keywords, k)
	}

	log.Printf("Extracted %d keywords", len(keywords))
	return keywords, nil
}

// ParseRequirements implements requirement parsing from text
func (a *TraditionalAnalyzer) ParseRequirements(ctx context.Context, text string) ([]Requirement, error) {
	if text == "" {
		return nil, errors.New("empty text provided")
	}

	log.Printf("Parsing requirements from text: %s", text)

	// Split text into sentences
	sentences := splitSentences(text)
	var requirements []Requirement

	for _, sentence := range sentences {
		// Skip empty sentences
		if strings.TrimSpace(sentence) == "" {
			continue
		}

		// Extract requirement type and clean text
		reqType, cleanText := extractRequirementTypeAndText(sentence)
		if reqType == "" {
			continue // Skip sentences that don't appear to be requirements
		}

		// Split compound requirements
		subRequirements := splitCompoundRequirements(cleanText)
		for _, subReq := range subRequirements {
			// Extract years of experience if present
			yearsNeeded := extractYearsNeeded(subReq)

			// Extract keywords
			keywords := a.extractRequirementKeywords(subReq)

			// Create requirement
			req := Requirement{
				Text:        strings.TrimSpace(subReq),
				Type:        reqType,
				Keywords:    keywords,
				YearsNeeded: yearsNeeded,
			}

			requirements = append(requirements, req)
		}
	}

	log.Printf("Parsed %d requirements", len(requirements))
	return requirements, nil
}

// splitSentences splits text into sentences
func splitSentences(text string) []string {
	// Split on common sentence terminators
	terminators := []string{". ", "! ", "? ", "; ", "\n", "\r\n"}
	sentences := []string{text}

	for _, terminator := range terminators {
		var newSentences []string
		for _, s := range sentences {
			parts := strings.Split(s, terminator)
			for _, part := range parts {
				if strings.TrimSpace(part) != "" {
					newSentences = append(newSentences, part)
				}
			}
		}
		sentences = newSentences
	}

	return sentences
}

// extractRequirementTypeAndText extracts the requirement type and cleans the text
func extractRequirementTypeAndText(text string) (string, string) {
	text = strings.TrimSpace(text)
	text = strings.TrimSuffix(text, ".")

	// Map requirement type prefixes to their canonical types
	typeMap := map[string]string{
		"must have":    "must-have",
		"required":     "must-have",
		"mandatory":    "must-have",
		"essential":    "must-have",
		"nice to have": "nice-to-have",
		"preferred":    "preferred",
		"optional":     "nice-to-have",
		"desirable":    "nice-to-have",
	}

	// Check for requirement type prefixes
	lowerText := strings.ToLower(text)
	for prefix, reqType := range typeMap {
		if strings.HasPrefix(lowerText, prefix+":") {
			return reqType, strings.TrimSpace(text[len(prefix)+1:])
		}
		if strings.HasPrefix(lowerText, prefix+" -") {
			return reqType, strings.TrimSpace(text[len(prefix)+2:])
		}
	}

	// Default to must-have if no type is specified
	return "must-have", text
}

// splitCompoundRequirements splits compound requirements into individual ones
func splitCompoundRequirements(text string) []string {
	// Clean up the text
	text = strings.TrimSpace(text)
	text = strings.TrimSuffix(text, ".")

	// Split on "and" and commas when they separate distinct requirements
	text = strings.ReplaceAll(text, " and ", ", ")
	parts := strings.Split(text, ",")

	var requirements []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			requirements = append(requirements, part)
		}
	}

	return requirements
}

// extractYearsNeeded extracts years of experience from text
func extractYearsNeeded(text string) int {
	text = strings.ToLower(text)
	patterns := []string{
		`(\d+)\+?\s*(?:years?|yrs?)`,
		`(\d+)\+?\s*(?:years?|yrs?)\s*(?:of)?\s*(?:experience|exp)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			years, err := strconv.Atoi(matches[1])
			if err == nil {
				return years
			}
		}
	}

	return 0
}

// extractRequirementKeywords extracts keywords from a requirement
func (a *TraditionalAnalyzer) extractRequirementKeywords(text string) []string {
	// Clean up the text
	text = strings.TrimSpace(text)
	text = strings.TrimSuffix(text, ".")

	// Add technical terms to the text to help with extraction
	var keywords []string

	// Add technical keywords with case-sensitive matching
	for term := range a.technicalKeywords {
		// Try different case variations
		variations := []string{
			strings.ToLower(text),                // lowercase
			text,                                 // original case
			strings.ToUpper(text),                // UPPERCASE
			strings.Title(strings.ToLower(text)), // Title Case
		}

		for _, v := range variations {
			if strings.Contains(v, term) {
				keywords = append(keywords, term)
				break
			}
		}
	}

	// Add level keywords with case-sensitive matching
	for term := range a.levelKeywords {
		// Try different case variations
		variations := []string{
			strings.ToLower(text),                // lowercase
			text,                                 // original case
			strings.ToUpper(text),                // UPPERCASE
			strings.Title(strings.ToLower(text)), // Title Case
		}

		for _, v := range variations {
			if strings.Contains(v, term) {
				keywords = append(keywords, term)
				break
			}
		}
	}

	// Deduplicate keywords
	seen := make(map[string]bool)
	result := make([]string, 0, len(keywords))
	for _, k := range keywords {
		if !seen[k] {
			seen[k] = true
			result = append(result, k)
		}
	}

	// Sort keywords alphabetically for consistent ordering
	sort.Strings(result)
	return result
}

// ValidateJobData validates the job data using traditional methods
func (a *TraditionalAnalyzer) ValidateJobData(ctx context.Context, job *types.JobDetail) (*ValidationResult, error) {
	if job == nil {
		return nil, errors.New("job is nil")
	}

	result := &ValidationResult{
		Valid:  true,
		Score:  1.0,
		Errors: make([]string, 0),
	}

	// Check required fields
	if job.Title == "" {
		result.Valid = false
		result.Score *= 0.7
		result.Errors = append(result.Errors, "missing job title")
	}

	if job.FullDescription == "" {
		result.Valid = false
		result.Score *= 0.5
		result.Errors = append(result.Errors, "missing job description")
	}

	if job.Company == "" {
		result.Valid = false
		result.Score *= 0.8
		result.Errors = append(result.Errors, "missing company name")
	}

	if job.Location == "" {
		result.Valid = false
		result.Score *= 0.9
		result.Errors = append(result.Errors, "missing location")
	}

	// Check quality of description
	if len(job.FullDescription) < 100 {
		result.Score *= 0.8
		result.Errors = append(result.Errors, "job description is too short")
	}

	// Check for required fields that enhance matching
	if job.WorkplaceType == "" {
		result.Score *= 0.9
		result.Errors = append(result.Errors, "missing workplace type")
	}

	if job.EmploymentType == "" {
		result.Score *= 0.9
		result.Errors = append(result.Errors, "missing employment type")
	}

	if job.Industry == "" {
		result.Score *= 0.9
		result.Errors = append(result.Errors, "missing industry")
	}

	if len(job.Requirements) == 0 {
		result.Score *= 0.8
		result.Errors = append(result.Errors, "no requirements specified")
	}

	if len(job.Skills) == 0 {
		result.Score *= 0.8
		result.Errors = append(result.Errors, "no skills specified")
	}

	return result, nil
}

*/
