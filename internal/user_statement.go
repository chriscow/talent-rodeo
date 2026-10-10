package internal

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"talent-rodeo/assets"

	"github.com/chriscow/minds"
	"github.com/getsentry/sentry-go"
	"github.com/sashabaranov/go-openai"
	tools "talent-rodeo/internal/llmtools"
)

const (
	userStatementsCollection = "user_statements"
)

// UserStatementVersion represents a versioned user statement stored in Firestore
type UserStatementVersion struct {
	ID            string // Deterministic ID: {userID}_v{versionNumber}
	VersionNumber int    // Sequential version (1, 2, 3...)
	IsLatest      bool   // True for current active version

	// Statement Content
	StatementText string // The actual statement text
	StatementHash string // SHA-256 hash for change detection

	// Generation Metadata
	GeneratedBy      string // "transcript", "feedback", "followup", "manual", "profile"
	GenerationReason string // Human-readable reason for change
	PromptFile       string // Versioned prompt file used
	PromptVersion    string // Extracted version from filename
	LLMModel         string // Model used for generation
	LLMTokensUsed    *int   // Token count if available

	// Source Data (for regeneration)
	SourceData          map[string]any       // Serialized Go struct managed by source system
	TransformationSteps []TransformationStep // Applied transformations

	// Timestamps and Tracking
	CreatedAt         time.Time // When this version was created
	CreatedBy         *string   // User ID if manual, system if auto
	PreviousVersionID *string   // Reference to previous version

	// Change Analysis
	ChangeType        string  // "initial", "enhancement", "correction", "feedback_driven"
	ChangeSummary     *string // LLM-generated summary of changes
	SignificantChange bool    // True if meaningful content change

	// Cascade Regeneration Fields
	DependsOnVersionID  *string  // Previous version this depends on
	DependentVersionIDs []string // Versions that depend on this one
	SystemType          string   // transcript, feedback, inbox, followup
	IsReplacedVersion   bool     // True if this version was replaced during cascade
}

// RegenerateProfileStatement recreates a written-preferences statement from its
// saved resume and form inputs, without inventing transcript or interview data.
func RegenerateProfileStatement(version UserStatementVersion) (string, error) {
	if version.GeneratedBy != "profile" || version.SourceData["source_type"] != "profile" {
		return "", fmt.Errorf("statement version is not sourced from a profile")
	}
	resumeText, ok := version.SourceData["resume_raw_text"].(string)
	if !ok || strings.TrimSpace(resumeText) == "" {
		return "", fmt.Errorf("profile source has no resume text")
	}
	preferences, ok := version.SourceData["preferences"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("profile source has no preferences")
	}
	roles, err := profileSourceStrings(preferences["target_roles"])
	if err != nil {
		return "", fmt.Errorf("decode target roles: %w", err)
	}
	workTypeNames, err := profileSourceStrings(preferences["work_types"])
	if err != nil {
		return "", fmt.Errorf("decode work arrangements: %w", err)
	}
	workTypes := make([]WorkType, 0, len(workTypeNames))
	for _, name := range workTypeNames {
		workTypes = append(workTypes, WorkType(name))
	}
	p := PersonalPreferences{
		TargetRoles: roles, WorkTypes: workTypes,
		Locations:     profileSourceString(preferences, "locations"),
		Seniority:     ExperienceLevel(profileSourceString(preferences, "seniority")),
		DesiredTraits: profileSourceString(preferences, "desired_traits"),
		AvoidedTraits: profileSourceString(preferences, "avoided_traits"),
		Timezone:      profileSourceString(preferences, "timezone"),
	}
	if err := p.Validate(); err != nil {
		return "", fmt.Errorf("invalid profile source: %w", err)
	}
	resume := Resume{RawText: resumeText}
	if value := profileSourceOptionalString(version.SourceData["resume_annotated"]); value != nil {
		resume.Annotated = value
	}
	if value := profileSourceOptionalString(version.SourceData["resume_search_analysis"]); value != nil {
		resume.SearchAnalysis = value
	}
	return personalStatement(resume, p), nil
}

func profileSourceString(source map[string]any, key string) string {
	switch value := source[key].(type) {
	case string:
		return value
	case ExperienceLevel:
		return string(value)
	default:
		return ""
	}
}

func profileSourceOptionalString(value any) *string {
	switch source := value.(type) {
	case string:
		return &source
	case *string:
		return source
	default:
		return nil
	}
}

func profileSourceStrings(value any) ([]string, error) {
	switch items := value.(type) {
	case []string:
		return append([]string(nil), items...), nil
	case []WorkType:
		result := make([]string, 0, len(items))
		for _, item := range items {
			result = append(result, string(item))
		}
		return result, nil
	case []any:
		result := make([]string, 0, len(items))
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("non-string source value")
			}
			result = append(result, text)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("missing string list")
	}
}

// TransformationStep represents a single step in the statement generation process
type TransformationStep struct {
	StepNumber         int            // Order of application
	TransformationType string         // "transcript_analysis", "feedback_integration", etc.
	InputData          map[string]any // Data used for this step
	OutputData         map[string]any // Result of transformation
	PromptUsed         string         // Exact prompt text used
	ProcessedAt        time.Time      // When this step was processed
}

// CascadeOperation tracks cascade regeneration operations
type CascadeOperation struct {
	ID               string
	UserID           string
	InitiatedAt      time.Time
	CompletedAt      *time.Time
	TriggerVersionID string   // Version that started the cascade
	TriggerPrompt    string   // New prompt applied to trigger version
	VersionsAffected []string // All version IDs that were regenerated
	DryRun           bool
	Status           string // "in_progress", "completed", "failed"
	Error            *string
}

// CascadeReport provides detailed information about a cascade operation
type CascadeReport struct {
	Operation          *CascadeOperation
	VersionsProcessed  int
	VersionsSuccessful int
	VersionsFailed     int
	Results            []CascadeVersionResult
	Duration           time.Duration
}

// CascadeVersionResult tracks the result of regenerating one version in a cascade
type CascadeVersionResult struct {
	VersionID       string
	Status          string // "success", "failed", "skipped"
	Error           *string
	NewVersionID    *string
	PromptUsed      string
	SystemType      string
	StatementLength int
}

// CreateInitialStatement creates the first version of a user's statement from transcript
func CreateInitialStatement(ctx context.Context, fs *FirestoreClient, userID string, transcript *Transcript, promptFile string) (*UserStatementVersion, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	if transcript == nil {
		return nil, fmt.Errorf("transcript is required")
	}
	if promptFile == "" {
		return nil, fmt.Errorf("promptFile is required")
	}

	// Check if user already has statement versions
	existing, err := GetLatestStatement(ctx, fs, userID)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("user %s already has statement versions", userID)
	}

	// Generate statement using transcript processing logic
	statementText, err := generateStatementFromTranscript(ctx, fs, transcript, promptFile, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate statement from transcript: %w", err)
	}

	// Extract prompt version from filename
	promptVersion := ExtractPromptVersion(promptFile)

	// Create source data for transcript-based generation
	sourceData := map[string]any{
		"transcript_text":     transcript.String(), // Convert transcript to string
		"messages":            transcript.Messages, // Store raw messages for regeneration
		"location_preference": nil,                 // TODO: Extract from transcript analysis
		"steering_query":      nil,                 // TODO: Extract from transcript analysis
		"appointment_id":      "",                  // TODO: Pass from caller
		"appointment_type":    "voice",             // TODO: Determine from transcript or pass from caller
		"extraction_metadata": map[string]any{},    // TODO: Add LLM extraction details
	}

	// Create transformation steps
	transformationSteps := []TransformationStep{
		{
			StepNumber:         1,
			TransformationType: "transcript_analysis",
			InputData:          sourceData,
			OutputData: map[string]any{
				"statement_text": statementText,
			},
			PromptUsed:  promptFile, // TODO: Load actual prompt content
			ProcessedAt: time.Now(),
		},
	}

	// Generate statement hash
	statementHash := generateStatementHash(statementText)

	// Create the version
	version := &UserStatementVersion{
		ID:                  fmt.Sprintf("%s_v%d", userID, 1),
		VersionNumber:       1,
		IsLatest:            true,
		StatementText:       statementText,
		StatementHash:       statementHash,
		GeneratedBy:         "transcript",
		GenerationReason:    "Initial statement generation from transcript",
		PromptFile:          promptFile,
		PromptVersion:       promptVersion,
		LLMModel:            "TODO: Get from config", // TODO: Add LLM model config
		LLMTokensUsed:       nil,                     // TODO: Track token usage
		SourceData:          sourceData,
		TransformationSteps: transformationSteps,
		CreatedAt:           time.Now(),
		CreatedBy:           nil, // System-generated
		PreviousVersionID:   nil, // First version
		ChangeType:          "initial",
		ChangeSummary:       nil, // No changes for first version
		SignificantChange:   true,
	}

	// Use transaction to ensure atomicity
	err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Store the version in subcollection
		versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(version.ID)
		if err := tx.Set(versionRef, version); err != nil {
			return fmt.Errorf("failed to create statement version: %w", err)
		}

		// Update User fields for backward compatibility
		userRef := fs.Collection(usersCollection).Doc(userID)
		updates := []firestore.Update{
			{Path: "Statement", Value: statementText},
			{Path: "CurrentStatementVersionID", Value: version.ID},
			{Path: "StatementLastUpdated", Value: version.CreatedAt},
		}
		if err := tx.Update(userRef, updates); err != nil {
			return fmt.Errorf("failed to update user with statement version: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create initial statement: %w", err)
	}

	// Invalidate user cache
	fs.EvictUserCache(userID)

	return version, nil
}

