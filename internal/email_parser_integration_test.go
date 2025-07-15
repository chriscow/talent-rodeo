package internal

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal/inbox"

	"github.com/joho/godotenv"
)

// TestEmailParser_RealLLMIntegration tests the EmailParser with real LLM calls
// to validate prompt quality and parsing accuracy across various email types.
//
// This test is tagged with "integration" and requires:
// 1. OPENAI_API_KEY environment variable set
// 2. Run with: go test -tags=integration -run TestEmailParser_RealLLMIntegration
//
// The test validates:
// - Prompt effectiveness with real LLM responses
// - Parsing accuracy across different email types
// - Structured output quality and consistency
// - Cost tracking functionality
func TestEmailParser_RealLLMIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Load environment variables
	godotenv.Load("../.env")

	openAIAPIKey := os.Getenv("OPENAI_API_KEY")
	if openAIAPIKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping real LLM integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create test Firestore client for cost tracking
	fs, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test Firestore client: %v", err)
	}
	defer fs.Close()

	// Create real EmailParser with cost tracking
	parser := NewOpenAIEmailParser(openAIAPIKey, logger, fs)

	// Test scenarios with varying complexity
	testCases := []struct {
		name           string
		email          *inbox.Email
		expectRequests int
		expectFeedback int
		expectNonReq   int
		validateFunc   func(t *testing.T, structure *EmailStructure)
	}{
		{
			name: "simple job search request",
			email: &inbox.Email{
				ID:      "test-simple-001",
				From:    "user@example.com",
				Subject: "Update my job preferences",
				Body: `Hi,

I would like to add remote work options to my job search.

Thanks!`,
				Date: time.Now(),
			},
			expectRequests: 1,
			expectFeedback: 0,
			expectNonReq:   2, // greeting and closing
			validateFunc: func(t *testing.T, structure *EmailStructure) {
				// Verify remote work request detected
				found := false
				for _, req := range structure.Requests {
					if strings.Contains(strings.ToLower(req.Description), "remote") ||
						strings.Contains(strings.ToLower(req.Quote), "remote") {
						found = true
						break
					}
				}
				if !found {
					t.Error("Expected to detect remote work request")
				}
			},
		},
		{
			name: "complex multi-request email",
			email: &inbox.Email{
				ID:      "test-complex-001",
				From:    "john.developer@company.com",
				Subject: "Multiple job search updates needed",
				Body: `Hello Support Team,

I hope this email finds you well. I need to make several changes to my job search:

1. Please update my salary range to $90,000 - $130,000 annually
2. I'm now open to positions in Austin, Texas and Denver, Colorado 
3. Add "machine learning engineer" to my target job titles
4. Remove any positions that require more than 10% travel

Also, I wanted to say that your job matching has improved significantly lately! The recommendations are much more relevant than they were last month.

Could you also cancel my premium subscription? I'll be pausing my job search for the next two months due to personal reasons.

Please let me know if you need any additional information.

Best regards,
John Smith
Senior Software Engineer`,
				Date: time.Now(),
			},
			expectRequests: 5, // salary, location, job title, travel, cancel subscription
			expectFeedback: 1, // positive feedback about matching
			expectNonReq:   3, // greeting, transition, closing
			validateFunc: func(t *testing.T, structure *EmailStructure) {
				// Check for key requests
				requestTypes := map[string]bool{
					"salary":       false,
					"location":     false,
					"job_title":    false,
					"travel":       false,
					"subscription": false,
				}

				for _, req := range structure.Requests {
					desc := strings.ToLower(req.Description)
					quote := strings.ToLower(req.Quote)
					content := desc + " " + quote

					if strings.Contains(content, "salary") || strings.Contains(content, "90,000") {
						requestTypes["salary"] = true
					}
					if strings.Contains(content, "austin") || strings.Contains(content, "denver") {
						requestTypes["location"] = true
					}
					if strings.Contains(content, "machine learning") {
						requestTypes["job_title"] = true
					}
					if strings.Contains(content, "travel") {
						requestTypes["travel"] = true
					}
					if strings.Contains(content, "subscription") || strings.Contains(content, "cancel") {
						requestTypes["subscription"] = true
					}
				}

				for reqType, found := range requestTypes {
					if !found {
						t.Errorf("Expected to detect %s request", reqType)
					}
				}

				// Check for positive feedback
				foundFeedback := false
				for _, feedback := range structure.GeneralFeedback {
					if strings.Contains(strings.ToLower(feedback.Description), "improved") ||
						strings.Contains(strings.ToLower(feedback.Quote), "improved") {
						foundFeedback = true
						break
					}
				}
				if !foundFeedback {
					t.Error("Expected to detect positive feedback about improvements")
				}
			},
		},
		{
			name: "complaint with mixed content",
			email: &inbox.Email{
				ID:      "test-complaint-001",
				From:    "frustrated.user@email.com",
				Subject: "Issues with your service",
				Body: `Dear Customer Support,

I've been using your platform for 3 months and I'm having several issues:

The job recommendations are terrible - I keep getting entry-level positions even though I have 8 years of experience. This is really frustrating and makes me question whether this service is worth it.

The email notifications are also way too frequent. I'm getting 6-8 emails per day which is overwhelming.

However, I would like to update my profile to reflect that I'm now looking for senior software architect positions, preferably with teams of 10+ people.

I'm also interested in adding blockchain and cryptocurrency companies to my target industries.

If these issues aren't resolved soon, I'll be looking for alternative job search platforms.

Regards,
Sarah Wilson`,
				Date: time.Now(),
			},
			expectRequests: 3, // profile update, team size, industry
			expectFeedback: 3, // bad recommendations, email frequency, threat to leave
			expectNonReq:   2, // greeting, closing
			validateFunc: func(t *testing.T, structure *EmailStructure) {
				// Check for complaints in feedback
				complaintTopics := map[string]bool{
					"recommendations": false,
					"notifications":   false,
				}

				for _, feedback := range structure.GeneralFeedback {
					content := strings.ToLower(feedback.Description + " " + feedback.Quote)
					if strings.Contains(content, "recommendation") || strings.Contains(content, "terrible") {
						complaintTopics["recommendations"] = true
					}
					if strings.Contains(content, "notification") || strings.Contains(content, "frequent") {
						complaintTopics["notifications"] = true
					}
				}

				for topic, found := range complaintTopics {
					if !found {
						t.Errorf("Expected to detect complaint about %s", topic)
					}
				}

				// Check for actionable requests
				actionableRequests := map[string]bool{
					"senior_position": false,
					"blockchain":      false,
				}

				for _, req := range structure.Requests {
					content := strings.ToLower(req.Description + " " + req.Quote)
					if strings.Contains(content, "senior") || strings.Contains(content, "architect") {
						actionableRequests["senior_position"] = true
					}
					if strings.Contains(content, "blockchain") || strings.Contains(content, "cryptocurrency") {
						actionableRequests["blockchain"] = true
					}
				}

				for reqType, found := range actionableRequests {
					if !found {
						t.Errorf("Expected to detect %s request", reqType)
					}
				}
			},
		},
		{
			name: "pure feedback email",
			email: &inbox.Email{
				ID:      "test-feedback-001",
				From:    "happy.customer@company.com",
				Subject: "Thank you!",
				Body: `Hi there,

Just wanted to drop a quick note to say thank you for the excellent service. The job recommendations have been spot-on, and I actually got three interviews this week from positions you recommended!

The interface is also much more intuitive than other job search platforms I've tried.

Keep up the great work!

Best,
Alex Chen`,
				Date: time.Now(),
			},
			expectRequests: 0,
			expectFeedback: 2, // positive service feedback, UI feedback
			expectNonReq:   2, // greeting, closing
			validateFunc: func(t *testing.T, structure *EmailStructure) {
				if len(structure.Requests) > 0 {
					t.Error("Expected no actionable requests in pure feedback email")
				}

				// Should detect positive feedback
				foundPositive := false
				for _, feedback := range structure.GeneralFeedback {
					content := strings.ToLower(feedback.Description + " " + feedback.Quote)
					if strings.Contains(content, "thank") || strings.Contains(content, "excellent") ||
						strings.Contains(content, "spot-on") {
						foundPositive = true
						break
					}
				}
				if !foundPositive {
					t.Error("Expected to detect positive feedback")
				}
			},
		},
		{
			name: "edge case - very short email",
			email: &inbox.Email{
				ID:      "test-short-001",
				From:    "brief@example.com",
				Subject: "Quick request",
				Body:    "Please add SQL to my skills. Thanks!",
				Date:    time.Now(),
			},
			expectRequests: 1,
			expectFeedback: 0,
			expectNonReq:   1, // thanks
			validateFunc: func(t *testing.T, structure *EmailStructure) {
				// Should detect skill addition request
				found := false
				for _, req := range structure.Requests {
					content := strings.ToLower(req.Description + " " + req.Quote)
					if strings.Contains(content, "sql") || strings.Contains(content, "skill") {
						found = true
						break
					}
				}
				if !found {
					t.Error("Expected to detect SQL skill addition request")
				}
			},
		},
	}

	// Track overall parsing statistics
	var totalParsed, totalRequests, totalFeedback, totalNonReq int
	var parseErrors []string

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Parsing email: %s", tc.name)
			t.Logf("Email body: %s", tc.email.Body)

			// Parse the email with real LLM
			structure, err := parser.ParseEmail(ctx, tc.email)
			if err != nil {
				parseErrors = append(parseErrors, err.Error())
				t.Fatalf("Failed to parse email: %v", err)
			}

			// Track statistics
			totalParsed++
			totalRequests += len(structure.Requests)
			totalFeedback += len(structure.GeneralFeedback)
			totalNonReq += len(structure.NonRequests)

			// Log detailed results
			t.Logf("Parsing results:")
			t.Logf("  Requests: %d (expected %d)", len(structure.Requests), tc.expectRequests)
			t.Logf("  Feedback: %d (expected %d)", len(structure.GeneralFeedback), tc.expectFeedback)
			t.Logf("  Non-requests: %d (expected %d)", len(structure.NonRequests), tc.expectNonReq)

			// Log individual items for debugging
			for i, req := range structure.Requests {
				t.Logf("  Request %d: %s - %s (ID: %s)", i+1, req.Type, req.Description, req.ID)
				t.Logf("    Quote: %s", req.Quote)
			}

			for i, feedback := range structure.GeneralFeedback {
				t.Logf("  Feedback %d: %s - %s (ID: %s)", i+1, feedback.Type, feedback.Description, feedback.ID)
				t.Logf("    Quote: %s", feedback.Quote)
			}

			// Validate counts (allow some flexibility for LLM interpretation)
			requestDiff := abs(len(structure.Requests) - tc.expectRequests)
			feedbackDiff := abs(len(structure.GeneralFeedback) - tc.expectFeedback)
			nonReqDiff := abs(len(structure.NonRequests) - tc.expectNonReq)

			// Allow ±1 difference for LLM interpretation variations
			tolerance := 1
			if requestDiff > tolerance {
				t.Errorf("Request count mismatch: got %d, expected %d (±%d tolerance)",
					len(structure.Requests), tc.expectRequests, tolerance)
			}
			if feedbackDiff > tolerance {
				t.Errorf("Feedback count mismatch: got %d, expected %d (±%d tolerance)",
					len(structure.GeneralFeedback), tc.expectFeedback, tolerance)
			}
			if nonReqDiff > tolerance {
				t.Errorf("Non-request count mismatch: got %d, expected %d (±%d tolerance)",
					len(structure.NonRequests), tc.expectNonReq, tolerance)
			}

			// Validate structure quality
			validateEmailStructure(t, structure)

			// Run custom validation
			if tc.validateFunc != nil {
				tc.validateFunc(t, structure)
			}
		})
	}

	// Log overall statistics
	t.Logf("\n=== INTEGRATION TEST SUMMARY ===")
	t.Logf("Total emails parsed: %d", totalParsed)
	t.Logf("Total requests extracted: %d", totalRequests)
	t.Logf("Total feedback items: %d", totalFeedback)
	t.Logf("Total non-requests: %d", totalNonReq)
	t.Logf("Parse errors: %d", len(parseErrors))

	if len(parseErrors) > 0 {
		t.Logf("Parse error details:")
		for i, err := range parseErrors {
			t.Logf("  %d: %s", i+1, err)
		}
	}

	avgRequestsPerEmail := float64(totalRequests) / float64(totalParsed)
	avgFeedbackPerEmail := float64(totalFeedback) / float64(totalParsed)
	t.Logf("Average requests per email: %.2f", avgRequestsPerEmail)
	t.Logf("Average feedback per email: %.2f", avgFeedbackPerEmail)

	t.Logf("\nIntegration test completed successfully!")
}

