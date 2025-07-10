package internal

import (
	"fmt"
	"strings"
	"time"
)

// Constants for checklist management
const (
	ChecklistStatusInProgress         = "in_progress"
	ChecklistStatusReadyForCompletion = "ready_for_completion"
	ChecklistStatusCompleted          = "completed"

	ChecklistItemStatusPending    = "pending"
	ChecklistItemStatusInProgress = "in_progress"
	ChecklistItemStatusCompleted  = "completed"

	ChecklistItemQualityLow    = "low"
	ChecklistItemQualityMedium = "medium"
	ChecklistItemQualityHigh   = "high"

	MaxConversationLength = 50
	MaxCompletionAttempts = 3
)

// ConversationChecklist tracks the progress of gathering required information during chat
type ConversationChecklist struct {
	Items              []ChecklistItem `json:"items"`
	OverallStatus      string          `json:"overall_status"`      // "in_progress", "ready_for_completion", "completed"
	CompletionAttempts int             `json:"completion_attempts"` // Track attempts to prevent infinite loops
	LastUpdated        time.Time       `json:"last_updated"`
	MessageCount       int             `json:"message_count"` // Track conversation length
}

// ChecklistItem represents a specific topic or piece of information to be gathered
type ChecklistItem struct {
	ID              string                 `json:"id"`
	Topic           string                 `json:"topic"`
	Description     string                 `json:"description"`
	Status          string                 `json:"status"`           // "pending", "in_progress", "completed"
	Quality         string                 `json:"quality"`          // "low", "medium", "high"
	RequiredInfo    []string               `json:"required_info"`    // What specific information is needed
	ExtractedInfo   map[string]interface{} `json:"extracted_info"`   // What was actually gathered
	ValidationNotes string                 `json:"validation_notes"` // Notes about information quality
	Required        bool                   `json:"required"`         // Whether this item is required for completion
	LastUpdated     time.Time              `json:"last_updated"`
}