// GetLatestStatement retrieves the current active statement version for a user
func GetLatestStatement(ctx context.Context, fs *FirestoreClient, userID string) (*UserStatementVersion, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}

	// First try to get the latest version ID from the user document
	user, exists, err := GetUser(ctx, fs, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	// If user has CurrentStatementVersionID, use it
	if user.CurrentStatementVersionID != nil {
		versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(*user.CurrentStatementVersionID)
		doc, err := versionRef.Get(ctx)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// Version referenced by user doesn't exist, fall back to query
				return getLatestStatementByQuery(ctx, fs, userID)
			}
			return nil, fmt.Errorf("failed to get statement version %s: %w", *user.CurrentStatementVersionID, err)
		}

		var version UserStatementVersion
		if err := doc.DataTo(&version); err != nil {
			return nil, fmt.Errorf("failed to unmarshal statement version: %w", err)
		}

		// Validate is_latest flag
		if !version.IsLatest {
			// Inconsistency detected, log and fall back to query
			slog.Warn("Statement version marked as current but IsLatest=false",
				"userID", userID,
				"versionID", version.ID)
			return getLatestStatementByQuery(ctx, fs, userID)
		}

		return &version, nil
	}

	// Fall back to querying for latest version
	return getLatestStatementByQuery(ctx, fs, userID)
}

// getLatestStatementByQuery finds the latest statement version by querying
func getLatestStatementByQuery(ctx context.Context, fs *FirestoreClient, userID string) (*UserStatementVersion, error) {
	// Query for the latest version (highest version number)
	iter := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).
		OrderBy("VersionNumber", firestore.Desc).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, nil // No versions found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query for latest statement: %w", err)
	}

	var version UserStatementVersion
	if err := doc.DataTo(&version); err != nil {
		return nil, fmt.Errorf("failed to unmarshal statement version: %w", err)
	}

	return &version, nil
}

// GetStatementHistory retrieves all versions for a user with pagination
func GetStatementHistory(ctx context.Context, fs *FirestoreClient, userID string, limit int, cursor *string) ([]UserStatementVersion, *string, error) {
	if fs == nil {
		return nil, nil, fmt.Errorf("firestore client is nil")
	}
	if userID == "" {
		return nil, nil, fmt.Errorf("userID is required")
	}
	if limit <= 0 {
		limit = 10 // Default limit
	}

	query := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).
		OrderBy("VersionNumber", firestore.Desc).
		Limit(limit)

	// Apply cursor if provided
	if cursor != nil && *cursor != "" {
		// Get the document for the cursor
		cursorDoc, err := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(*cursor).Get(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get cursor document: %w", err)
		}
		query = query.StartAfter(cursorDoc)
	}

	iter := query.Documents(ctx)
	var versions []UserStatementVersion
	var lastDoc *firestore.DocumentSnapshot

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to iterate statement versions: %w", err)
		}

		var version UserStatementVersion
		if err := doc.DataTo(&version); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal statement version: %w", err)
		}

		versions = append(versions, version)
		lastDoc = doc
	}

	// Generate next cursor if we have more results
	var nextCursor *string
	if len(versions) == limit && lastDoc != nil {
		nextCursor = &lastDoc.Ref.ID
	}

	return versions, nextCursor, nil
}

// ValidateUserStatement validates statement content and structure
func ValidateUserStatement(userID string, statementText string) error {
	if userID == "" {
		return fmt.Errorf("userID is required")
	}
	if strings.TrimSpace(statementText) == "" {
		return fmt.Errorf("statement text cannot be empty")
	}

	// TODO: Add future validation rules (length, format, etc.)
	// For now, just basic validation
	if len(statementText) > 10000 {
		return fmt.Errorf("statement text too long (max 10000 characters)")
	}

	return nil
}

// SyncUserStatementField ensures User.Statement field matches latest version
func SyncUserStatementField(ctx context.Context, fs *FirestoreClient, userID string) error {
	if fs == nil {
		return fmt.Errorf("firestore client is nil")
	}
	if userID == "" {
		return fmt.Errorf("userID is required")
	}

	// Get latest statement version
	latest, err := GetLatestStatement(ctx, fs, userID)
	if err != nil {
		return fmt.Errorf("failed to get latest statement: %w", err)
	}

	// Use transaction to ensure atomicity
	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		userRef := fs.Collection(usersCollection).Doc(userID)

		if latest == nil {
			// No versions exist, clear the fields
			updates := []firestore.Update{
				{Path: "Statement", Value: nil},
				{Path: "CurrentStatementVersionID", Value: nil},
				{Path: "StatementLastUpdated", Value: nil},
			}
			return tx.Update(userRef, updates)
		}

		// Update fields to match latest version
		updates := []firestore.Update{
			{Path: "Statement", Value: latest.StatementText},
			{Path: "CurrentStatementVersionID", Value: latest.ID},
			{Path: "StatementLastUpdated", Value: latest.CreatedAt},
		}
		return tx.Update(userRef, updates)
	})
}

// GetStatementForJobAnalysis retrieves statement text optimized for job analysis
func GetStatementForJobAnalysis(ctx context.Context, fs *FirestoreClient, userID string) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("firestore client is nil")
	}
	if userID == "" {
		return "", fmt.Errorf("userID is required")
	}

	// First try to get from User.Statement field for performance
	user, exists, err := GetUser(ctx, fs, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	if !exists {
		return "", fmt.Errorf("user not found: %s", userID)
	}

	// If User.Statement exists, use it for performance
	if user.Statement != nil && strings.TrimSpace(*user.Statement) != "" {
		return *user.Statement, nil
	}

	// Fallback to getting latest version
	latest, err := GetLatestStatement(ctx, fs, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get latest statement: %w", err)
	}
	if latest == nil {
		return "", fmt.Errorf("no statement found for user %s", userID)
	}

	// Sync the User.Statement field for future performance
	if err := SyncUserStatementField(ctx, fs, userID); err != nil {
		// Log error but don't fail the request
		slog.Warn("Failed to sync User.Statement field", "userID", userID, "error", err)
	}

	return latest.StatementText, nil
}

// ExtractPromptVersion extracts version from prompt filename
func ExtractPromptVersion(filename string) string {
	// Extract version from patterns like:
	// "transcript.user-statement.v001.txt" -> "v001"
	// "statement-enhancement-with-questions.v002.txt" -> "v002"
	// "feedback-statement-update.v001.txt" -> "v001"

	parts := strings.Split(filename, ".")
	for _, part := range parts {
		if strings.HasPrefix(part, "v") && len(part) >= 2 {
			// Check if the rest are digits
			versionNum := part[1:]
			if len(versionNum) > 0 {
				// Basic validation - should be numeric
				isNumeric := true
				for _, char := range versionNum {
					if char < '0' || char > '9' {
						isNumeric = false
						break
					}
				}
				if isNumeric {
					return part
				}
			}
		}
	}

	return "v001" // Default version if not found
}

// EnhanceStatementWithAppFeedback creates new version incorporating application-based user feedback
func EnhanceStatementWithAppFeedback(ctx context.Context, fs *FirestoreClient, userID string, feedbackData map[string]any, promptFile string) (*UserStatementVersion, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	if feedbackData == nil {
		return nil, fmt.Errorf("feedbackData is required")
	}
	if promptFile == "" {
		return nil, fmt.Errorf("promptFile is required")
	}

	// Get latest statement version
	previousVersion, err := GetLatestStatement(ctx, fs, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest statement: %w", err)
	}
	if previousVersion == nil {
		return nil, fmt.Errorf("no existing statement found for user %s", userID)
	}

	// Generate enhanced statement using feedback
	enhancedText, err := generateEnhancedStatementFromFeedback(ctx, fs, previousVersion.StatementText, feedbackData, promptFile)
	if err != nil {
		return nil, fmt.Errorf("failed to generate enhanced statement: %w", err)
	}

	// Extract prompt version from filename
	promptVersion := ExtractPromptVersion(promptFile)

	// Create source data preserving previous source and adding feedback
	sourceData := make(map[string]any)
	// Copy previous source data
	for k, v := range previousVersion.SourceData {
		sourceData[k] = v
	}
	// Add feedback data
	sourceData["feedback_data"] = feedbackData
	sourceData["enhancement_reason"] = "feedback_integration"

	// Create transformation steps
	transformationSteps := append([]TransformationStep{}, previousVersion.TransformationSteps...)
	transformationSteps = append(transformationSteps, TransformationStep{
		StepNumber:         len(transformationSteps) + 1,
		TransformationType: "feedback_integration",
		InputData: map[string]any{
			"previous_statement": previousVersion.StatementText,
			"feedback_data":      feedbackData,
		},
		OutputData: map[string]any{
			"enhanced_statement": enhancedText,
		},
		PromptUsed:  promptFile, // TODO: Load actual prompt content
		ProcessedAt: time.Now(),
	})

	// Generate statement hash
	statementHash := generateStatementHash(enhancedText)

	// Create the new version
	newVersionNumber := previousVersion.VersionNumber + 1
	version := &UserStatementVersion{
		ID:                  fmt.Sprintf("%s_v%d", userID, newVersionNumber),
		VersionNumber:       newVersionNumber,
		IsLatest:            true,
		StatementText:       enhancedText,
		StatementHash:       statementHash,
		GeneratedBy:         "feedback",
		GenerationReason:    "Enhanced statement based on user feedback",
		PromptFile:          promptFile,
		PromptVersion:       promptVersion,
		LLMModel:            "TODO: Get from config", // TODO: Add LLM model config
		LLMTokensUsed:       nil,                     // TODO: Track token usage
		SourceData:          sourceData,
		TransformationSteps: transformationSteps,
		CreatedAt:           time.Now(),
		CreatedBy:           nil, // System-generated
		PreviousVersionID:   &previousVersion.ID,
		ChangeType:          "feedback_driven",
		ChangeSummary:       nil,  // TODO: Generate change summary
		SignificantChange:   true, // TODO: Analyze if change is significant
	}

	// Use transaction to ensure atomicity
	err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Mark previous version as not latest
		previousVersionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(previousVersion.ID)
		if err := tx.Update(previousVersionRef, []firestore.Update{
			{Path: "IsLatest", Value: false},
		}); err != nil {
			return fmt.Errorf("failed to update previous version: %w", err)
		}

		// Store the new version in subcollection
		versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(version.ID)
		if err := tx.Set(versionRef, version); err != nil {
			return fmt.Errorf("failed to create statement version: %w", err)
		}

		// Update User fields for backward compatibility
		userRef := fs.Collection(usersCollection).Doc(userID)
		updates := []firestore.Update{
			{Path: "Statement", Value: enhancedText},
			{Path: "CurrentStatementVersionID", Value: version.ID},
			{Path: "StatementLastUpdated", Value: version.CreatedAt},
		}
		if err := tx.Update(userRef, updates); err != nil {
			return fmt.Errorf("failed to update user with statement version: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to enhance statement with feedback: %w", err)
	}

	// Invalidate user cache
	fs.EvictUserCache(userID)

	return version, nil
}

