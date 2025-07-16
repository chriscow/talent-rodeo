package internal

import (
	"context"
	"time"
	"talent-rodeo/internal/inbox"
)

// HoldCriteria defines criteria for when to hold an email response
type HoldCriteria struct {
	// Enable/disable holding responses
	Enabled bool
	
	// Hold all responses regardless of content
	HoldAll bool
	
	// Hold responses with escalated items
	HoldEscalations bool
	
	// Hold responses for specific users (by email)
	HoldUserEmails []string
	
	// Hold responses containing specific keywords
	HoldKeywords []string
	
	// Hold responses based on response type
	HoldConfirmations bool
	HoldPartialResponses bool
	HoldEscalationResponses bool
	
	// Hold responses during specific time periods
	HoldAfterHours bool
	BusinessHoursStart int // 0-23
	BusinessHoursEnd   int // 0-23
	
	// Hold responses with high confidence/quality scores
	HoldHighConfidence bool
	ConfidenceThreshold float64
	
	// Auto-expire settings
	DefaultExpirationHours int
	MaxExpirationHours     int
}

// DefaultHoldCriteria returns sensible default hold criteria
func DefaultHoldCriteria() HoldCriteria {
	return HoldCriteria{
		Enabled:                 false, // Disabled by default
		HoldAll:                 false,
		HoldEscalations:         true,  // Hold responses with escalations
		HoldUserEmails:          []string{},
		HoldKeywords:            []string{},
		HoldConfirmations:       false,
		HoldPartialResponses:    true,  // Hold partial responses
		HoldEscalationResponses: true,  // Hold escalation responses
		HoldAfterHours:          false,
		BusinessHoursStart:      9,     // 9 AM
		BusinessHoursEnd:        17,    // 5 PM
		HoldHighConfidence:      false,
		ConfidenceThreshold:     0.8,
		DefaultExpirationHours:  24,    // 24 hours
		MaxExpirationHours:      72,    // 3 days
	}
}

// HoldDecisionContext contains context for making hold decisions
type HoldDecisionContext struct {
	OriginalEmail   *inbox.Email
	Response        *inbox.EmailResponse
	HandlingResults []*inbox.HandlingResult
	UserEmail       string
	Timestamp       time.Time
}

// HoldDecision represents the decision about whether to hold a response
type HoldDecision struct {
	ShouldHold bool
	Reason     string
	Criteria   string
	ExpiresAt  time.Time
}

// HoldDecisionEvaluator evaluates whether an email response should be held
type HoldDecisionEvaluator struct {
	criteria HoldCriteria
}

// NewHoldDecisionEvaluator creates a new hold decision evaluator
func NewHoldDecisionEvaluator(criteria HoldCriteria) *HoldDecisionEvaluator {
	return &HoldDecisionEvaluator{
		criteria: criteria,
	}
}

// EvaluateHoldDecision determines if a response should be held based on criteria
func (hde *HoldDecisionEvaluator) EvaluateHoldDecision(ctx context.Context, context HoldDecisionContext) HoldDecision {
	// If holding is disabled, don't hold anything
	if !hde.criteria.Enabled {
		return HoldDecision{
			ShouldHold: false,
			Reason:     "holding disabled",
			Criteria:   "disabled",
		}
	}
	
	// Check if we should hold all responses
	if hde.criteria.HoldAll {
		return HoldDecision{
			ShouldHold: true,
			Reason:     "configured to hold all responses",
			Criteria:   "hold_all",
			ExpiresAt:  context.Timestamp.Add(time.Duration(hde.criteria.DefaultExpirationHours) * time.Hour),
		}
	}
	
	// Check response type criteria
	if decision := hde.evaluateResponseType(context); decision.ShouldHold {
		return decision
	}
	
	// Check user email criteria
	if decision := hde.evaluateUserEmail(context); decision.ShouldHold {
		return decision
	}
	
	// Check keyword criteria
	if decision := hde.evaluateKeywords(context); decision.ShouldHold {
		return decision
	}
	
	// Check business hours criteria
	if decision := hde.evaluateBusinessHours(context); decision.ShouldHold {
		return decision
	}
	
	// Check escalation criteria
	if decision := hde.evaluateEscalations(context); decision.ShouldHold {
		return decision
	}
	
	// Default: don't hold
	return HoldDecision{
		ShouldHold: false,
		Reason:     "no hold criteria matched",
		Criteria:   "no_match",
	}
}

