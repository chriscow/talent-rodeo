package internal

import (
	"strings"
	"testing"
	"time"
)

func TestInitializeConversationChecklist(t *testing.T) {
	checklist := InitializeConversationChecklist()

	if checklist == nil {
		t.Fatal("Expected checklist to be initialized, got nil")
	}

	if checklist.OverallStatus != ChecklistStatusInProgress {
		t.Errorf("Expected status to be %s, got %s", ChecklistStatusInProgress, checklist.OverallStatus)
	}

	if checklist.CompletionAttempts != 0 {
		t.Errorf("Expected completion attempts to be 0, got %d", checklist.CompletionAttempts)
	}

	if checklist.MessageCount != 0 {
		t.Errorf("Expected message count to be 0, got %d", checklist.MessageCount)
	}

	expectedItemCount := 8
	if len(checklist.Items) != expectedItemCount {
		t.Errorf("Expected %d checklist items, got %d", expectedItemCount, len(checklist.Items))
	}

	// Check that all required items are present
	requiredTopics := []string{
		"introduction", "warm_up", "deal_breakers", "search_strategy",
		"work_environment", "location_preferences", "validation", "final_confirmation",
	}

	for _, topicID := range requiredTopics {
		found := false
		for _, item := range checklist.Items {
			if item.ID == topicID {
				found = true
				if item.Status != ChecklistItemStatusPending {
					t.Errorf("Expected item %s to have status %s, got %s", topicID, ChecklistItemStatusPending, item.Status)
				}
				if item.Quality != ChecklistItemQualityLow {
					t.Errorf("Expected item %s to have quality %s, got %s", topicID, ChecklistItemQualityLow, item.Quality)
				}
				break
			}
		}
		if !found {
			t.Errorf("Required topic %s not found in checklist", topicID)
		}
	}
}

func TestUpdateChecklistItem(t *testing.T) {
	checklist := InitializeConversationChecklist()

	// Test updating an existing item
	extractedInfo := map[string]interface{}{
		"name_confirmed": true,
		"good_time_to_chat": true,
	}

	err := UpdateChecklistItem(checklist, "introduction", ChecklistItemStatusCompleted, ChecklistItemQualityHigh, extractedInfo, "Successfully gathered introduction info")
	if err != nil {
		t.Fatalf("Failed to update checklist item: %v", err)
	}

	// Verify the update
	item, err := GetChecklistItem(checklist, "introduction")
	if err != nil {
		t.Fatalf("Failed to get checklist item: %v", err)
	}

	if item.Status != ChecklistItemStatusCompleted {
		t.Errorf("Expected status %s, got %s", ChecklistItemStatusCompleted, item.Status)
	}

	if item.Quality != ChecklistItemQualityHigh {
		t.Errorf("Expected quality %s, got %s", ChecklistItemQualityHigh, item.Quality)
	}

	if item.ValidationNotes != "Successfully gathered introduction info" {
		t.Errorf("Expected validation notes to be updated, got %s", item.ValidationNotes)
	}

	if !item.ExtractedInfo["name_confirmed"].(bool) {
		t.Error("Expected name_confirmed to be true")
	}

	// Test updating non-existent item
	err = UpdateChecklistItem(checklist, "nonexistent", ChecklistItemStatusCompleted, ChecklistItemQualityHigh, nil, "")
	if err == nil {
		t.Error("Expected error when updating non-existent item")
	}
}

func TestGetChecklistItem(t *testing.T) {
	checklist := InitializeConversationChecklist()

	// Test getting existing item
	item, err := GetChecklistItem(checklist, "introduction")
	if err != nil {
		t.Fatalf("Failed to get checklist item: %v", err)
	}

	if item.ID != "introduction" {
		t.Errorf("Expected item ID to be 'introduction', got %s", item.ID)
	}

	// Test getting non-existent item
	_, err = GetChecklistItem(checklist, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent item")
	}
}

func TestValidateChecklistCompletion(t *testing.T) {
	checklist := InitializeConversationChecklist()

	// Test initial state - should not be complete
	canComplete, issues, err := ValidateChecklistCompletion(checklist)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if canComplete {
		t.Error("Expected checklist to not be complete initially")
	}

	if len(issues) == 0 {
		t.Error("Expected issues to be reported for incomplete checklist")
	}

	// Complete all required items with high quality
	requiredItems := []string{"introduction", "warm_up", "deal_breakers", "search_strategy", "work_environment", "location_preferences", "final_confirmation"}
	
	for _, itemID := range requiredItems {
		err := UpdateChecklistItem(checklist, itemID, ChecklistItemStatusCompleted, ChecklistItemQualityHigh, nil, "")
		if err != nil {
			t.Fatalf("Failed to update item %s: %v", itemID, err)
		}
	}

	// Test completion - should be complete now
	canComplete, issues, err = ValidateChecklistCompletion(checklist)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if !canComplete {
		t.Errorf("Expected checklist to be complete, issues: %v", issues)
	}

	if len(issues) > 0 {
		t.Errorf("Expected no issues for complete checklist, got: %v", issues)
	}
}

func TestValidateChecklistCompletionWithLowerThreshold(t *testing.T) {
	checklist := InitializeConversationChecklist()

	// Set high completion attempts to trigger lower threshold
	checklist.CompletionAttempts = MaxCompletionAttempts + 1

	// Mark all required items as at least in_progress (lower threshold only requires items to be started)
	requiredItems := []string{"introduction", "warm_up", "deal_breakers", "search_strategy", "work_environment", "location_preferences", "final_confirmation"}
	
	for _, itemID := range requiredItems {
		UpdateChecklistItem(checklist, itemID, ChecklistItemStatusInProgress, ChecklistItemQualityLow, nil, "")
	}

	// Should be able to complete with lower threshold
	canComplete, issues, err := ValidateChecklistCompletion(checklist)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if !canComplete {
		t.Errorf("Expected checklist to be complete with lower threshold, issues: %v", issues)
	}
}