// EnhanceStatementWithAnswers creates new version incorporating followup question responses
func EnhanceStatementWithAnswers(ctx context.Context, fs *FirestoreClient, userID string, questionAnswers []map[string]any, promptFile string) (*UserStatementVersion, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	if len(questionAnswers) == 0 {
		return nil, fmt.Errorf("questionAnswers is required")
	}
	if promptFile == "" {
		return nil, fmt.Errorf("promptFile is required")
	}

	// Get latest statement version
	previousVersion, err := GetLatestStatement(ctx, fs, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest statement: %w", err)
	}
	if previousVersion == nil {
		return nil, fmt.Errorf("no existing statement found for user %s", userID)
	}

	// Generate enhanced statement using question insights
	// TODO: This will be implemented when we integrate with the followup system
	enhancedText := "TODO: Generate enhanced statement from question answers"

	// Extract prompt version from filename
	promptVersion := ExtractPromptVersion(promptFile)

	// Create source data preserving previous source and adding answers
	sourceData := make(map[string]any)
	// Copy previous source data
	for k, v := range previousVersion.SourceData {
		sourceData[k] = v
	}
	// Add question answer data
	sourceData["question_answers"] = questionAnswers
	sourceData["enhancement_reason"] = "followup_questions"

	// Create transformation steps
	transformationSteps := append([]TransformationStep{}, previousVersion.TransformationSteps...)
	transformationSteps = append(transformationSteps, TransformationStep{
		StepNumber:         len(transformationSteps) + 1,
		TransformationType: "followup_integration",
		InputData: map[string]any{
			"previous_statement": previousVersion.StatementText,
			"question_answers":   questionAnswers,
		},
		OutputData: map[string]any{
			"enhanced_statement": enhancedText,
		},
		PromptUsed:  promptFile, // TODO: Load actual prompt content
		ProcessedAt: time.Now(),
	})

	// Generate statement hash
	statementHash := generateStatementHash(enhancedText)

	// Create the new version
	newVersionNumber := previousVersion.VersionNumber + 1
	version := &UserStatementVersion{
		ID:                  fmt.Sprintf("%s_v%d", userID, newVersionNumber),
		VersionNumber:       newVersionNumber,
		IsLatest:            true,
		StatementText:       enhancedText,
		StatementHash:       statementHash,
		GeneratedBy:         "followup",
		GenerationReason:    "Enhanced statement based on followup question answers",
		PromptFile:          promptFile,
		PromptVersion:       promptVersion,
		LLMModel:            "TODO: Get from config", // TODO: Add LLM model config
		LLMTokensUsed:       nil,                     // TODO: Track token usage
		SourceData:          sourceData,
		TransformationSteps: transformationSteps,
		CreatedAt:           time.Now(),
		CreatedBy:           nil, // System-generated
		PreviousVersionID:   &previousVersion.ID,
		ChangeType:          "enhancement",
		ChangeSummary:       nil,  // TODO: Generate change summary
		SignificantChange:   true, // TODO: Analyze if change is significant
	}

	// Use transaction to ensure atomicity
	err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Mark previous version as not latest
		previousVersionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(previousVersion.ID)
		if err := tx.Update(previousVersionRef, []firestore.Update{
			{Path: "IsLatest", Value: false},
		}); err != nil {
			return fmt.Errorf("failed to update previous version: %w", err)
		}

		// Store the new version in subcollection
		versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(version.ID)
		if err := tx.Set(versionRef, version); err != nil {
			return fmt.Errorf("failed to create statement version: %w", err)
		}

		// Update User fields for backward compatibility
		userRef := fs.Collection(usersCollection).Doc(userID)
		updates := []firestore.Update{
			{Path: "Statement", Value: enhancedText},
			{Path: "CurrentStatementVersionID", Value: version.ID},
			{Path: "StatementLastUpdated", Value: version.CreatedAt},
		}
		if err := tx.Update(userRef, updates); err != nil {
			return fmt.Errorf("failed to update user with statement version: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to enhance statement with answers: %w", err)
	}

	// Invalidate user cache
	fs.EvictUserCache(userID)

	return version, nil
}

// generateStatementFromTranscript generates a user statement from transcript using LLM
func generateStatementFromTranscript(ctx context.Context, fs *FirestoreClient, transcript *Transcript, promptFile string, userID string) (string, error) {
	// Get user to access location preferences and steering query
	user, exists, err := GetUser(ctx, fs, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	if !exists {
		return "", fmt.Errorf("user not found: %s", userID)
	}

	// Load prompt template from embedded assets
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/" + promptFile)
	if err != nil {
		return "", fmt.Errorf("failed to read user statement prompt: %w", err)
	}

	// Generate prompt using tools.GeneratePrompt
	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"Transcript":         transcript.String(),
		"LocationPreference": user.LocationPreference,
		"SteeringQuery":      user.SteeringQuery,
	})
	if err != nil {
		SentryReportError(ctx, err, sentry.LevelError, nil)
		return "", fmt.Errorf("failed to generate prompt: %w", err)
	}

	// Ask LLM to generate user statement
	resp := openai.ChatCompletionResponse{}
	model := getModelForContext(ctx)
	statement, err := tools.Ask(ctx, prompt, tools.WithModel(model), tools.WithMaxTokens(8192), tools.WantCompletionResponse(&resp))
	if err != nil {
		SentryReportError(ctx, err, sentry.LevelError, nil)
		return "", fmt.Errorf("failed to ask LLM: %w", err)
	}

	// Record LLM cost for user statement generation
	if err := RecordLLMCost(ctx, fs, &userID, tools.DeepSeekFlash, resp, "user_statement_generation", nil); err != nil {
		// Log error but don't fail the operation
		slog.Warn("Failed to record LLM cost for user statement generation", "error", err, "userID", userID)
	}

	return statement, nil
}

// generateEnhancedStatementFromFeedback generates an enhanced statement using feedback data
func generateEnhancedStatementFromFeedback(ctx context.Context, fs *FirestoreClient, currentStatement string, feedbackData map[string]any, promptFile string) (string, error) {
	// Load prompt template from embedded assets
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/" + promptFile)
	if err != nil {
		return "", fmt.Errorf("failed to read statement update prompt: %w", err)
	}

	// Prepare feedback data for prompt generation
	promptData := prepareFeedbackDataForPrompt(feedbackData)
	promptData["CURRENT_STATEMENT"] = currentStatement

	// Generate prompt using tools.GeneratePrompt
	prompt, err := tools.GeneratePrompt(tmpl, promptData)
	if err != nil {
		SentryReportError(ctx, err, sentry.LevelError, nil)
		return "", fmt.Errorf("failed to generate prompt: %w", err)
	}

	// Ask LLM to generate enhanced statement
	resp := openai.ChatCompletionResponse{}
	model := getModelForContext(ctx)
	enhancedStatement, err := tools.Ask(ctx, prompt, tools.WithModel(model), tools.WithMaxTokens(8192), tools.WantCompletionResponse(&resp))
	if err != nil {
		SentryReportError(ctx, err, sentry.LevelError, nil)
		return "", fmt.Errorf("failed to ask LLM: %w", err)
	}

	// Log successful generation
	slog.Debug("Generated enhanced statement from feedback",
		"originalLength", len(currentStatement),
		"enhancedLength", len(enhancedStatement))

	return enhancedStatement, nil
}

// prepareFeedbackDataForPrompt converts feedback data to template-friendly format
func prepareFeedbackDataForPrompt(feedbackData map[string]any) map[string]any {
	promptData := make(map[string]any)

	// Extract and format liked themes
	if likedThemes, ok := feedbackData["liked_themes"]; ok {
		promptData["LIKED_THEMES"] = formatThemesForPromptTemplate(likedThemes, "liked")
	} else {
		promptData["LIKED_THEMES"] = "No liked themes identified."
	}

	// Extract and format disliked themes
	if dislikedThemes, ok := feedbackData["disliked_themes"]; ok {
		promptData["DISLIKED_THEMES"] = formatThemesForPromptTemplate(dislikedThemes, "disliked")
	} else {
		promptData["DISLIKED_THEMES"] = "No disliked themes identified."
	}

	// Extract confidence score
	if confidence, ok := feedbackData["confidence_score"].(float64); ok {
		promptData["CONFIDENCE_SCORE"] = fmt.Sprintf("%.2f", confidence)
	} else {
		promptData["CONFIDENCE_SCORE"] = "0.00"
	}

	// Extract reasoning
	if reasoning, ok := feedbackData["reasoning"].(string); ok {
		promptData["REASONING"] = reasoning
	} else {
		promptData["REASONING"] = "No specific reasoning provided"
	}

	return promptData
}