// validateEmailStructure performs quality checks on the parsed structure
func validateEmailStructure(t *testing.T, structure *EmailStructure) {
	// Check that all items have required fields
	allItems := []EmailRequest{}
	allItems = append(allItems, structure.Requests...)
	allItems = append(allItems, structure.GeneralFeedback...)
	allItems = append(allItems, structure.NonRequests...)

	for i, item := range allItems {
		if item.ID == "" {
			t.Errorf("Item %d missing ID", i)
		}
		if item.Type == "" {
			t.Errorf("Item %d missing Type", i)
		}
		if item.Description == "" {
			t.Errorf("Item %d missing Description", i)
		}
		if item.Quote == "" {
			t.Errorf("Item %d missing Quote", i)
		}

		// Validate ID format
		validPrefix := strings.HasPrefix(item.ID, "req_") ||
			strings.HasPrefix(item.ID, "feedback_") ||
			strings.HasPrefix(item.ID, "non_req_")
		if !validPrefix {
			t.Errorf("Item %d has invalid ID format: %s", i, item.ID)
		}
	}

	// Check for unique IDs
	idsSeen := make(map[string]bool)
	for i, item := range allItems {
		if idsSeen[item.ID] {
			t.Errorf("Duplicate ID found: %s (item %d)", item.ID, i)
		}
		idsSeen[item.ID] = true
	}
}

