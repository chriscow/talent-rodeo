package internal

import (
	"testing"
)

func TestGetHybridCompletionStatus(t *testing.T) {
	// Test with nil checklist
	status := GetHybridCompletionStatus(nil)
	if status == nil {
		t.Fatal("Expected non-nil status for nil checklist")
	}
	if status.CanComplete {
		t.Error("Expected false for CanComplete with nil checklist")
	}

	// Test with empty checklist
	checklist := InitializeConversationChecklist()
	status = GetHybridCompletionStatus(checklist)
	
	if status == nil {
		t.Fatal("Expected non-nil status")
	}
	
	// Should not be able to complete with empty checklist
	if status.HardComplete {
		t.Error("Expected false for HardComplete with empty checklist")
	}
	
	// Test with partially completed checklist
	checklist.Items[0].Status = ChecklistItemStatusCompleted
	checklist.Items[0].Quality = ChecklistItemQualityHigh
	checklist.Items[1].Status = ChecklistItemStatusCompleted
	checklist.Items[1].Quality = ChecklistItemQualityMedium
	
	status = GetHybridCompletionStatus(checklist)
	
	if status.QualityMetrics.CompletedItems != 2 {
		t.Errorf("Expected 2 completed items, got %d", status.QualityMetrics.CompletedItems)
	}
	
	if status.QualityMetrics.HighQualityItems != 1 {
		t.Errorf("Expected 1 high quality item, got %d", status.QualityMetrics.HighQualityItems)
	}
	
	// Test completion score calculation
	if status.CompletionScore <= 0 {
		t.Error("Expected positive completion score")
	}
	
	// Test with all required items completed
	for i := range checklist.Items {
		if checklist.Items[i].Required {
			checklist.Items[i].Status = ChecklistItemStatusCompleted
			checklist.Items[i].Quality = ChecklistItemQualityHigh
		}
	}
	
	status = GetHybridCompletionStatus(checklist)
	
	if !status.HardComplete {
		t.Error("Expected true for HardComplete with all required items completed")
	}
	
	if !status.CanComplete {
		t.Error("Expected true for CanComplete with all required items completed")
	}
	
	// Test soft completion with engagement
	checklist2 := InitializeConversationChecklist()
	checklist2.MessageCount = 25 // High engagement
	
	// Complete 50% of required items
	requiredCount := 0
	for i := range checklist2.Items {
		if checklist2.Items[i].Required {
			requiredCount++
		}
	}
	
	completedCount := 0
	// Complete exactly 50% or more to meet soft completion criteria
	targetCompleted := int(float64(requiredCount) * 0.6) // Use 60% to be safe
	if targetCompleted == 0 {
		targetCompleted = 1 // Ensure at least 1 completed
	}
	
	for i := range checklist2.Items {
		if checklist2.Items[i].Required && completedCount < targetCompleted {
			checklist2.Items[i].Status = ChecklistItemStatusCompleted
			checklist2.Items[i].Quality = ChecklistItemQualityMedium
			completedCount++
		}
	}
	
	status = GetHybridCompletionStatus(checklist2)
	
	if !status.SoftComplete {
		t.Errorf("Expected true for SoftComplete with %d/%d completion and high engagement. Score: %.2f", completedCount, requiredCount, status.CompletionScore)
	}
	
	// Test escalated completion with max attempts
	checklist3 := InitializeConversationChecklist()
	checklist3.CompletionAttempts = MaxCompletionAttempts
	
	status = GetHybridCompletionStatus(checklist3)
	
	if !status.CanComplete {
		t.Error("Expected true for CanComplete with max attempts reached")
	}
	
	// Test guidance generation
	if status.Guidance == "" {
		t.Error("Expected non-empty guidance")
	}
}

func TestCompletionStatusTypes(t *testing.T) {
	
	// Test different scenarios
	testCases := []struct {
		name           string
		setupFunc      func(*ConversationChecklist)
		expectHard     bool
		expectSoft     bool
		expectComplete bool
	}{
		{
			name: "No completion",
			setupFunc: func(c *ConversationChecklist) {
				// Leave as is - all pending
			},
			expectHard:     false,
			expectSoft:     false,
			expectComplete: false,
		},
		{
			name: "High engagement soft completion",
			setupFunc: func(c *ConversationChecklist) {
				c.MessageCount = 25
				// Complete 60% of required items
				completed := 0
				required := 0
				for i := range c.Items {
					if c.Items[i].Required {
						required++
					}
				}
				target := int(float64(required) * 0.6)
				for i := range c.Items {
					if c.Items[i].Required && completed < target {
						c.Items[i].Status = ChecklistItemStatusCompleted
						c.Items[i].Quality = ChecklistItemQualityMedium
						completed++
					}
				}
			},
			expectHard:     false,
			expectSoft:     true,
			expectComplete: true,
		},
		{
			name: "Max attempts escalation",
			setupFunc: func(c *ConversationChecklist) {
				c.CompletionAttempts = MaxCompletionAttempts
				c.MessageCount = 10
			},
			expectHard:     false,
			expectSoft:     false,
			expectComplete: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			checklist := InitializeConversationChecklist()
			tc.setupFunc(checklist)
			
			status := GetHybridCompletionStatus(checklist)
			
			if status.HardComplete != tc.expectHard {
				t.Errorf("Expected HardComplete=%v, got %v", tc.expectHard, status.HardComplete)
			}
			
			if status.SoftComplete != tc.expectSoft {
				t.Errorf("Expected SoftComplete=%v, got %v", tc.expectSoft, status.SoftComplete)
			}
			
			if status.CanComplete != tc.expectComplete {
				t.Errorf("Expected CanComplete=%v, got %v", tc.expectComplete, status.CanComplete)
			}
		})
	}
}