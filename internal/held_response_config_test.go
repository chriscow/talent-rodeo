package internal

import (
	"context"
	"testing"
	"time"
	"talent-rodeo/internal/inbox"
)

func TestDefaultHoldCriteria(t *testing.T) {
	criteria := DefaultHoldCriteria()
	
	// Check default values
	if criteria.Enabled {
		t.Error("Expected Enabled to be false by default")
	}
	if criteria.HoldAll {
		t.Error("Expected HoldAll to be false by default")
	}
	if !criteria.HoldEscalations {
		t.Error("Expected HoldEscalations to be true by default")
	}
	if !criteria.HoldPartialResponses {
		t.Error("Expected HoldPartialResponses to be true by default")
	}
	if !criteria.HoldEscalationResponses {
		t.Error("Expected HoldEscalationResponses to be true by default")
	}
	if criteria.HoldConfirmations {
		t.Error("Expected HoldConfirmations to be false by default")
	}
	if criteria.DefaultExpirationHours != 24 {
		t.Errorf("Expected DefaultExpirationHours to be 24, got %d", criteria.DefaultExpirationHours)
	}
	if criteria.BusinessHoursStart != 9 {
		t.Errorf("Expected BusinessHoursStart to be 9, got %d", criteria.BusinessHoursStart)
	}
	if criteria.BusinessHoursEnd != 17 {
		t.Errorf("Expected BusinessHoursEnd to be 17, got %d", criteria.BusinessHoursEnd)
	}
}

func TestHoldDecisionEvaluator_EvaluateHoldDecision_Disabled(t *testing.T) {
	criteria := DefaultHoldCriteria()
	criteria.Enabled = false
	
	evaluator := NewHoldDecisionEvaluator(criteria)
	
	decisionContext := HoldDecisionContext{
		OriginalEmail: &inbox.Email{
			ID:      "test-id",
			From:    "test@example.com",
			Subject: "Test Subject",
			Body:    "Test Body",
		},
		Response: &inbox.EmailResponse{
			Type:    inbox.ResponseTypeConfirmation,
			Subject: "RE: Test Subject",
			Body:    "Test response body",
		},
		UserEmail: "test@example.com",
		Timestamp: time.Now(),
	}
	
	decision := evaluator.EvaluateHoldDecision(context.Background(), decisionContext)
	
	if decision.ShouldHold {
		t.Error("Expected decision to not hold when disabled")
	}
	if decision.Reason != "holding disabled" {
		t.Errorf("Expected reason 'holding disabled', got '%s'", decision.Reason)
	}
}

func TestHoldDecisionEvaluator_EvaluateHoldDecision_HoldAll(t *testing.T) {
	criteria := DefaultHoldCriteria()
	criteria.Enabled = true
	criteria.HoldAll = true
	
	evaluator := NewHoldDecisionEvaluator(criteria)
	
	decisionContext := HoldDecisionContext{
		OriginalEmail: &inbox.Email{
			ID:      "test-id",
			From:    "test@example.com",
			Subject: "Test Subject",
			Body:    "Test Body",
		},
		Response: &inbox.EmailResponse{
			Type:    inbox.ResponseTypeConfirmation,
			Subject: "RE: Test Subject",
			Body:    "Test response body",
		},
		UserEmail: "test@example.com",
		Timestamp: time.Now(),
	}
	
	decision := evaluator.EvaluateHoldDecision(context.Background(), decisionContext)
	
	if !decision.ShouldHold {
		t.Error("Expected decision to hold when HoldAll is true")
	}
	if decision.Reason != "configured to hold all responses" {
		t.Errorf("Expected reason 'configured to hold all responses', got '%s'", decision.Reason)
	}
	if decision.Criteria != "hold_all" {
		t.Errorf("Expected criteria 'hold_all', got '%s'", decision.Criteria)
	}
}