// formatThemesForPromptTemplate formats themes for Go template usage
func formatThemesForPromptTemplate(themesData any, preference string) string {
	switch themes := themesData.(type) {
	case []map[string]any:
		if len(themes) == 0 {
			return fmt.Sprintf("No %s themes identified.", preference)
		}
		var parts []string
		for _, theme := range themes {
			if themeText, ok := theme["theme"].(string); ok {
				if keywords, ok := theme["keywords"].([]string); ok {
					keywordStr := strings.Join(keywords, ", ")
					parts = append(parts, fmt.Sprintf("- %s (keywords: %s)", themeText, keywordStr))
				} else {
					parts = append(parts, fmt.Sprintf("- %s", themeText))
				}
			}
		}
		return strings.Join(parts, "\n")
	case []string:
		if len(themes) == 0 {
			return fmt.Sprintf("No %s themes identified.", preference)
		}
		var parts []string
		for _, theme := range themes {
			parts = append(parts, fmt.Sprintf("- %s", theme))
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprintf("No %s themes identified.", preference)
	}
}

// prepareFeedbackSummaryForPrompt extracts key feedback data for prompt generation
func prepareFeedbackSummaryForPrompt(feedbackData map[string]any) (map[string]any, error) {
	summary := make(map[string]any)

	// Extract semantic analysis if present
	if analysis, ok := feedbackData["semantic_analysis"]; ok {
		summary["semantic_analysis"] = analysis
	}

	// Extract liked/disliked themes
	if likedThemes, ok := feedbackData["liked_themes"]; ok {
		summary["liked_themes"] = likedThemes
	}
	if dislikedThemes, ok := feedbackData["disliked_themes"]; ok {
		summary["disliked_themes"] = dislikedThemes
	}

	// Extract confidence score
	if confidence, ok := feedbackData["confidence_score"]; ok {
		summary["confidence_score"] = confidence
	}

	// Extract reasoning
	if reasoning, ok := feedbackData["reasoning"]; ok {
		summary["reasoning"] = reasoning
	}

	// If no recognized feedback data, create basic summary
	if len(summary) == 0 {
		summary["message"] = "No specific feedback themes identified"
		summary["confidence_score"] = 0.0
	}

	return summary, nil
}

// generateFeedbackPrompt creates a prompt for feedback-based statement enhancement
func generateFeedbackPrompt(tmpl []byte, currentStatement string, feedbackSummary map[string]any) (string, error) {
	prompt := string(tmpl)

	// Replace current statement
	prompt = strings.ReplaceAll(prompt, "{{CURRENT_STATEMENT}}", currentStatement)

	// Format liked themes
	likedThemesText := formatThemesForPrompt(feedbackSummary["liked_themes"], "liked")
	prompt = strings.ReplaceAll(prompt, "{{LIKED_THEMES}}", likedThemesText)

	// Format disliked themes
	dislikedThemesText := formatThemesForPrompt(feedbackSummary["disliked_themes"], "disliked")
	prompt = strings.ReplaceAll(prompt, "{{DISLIKED_THEMES}}", dislikedThemesText)

	// Add confidence score
	if confidence, ok := feedbackSummary["confidence_score"].(float64); ok {
		prompt = strings.ReplaceAll(prompt, "{{CONFIDENCE_SCORE}}", fmt.Sprintf("%.2f", confidence))
	} else {
		prompt = strings.ReplaceAll(prompt, "{{CONFIDENCE_SCORE}}", "0.00")
	}

	// Add reasoning
	if reasoning, ok := feedbackSummary["reasoning"].(string); ok {
		prompt = strings.ReplaceAll(prompt, "{{REASONING}}", reasoning)
	} else {
		prompt = strings.ReplaceAll(prompt, "{{REASONING}}", "No specific reasoning provided")
	}

	return prompt, nil
}

// formatThemesForPrompt formats themes data for inclusion in prompts
func formatThemesForPrompt(themesData any, preference string) string {
	if themesData == nil {
		return fmt.Sprintf("No %s themes identified.", preference)
	}

	// Try to handle different theme data formats
	switch themes := themesData.(type) {
	case []map[string]any:
		if len(themes) == 0 {
			return fmt.Sprintf("No %s themes identified.", preference)
		}
		var parts []string
		for _, theme := range themes {
			if themeText, ok := theme["theme"].(string); ok {
				if keywords, ok := theme["keywords"].([]string); ok {
					keywordStr := strings.Join(keywords, ", ")
					parts = append(parts, fmt.Sprintf("- %s (keywords: %s)", themeText, keywordStr))
				} else {
					parts = append(parts, fmt.Sprintf("- %s", themeText))
				}
			}
		}
		return strings.Join(parts, "\n")
	case []string:
		if len(themes) == 0 {
			return fmt.Sprintf("No %s themes identified.", preference)
		}
		var parts []string
		for _, theme := range themes {
			parts = append(parts, fmt.Sprintf("- %s", theme))
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprintf("No %s themes identified.", preference)
	}
}

// StatementComparison represents the result of comparing two statement versions
type StatementComparison struct {
	Version1           *UserStatementVersion
	Version2           *UserStatementVersion
	IdenticalContent   bool
	IdenticalHash      bool
	LengthDifference   int
	CharacterChanges   int
	WordChanges        int
	SemanticDifference *string // LLM-generated semantic analysis
	ChangeImpactScore  float64 // 0.0 (minimal) to 1.0 (major change)
	GenerationGap      int     // Number of versions between compared versions
	TimeDifference     time.Duration
	SourceChanges      []string // List of source data differences
}

// CompareStatementVersions analyzes differences between two statement versions
func CompareStatementVersions(ctx context.Context, fs *FirestoreClient, version1, version2 *UserStatementVersion, performSemanticAnalysis bool) (*StatementComparison, error) {
	if version1 == nil || version2 == nil {
		return nil, fmt.Errorf("both versions are required for comparison")
	}

	comparison := &StatementComparison{
		Version1: version1,
		Version2: version2,
	}

	// Basic content comparison
	comparison.IdenticalContent = version1.StatementText == version2.StatementText
	comparison.IdenticalHash = version1.StatementHash == version2.StatementHash
	comparison.LengthDifference = len(version2.StatementText) - len(version1.StatementText)
	comparison.GenerationGap = absInt(version2.VersionNumber - version1.VersionNumber)
	comparison.TimeDifference = version2.CreatedAt.Sub(version1.CreatedAt)

	// Character-level changes
	comparison.CharacterChanges = calculateCharacterChanges(version1.StatementText, version2.StatementText)

	// Word-level changes
	comparison.WordChanges = calculateWordChanges(version1.StatementText, version2.StatementText)

	// Source data changes
	comparison.SourceChanges = analyzeSourceDataChanges(version1.SourceData, version2.SourceData)

	// Calculate impact score based on various factors
	comparison.ChangeImpactScore = calculateChangeImpactScore(comparison)

	// Optional semantic analysis
	if performSemanticAnalysis && !comparison.IdenticalContent {
		semanticDiff, err := generateSemanticDifference(ctx, version1.StatementText, version2.StatementText)
		if err != nil {
			// Log error but don't fail the comparison
			slog.Warn("Failed to generate semantic difference", "error", err)
		} else {
			comparison.SemanticDifference = &semanticDiff
		}
	}

	return comparison, nil
}

// calculateCharacterChanges counts character-level differences between two texts
func calculateCharacterChanges(text1, text2 string) int {
	// Simple character difference count (Levenshtein distance would be more accurate)
	minLen := min(len(text1), len(text2))

	changes := absInt(len(text1) - len(text2)) // Length difference

	// Count character differences in the common portion
	for i := 0; i < minLen; i++ {
		if text1[i] != text2[i] {
			changes++
		}
	}

	return changes
}

// calculateWordChanges counts word-level differences between two texts
func calculateWordChanges(text1, text2 string) int {
	words1 := strings.Fields(strings.ToLower(text1))
	words2 := strings.Fields(strings.ToLower(text2))

	// Create word frequency maps
	freq1 := make(map[string]int)
	freq2 := make(map[string]int)

	for _, word := range words1 {
		freq1[word]++
	}
	for _, word := range words2 {
		freq2[word]++
	}

	// Count differences
	changes := 0
	allWords := make(map[string]bool)

	for word := range freq1 {
		allWords[word] = true
	}
	for word := range freq2 {
		allWords[word] = true
	}

	for word := range allWords {
		count1 := freq1[word]
		count2 := freq2[word]
		changes += absInt(count1 - count2)
	}

	return changes
}

// analyzeSourceDataChanges identifies what source data changed between versions
func analyzeSourceDataChanges(source1, source2 map[string]any) []string {
	var changes []string

	// Get all keys from both sources
	allKeys := make(map[string]bool)
	for key := range source1 {
		allKeys[key] = true
	}
	for key := range source2 {
		allKeys[key] = true
	}

	for key := range allKeys {
		val1, exists1 := source1[key]
		val2, exists2 := source2[key]

		if !exists1 && exists2 {
			changes = append(changes, fmt.Sprintf("Added %s", key))
		} else if exists1 && !exists2 {
			changes = append(changes, fmt.Sprintf("Removed %s", key))
		} else if exists1 && exists2 {
			// Compare values (simple string comparison for now)
			str1 := fmt.Sprintf("%v", val1)
			str2 := fmt.Sprintf("%v", val2)
			if str1 != str2 {
				changes = append(changes, fmt.Sprintf("Modified %s", key))
			}
		}
	}

	return changes
}

// calculateChangeImpactScore calculates a score representing the magnitude of change
func calculateChangeImpactScore(comparison *StatementComparison) float64 {
	if comparison.IdenticalContent {
		return 0.0
	}

	score := 0.0

	// Length change impact (0.0 to 0.3)
	if comparison.Version1 != nil && len(comparison.Version1.StatementText) > 0 {
		lengthChangeRatio := float64(absInt(comparison.LengthDifference)) / float64(len(comparison.Version1.StatementText))
		score += min64(lengthChangeRatio, 0.3)
	}

	// Word change impact (0.0 to 0.4)
	if comparison.Version1 != nil {
		words1 := strings.Fields(comparison.Version1.StatementText)
		if len(words1) > 0 {
			wordChangeRatio := float64(comparison.WordChanges) / float64(len(words1))
			score += min64(wordChangeRatio, 0.4)
		}
	}

	// Source data change impact (0.0 to 0.3)
	if len(comparison.SourceChanges) > 0 {
		sourceChangeScore := min64(float64(len(comparison.SourceChanges))/10.0, 0.3) // Max 10 changes for full score
		score += sourceChangeScore
	}

	// Cap at 1.0
	return min64(score, 1.0)
}

// generateSemanticDifference uses LLM to analyze semantic differences
func generateSemanticDifference(ctx context.Context, text1, text2 string) (string, error) {
	prompt := fmt.Sprintf(`Analyze the semantic differences between these two user statements and provide a concise summary of the changes in meaning, intent, or preferences:

Statement 1: %s

Statement 2: %s

Focus on:
- Changes in career goals or preferences
- New or removed skills/technologies
- Location or work arrangement changes
- Industry or role changes
- Any shifts in priorities or values

Provide a clear, one-paragraph summary of the key semantic changes.`, text1, text2)

	model := getModelForContext(ctx)
	analysis, err := tools.Ask(ctx, prompt, tools.WithModel(model), tools.WithMaxTokens(1024))
	if err != nil {
		// Fallback to simple heuristic analysis if LLM fails
		return generateSimpleSemanticDifference(text1, text2), nil
	}

	return analysis, nil
}

// generateSimpleSemanticDifference provides a fallback heuristic-based analysis
func generateSimpleSemanticDifference(text1, text2 string) string {
	words1 := strings.Fields(strings.ToLower(text1))
	words2 := strings.Fields(strings.ToLower(text2))

	// Find added and removed words
	wordSet1 := make(map[string]bool)
	wordSet2 := make(map[string]bool)

	for _, word := range words1 {
		wordSet1[word] = true
	}
	for _, word := range words2 {
		wordSet2[word] = true
	}

	var added, removed []string
	for word := range wordSet2 {
		if !wordSet1[word] {
			added = append(added, word)
		}
	}
	for word := range wordSet1 {
		if !wordSet2[word] {
			removed = append(removed, word)
		}
	}

	analysis := "Semantic changes detected:\n"
	if len(added) > 0 {
		analysis += fmt.Sprintf("Added concepts: %s\n", strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		analysis += fmt.Sprintf("Removed concepts: %s\n", strings.Join(removed, ", "))
	}

	if len(added) == 0 && len(removed) == 0 {
		analysis = "No significant semantic changes detected."
	}

	return analysis
}

// Helper functions
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// generateStatementHash creates a SHA-256 hash of the statement text
func generateStatementHash(statementText string) string {
	hasher := sha256.New()
	hasher.Write([]byte(strings.TrimSpace(statementText)))
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

// getModelForContext returns the appropriate LLM model for the current context
// Checks for test context override, otherwise uses DeepSeekFlash.
func getModelForContext(ctx context.Context) string {
	// Check if we're in a test context with mock model preference
	if ctx.Value("test_use_mock_model") != nil {
		return tools.MockModel
	}
	return tools.DeepSeekFlash
}

// RegenerateStatement recreates a statement version from its source data with a new prompt
func RegenerateStatement(ctx context.Context, fs *FirestoreClient, versionID string, newPromptFile string) (*UserStatementVersion, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}
	if versionID == "" {
		return nil, fmt.Errorf("versionID is required")
	}
	if newPromptFile == "" {
		return nil, fmt.Errorf("newPromptFile is required")
	}

	// Get the existing version
	parts := strings.Split(versionID, "_v")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid version ID format: %s", versionID)
	}
	userID := parts[0]

	// Load existing version
	versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(versionID)
	doc, err := versionRef.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get version %s: %w", versionID, err)
	}

	var existingVersion UserStatementVersion
	if err := doc.DataTo(&existingVersion); err != nil {
		return nil, fmt.Errorf("failed to decode version %s: %w", versionID, err)
	}

	// Validate that we have source data to regenerate from
	if existingVersion.SourceData == nil {
		return nil, fmt.Errorf("version %s has no source data for regeneration", versionID)
	}

	// Check prompt compatibility
	compatibility, err := CheckPromptCompatibility(existingVersion.PromptFile, newPromptFile)
	if err != nil {
		return nil, fmt.Errorf("failed to check prompt compatibility: %w", err)
	}
	if !compatibility.IsCompatible {
		return nil, fmt.Errorf("prompt incompatibility: %s", compatibility.Reason)
	}

	// Validate prompt is suitable for the generation method
	if err := ValidatePromptForGeneration(newPromptFile, existingVersion.GeneratedBy); err != nil {
		return nil, fmt.Errorf("prompt validation failed: %w", err)
	}

	// Extract prompt version from filename
	newPromptVersion := ExtractPromptVersion(newPromptFile)

	// Determine regeneration type based on original generation method
	var newStatement string
	switch existingVersion.GeneratedBy {
	case "transcript":
		// Regenerate from transcript data
		newStatement, err = regenerateFromTranscript(ctx, fs, userID, existingVersion.SourceData, newPromptFile)
		if err != nil {
			return nil, fmt.Errorf("failed to regenerate from transcript: %w", err)
		}

	case "feedback":
		// Regenerate from feedback data
		newStatement, err = regenerateFromFeedback(ctx, fs, userID, existingVersion.SourceData, newPromptFile)
		if err != nil {
			return nil, fmt.Errorf("failed to regenerate from feedback: %w", err)
		}

	case "followup":
		// Regenerate from followup answers
		newStatement, err = regenerateFromFollowup(ctx, fs, userID, existingVersion.SourceData, newPromptFile)
		if err != nil {
			return nil, fmt.Errorf("failed to regenerate from followup: %w", err)
		}

	default:
		return nil, fmt.Errorf("unknown generation method: %s", existingVersion.GeneratedBy)
	}

	// Create new version with regenerated content
	newVersionNumber := existingVersion.VersionNumber + 1000 // Use large offset to distinguish regenerated versions
	newVersionID := fmt.Sprintf("%s_v%d", userID, newVersionNumber)

	newVersion := &UserStatementVersion{
		ID:               newVersionID,
		VersionNumber:    newVersionNumber,
		IsLatest:         false, // Don't automatically make regenerated versions latest
		StatementText:    newStatement,
		StatementHash:    generateStatementHash(newStatement),
		GeneratedBy:      existingVersion.GeneratedBy + "_regenerated",
		GenerationReason: fmt.Sprintf("Regenerated from version %d with prompt %s", existingVersion.VersionNumber, newPromptFile),
		PromptFile:       newPromptFile,
		PromptVersion:    newPromptVersion,
		LLMModel:         getModelForContext(ctx),
		SourceData:       existingVersion.SourceData, // Preserve original source data
		TransformationSteps: append(existingVersion.TransformationSteps, TransformationStep{
			StepNumber:         len(existingVersion.TransformationSteps) + 1,
			TransformationType: "regeneration",
			InputData: map[string]any{
				"original_version_id": versionID,
				"new_prompt_file":     newPromptFile,
				"new_prompt_version":  newPromptVersion,
			},
			OutputData: map[string]any{
				"statement_length": len(newStatement),
				"statement_hash":   generateStatementHash(newStatement),
			},
			PromptUsed:  newPromptFile,
			ProcessedAt: time.Now(),
		}),
		CreatedAt:         time.Now(),
		PreviousVersionID: &existingVersion.ID,
		ChangeType:        "regeneration",
		SignificantChange: true, // Regeneration is always considered significant
	}

	// Save the new regenerated version
	err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Save the new version
		versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(newVersion.ID)
		if err := tx.Set(versionRef, newVersion); err != nil {
			return fmt.Errorf("failed to create regenerated version: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save regenerated version: %w", err)
	}

	// Log successful regeneration
	slog.Info("Statement regenerated successfully",
		"user_id", userID,
		"original_version_id", versionID,
		"new_version_id", newVersionID,
		"original_prompt", existingVersion.PromptFile,
		"new_prompt", newPromptFile,
		"generation_method", existingVersion.GeneratedBy,
	)

	return newVersion, nil
}

// RegenerateAllVersions regenerates all versions for a user with updated prompts
// ProgressCallback is called for each version processed during regeneration
type ProgressCallback func(current, total int, versionID string, status string)

func RegenerateAllVersions(ctx context.Context, fs *FirestoreClient, userID string, promptMapping map[string]string, dryRun bool) (*RegenerationReport, error) {
	return RegenerateAllVersionsWithProgress(ctx, fs, userID, promptMapping, dryRun, nil)
}

// RegenerateAllVersionsWithProgress regenerates all versions with optional progress callback
func RegenerateAllVersionsWithProgress(ctx context.Context, fs *FirestoreClient, userID string, promptMapping map[string]string, dryRun bool, progressCallback ProgressCallback) (*RegenerationReport, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}

	report := &RegenerationReport{
		UserID:        userID,
		StartTime:     time.Now(),
		DryRun:        dryRun,
		TotalVersions: 0,
		Processed:     0,
		Successful:    0,
		Failed:        0,
		Errors:        []string{},
		Results:       []RegenerationResult{},
	}

	// Get all versions for the user
	versions, _, err := GetStatementHistory(ctx, fs, userID, 1000, nil) // Get up to 1000 versions
	if err != nil {
		return nil, fmt.Errorf("failed to get statement history: %w", err)
	}

	report.TotalVersions = len(versions)

	for i, version := range versions {
		report.Processed++

		// Call progress callback if provided
		if progressCallback != nil {
			progressCallback(i+1, len(versions), version.ID, "processing")
		}

		// Determine new prompt file based on mapping or use existing
		newPromptFile := version.PromptFile
		if mapping, exists := promptMapping[version.PromptFile]; exists {
			newPromptFile = mapping
		}

		result := RegenerationResult{
			OriginalVersionID: version.ID,
			OriginalPrompt:    version.PromptFile,
			NewPrompt:         newPromptFile,
			GenerationMethod:  version.GeneratedBy,
		}

		if dryRun {
			// In dry-run mode, validate and report what would happen
			result.Status = "would_regenerate"

			// Check for source data
			if version.SourceData == nil {
				result.Status = "no_source_data"
				result.Error = "No source data available for regeneration"
			} else {
				// Check prompt compatibility
				compatibility, err := CheckPromptCompatibility(version.PromptFile, newPromptFile)
				if err != nil {
					result.Status = "failed"
					result.Error = fmt.Sprintf("Compatibility check failed: %v", err)
				} else if !compatibility.IsCompatible {
					result.Status = "failed"
					result.Error = fmt.Sprintf("Prompt incompatibility: %s", compatibility.Reason)
				} else {
					// Check if prompt is suitable for generation method
					if err := ValidatePromptForGeneration(newPromptFile, version.GeneratedBy); err != nil {
						result.Status = "failed"
						result.Error = fmt.Sprintf("Prompt validation failed: %v", err)
					}
				}
			}
		} else {
			// Actually regenerate the version
			newVersion, err := RegenerateStatement(ctx, fs, version.ID, newPromptFile)
			if err != nil {
				result.Status = "failed"
				result.Error = err.Error()
				report.Failed++
				report.Errors = append(report.Errors, fmt.Sprintf("Version %s: %v", version.ID, err))
			} else {
				result.Status = "success"
				result.NewVersionID = newVersion.ID
				result.NewStatementLength = len(newVersion.StatementText)
				report.Successful++
			}
		}

		report.Results = append(report.Results, result)

		// Call progress callback with final status if provided
		if progressCallback != nil {
			progressCallback(i+1, len(versions), version.ID, result.Status)
		}
	}

	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)

	return report, nil
}