// evaluateResponseType checks if response type should be held
func (hde *HoldDecisionEvaluator) evaluateResponseType(context HoldDecisionContext) HoldDecision {
	switch context.Response.Type {
	case inbox.ResponseTypeConfirmation:
		if hde.criteria.HoldConfirmations {
			return HoldDecision{
				ShouldHold: true,
				Reason:     "confirmation response type configured to hold",
				Criteria:   "response_type_confirmation",
				ExpiresAt:  context.Timestamp.Add(time.Duration(hde.criteria.DefaultExpirationHours) * time.Hour),
			}
		}
	case inbox.ResponseTypePartial:
		if hde.criteria.HoldPartialResponses {
			return HoldDecision{
				ShouldHold: true,
				Reason:     "partial response type configured to hold",
				Criteria:   "response_type_partial",
				ExpiresAt:  context.Timestamp.Add(time.Duration(hde.criteria.DefaultExpirationHours) * time.Hour),
			}
		}
	case inbox.ResponseTypeEscalation:
		if hde.criteria.HoldEscalationResponses {
			return HoldDecision{
				ShouldHold: true,
				Reason:     "escalation response type configured to hold",
				Criteria:   "response_type_escalation",
				ExpiresAt:  context.Timestamp.Add(time.Duration(hde.criteria.DefaultExpirationHours) * time.Hour),
			}
		}
	}
	
	return HoldDecision{ShouldHold: false}
}

// evaluateUserEmail checks if user email should be held
func (hde *HoldDecisionEvaluator) evaluateUserEmail(context HoldDecisionContext) HoldDecision {
	for _, email := range hde.criteria.HoldUserEmails {
		if context.UserEmail == email {
			return HoldDecision{
				ShouldHold: true,
				Reason:     "user email configured to hold",
				Criteria:   "user_email",
				ExpiresAt:  context.Timestamp.Add(time.Duration(hde.criteria.DefaultExpirationHours) * time.Hour),
			}
		}
	}
	
	return HoldDecision{ShouldHold: false}
}

// evaluateKeywords checks if response contains keywords that should be held
func (hde *HoldDecisionEvaluator) evaluateKeywords(context HoldDecisionContext) HoldDecision {
	responseText := context.Response.Body + " " + context.Response.HTMLBody
	
	for _, keyword := range hde.criteria.HoldKeywords {
		if keyword != "" && contains(responseText, keyword) {
			return HoldDecision{
				ShouldHold: true,
				Reason:     "response contains hold keyword: " + keyword,
				Criteria:   "keyword",
				ExpiresAt:  context.Timestamp.Add(time.Duration(hde.criteria.DefaultExpirationHours) * time.Hour),
			}
		}
	}
	
	return HoldDecision{ShouldHold: false}
}

// evaluateBusinessHours checks if response is outside business hours
func (hde *HoldDecisionEvaluator) evaluateBusinessHours(context HoldDecisionContext) HoldDecision {
	if !hde.criteria.HoldAfterHours {
		return HoldDecision{ShouldHold: false}
	}
	
	hour := context.Timestamp.Hour()
	if hour < hde.criteria.BusinessHoursStart || hour >= hde.criteria.BusinessHoursEnd {
		return HoldDecision{
			ShouldHold: true,
			Reason:     "response generated outside business hours",
			Criteria:   "business_hours",
			ExpiresAt:  context.Timestamp.Add(time.Duration(hde.criteria.DefaultExpirationHours) * time.Hour),
		}
	}
	
	return HoldDecision{ShouldHold: false}
}

// evaluateEscalations checks if response has escalated items
func (hde *HoldDecisionEvaluator) evaluateEscalations(context HoldDecisionContext) HoldDecision {
	if !hde.criteria.HoldEscalations {
		return HoldDecision{ShouldHold: false}
	}
	
	if context.Response.EscalatedCount() > 0 {
		return HoldDecision{
			ShouldHold: true,
			Reason:     "response contains escalated items",
			Criteria:   "escalations",
			ExpiresAt:  context.Timestamp.Add(time.Duration(hde.criteria.DefaultExpirationHours) * time.Hour),
		}
	}
	
	return HoldDecision{ShouldHold: false}
}

// Helper function to check if string contains substring (case-insensitive)
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return false
	}
	
	// Simple case-insensitive substring search
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if toLower(haystack[i+j]) != toLower(needle[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Helper function to convert character to lowercase
func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}