func TestValidateChecklistCompletionWithMaxMessages(t *testing.T) {
	checklist := InitializeConversationChecklist()

	// Set message count to max to trigger lower threshold
	checklist.MessageCount = MaxConversationLength + 1

	// Mark all required items as at least in_progress (lower threshold only requires items to be started)
	requiredItems := []string{"introduction", "warm_up", "deal_breakers", "search_strategy", "work_environment", "location_preferences", "final_confirmation"}
	
	for _, itemID := range requiredItems {
		UpdateChecklistItem(checklist, itemID, ChecklistItemStatusInProgress, ChecklistItemQualityLow, nil, "")
	}

	// Should be able to complete with lower threshold due to max messages
	canComplete, issues, err := ValidateChecklistCompletion(checklist)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if !canComplete {
		t.Errorf("Expected checklist to be complete with max messages threshold, issues: %v", issues)
	}
}

func TestIncrementCompletionAttempt(t *testing.T) {
	checklist := InitializeConversationChecklist()

	initialAttempts := checklist.CompletionAttempts
	IncrementCompletionAttempt(checklist)

	if checklist.CompletionAttempts != initialAttempts+1 {
		t.Errorf("Expected completion attempts to be %d, got %d", initialAttempts+1, checklist.CompletionAttempts)
	}
}

func TestIncrementMessageCount(t *testing.T) {
	checklist := InitializeConversationChecklist()

	initialCount := checklist.MessageCount
	IncrementMessageCount(checklist)

	if checklist.MessageCount != initialCount+1 {
		t.Errorf("Expected message count to be %d, got %d", initialCount+1, checklist.MessageCount)
	}
}

func TestGetChecklistSummary(t *testing.T) {
	checklist := InitializeConversationChecklist()

	// Test with initial state
	summary := GetChecklistSummary(checklist)

	if summary["overall_status"] != ChecklistStatusInProgress {
		t.Errorf("Expected overall status %s, got %v", ChecklistStatusInProgress, summary["overall_status"])
	}

	if summary["total_items"].(int) != 8 {
		t.Errorf("Expected 8 total items, got %v", summary["total_items"])
	}

	if summary["completed_items"].(int) != 0 {
		t.Errorf("Expected 0 completed items, got %v", summary["completed_items"])
	}

	if summary["can_complete"].(bool) {
		t.Error("Expected can_complete to be false initially")
	}

	// Test with nil checklist
	nilSummary := GetChecklistSummary(nil)
	if nilSummary["error"] == nil {
		t.Error("Expected error for nil checklist")
	}

	// Complete some items and test again
	UpdateChecklistItem(checklist, "introduction", ChecklistItemStatusCompleted, ChecklistItemQualityHigh, nil, "")
	UpdateChecklistItem(checklist, "warm_up", ChecklistItemStatusCompleted, ChecklistItemQualityMedium, nil, "")

	summary = GetChecklistSummary(checklist)
	
	if summary["completed_items"].(int) != 2 {
		t.Errorf("Expected 2 completed items, got %v", summary["completed_items"])
	}

	if summary["high_quality_items"].(int) != 1 {
		t.Errorf("Expected 1 high quality item, got %v", summary["high_quality_items"])
	}
}

func TestChecklistWithQualityIssues(t *testing.T) {
	checklist := InitializeConversationChecklist()

	// Complete all required items but with low quality
	requiredItems := []string{"introduction", "warm_up", "deal_breakers", "search_strategy", "work_environment", "location_preferences", "final_confirmation"}
	
	for _, itemID := range requiredItems {
		err := UpdateChecklistItem(checklist, itemID, ChecklistItemStatusCompleted, ChecklistItemQualityLow, nil, "")
		if err != nil {
			t.Fatalf("Failed to update item %s: %v", itemID, err)
		}
	}

	// Should not be complete due to low quality
	canComplete, issues, err := ValidateChecklistCompletion(checklist)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if canComplete {
		t.Error("Expected checklist to not be complete with low quality items")
	}

	if len(issues) == 0 {
		t.Error("Expected quality issues to be reported")
	}

	// Check that issues mention quality
	qualityIssueFound := false
	for _, issue := range issues {
		if strings.Contains(issue, "low quality information") {
			qualityIssueFound = true
			break
		}
	}
	if !qualityIssueFound {
		t.Errorf("Expected quality issues to be reported, got: %v", issues)
	}
}

func TestChecklistItemLastUpdatedTracking(t *testing.T) {
	checklist := InitializeConversationChecklist()

	// Get initial time
	initialTime := checklist.Items[0].LastUpdated

	// Wait a small amount to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Update an item
	err := UpdateChecklistItem(checklist, "introduction", ChecklistItemStatusInProgress, ChecklistItemQualityMedium, nil, "Updated")
	if err != nil {
		t.Fatalf("Failed to update checklist item: %v", err)
	}

	// Check that LastUpdated was updated
	item, _ := GetChecklistItem(checklist, "introduction")
	if !item.LastUpdated.After(initialTime) {
		t.Error("Expected LastUpdated to be updated when item is modified")
	}

	if !checklist.LastUpdated.After(initialTime) {
		t.Error("Expected checklist LastUpdated to be updated when item is modified")
	}
}