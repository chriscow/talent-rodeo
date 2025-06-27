package main

import (
	"context"
	"os"
	"testing"

	"github.com/chriscow/vapi-go"
)

// TestRealCallSuccessEvaluationBug tests the specific bug where SuccessEvaluation: false
// doesn't properly override successful end reasons using real VAPI call data
func TestRealCallSuccessEvaluationBug(t *testing.T) {
	// Skip if no VAPI API key
	if os.Getenv("VAPI_API_KEY") == "" {
		t.Skip("Skipping real VAPI call test: VAPI_API_KEY not set")
	}

	// This is the specific call ID that demonstrates the bug
	callID := "1e41f72b-c828-4260-b891-60a3a0b9e5ef"

	// Get the real call data from VAPI
	call, err := vapi.GetCall(context.Background(), callID)
	if err != nil {
		t.Fatalf("Failed to get call from VAPI: %v", err)
	}

	// Extract the end-of-call report data
	// Handle the pointer types correctly
	var endReason string
	if call.EndedReason != nil {
		endReason = *call.EndedReason
	} else {
		endReason = "unknown"
	}
	
	// Create the analysis structure based on the real call data
	var analysis *vapi.Analysis
	if call.Analysis != nil {
		analysis = &vapi.Analysis{
			SuccessEvaluation: call.Analysis.SuccessEvaluation,
		}
	}

	report := &vapi.EndOfCallReport{
		EndedReason: endReason,
		Analysis:    analysis,
	}

	// Log the real call data for inspection
	t.Logf("\n=== Real VAPI Call Data ===")
	t.Logf("Call ID: %s", callID)
	t.Logf("End Reason: %s", endReason)
	if analysis != nil {
		t.Logf("Success Evaluation: %v", analysis.SuccessEvaluation)
		t.Logf("Success Evaluation Type: %T", analysis.SuccessEvaluation)
	} else {
		t.Logf("Analysis: nil")
	}
	t.Logf("==========================\n")

	// Test the current evaluateCallSuccess logic
	// Create app with minimal logger to avoid nil pointer
	app := &application{
		logger: testLogger,
	}
	shouldProcessFailure, err := app.evaluateCallSuccess(report, endReason)
	if err != nil {
		t.Fatalf("evaluateCallSuccess returned error: %v", err)
	}

	// Based on the user's description, this should be a failure case
	// End reason: "assistant-ended-call" (normally successful)
	// Success evaluation: false (should override to failure)
	// Expected result: shouldProcessFailure = true (failure)

	t.Logf("Current evaluation result:")
	t.Logf("- shouldProcessFailure: %v", shouldProcessFailure)
	t.Logf("- Expected: true (should be a failure)")

	// Check if this demonstrates the bug
	if endReason == "assistant-ended-call" && analysis != nil {
		// Check the type and value of SuccessEvaluation
		switch successEval := analysis.SuccessEvaluation.(type) {
		case bool:
			if !successEval { // false
				if !shouldProcessFailure {
					t.Errorf("BUG DETECTED: End reason '%s' with SuccessEvaluation=false should result in failure, but got success", endReason)
					t.Errorf("This call should be marked as FAILED, not COMPLETED")
				} else {
					t.Logf("✓ Correctly identified as failure despite successful end reason")
				}
			}
		case string:
			if successEval == "false" {
				if !shouldProcessFailure {
					t.Errorf("BUG DETECTED: End reason '%s' with SuccessEvaluation='false' should result in failure, but got success", endReason)
					t.Errorf("This call should be marked as FAILED, not COMPLETED")
				} else {
					t.Logf("✓ Correctly identified as failure despite successful end reason")
				}
			}
		default:
			t.Logf("SuccessEvaluation has unexpected type: %T", successEval)
		}
	}

	// Additional validation - create a comprehensive test case
	testCases := []struct {
		name           string
		endReason      string
		successEval    interface{}
		expectedResult bool // true = should process failure
		description    string
	}{
		{
			name:           "Real Call Case - Assistant Ended + False",
			endReason:      endReason,
			successEval:    analysis.SuccessEvaluation,
			expectedResult: true, // Should be failure based on user's description
			description:    "Real VAPI call that should be marked as failure",
		},
		{
			name:           "Assistant Ended + Bool False",
			endReason:      "assistant-ended-call",
			successEval:    false,
			expectedResult: true, // Should be failure
			description:    "Boolean false should override successful end reason",
		},
		{
			name:           "Assistant Ended + String False",
			endReason:      "assistant-ended-call",
			successEval:    "false",
			expectedResult: true, // Should be failure
			description:    "String 'false' should override successful end reason",
		},
		{
			name:           "Assistant Ended + Bool True",
			endReason:      "assistant-ended-call",
			successEval:    true,
			expectedResult: false, // Should be success
			description:    "Boolean true with successful end reason should be success",
		},
		{
			name:           "Customer Ended + Bool False",
			endReason:      "customer-ended-call",
			successEval:    false,
			expectedResult: true, // Should be failure
			description:    "Boolean false should override any successful end reason",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testReport := &vapi.EndOfCallReport{
				EndedReason: tc.endReason,
				Analysis: &vapi.Analysis{
					SuccessEvaluation: tc.successEval,
				},
			}

			// Use the same app instance with logger
			result, err := app.evaluateCallSuccess(testReport, tc.endReason)
			if err != nil {
				t.Fatalf("evaluateCallSuccess returned error: %v", err)
			}

			if result != tc.expectedResult {
				t.Errorf("FAILED: %s", tc.description)
				t.Errorf("Expected shouldProcessFailure=%v, got %v", tc.expectedResult, result)
				t.Errorf("End reason: %s, Success evaluation: %v (%T)", tc.endReason, tc.successEval, tc.successEval)
			} else {
				t.Logf("✓ PASSED: %s", tc.description)
			}
		})
	}
}

