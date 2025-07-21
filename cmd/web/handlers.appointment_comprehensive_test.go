package main

import (
	"strings"
	"testing"

	"github.com/chriscow/vapi-go"
)

// TestComprehensiveSuccessEvaluationHandling tests all combinations of end reasons and success evaluations
// to ensure our fix handles every case correctly
func TestComprehensiveSuccessEvaluationHandling(t *testing.T) {
	app := &application{
		logger: testLogger,
	}

	testCases := []struct {
		name              string
		endReason         string
		successEvaluation interface{}
		expectedFailure   bool
		description       string
	}{
		// ========== VOICEMAIL CASES (always failure) ==========
		{
			name:              "Voicemail + Bool True",
			endReason:         "voicemail",
			successEvaluation: true,
			expectedFailure:   true,
			description:       "Voicemail always fails regardless of success evaluation",
		},
		{
			name:              "Voicemail + Bool False",
			endReason:         "voicemail",
			successEvaluation: false,
			expectedFailure:   true,
			description:       "Voicemail always fails regardless of success evaluation",
		},
		{
			name:              "Voicemail + String True",
			endReason:         "voicemail",
			successEvaluation: "true",
			expectedFailure:   true,
			description:       "Voicemail always fails regardless of success evaluation",
		},
		{
			name:              "Voicemail + Success String",
			endReason:         "voicemail",
			successEvaluation: "The call was successful",
			expectedFailure:   true,
			description:       "Voicemail always fails regardless of success evaluation",
		},
		{
			name:              "Voicemail + No Analysis",
			endReason:         "voicemail",
			successEvaluation: nil,
			expectedFailure:   true,
			description:       "Voicemail always fails regardless of success evaluation",
		},

		// ========== ERROR CASES (always failure) ==========
		{
			name:              "Call Error + Bool True",
			endReason:         "call.start.error-vapifault-get-org",
			successEvaluation: true,
			expectedFailure:   true,
			description:       "Technical errors always fail regardless of success evaluation",
		},
		{
			name:              "Busy + Success String",
			endReason:         "busy",
			successEvaluation: "The call was successful",
			expectedFailure:   true,
			description:       "Busy signals always fail regardless of success evaluation",
		},

		// ========== SUCCESS END REASONS + SUCCESS EVALUATIONS ==========
		{
			name:              "Assistant Ended + Bool True",
			endReason:         "assistant-ended-call",
			successEvaluation: true,
			expectedFailure:   false,
			description:       "Both indicate success",
		},
		{
			name:              "Customer Ended + String True",
			endReason:         "customer-ended-call",
			successEvaluation: "true",
			expectedFailure:   false,
			description:       "Both indicate success",
		},
		{
			name:              "Max Duration + Success String",
			endReason:         "exceeded-max-duration",
			successEvaluation: "The call was successful and achieved its goals",
			expectedFailure:   false,
			description:       "Both indicate success",
		},
		{
			name:              "Assistant Ended + No Analysis",
			endReason:         "assistant-ended-call",
			successEvaluation: nil,
			expectedFailure:   false,
			description:       "Success end reason with no analysis defaults to success",
		},

		// ========== SUCCESS END REASONS + FAILURE EVALUATIONS (the key bug cases) ==========
		{
			name:              "Assistant Ended + Bool False [BUG CASE]",
			endReason:         "assistant-ended-call",
			successEvaluation: false,
			expectedFailure:   true,
			description:       "AI override: technical success but business failure",
		},
		{
			name:              "Customer Ended + String False [BUG CASE]",
			endReason:         "customer-ended-call",
			successEvaluation: "false",
			expectedFailure:   true,
			description:       "AI override: customer ended but call didn't achieve objective",
		},
		{
			name:              "Max Duration + Failure String [BUG CASE]",
			endReason:         "exceeded-max-duration",
			successEvaluation: "The call did not achieve its objective despite running full duration",
			expectedFailure:   true,
			description:       "AI override: time ran out but call was ineffective",
		},
		{
			name:              "Assistant Ended + Ambiguous String [BUG CASE]",
			endReason:         "assistant-ended-call",
			successEvaluation: "The candidate seemed disinterested and hung up quickly",
			expectedFailure:   true,
			description:       "AI override: assistant ended but call was poor quality",
		},

		// ========== REAL PRODUCTION CASE ==========
		{
			name:              "Real Production Case: 1e41f72b-c828-4260-b891-60a3a0b9e5ef",
			endReason:         "assistant-ended-call",
			successEvaluation: "false", // This was the actual value from the API
			expectedFailure:   true,
			description:       "Real case that was incorrectly marked as completed",
		},

		// ========== EDGE CASES ==========
		{
			name:              "Assistant Ended + Integer (Invalid)",
			endReason:         "assistant-ended-call",
			successEvaluation: 42,
			expectedFailure:   true,
			description:       "Invalid success evaluation type should default to failure",
		},
		{
			name:              "Assistant Ended + Empty String",
			endReason:         "assistant-ended-call",
			successEvaluation: "",
			expectedFailure:   true,
			description:       "Empty string should be treated as failure",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create the report
			var analysis *vapi.Analysis
			if tc.successEvaluation != nil {
				analysis = &vapi.Analysis{
					SuccessEvaluation: tc.successEvaluation,
				}
			}

			report := &vapi.EndOfCallReport{
				EndedReason: tc.endReason,
				Analysis:    analysis,
			}

			// Test the evaluation
			shouldProcessFailure, err := app.evaluateCallSuccess(report, tc.endReason)
			if err != nil {
				t.Fatalf("evaluateCallSuccess returned error: %v", err)
			}

			// Check the result
			if shouldProcessFailure != tc.expectedFailure {
				t.Errorf("❌ FAILED: %s", tc.description)
				t.Errorf("   End Reason: %s", tc.endReason)
				t.Errorf("   Success Evaluation: %v (%T)", tc.successEvaluation, tc.successEvaluation)
				t.Errorf("   Expected Failure: %v", tc.expectedFailure)
				t.Errorf("   Actual Failure: %v", shouldProcessFailure)

				if tc.expectedFailure {
					t.Errorf("   ❌ This should result in FAILED appointment status")
				} else {
					t.Errorf("   ❌ This should result in COMPLETED appointment status")
				}
			} else {
				// Success case
				if tc.expectedFailure {
					t.Logf("✅ CORRECT: %s → FAILED appointment", tc.description)
				} else {
					t.Logf("✅ CORRECT: %s → COMPLETED appointment", tc.description)
				}
			}
		})
	}

	// Summary statistics
	var totalCases, failureCases, successCases int
	for _, tc := range testCases {
		totalCases++
		if tc.expectedFailure {
			failureCases++
		} else {
			successCases++
		}
	}

	t.Logf("\n=== Test Summary ===")
	t.Logf("Total test cases: %d", totalCases)
	t.Logf("Expected failures: %d", failureCases)
	t.Logf("Expected successes: %d", successCases)
	t.Logf("===================")
}