// regenerateFromTranscript recreates a statement from transcript source data
func regenerateFromTranscript(ctx context.Context, fs *FirestoreClient, userID string, sourceData map[string]any, promptFile string) (string, error) {
	// Extract transcript data from source
	transcriptText, exists := sourceData["transcript_text"].(string)
	if !exists {
		return "", fmt.Errorf("transcript_text not found in source data")
	}

	// Reconstruct transcript from source data
	transcript := &Transcript{}
	if messagesData, exists := sourceData["messages"]; exists {
		// Convert messages back to transcript format
		if messagesBytes, err := json.Marshal(messagesData); err == nil {
			var messages []minds.Message
			if err := json.Unmarshal(messagesBytes, &messages); err == nil {
				transcript.Messages = messages
			}
		}
	}

	// If we couldn't reconstruct the full transcript, create a simple one
	if len(transcript.Messages) == 0 {
		transcript.Messages = []minds.Message{
			{Role: minds.RoleUser, Content: transcriptText},
		}
	}

	// Generate new statement using the original function
	return generateStatementFromTranscript(ctx, fs, transcript, promptFile, userID)
}

// regenerateFromFeedback recreates a statement from feedback source data
func regenerateFromFeedback(ctx context.Context, fs *FirestoreClient, userID string, sourceData map[string]any, promptFile string) (string, error) {
	// Get current statement from original source data or reconstruct
	var currentStatement string
	if transcriptText, exists := sourceData["transcript_text"].(string); exists {
		// Use the original transcript as base if available
		currentStatement = transcriptText
	} else {
		return "", fmt.Errorf("no base statement available for feedback regeneration")
	}

	// Extract feedback data
	feedbackData, exists := sourceData["feedback_data"]
	if !exists {
		return "", fmt.Errorf("feedback_data not found in source data")
	}

	// Convert feedback data to proper format
	feedbackMap, ok := feedbackData.(map[string]any)
	if !ok {
		return "", fmt.Errorf("feedback_data is not in expected format")
	}

	// Generate enhanced statement using the original function
	return generateEnhancedStatementFromFeedback(ctx, fs, currentStatement, feedbackMap, promptFile)
}