func TestHoldDecisionEvaluator_EvaluateResponseType(t *testing.T) {
	tests := []struct {
		name           string
		responseType   inbox.ResponseType
		holdConfirm    bool
		holdPartial    bool
		holdEscalation bool
		shouldHold     bool
		criteria       string
	}{
		{
			name:           "hold confirmation when enabled",
			responseType:   inbox.ResponseTypeConfirmation,
			holdConfirm:    true,
			holdPartial:    false,
			holdEscalation: false,
			shouldHold:     true,
			criteria:       "response_type_confirmation",
		},
		{
			name:           "don't hold confirmation when disabled",
			responseType:   inbox.ResponseTypeConfirmation,
			holdConfirm:    false,
			holdPartial:    false,
			holdEscalation: false,
			shouldHold:     false,
			criteria:       "",
		},
		{
			name:           "hold partial when enabled",
			responseType:   inbox.ResponseTypePartial,
			holdConfirm:    false,
			holdPartial:    true,
			holdEscalation: false,
			shouldHold:     true,
			criteria:       "response_type_partial",
		},
		{
			name:           "hold escalation when enabled",
			responseType:   inbox.ResponseTypeEscalation,
			holdConfirm:    false,
			holdPartial:    false,
			holdEscalation: true,
			shouldHold:     true,
			criteria:       "response_type_escalation",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			criteria := DefaultHoldCriteria()
			criteria.Enabled = true
			criteria.HoldAll = false
			criteria.HoldConfirmations = tt.holdConfirm
			criteria.HoldPartialResponses = tt.holdPartial
			criteria.HoldEscalationResponses = tt.holdEscalation
			
			evaluator := NewHoldDecisionEvaluator(criteria)
			
			decisionContext := HoldDecisionContext{
				OriginalEmail: &inbox.Email{
					ID:      "test-id",
					From:    "test@example.com",
					Subject: "Test Subject",
					Body:    "Test Body",
				},
				Response: &inbox.EmailResponse{
					Type:    tt.responseType,
					Subject: "RE: Test Subject",
					Body:    "Test response body",
				},
				UserEmail: "test@example.com",
				Timestamp: time.Now(),
			}
			
			decision := evaluator.EvaluateHoldDecision(context.Background(), decisionContext)
			
			if decision.ShouldHold != tt.shouldHold {
				t.Errorf("Expected shouldHold %v, got %v", tt.shouldHold, decision.ShouldHold)
			}
			if tt.shouldHold && decision.Criteria != tt.criteria {
				t.Errorf("Expected criteria '%s', got '%s'", tt.criteria, decision.Criteria)
			}
		})
	}
}

func TestHoldDecisionEvaluator_EvaluateUserEmail(t *testing.T) {
	criteria := DefaultHoldCriteria()
	criteria.Enabled = true
	criteria.HoldAll = false
	criteria.HoldUserEmails = []string{"hold@example.com", "another@example.com"}
	
	evaluator := NewHoldDecisionEvaluator(criteria)
	
	tests := []struct {
		name       string
		userEmail  string
		shouldHold bool
	}{
		{"hold listed email", "hold@example.com", true},
		{"hold another listed email", "another@example.com", true},
		{"don't hold unlisted email", "normal@example.com", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decisionContext := HoldDecisionContext{
				OriginalEmail: &inbox.Email{
					ID:      "test-id",
					From:    "test@example.com",
					Subject: "Test Subject",
					Body:    "Test Body",
				},
				Response: &inbox.EmailResponse{
					Type:    inbox.ResponseTypeConfirmation,
					Subject: "RE: Test Subject",
					Body:    "Test response body",
				},
				UserEmail: tt.userEmail,
				Timestamp: time.Now(),
			}
			
			decision := evaluator.EvaluateHoldDecision(context.Background(), decisionContext)
			
			if decision.ShouldHold != tt.shouldHold {
				t.Errorf("Expected shouldHold %v, got %v", tt.shouldHold, decision.ShouldHold)
			}
			if tt.shouldHold && decision.Criteria != "user_email" {
				t.Errorf("Expected criteria 'user_email', got '%s'", decision.Criteria)
			}
		})
	}
}