// TestSpecificBugScenarios tests the exact scenarios that were problematic
func TestSpecificBugScenarios(t *testing.T) {
	app := &application{
		logger: testLogger,
	}

	// These are the specific problematic scenarios that should be marked as FAILED
	bugScenarios := []struct {
		name        string
		endReason   string
		successEval interface{}
		description string
	}{
		{
			name:        "Technical Success, Business Failure - Boolean",
			endReason:   "assistant-ended-call",
			successEval: false,
			description: "Assistant ended call but AI determined it was not successful",
		},
		{
			name:        "Technical Success, Business Failure - String",
			endReason:   "customer-ended-call",
			successEval: "false",
			description: "Customer ended call but AI analysis shows failure",
		},
		{
			name:        "Full Duration, No Success",
			endReason:   "exceeded-max-duration",
			successEval: false,
			description: "Call ran full time but didn't achieve objective",
		},
		{
			name:        "Descriptive Failure",
			endReason:   "assistant-ended-call",
			successEval: "The candidate was not interested and seemed distracted throughout",
			description: "AI provides detailed failure analysis",
		},
	}

	t.Logf("\n=== Testing Specific Bug Scenarios ===")

	for _, scenario := range bugScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			report := &vapi.EndOfCallReport{
				EndedReason: scenario.endReason,
				Analysis: &vapi.Analysis{
					SuccessEvaluation: scenario.successEval,
				},
			}

			shouldProcessFailure, err := app.evaluateCallSuccess(report, scenario.endReason)
			if err != nil {
				t.Fatalf("evaluateCallSuccess returned error: %v", err)
			}

			if !shouldProcessFailure {
				t.Errorf("🚨 BUG DETECTED: %s", scenario.description)
				t.Errorf("   This should be marked as FAILED but would be marked as COMPLETED")
				t.Errorf("   End Reason: %s", scenario.endReason)
				t.Errorf("   Success Evaluation: %v", scenario.successEval)
			} else {
				t.Logf("✅ FIXED: %s correctly marked as FAILED", scenario.description)
			}
		})
	}

	t.Logf("========================================")
}

// TestFailureReasonToDescriptionMapping tests our user-friendly failure descriptions
func TestFailureReasonToDescriptionMapping(t *testing.T) {
	app := &application{}

	mappings := map[string]string{
		"voicemail":                          "the call went to voicemail",
		"busy":                               "your phone was busy",
		"no-answer":                          "there was no answer",
		"failed":                             "the call failed to connect",
		"cancelled":                          "the call was cancelled",
		"dial-busy":                          "we couldn't connect the call",
		"dial-failed":                        "we couldn't connect the call",
		"pipeline-error-openai":              "there was a technical issue with our AI system",
		"pipeline-error-deepgram":            "there was a technical issue with our AI system",
		"call.start.error-vapifault-get-org": "there was a technical issue",
		"random-error-code":                  "there was a technical issue",
		"unknown-failure":                    "the call did not complete successfully",
		"":                                   "there was an issue with the call",
	}

	for endReason, expectedDescription := range mappings {
		t.Run("EndReason_"+strings.ReplaceAll(endReason, ".", "_"), func(t *testing.T) {
			description := app.getFailureReasonDescription(endReason)
			if description != expectedDescription {
				t.Errorf("Mapping failed for '%s'", endReason)
				t.Errorf("Expected: %s", expectedDescription)
				t.Errorf("Got: %s", description)
			} else {
				t.Logf("✅ '%s' → '%s'", endReason, description)
			}
		})
	}
}