// regenerateFromFollowup recreates a statement from followup answers source data
func regenerateFromFollowup(ctx context.Context, fs *FirestoreClient, userID string, sourceData map[string]any, promptFile string) (string, error) {
	// Get the base statement
	var currentStatement string
	if transcriptText, exists := sourceData["transcript_text"].(string); exists {
		currentStatement = transcriptText
	} else {
		return "", fmt.Errorf("no base statement available for followup regeneration")
	}

	// Extract question answers
	answersData, exists := sourceData["question_answers"]
	if !exists {
		return "", fmt.Errorf("question_answers not found in source data")
	}

	// Convert to proper format
	var questionAnswers []map[string]any
	if answersBytes, err := json.Marshal(answersData); err == nil {
		json.Unmarshal(answersBytes, &questionAnswers)
	}

	if len(questionAnswers) == 0 {
		return "", fmt.Errorf("no valid question answers found in source data")
	}

	// Load prompt template
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/" + promptFile)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file: %w", err)
	}

	// Prepare data for prompt generation
	promptData := map[string]any{
		"CURRENT_STATEMENT": currentStatement,
		"QUESTION_ANSWERS":  questionAnswers,
		"QUESTIONS_COUNT":   len(questionAnswers),
	}

	// Generate prompt
	prompt, err := tools.GeneratePrompt(tmpl, promptData)
	if err != nil {
		return "", fmt.Errorf("failed to generate prompt: %w", err)
	}

	// Ask LLM to generate enhanced statement
	model := getModelForContext(ctx)
	enhancedStatement, err := tools.Ask(ctx, prompt, tools.WithModel(model), tools.WithMaxTokens(8192))
	if err != nil {
		return "", fmt.Errorf("failed to ask LLM: %w", err)
	}

	return strings.TrimSpace(enhancedStatement), nil
}

// RegenerationReport contains the results of a bulk regeneration operation
type RegenerationReport struct {
	UserID        string
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
	DryRun        bool
	TotalVersions int
	Processed     int
	Successful    int
	Failed        int
	Errors        []string
	Results       []RegenerationResult
}

// RegenerationResult contains the result of regenerating a single version
type RegenerationResult struct {
	OriginalVersionID  string
	NewVersionID       string
	OriginalPrompt     string
	NewPrompt          string
	GenerationMethod   string
	Status             string // "success", "failed", "would_regenerate", "no_source_data"
	Error              string
	NewStatementLength int
}

// PromptVersion represents a parsed prompt version
type PromptVersion struct {
	Name     string // Base name without version (e.g., "transcript.user-statement")
	Version  string // Version string (e.g., "v001")
	FullName string // Full filename (e.g., "transcript.user-statement.v001.txt")
}

// PromptCompatibility represents compatibility info between prompt versions
type PromptCompatibility struct {
	IsCompatible      bool
	Reason            string
	RecommendedPrompt string
}

// ParsePromptVersion parses a prompt filename to extract version information
func ParsePromptVersion(promptFile string) (*PromptVersion, error) {
	// Remove .txt extension if present
	name := strings.TrimSuffix(promptFile, ".txt")

	// Pattern to match versioned prompts: name.vXXX or name.XXX
	versionPattern := regexp.MustCompile(`^(.+)\.v?(\d{3})$`)
	matches := versionPattern.FindStringSubmatch(name)

	if len(matches) != 3 {
		return &PromptVersion{
			Name:     name,
			Version:  "unknown",
			FullName: promptFile,
		}, nil
	}

	baseName := matches[1]
	version := matches[2]

	// Normalize version to vXXX format
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	return &PromptVersion{
		Name:     baseName,
		Version:  version,
		FullName: promptFile,
	}, nil
}

// CheckPromptCompatibility checks if two prompt versions are compatible
func CheckPromptCompatibility(originalPrompt, newPrompt string) (*PromptCompatibility, error) {
	originalVersion, err := ParsePromptVersion(originalPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse original prompt version: %w", err)
	}

	newVersion, err := ParsePromptVersion(newPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse new prompt version: %w", err)
	}

	// Check if they're the same base prompt type
	if originalVersion.Name != newVersion.Name {
		return &PromptCompatibility{
			IsCompatible: false,
			Reason:       fmt.Sprintf("Different prompt types: %s vs %s", originalVersion.Name, newVersion.Name),
		}, nil
	}

	// Check if new version is newer or same
	if originalVersion.Version == newVersion.Version {
		return &PromptCompatibility{
			IsCompatible: true,
			Reason:       "Same version",
		}, nil
	}

	// For now, allow any version change within the same prompt type
	// In the future, we could add more sophisticated version compatibility rules
	return &PromptCompatibility{
		IsCompatible: true,
		Reason:       fmt.Sprintf("Version change from %s to %s", originalVersion.Version, newVersion.Version),
	}, nil
}

// GetRecommendedPromptMapping returns a mapping of old prompts to recommended new versions
func GetRecommendedPromptMapping() map[string]string {
	return map[string]string{
		"transcript.user-statement.v001.txt": "transcript.user-statement.v001.txt", // Latest
		"statement-update.v001.txt":          "statement-update.v001.txt",          // Latest
		"feedback-analysis.v001.txt":         "feedback-analysis.v001.txt",         // Latest
	}
}

// ValidatePromptForGeneration validates that a prompt file is suitable for a specific generation method
func ValidatePromptForGeneration(promptFile, generationMethod string) error {
	promptVersion, err := ParsePromptVersion(promptFile)
	if err != nil {
		return fmt.Errorf("invalid prompt file format: %w", err)
	}

	// Check if prompt type matches generation method
	switch generationMethod {
	case "transcript":
		if !strings.Contains(promptVersion.Name, "transcript") && !strings.Contains(promptVersion.Name, "user-statement") {
			return fmt.Errorf("prompt %s is not suitable for transcript generation", promptFile)
		}
	case "feedback":
		if !strings.Contains(promptVersion.Name, "feedback") && !strings.Contains(promptVersion.Name, "statement-update") {
			return fmt.Errorf("prompt %s is not suitable for feedback generation", promptFile)
		}
	case "followup":
		if !strings.Contains(promptVersion.Name, "followup") && !strings.Contains(promptVersion.Name, "statement-update") {
			return fmt.Errorf("prompt %s is not suitable for followup generation", promptFile)
		}
	}

	return nil
}