func TestHoldDecisionEvaluator_EvaluateKeywords(t *testing.T) {
	criteria := DefaultHoldCriteria()
	criteria.Enabled = true
	criteria.HoldAll = false
	criteria.HoldKeywords = []string{"urgent", "important", "ASAP"}
	
	evaluator := NewHoldDecisionEvaluator(criteria)
	
	tests := []struct {
		name         string
		responseBody string
		htmlBody     string
		shouldHold   bool
	}{
		{"hold when keyword in body", "This is urgent", "", true},
		{"hold when keyword in HTML", "", "<p>This is urgent</p>", true},
		{"hold case insensitive", "This is URGENT", "", true},
		{"hold different keyword", "This is important", "", true},
		{"don't hold without keywords", "This is normal", "", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decisionContext := HoldDecisionContext{
				OriginalEmail: &inbox.Email{
					ID:      "test-id",
					From:    "test@example.com",
					Subject: "Test Subject",
					Body:    "Test Body",
				},
				Response: &inbox.EmailResponse{
					Type:        inbox.ResponseTypeConfirmation,
					Subject:     "RE: Test Subject",
					Body:        tt.responseBody,
					HTMLBody:    tt.htmlBody,
				},
				UserEmail: "test@example.com",
				Timestamp: time.Now(),
			}
			
			decision := evaluator.EvaluateHoldDecision(context.Background(), decisionContext)
			
			if decision.ShouldHold != tt.shouldHold {
				t.Errorf("Expected shouldHold %v, got %v", tt.shouldHold, decision.ShouldHold)
			}
			if tt.shouldHold && decision.Criteria != "keyword" {
				t.Errorf("Expected criteria 'keyword', got '%s'", decision.Criteria)
			}
		})
	}
}

func TestHoldDecisionEvaluator_EvaluateBusinessHours(t *testing.T) {
	criteria := DefaultHoldCriteria()
	criteria.Enabled = true
	criteria.HoldAll = false
	criteria.HoldAfterHours = true
	criteria.BusinessHoursStart = 9  // 9 AM
	criteria.BusinessHoursEnd = 17   // 5 PM
	
	evaluator := NewHoldDecisionEvaluator(criteria)
	
	tests := []struct {
		name       string
		hour       int
		shouldHold bool
	}{
		{"hold early morning", 6, true},
		{"don't hold business hours start", 9, false},
		{"don't hold business hours middle", 12, false},
		{"don't hold business hours end-1", 16, false},
		{"hold after business hours", 17, true},
		{"hold evening", 22, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp := time.Date(2023, 1, 1, tt.hour, 0, 0, 0, time.UTC)
			
			decisionContext := HoldDecisionContext{
				OriginalEmail: &inbox.Email{
					ID:      "test-id",
					From:    "test@example.com",
					Subject: "Test Subject",
					Body:    "Test Body",
				},
				Response: &inbox.EmailResponse{
					Type:    inbox.ResponseTypeConfirmation,
					Subject: "RE: Test Subject",
					Body:    "Test response body",
				},
				UserEmail: "test@example.com",
				Timestamp: timestamp,
			}
			
			decision := evaluator.EvaluateHoldDecision(context.Background(), decisionContext)
			
			if decision.ShouldHold != tt.shouldHold {
				t.Errorf("Expected shouldHold %v, got %v", tt.shouldHold, decision.ShouldHold)
			}
			if tt.shouldHold && decision.Criteria != "business_hours" {
				t.Errorf("Expected criteria 'business_hours', got '%s'", decision.Criteria)
			}
		})
	}
}