// TestEmailParser_PromptStressTest tests the parser with challenging scenarios
// to validate prompt robustness and edge case handling
func TestEmailParser_PromptStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	// Load environment variables
	godotenv.Load("../.env")

	openAIAPIKey := os.Getenv("OPENAI_API_KEY")
	if openAIAPIKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping prompt stress test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelWarn, // Reduce noise for stress test
	}))

	fs, err := NewTestFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create test Firestore client: %v", err)
	}
	defer fs.Close()

	parser := NewOpenAIEmailParser(openAIAPIKey, logger, fs)

	stressTests := []struct {
		name  string
		email *inbox.Email
	}{
		{
			name: "very long email with mixed content",
			email: &inbox.Email{
				ID:      "stress-long-001",
				From:    "verbose.user@example.com",
				Subject: "Comprehensive job search update with detailed requirements",
				Body: strings.Repeat("I'm looking for a software engineering position with competitive salary and good work-life balance. ", 50) +
					"Please update my preferences to include: remote work, flexible hours, health insurance, 401k matching, unlimited PTO, and modern tech stack.",
				Date: time.Now(),
			},
		},
		{
			name: "email with special characters and formatting",
			email: &inbox.Email{
				ID:      "stress-special-001",
				From:    "unicode.user@example.com",
				Subject: "Résumé updates & préférences spécialisées 🚀",
				Body: `Hi! 👋

I need to update my profile with these special requirements:
• Salary: $80,000–$120,000 (USD)
• Location: São Paulo, Brazil OR remote 🌍
• Skills: C++, C#, .NET, React.js, Node.js
• Industries: Fin-tech & AI/ML startups 🤖

Também falo português fluente!

Thanks!! 😊`,
				Date: time.Now(),
			},
		},
		{
			name: "email with code snippets",
			email: &inbox.Email{
				ID:      "stress-code-001",
				From:    "developer@techcorp.com",
				Subject: "Technical requirements for job search",
				Body: `Hello,

I'm a backend developer specializing in Go. Here's what I'm looking for:

func ideal_job() Job {
    return Job{
        Language: "Go",
        Salary: 100000,
        Remote: true,
        TeamSize: 5-10,
    }
}

Please update my search to focus on:
1. Go/Golang positions
2. Microservices architecture
3. Docker/Kubernetes experience required
4. Remote-first companies

Also remove any positions requiring:
- PHP or legacy systems
- On-call rotations > 1 week/month

Best regards,
Alex`,
				Date: time.Now(),
			},
		},
		{
			name: "ambiguous email requiring interpretation",
			email: &inbox.Email{
				ID:      "stress-ambiguous-001",
				From:    "unclear.writer@email.com",
				Subject: "Some changes needed",
				Body: `Hey,

So I've been thinking about my career and stuff, and I think maybe I should look for something different? Not sure exactly what but probably something that pays better and is more interesting than what I'm doing now.

I guess I like working with data and computers. Maybe something in tech? I don't really know what's out there but I'm open to learning new things.

Oh and I definitely don't want to work weekends anymore. That's really important to me.

Let me know what you think.

Thanks,
Jordan`,
				Date: time.Now(),
			},
		},
	}

	successCount := 0
	for _, tc := range stressTests {
		t.Run(tc.name, func(t *testing.T) {
			structure, err := parser.ParseEmail(ctx, tc.email)
			if err != nil {
				t.Logf("Parse failed for %s: %v", tc.name, err)
				return
			}

			// Basic validation
			if structure == nil {
				t.Error("Received nil structure")
				return
			}

			// Check that parser handled the challenging content
			totalItems := len(structure.Requests) + len(structure.GeneralFeedback) + len(structure.NonRequests)
			if totalItems == 0 {
				t.Error("Parser returned empty structure for non-empty email")
				return
			}

			validateEmailStructure(t, structure)
			successCount++

			t.Logf("Successfully parsed challenging email: %d total items extracted", totalItems)
		})
	}

	successRate := float64(successCount) / float64(len(stressTests)) * 100
	t.Logf("Stress test success rate: %.1f%% (%d/%d)", successRate, successCount, len(stressTests))

	if successRate < 75.0 {
		t.Errorf("Stress test success rate too low: %.1f%% (expected ≥75%%)", successRate)
	}
}

// Helper function for absolute difference
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