// TestSuccessEvaluationPrecedence specifically tests the precedence logic
func TestSuccessEvaluationPrecedence(t *testing.T) {
	app := &application{
		logger: testLogger,
	}

	// The key test: SuccessEvaluation should ALWAYS override end reason
	// Even if end reason indicates success, if SuccessEvaluation is false, it should be a failure

	testCases := []struct {
		name        string
		endReason   string
		successEval interface{}
		shouldFail  bool
		description string
	}{
		// Cases where SuccessEvaluation should override successful end reasons
		{
			name:        "Override Assistant Ended with Bool False",
			endReason:   "assistant-ended-call",
			successEval: false,
			shouldFail:  true,
			description: "AI determined call failed despite assistant ending it",
		},
		{
			name:        "Override Customer Ended with Bool False", 
			endReason:   "customer-ended-call",
			successEval: false,
			shouldFail:  true,
			description: "AI determined call failed despite customer ending it",
		},
		{
			name:        "Override Max Duration with String False",
			endReason:   "exceeded-max-duration",
			successEval: "false",
			shouldFail:  true,
			description: "AI determined call failed despite reaching max duration",
		},
		{
			name:        "Override Assistant Ended with Failure String",
			endReason:   "assistant-ended-call",
			successEval: "The call did not achieve its objective",
			shouldFail:  true,
			description: "AI analysis indicates failure despite technical success",
		},

		// Cases where both agree on success
		{
			name:        "Both Indicate Success - Bool True",
			endReason:   "assistant-ended-call",
			successEval: true,
			shouldFail:  false,
			description: "Both end reason and AI agree on success",
		},
		{
			name:        "Both Indicate Success - String True",
			endReason:   "customer-ended-call",
			successEval: "true",
			shouldFail:  false,
			description: "Both end reason and AI agree on success",
		},
		{
			name:        "Both Indicate Success - Success String",
			endReason:   "assistant-ended-call",
			successEval: "The call was successful and the candidate was engaged",
			shouldFail:  false,
			description: "Both end reason and AI agree on success",
		},

		// Cases where both agree on failure
		{
			name:        "Both Indicate Failure - Voicemail",
			endReason:   "voicemail", 
			successEval: false,
			shouldFail:  true,
			description: "Both end reason and AI agree on failure",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			report := &vapi.EndOfCallReport{
				EndedReason: tc.endReason,
				Analysis: &vapi.Analysis{
					SuccessEvaluation: tc.successEval,
				},
			}

			shouldProcessFailure, err := app.evaluateCallSuccess(report, tc.endReason)
			if err != nil {
				t.Fatalf("evaluateCallSuccess returned error: %v", err)
			}

			if shouldProcessFailure != tc.shouldFail {
				t.Errorf("PRECEDENCE FAILURE: %s", tc.description)
				t.Errorf("Expected shouldProcessFailure=%v, got %v", tc.shouldFail, shouldProcessFailure)
				t.Errorf("End reason: %s, Success evaluation: %v (%T)", tc.endReason, tc.successEval, tc.successEval)
				
				if tc.shouldFail {
					t.Errorf("This call should result in FAILED appointment status")
				} else {
					t.Errorf("This call should result in COMPLETED appointment status")
				}
			} else {
				t.Logf("✓ CORRECT: %s", tc.description)
			}
		})
	}
}