// BuildDependencyChain builds the forward dependency chain from a starting version
func BuildDependencyChain(ctx context.Context, fs *FirestoreClient, userID string, fromVersionID string) ([]*UserStatementVersion, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	if fromVersionID == "" {
		return nil, fmt.Errorf("fromVersionID is required")
	}

	// Get all versions for user ordered by version number
	allVersions, _, err := GetStatementHistory(ctx, fs, userID, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get statement history: %w", err)
	}

	// Find the starting version and its position
	var startIndex int = -1
	var targetVersion *UserStatementVersion
	for i, version := range allVersions {
		if version.ID == fromVersionID {
			startIndex = len(allVersions) - 1 - i // Convert from desc order to asc index
			targetVersion = &version
			break
		}
	}

	if startIndex == -1 || targetVersion == nil {
		return nil, fmt.Errorf("version %s not found", fromVersionID)
	}

	// Build dependency chain from the target version forward
	var dependencyChain []*UserStatementVersion

	// Reverse the slice to get ascending order (oldest to newest)
	reversedVersions := make([]UserStatementVersion, len(allVersions))
	for i, version := range allVersions {
		reversedVersions[len(allVersions)-1-i] = version
	}

	// Add all versions from the start index forward
	for i := startIndex; i < len(reversedVersions); i++ {
		version := reversedVersions[i]
		dependencyChain = append(dependencyChain, &version)
	}

	return dependencyChain, nil
}

// RegenerateCascade regenerates a version and all subsequent dependent versions
func RegenerateCascade(ctx context.Context, fs *FirestoreClient, registry *StatementGeneratorRegistry, versionID string, _ string, dryRun bool) (*CascadeReport, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}
	if registry == nil {
		return nil, fmt.Errorf("statement regenerator registry is nil")
	}
	if versionID == "" {
		return nil, fmt.Errorf("versionID is required")
	}

	// Extract userID from versionID
	parts := strings.Split(versionID, "_v")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid version ID format: %s", versionID)
	}
	userID := parts[0]

	// Create cascade operation record
	operationID := fmt.Sprintf("%s_cascade_%d", userID, time.Now().Unix())
	operation := &CascadeOperation{
		ID:               operationID,
		UserID:           userID,
		InitiatedAt:      time.Now(),
		TriggerVersionID: versionID,
		TriggerPrompt:    "system-defined", // Each system uses its own current prompt
		DryRun:           dryRun,
		Status:           "in_progress",
		VersionsAffected: []string{},
	}

	// Initialize report
	report := &CascadeReport{
		Operation:          operation,
		VersionsProcessed:  0,
		VersionsSuccessful: 0,
		VersionsFailed:     0,
		Results:            []CascadeVersionResult{},
	}

	startTime := time.Now()

	// Build dependency chain forward from target version
	dependencyChain, err := BuildDependencyChain(ctx, fs, userID, versionID)
	if err != nil {
		operation.Status = "failed"
		operation.Error = &[]string{fmt.Sprintf("failed to build dependency chain: %v", err)}[0]
		report.Duration = time.Since(startTime)
		return report, fmt.Errorf("failed to build dependency chain: %w", err)
	}

	if len(dependencyChain) == 0 {
		operation.Status = "completed"
		completedAt := time.Now()
		operation.CompletedAt = &completedAt
		report.Duration = time.Since(startTime)
		return report, nil
	}

	// Process each version in the dependency chain
	for _, version := range dependencyChain {
		report.VersionsProcessed++

		// Check if a regenerator exists for this system type
		regenerator := registry.GetRegenerator(version.GeneratedBy)
		if regenerator == nil {
			// For unknown or non-existent systems, skip regeneration
			result := CascadeVersionResult{
				VersionID:  version.ID,
				Status:     "skipped",
				Error:      &[]string{fmt.Sprintf("no regenerator registered for system: %s", version.GeneratedBy)}[0],
				SystemType: version.GeneratedBy,
			}
			report.Results = append(report.Results, result)
			continue
		}

		result := CascadeVersionResult{
			VersionID:  version.ID,
			PromptUsed: version.PromptFile, // Use the original prompt file from the version
			SystemType: version.GeneratedBy,
		}

		if dryRun {
			// In dry run mode, just validate and report what would happen
			result.Status = "would_regenerate"
			result.StatementLength = len(version.StatementText)

			// Validate that we have source data for regeneration
			if version.SourceData == nil {
				result.Status = "failed"
				result.Error = &[]string{"no source data available for regeneration"}[0]
				report.VersionsFailed++
			} else {
				// In dry run, assume regeneration would work if we have a regenerator and source data
				result.Status = "would_regenerate"
				report.VersionsSuccessful++
			}
		} else {
			// Actually regenerate the version using the system-specific regenerator
			newStatementText, err := regenerator.GenerateStatement(ctx, version)
			if err != nil {
				result.Status = "failed"
				result.Error = &[]string{err.Error()}[0]
				report.VersionsFailed++
			} else {
				// Create a new version with the regenerated statement
				newVersion, err := createReplacementVersion(ctx, fs, version, newStatementText)
				if err != nil {
					result.Status = "failed"
					result.Error = &[]string{fmt.Sprintf("failed to create replacement version: %v", err)}[0]
					report.VersionsFailed++
				} else {
					result.Status = "success"
					result.NewVersionID = &newVersion.ID
					result.StatementLength = len(newVersion.StatementText)
					operation.VersionsAffected = append(operation.VersionsAffected, newVersion.ID)
					report.VersionsSuccessful++
				}
			}
		}

		report.Results = append(report.Results, result)
	}

	// Complete the operation
	operation.Status = "completed"
	completedAt := time.Now()
	operation.CompletedAt = &completedAt
	report.Duration = time.Since(startTime)

	// Update User.Statement field with final version if not dry run
	if !dryRun && len(dependencyChain) > 0 {
		if err := SyncUserStatementField(ctx, fs, userID); err != nil {
			slog.Warn("Failed to sync User.Statement field after cascade", "userID", userID, "error", err)
		}
	}

	return report, nil
}

// createReplacementVersion creates a new version that replaces the original version
// with the regenerated statement text, maintaining all other metadata.
func createReplacementVersion(ctx context.Context, fs *FirestoreClient, originalVersion *UserStatementVersion, newStatementText string) (*UserStatementVersion, error) {
	// Extract userID from version ID
	parts := strings.Split(originalVersion.ID, "_v")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid version ID format: %s", originalVersion.ID)
	}
	userID := parts[0]

	// Create a new version with the same metadata but new statement
	newVersion := &UserStatementVersion{
		ID:            originalVersion.ID, // Keep the same ID to replace in-place
		VersionNumber: originalVersion.VersionNumber,
		IsLatest:      originalVersion.IsLatest,

		// Updated statement content
		StatementText: newStatementText,
		StatementHash: generateStatementHash(newStatementText),

		// Preserve original generation metadata
		GeneratedBy:      originalVersion.GeneratedBy,
		GenerationReason: "cascade_regeneration",
		PromptFile:       originalVersion.PromptFile,
		PromptVersion:    originalVersion.PromptVersion,
		LLMModel:         originalVersion.LLMModel,
		LLMTokensUsed:    nil, // Reset token count since we regenerated

		// Preserve source data and transformation steps
		SourceData:          originalVersion.SourceData,
		TransformationSteps: originalVersion.TransformationSteps,

		// Update timestamps
		CreatedAt:         time.Now(), // Update creation time for regeneration
		CreatedBy:         originalVersion.CreatedBy,
		PreviousVersionID: originalVersion.PreviousVersionID,

		// Update change metadata
		ChangeType:        "cascade_regeneration",
		ChangeSummary:     &[]string{"Regenerated during cascade operation"}[0],
		SignificantChange: originalVersion.StatementText != newStatementText,

		// Preserve cascade fields
		DependsOnVersionID:  originalVersion.DependsOnVersionID,
		DependentVersionIDs: originalVersion.DependentVersionIDs,
		SystemType:          originalVersion.SystemType,
		IsReplacedVersion:   true, // Mark as replaced during cascade
	}

	// Save the updated version using transaction
	err := fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Update the version in subcollection
		versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(newVersion.ID)
		if err := tx.Set(versionRef, newVersion); err != nil {
			return fmt.Errorf("failed to update statement version: %w", err)
		}

		// Update User fields if this is the latest version
		if newVersion.IsLatest {
			userRef := fs.Collection(usersCollection).Doc(userID)
			updates := []firestore.Update{
				{Path: "Statement", Value: newVersion.StatementText},
				{Path: "CurrentStatementVersionID", Value: newVersion.ID},
				{Path: "StatementLastUpdated", Value: newVersion.CreatedAt},
			}
			if err := tx.Update(userRef, updates); err != nil {
				return fmt.Errorf("failed to update user statement fields: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save replacement version: %w", err)
	}

	return newVersion, nil
}