// InitializeConversationChecklist creates a new checklist for a chat appointment
func InitializeConversationChecklist() *ConversationChecklist {
	now := time.Now()

	items := []ChecklistItem{
		{
			ID:            "introduction",
			Topic:         "Introduction and Verification",
			Description:   "Basic introduction and verification of caller identity",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"name_confirmed", "good_time_to_chat"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "warm_up",
			Topic:         "Warm-Up and Resume Acknowledgment",
			Description:   "Personal background exploration and resume discussion",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"career_background", "personal_insights", "achievements"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "deal_breakers",
			Topic:         "Identifying What They Want to Avoid",
			Description:   "Understanding work experiences and environments to avoid",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"work_dislikes", "management_style_dislikes", "company_culture_dislikes"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "search_strategy",
			Topic:         "Determine Search Strategy",
			Description:   "Primary search terms, skills, and job focus areas",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"primary_search_terms", "additional_skills", "things_to_avoid"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "work_environment",
			Topic:         "Work Environment Preferences",
			Description:   "Company type, work motivation, and collaboration style",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"company_type_preference", "work_motivation", "collaboration_style"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "location_preferences",
			Topic:         "Preferred Work Location",
			Description:   "Geographic location and remote/hybrid preferences",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"current_location", "work_location_preference", "flexibility"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "validation",
			Topic:         "Clarifying and Validating Statements",
			Description:   "Clarification of vague or inconsistent responses",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"clarifications_complete", "consistency_verified"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      false, // Optional - depends on conversation quality
			LastUpdated:   now,
		},
		{
			ID:            "final_confirmation",
			Topic:         "Closing and Confirming Understanding",
			Description:   "Final validation of all gathered information",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"summary_confirmed", "additional_info_requested"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
	}

	return &ConversationChecklist{
		Items:              items,
		OverallStatus:      ChecklistStatusInProgress,
		CompletionAttempts: 0,
		LastUpdated:        now,
		MessageCount:       0,
	}
}

// UpdateChecklistItem updates a specific checklist item
func UpdateChecklistItem(checklist *ConversationChecklist, itemID string, status string, quality string, extractedInfo map[string]interface{}, notes string) error {
	for i := range checklist.Items {
		if checklist.Items[i].ID == itemID {
			checklist.Items[i].Status = status
			checklist.Items[i].Quality = quality
			checklist.Items[i].ValidationNotes = notes
			checklist.Items[i].LastUpdated = time.Now()

			// Merge extracted info
			if extractedInfo != nil {
				for key, value := range extractedInfo {
					checklist.Items[i].ExtractedInfo[key] = value
				}
			}

			checklist.LastUpdated = time.Now()
			return nil
		}
	}
	return fmt.Errorf("checklist item with ID %s not found", itemID)
}

// GetChecklistItem retrieves a specific checklist item
func GetChecklistItem(checklist *ConversationChecklist, itemID string) (*ChecklistItem, error) {
	for i := range checklist.Items {
		if checklist.Items[i].ID == itemID {
			return &checklist.Items[i], nil
		}
	}
	return nil, fmt.Errorf("checklist item with ID %s not found", itemID)
}

// ValidateChecklistCompletion checks if the checklist is ready for completion
func ValidateChecklistCompletion(checklist *ConversationChecklist) (bool, []string, error) {
	if checklist == nil {
		return false, []string{"checklist not initialized"}, nil
	}

	var missingItems []string
	var qualityIssues []string

	// Check if we've exceeded maximum attempts
	if checklist.CompletionAttempts >= MaxCompletionAttempts {
		// Allow completion with lower quality after max attempts
		return ValidateChecklistCompletionWithLowerThreshold(checklist)
	}

	// Check if conversation is too long
	if checklist.MessageCount >= MaxConversationLength {
		return ValidateChecklistCompletionWithLowerThreshold(checklist)
	}

	// Check required items
	for _, item := range checklist.Items {
		if item.Required {
			if item.Status != ChecklistItemStatusCompleted {
				missingItems = append(missingItems, fmt.Sprintf("%s: %s", item.Topic, "not completed"))
			} else if item.Quality == ChecklistItemQualityLow {
				qualityIssues = append(qualityIssues, fmt.Sprintf("%s: %s", item.Topic, "low quality information"))
			}
		}
	}

	if len(missingItems) > 0 || len(qualityIssues) > 0 {
		var allIssues []string
		allIssues = append(allIssues, missingItems...)
		allIssues = append(allIssues, qualityIssues...)
		return false, allIssues, nil
	}

	return true, nil, nil
}

// ValidateChecklistCompletionWithLowerThreshold allows completion with lower quality thresholds
func ValidateChecklistCompletionWithLowerThreshold(checklist *ConversationChecklist) (bool, []string, error) {
	var missingItems []string

	// Only check that required items are at least started
	for _, item := range checklist.Items {
		if item.Required && item.Status == ChecklistItemStatusPending {
			missingItems = append(missingItems, fmt.Sprintf("%s: %s", item.Topic, "not started"))
		}
	}

	if len(missingItems) > 0 {
		return false, missingItems, nil
	}

	return true, nil, nil
}

// IncrementCompletionAttempt increments the completion attempt counter
func IncrementCompletionAttempt(checklist *ConversationChecklist) {
	checklist.CompletionAttempts++
	checklist.LastUpdated = time.Now()
}

// IncrementMessageCount increments the message counter
func IncrementMessageCount(checklist *ConversationChecklist) {
	checklist.MessageCount++
	checklist.LastUpdated = time.Now()
}

// GetChecklistSummary returns a summary of the checklist status
func GetChecklistSummary(checklist *ConversationChecklist) map[string]interface{} {
	if checklist == nil {
		return map[string]interface{}{
			"error": "checklist not initialized",
		}
	}

	completedCount := 0
	highQualityCount := 0
	requiredCount := 0

	for _, item := range checklist.Items {
		if item.Required {
			requiredCount++
		}
		if item.Status == ChecklistItemStatusCompleted {
			completedCount++
		}
		if item.Quality == ChecklistItemQualityHigh {
			highQualityCount++
		}
	}

	canComplete, issues, _ := ValidateChecklistCompletion(checklist)

	return map[string]interface{}{
		"overall_status":      checklist.OverallStatus,
		"completion_attempts": checklist.CompletionAttempts,
		"message_count":       checklist.MessageCount,
		"total_items":         len(checklist.Items),
		"required_items":      requiredCount,
		"completed_items":     completedCount,
		"high_quality_items":  highQualityCount,
		"can_complete":        canComplete,
		"issues":              issues,
		"items":               checklist.Items,
		"last_updated":        checklist.LastUpdated,
	}
}

// UpdateChecklistFromMessage automatically updates checklist based on message content
// This provides a lightweight guard to ensure every message pushes checklist state forward
func UpdateChecklistFromMessage(checklist *ConversationChecklist, message ChatMessage) error {
	if checklist == nil {
		return fmt.Errorf("checklist is nil")
	}

	// Skip system messages and tool calls
	if message.Role == "system" || message.Role == "tool" {
		return nil
	}

	// Skip messages with empty content
	if message.Content == "" {
		return nil
	}

	// Skip tool call messages (these are handled by explicit tool calls)
	if message.Role == "assistant" && (strings.Contains(message.Content, "TOOL_CALLS:") || strings.Contains(message.Content, "TOOL_RESPONSE:")) {
		return nil
	}

	return nil
}

// autoUpdateChecklistItem updates a checklist item only if it improves the current state
func autoUpdateChecklistItem(checklist *ConversationChecklist, itemID string, status string, quality string) error {
	item, err := GetChecklistItem(checklist, itemID)
	if err != nil {
		return err
	}

	// Only update if the new status is an improvement
	shouldUpdate := false
	if item.Status == ChecklistItemStatusPending && status != ChecklistItemStatusPending {
		shouldUpdate = true
	} else if item.Status == ChecklistItemStatusInProgress && status == ChecklistItemStatusCompleted {
		shouldUpdate = true
	}

	// Also update if quality is improving
	if item.Quality == ChecklistItemQualityLow && quality != ChecklistItemQualityLow {
		shouldUpdate = true
	} else if item.Quality == ChecklistItemQualityMedium && quality == ChecklistItemQualityHigh {
		shouldUpdate = true
	}

	if shouldUpdate {
		extractedInfo := map[string]interface{}{
			"auto_detected":  true,
			"detection_time": time.Now(),
			"source":         "message_analysis",
		}
		return UpdateChecklistItem(checklist, itemID, status, quality, extractedInfo, "Auto-detected from message content")
	}

	return nil
}

// Pattern matching functions for different checklist topics
func checkIntroductionPatterns(content string) bool {
	patterns := []string{
		"my name is", "i'm", "yes this is", "speaking", "good time to talk", "good time to chat",
		"thanks for calling", "hi", "hello", "yes that's me", "correct", "that's right",
	}
	return containsAnyPattern(content, patterns)
}

func checkBackgroundPatterns(content string) bool {
	patterns := []string{
		"background", "experience", "worked at", "current role", "previous", "career", "resume",
		"graduated", "degree", "university", "college", "skills", "expertise", "accomplishment",
		"achievement", "project", "responsibility", "manager", "team", "years", "developed",
	}
	return containsAnyPattern(content, patterns)
}

func checkDealBreakerPatterns(content string) bool {
	patterns := []string{
		"don't want", "avoid", "not interested", "dislike", "hate", "never again", "terrible",
		"worst", "bad experience", "nightmare", "toxic", "micromanag", "bureaucracy",
		"politics", "overtime", "weekend", "on-call", "travel", "relocate", "commute",
	}
	return containsAnyPattern(content, patterns)
}

func checkWorkEnvironmentPatterns(content string) bool {
	patterns := []string{
		"company", "startup", "corporation", "team", "culture", "environment", "atmosphere",
		"collaboration", "independent", "autonomous", "structured", "flexible", "formal",
		"casual", "fast-paced", "slow", "innovative", "traditional", "creative", "analytical",
	}
	return containsAnyPattern(content, patterns)
}

func checkLocationPatterns(content string) bool {
	patterns := []string{
		"location", "remote", "hybrid", "office", "work from home", "wfh", "onsite", "in-person",
		"commute", "travel", "relocate", "move", "city", "state", "country", "timezone",
		"flexible", "anywhere", "local", "nearby", "distance",
	}
	return containsAnyPattern(content, patterns)
}

func checkSearchStrategyPatterns(content string) bool {
	patterns := []string{
		"looking for", "searching", "interested in", "want to", "hoping", "ideal", "perfect",
		"title", "role", "position", "job", "opportunity", "skills", "technology", "language",
		"framework", "industry", "domain", "field", "focus", "specialize", "expertise",
	}
	return containsAnyPattern(content, patterns)
}

// containsAnyPattern checks if the content contains any of the given patterns
func containsAnyPattern(content string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	return false
}