func TestHoldDecisionEvaluator_EvaluateEscalations(t *testing.T) {
	criteria := DefaultHoldCriteria()
	criteria.Enabled = true
	criteria.HoldAll = false
	criteria.HoldEscalations = true
	
	evaluator := NewHoldDecisionEvaluator(criteria)
	
	tests := []struct {
		name           string
		escalatedItems []string
		shouldHold     bool
	}{
		{"hold when escalated items exist", []string{"item1", "item2"}, true},
		{"don't hold when no escalated items", []string{}, false},
		{"don't hold when nil escalated items", nil, false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decisionContext := HoldDecisionContext{
				OriginalEmail: &inbox.Email{
					ID:      "test-id",
					From:    "test@example.com",
					Subject: "Test Subject",
					Body:    "Test Body",
				},
				Response: &inbox.EmailResponse{
					Type:           inbox.ResponseTypeConfirmation,
					Subject:        "RE: Test Subject",
					Body:           "Test response body",
					EscalatedItems: tt.escalatedItems,
				},
				UserEmail: "test@example.com",
				Timestamp: time.Now(),
			}
			
			decision := evaluator.EvaluateHoldDecision(context.Background(), decisionContext)
			
			if decision.ShouldHold != tt.shouldHold {
				t.Errorf("Expected shouldHold %v, got %v", tt.shouldHold, decision.ShouldHold)
			}
			if tt.shouldHold && decision.Criteria != "escalations" {
				t.Errorf("Expected criteria 'escalations', got '%s'", decision.Criteria)
			}
		})
	}
}

func TestHoldDecisionEvaluator_NoMatch(t *testing.T) {
	criteria := DefaultHoldCriteria()
	criteria.Enabled = true
	criteria.HoldAll = false
	criteria.HoldConfirmations = false
	criteria.HoldPartialResponses = false
	criteria.HoldEscalationResponses = false
	criteria.HoldEscalations = false
	criteria.HoldAfterHours = false
	criteria.HoldUserEmails = []string{}
	criteria.HoldKeywords = []string{}
	
	evaluator := NewHoldDecisionEvaluator(criteria)
	
	decisionContext := HoldDecisionContext{
		OriginalEmail: &inbox.Email{
			ID:      "test-id",
			From:    "test@example.com",
			Subject: "Test Subject",
			Body:    "Test Body",
		},
		Response: &inbox.EmailResponse{
			Type:    inbox.ResponseTypeConfirmation,
			Subject: "RE: Test Subject",
			Body:    "Test response body",
		},
		UserEmail: "test@example.com",
		Timestamp: time.Now(),
	}
	
	decision := evaluator.EvaluateHoldDecision(context.Background(), decisionContext)
	
	if decision.ShouldHold {
		t.Error("Expected decision to not hold when no criteria match")
	}
	if decision.Reason != "no hold criteria matched" {
		t.Errorf("Expected reason 'no hold criteria matched', got '%s'", decision.Reason)
	}
	if decision.Criteria != "no_match" {
		t.Errorf("Expected criteria 'no_match', got '%s'", decision.Criteria)
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		haystack string
		needle   string
		want     bool
	}{
		{"found exact match", "hello world", "world", true},
		{"found case insensitive", "Hello World", "world", true},
		{"found case insensitive reverse", "hello world", "WORLD", true},
		{"not found", "hello world", "test", false},
		{"empty needle", "hello world", "", false},
		{"empty haystack", "", "test", false},
		{"both empty", "", "", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.haystack, tt.needle); got != tt.want {
				t.Errorf("contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToLower(t *testing.T) {
	tests := []struct {
		name string
		c    byte
		want byte
	}{
		{"uppercase A", 'A', 'a'},
		{"uppercase Z", 'Z', 'z'},
		{"lowercase a", 'a', 'a'},
		{"lowercase z", 'z', 'z'},
		{"digit", '5', '5'},
		{"special char", '!', '!'},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toLower(tt.c); got != tt.want {
				t.Errorf("toLower() = %v, want %v", got, tt.want)
			}
		})
	}
}