// regenerateAndReplaceVersion regenerates a version and replaces it in-place
func regenerateAndReplaceVersion(ctx context.Context, fs *FirestoreClient, originalVersion *UserStatementVersion, newPromptFile string) (*UserStatementVersion, error) {
	// Extract userID from version ID
	parts := strings.Split(originalVersion.ID, "_v")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid version ID format: %s", originalVersion.ID)
	}
	userID := parts[0]

	// Validate that we have source data to regenerate from
	if originalVersion.SourceData == nil {
		return nil, fmt.Errorf("version %s has no source data for regeneration", originalVersion.ID)
	}

	// Check prompt compatibility
	compatibility, err := CheckPromptCompatibility(originalVersion.PromptFile, newPromptFile)
	if err != nil {
		return nil, fmt.Errorf("failed to check prompt compatibility: %w", err)
	}
	if !compatibility.IsCompatible {
		return nil, fmt.Errorf("prompt incompatibility: %s", compatibility.Reason)
	}

	// Validate prompt is suitable for the generation method
	if err := ValidatePromptForGeneration(newPromptFile, originalVersion.GeneratedBy); err != nil {
		return nil, fmt.Errorf("prompt validation failed: %w", err)
	}

	// Extract prompt version from filename
	newPromptVersion := ExtractPromptVersion(newPromptFile)

	// Determine regeneration type based on original generation method
	var newStatement string
	switch originalVersion.GeneratedBy {
	case "transcript":
		newStatement, err = regenerateFromTranscript(ctx, fs, userID, originalVersion.SourceData, newPromptFile)
	case "feedback":
		newStatement, err = regenerateFromFeedback(ctx, fs, userID, originalVersion.SourceData, newPromptFile)
	case "followup":
		newStatement, err = regenerateFromFollowup(ctx, fs, userID, originalVersion.SourceData, newPromptFile)
	default:
		return nil, fmt.Errorf("unknown generation method: %s", originalVersion.GeneratedBy)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to regenerate statement: %w", err)
	}

	// Create updated version (replace the original)
	updatedVersion := *originalVersion // Copy original
	updatedVersion.StatementText = newStatement
	updatedVersion.StatementHash = generateStatementHash(newStatement)
	updatedVersion.PromptFile = newPromptFile
	updatedVersion.PromptVersion = newPromptVersion
	updatedVersion.LLMModel = getModelForContext(ctx)
	updatedVersion.GenerationReason = fmt.Sprintf("Cascade regenerated with prompt %s", newPromptFile)

	// Add transformation step for cascade regeneration
	updatedVersion.TransformationSteps = append(updatedVersion.TransformationSteps, TransformationStep{
		StepNumber:         len(updatedVersion.TransformationSteps) + 1,
		TransformationType: "cascade_regeneration",
		InputData: map[string]any{
			"original_prompt":   originalVersion.PromptFile,
			"new_prompt":        newPromptFile,
			"cascade_operation": true,
		},
		OutputData: map[string]any{
			"statement_length": len(newStatement),
			"statement_hash":   generateStatementHash(newStatement),
		},
		PromptUsed:  newPromptFile,
		ProcessedAt: time.Now(),
	})

	// Update system type if not already set
	if updatedVersion.SystemType == "" {
		updatedVersion.SystemType = originalVersion.GeneratedBy
	}

	// Save the updated version (replace original)
	err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(originalVersion.ID)
		if err := tx.Set(versionRef, &updatedVersion); err != nil {
			return fmt.Errorf("failed to update version during cascade: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save updated version: %w", err)
	}

	// Log successful regeneration
	slog.Info("Version regenerated in cascade",
		"user_id", userID,
		"version_id", originalVersion.ID,
		"original_prompt", originalVersion.PromptFile,
		"new_prompt", newPromptFile,
		"generation_method", originalVersion.GeneratedBy,
	)

	return &updatedVersion, nil
}

// EnhanceStatementWithInboxFeedback creates new version incorporating email-based inbox feedback
func EnhanceStatementWithInboxFeedback(ctx context.Context, fs *FirestoreClient, userID string, emailContent, adjustmentRequest, userEmail string, promptFile string) (*UserStatementVersion, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client is nil")
	}
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	if emailContent == "" {
		return nil, fmt.Errorf("emailContent is required")
	}
	if adjustmentRequest == "" {
		return nil, fmt.Errorf("adjustmentRequest is required")
	}
	if promptFile == "" {
		return nil, fmt.Errorf("promptFile is required")
	}

	// Get latest statement version
	previousVersion, err := GetLatestStatement(ctx, fs, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest statement: %w", err)
	}
	if previousVersion == nil {
		return nil, fmt.Errorf("no existing statement found for user %s", userID)
	}

	// Generate enhanced statement using inbox feedback
	enhancedText, err := generateEnhancedStatementFromInboxFeedback(ctx, fs, previousVersion.StatementText, emailContent, adjustmentRequest, userEmail, promptFile)
	if err != nil {
		return nil, fmt.Errorf("failed to generate enhanced statement from inbox feedback: %w", err)
	}

	// Extract prompt version from filename
	promptVersion := ExtractPromptVersion(promptFile)

	// Create source data preserving previous source and adding inbox feedback
	sourceData := make(map[string]any)
	// Copy previous source data
	for k, v := range previousVersion.SourceData {
		sourceData[k] = v
	}
	// Add inbox feedback data
	sourceData["inbox_feedback_data"] = map[string]any{
		"source_type":        "email_feedback",
		"email_content":      emailContent,
		"adjustment_request": adjustmentRequest,
		"user_email":         userEmail,
		"processed_at":       time.Now().Format(time.RFC3339),
	}
	sourceData["enhancement_reason"] = "inbox_feedback_integration"

	// Create transformation steps
	transformationSteps := append([]TransformationStep{}, previousVersion.TransformationSteps...)
	transformationSteps = append(transformationSteps, TransformationStep{
		StepNumber:         len(transformationSteps) + 1,
		TransformationType: "inbox_feedback_integration",
		InputData: map[string]any{
			"previous_statement": previousVersion.StatementText,
			"email_content":      emailContent,
			"adjustment_request": adjustmentRequest,
		},
		OutputData: map[string]any{
			"enhanced_statement": enhancedText,
		},
		PromptUsed:  promptFile,
		ProcessedAt: time.Now(),
	})

	// Create new version
	version := &UserStatementVersion{
		ID:                  fmt.Sprintf("%s_v%d", userID, previousVersion.VersionNumber+1),
		VersionNumber:       previousVersion.VersionNumber + 1,
		IsLatest:            true,
		StatementText:       enhancedText,
		GeneratedBy:         "feedback", // Using same GeneratedBy as regular feedback for consistency
		GenerationReason:    fmt.Sprintf("Enhanced statement based on inbox email feedback: %s", adjustmentRequest),
		PromptFile:          promptFile,
		PromptVersion:       promptVersion,
		SourceData:          sourceData,
		TransformationSteps: transformationSteps,
		CreatedAt:           time.Now(),
		PreviousVersionID:   &previousVersion.ID,
		ChangeSummary:       nil,  // TODO: Generate change summary
		SignificantChange:   true, // TODO: Analyze if change is significant
	}

	// Use transaction to ensure atomicity
	err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Mark previous version as not latest
		previousVersionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(previousVersion.ID)
		if err := tx.Update(previousVersionRef, []firestore.Update{
			{Path: "IsLatest", Value: false},
		}); err != nil {
			return fmt.Errorf("failed to update previous version: %w", err)
		}

		// Store the new version in subcollection
		versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(version.ID)
		if err := tx.Set(versionRef, version); err != nil {
			return fmt.Errorf("failed to create statement version: %w", err)
		}

		// Update User fields for backward compatibility
		userRef := fs.Collection(usersCollection).Doc(userID)
		updates := []firestore.Update{
			{Path: "Statement", Value: enhancedText},
			{Path: "CurrentStatementVersionID", Value: version.ID},
			{Path: "StatementLastUpdated", Value: version.CreatedAt},
		}
		if err := tx.Update(userRef, updates); err != nil {
			return fmt.Errorf("failed to update user with statement version: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create inbox feedback statement version: %w", err)
	}

	// Log success
	slog.Info("Inbox feedback statement version created",
		"user_id", userID,
		"version_id", version.ID,
		"version_number", version.VersionNumber,
		"adjustment_request", adjustmentRequest,
		"text_length", len(enhancedText),
	)

	// Invalidate user cache
	fs.EvictUserCache(userID)

	return version, nil
}

// generateEnhancedStatementFromInboxFeedback generates an enhanced statement using inbox feedback
func generateEnhancedStatementFromInboxFeedback(ctx context.Context, fs *FirestoreClient, currentStatement, emailContent, adjustmentRequest, userEmail, promptFile string) (string, error) {
	// Load prompt template from embedded assets
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/" + promptFile)
	if err != nil {
		return "", fmt.Errorf("failed to read inbox feedback prompt: %w", err)
	}

	// Prepare inbox feedback data for prompt generation
	promptData := prepareInboxDataForPrompt(emailContent, adjustmentRequest, userEmail)
	promptData["CURRENT_STATEMENT"] = currentStatement

	// Generate prompt using tools.GeneratePrompt
	prompt, err := tools.GeneratePrompt(tmpl, promptData)
	if err != nil {
		SentryReportError(ctx, err, sentry.LevelError, nil)
		return "", fmt.Errorf("failed to generate inbox feedback prompt: %w", err)
	}

	// Ask LLM to generate enhanced statement
	resp := openai.ChatCompletionResponse{}
	model := getModelForContext(ctx)
	enhancedStatement, err := tools.Ask(ctx, prompt, tools.WithModel(model), tools.WithMaxTokens(8192), tools.WantCompletionResponse(&resp))
	if err != nil {
		SentryReportError(ctx, err, sentry.LevelError, nil)
		return "", fmt.Errorf("failed to ask LLM for inbox feedback enhancement: %w", err)
	}

	// Log successful generation
	slog.Debug("Generated enhanced statement from inbox feedback",
		"originalLength", len(currentStatement),
		"enhancedLength", len(enhancedStatement),
		"adjustmentRequest", adjustmentRequest)

	return enhancedStatement, nil
}

// prepareInboxDataForPrompt converts inbox feedback data to template-friendly format
func prepareInboxDataForPrompt(emailContent, adjustmentRequest, userEmail string) map[string]any {
	return map[string]any{
		"EMAIL_CONTENT":      emailContent,
		"ADJUSTMENT_REQUEST": adjustmentRequest,
		"USER_EMAIL":         userEmail,
		"PROCESSED_AT":       time.Now().Format(time.RFC3339),
	}
}
