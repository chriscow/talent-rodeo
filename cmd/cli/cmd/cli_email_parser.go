package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/inbox"
	"time"

	"github.com/spf13/cobra"
)

var emailParserCmd = &cobra.Command{
	Use:   "email-parser",
	Short: "Test and validate email parsing functionality",
	Long: `Test and validate the LLM-based email parsing system.

The email parser analyzes incoming emails and extracts structured information
including actionable requests, feedback, and non-actionable content. This helps
provide granular tracking of handled vs unhandled requests within emails.

This command group allows you to:
- Test email parsing with real LLM calls
- Validate prompt effectiveness
- Analyze parsing accuracy
- Debug parsing issues with specific emails`,
}

var testEmailParserCmd = &cobra.Command{
	Use:   "test [email-content]",
	Short: "Test email parsing with real LLM calls",
	Long: `Test the email parser with a sample email or provided content.

This command uses real LLM API calls to parse email content and displays
the structured output including requests, feedback, and non-actionable items.

Examples:
  trcli email-parser test                                    # Test with sample email
  trcli email-parser test "Please add remote work to my preferences"  # Test specific content
  trcli email-parser test --file email.txt                  # Test from file
  trcli email-parser test --interactive                     # Interactive testing mode`,
	RunE: testEmailParser,
}

var validateParserCmd = &cobra.Command{
	Use:   "validate",
	Short: "Run comprehensive validation tests on email parser",
	Long: `Run a suite of validation tests to ensure email parser quality.

This command runs predefined test scenarios to validate:
- Parsing accuracy across different email types
- Prompt effectiveness with various content
- Structure quality and consistency
- Edge case handling

This uses real LLM API calls and may incur costs.`,
	RunE: validateEmailParser,
}

var batchTestCmd = &cobra.Command{
	Use:   "batch [directory]",
	Short: "Run batch tests on a directory of email files",
	Long: `Run email parser tests on all .txt files in a directory.

This command processes all .txt files in the specified directory (or current
directory if not specified) and generates a comprehensive report showing
parsing results for each email.

Examples:
  trcli email-parser batch                                # Test files in current directory
  trcli email-parser batch ./test_emails                 # Test files in specific directory
  trcli email-parser batch ./test_emails --json          # Output detailed JSON results`,
	RunE: batchTestEmailParser,
}

var (
	testFromFile    string
	testInteractive bool
	testJSON        bool
	testVerbose     bool
)

func init() {
	RootCmd.AddCommand(emailParserCmd)
	emailParserCmd.AddCommand(testEmailParserCmd)
	emailParserCmd.AddCommand(validateParserCmd)
	emailParserCmd.AddCommand(batchTestCmd)

	// Test command flags
	testEmailParserCmd.Flags().StringVar(&testFromFile, "file", "", "Read email content from file")
	testEmailParserCmd.Flags().BoolVar(&testInteractive, "interactive", false, "Interactive testing mode")
	testEmailParserCmd.Flags().BoolVar(&testJSON, "json", false, "Output results in JSON format")
	testEmailParserCmd.Flags().BoolVar(&testVerbose, "verbose", false, "Verbose output with detailed logging")

	// Validate command flags
	validateParserCmd.Flags().BoolVar(&testJSON, "json", false, "Output results in JSON format")
	validateParserCmd.Flags().BoolVar(&testVerbose, "verbose", false, "Verbose output with detailed logging")

	// Batch command flags
	batchTestCmd.Flags().BoolVar(&testJSON, "json", false, "Output results in JSON format")
	batchTestCmd.Flags().BoolVar(&testVerbose, "verbose", false, "Verbose output with detailed logging")
}

func testEmailParser(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Set environment variable to suppress debug output when using JSON
	if testJSON {
		os.Setenv("CLI_JSON_MODE", "1")
	}

	// Setup logging
	logLevel := slog.LevelInfo
	if testVerbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Check for OpenAI API key
	openAIAPIKey := os.Getenv("OPENAI_API_KEY")
	if openAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}

	// Create Firestore client for cost tracking
	fs, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer fs.Close()

	// Create email parser
	parser := internal.NewOpenAIEmailParser(logger, fs)

	// Get email content
	var emailContent string
	var emailSubject string

	if testFromFile != "" {
		// Read from file
		content, err := os.ReadFile(testFromFile)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", testFromFile, err)
		}
		emailContent = string(content)
		emailSubject = "Email from file: " + testFromFile
	} else if testInteractive {
		// Interactive mode
		return runInteractiveEmailParser(ctx, parser)
	} else if len(args) > 0 {
		// Use provided content
		emailContent = strings.Join(args, " ")
		emailSubject = "Test email"
	} else {
		// Use sample email
		emailContent = getSampleEmailContent()
		emailSubject = "Sample job search update"
	}

	// Create test email
	email := &inbox.Email{
		ID:      "cli-test-" + fmt.Sprintf("%d", time.Now().Unix()),
		From:    "test.user@example.com",
		Subject: emailSubject,
		Body:    emailContent,
		Date:    time.Now(),
	}

	// Parse the email
	if !testJSON {
		fmt.Printf("Parsing email content:\n")
		fmt.Printf("Subject: %s\n", email.Subject)
		fmt.Printf("Body: %s\n\n", email.Body)
	}

	structure, err := parser.ParseEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("failed to parse email: %w", err)
	}

	// Output results
	if testJSON {
		return outputEmailStructureJSON(structure)
	}
	return outputEmailStructureHuman(structure)
}

func validateEmailParser(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup logging
	logLevel := slog.LevelWarn
	if testVerbose {
		logLevel = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Check for OpenAI API key
	openAIAPIKey := os.Getenv("OPENAI_API_KEY")
	if openAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}

	// Create Firestore client
	fs, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer fs.Close()

	// Create email parser
	parser := internal.NewOpenAIEmailParser(logger, fs)

	// Run validation test suite
	testCases := getValidationTestCases()

	fmt.Printf("Running email parser validation suite...\n")
	fmt.Printf("Test cases: %d\n", len(testCases))
	fmt.Printf("This will make real LLM API calls and may incur costs.\n\n")

	var results []ValidationResult
	successCount := 0

	for i, tc := range testCases {
		fmt.Printf("[%d/%d] Testing: %s\n", i+1, len(testCases), tc.Name)

		result := ValidationResult{
			Name:      tc.Name,
			Success:   false,
			StartTime: time.Now(),
		}

		structure, err := parser.ParseEmail(ctx, tc.Email)
		result.Duration = time.Since(result.StartTime)

		if err != nil {
			result.Error = err.Error()
			fmt.Printf("  ❌ FAILED: %v\n", err)
		} else {
			// Validate results
			if validateTestCase(tc, structure) {
				result.Success = true
				result.Structure = structure
				successCount++
				fmt.Printf("  ✅ PASSED\n")
			} else {
				result.Error = "validation failed - unexpected parsing results"
				fmt.Printf("  ❌ FAILED: validation failed\n")
			}
		}

		results = append(results, result)
	}

	// Output summary
	successRate := float64(successCount) / float64(len(testCases)) * 100

	fmt.Printf("\n=== VALIDATION RESULTS ===\n")
	fmt.Printf("Total tests: %d\n", len(testCases))
	fmt.Printf("Passed: %d\n", successCount)
	fmt.Printf("Failed: %d\n", len(testCases)-successCount)
	fmt.Printf("Success rate: %.1f%%\n", successRate)

	if testJSON {
		return outputValidationResultsJSON(results)
	}

	// Show failed tests
	if successCount < len(testCases) {
		fmt.Printf("\nFailed tests:\n")
		for _, result := range results {
			if !result.Success {
				fmt.Printf("  - %s: %s\n", result.Name, result.Error)
			}
		}
	}

	if successRate < 80.0 {
		return fmt.Errorf("validation success rate too low: %.1f%% (expected ≥80%%)", successRate)
	}

	fmt.Printf("\nValidation completed successfully! 🎉\n")
	return nil
}

func runInteractiveEmailParser(ctx context.Context, parser internal.EmailParser) error {
	fmt.Printf("Interactive Email Parser Testing\n")
	fmt.Printf("Enter email content (press Ctrl+D when finished):\n\n")

	// Read email content from stdin
	var lines []string
	fmt.Printf("Subject: ")
	var subject string
	fmt.Scanln(&subject)

	fmt.Printf("Body (enter empty line to finish):\n")
	for {
		var line string
		if _, err := fmt.Scanln(&line); err != nil {
			break
		}
		if line == "" {
			break
		}
		lines = append(lines, line)
	}

	emailContent := strings.Join(lines, "\n")
	if emailContent == "" {
		return fmt.Errorf("no email content provided")
	}

	// Create and parse email
	email := &inbox.Email{
		ID:      "interactive-test",
		From:    "interactive.user@example.com",
		Subject: subject,
		Body:    emailContent,
		Date:    time.Now(),
	}

	structure, err := parser.ParseEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("failed to parse email: %w", err)
	}

	return outputEmailStructureHuman(structure)
}

func outputEmailStructureHuman(structure *internal.EmailStructure) error {
	fmt.Printf("=== PARSING RESULTS ===\n\n")

	fmt.Printf("📋 REQUESTS (%d):\n", len(structure.Requests))
	for i, req := range structure.Requests {
		fmt.Printf("  %d. [%s] %s\n", i+1, req.Type, req.Description)
		fmt.Printf("     Quote: \"%s\"\n", req.Quote)
		fmt.Printf("     ID: %s\n\n", req.ID)
	}

	fmt.Printf("💬 FEEDBACK (%d):\n", len(structure.GeneralFeedback))
	for i, feedback := range structure.GeneralFeedback {
		fmt.Printf("  %d. [%s] %s\n", i+1, feedback.Type, feedback.Description)
		fmt.Printf("     Quote: \"%s\"\n", feedback.Quote)
		fmt.Printf("     ID: %s\n\n", feedback.ID)
	}

	fmt.Printf("📝 NON-REQUESTS (%d):\n", len(structure.NonRequests))
	for i, nonReq := range structure.NonRequests {
		fmt.Printf("  %d. [%s] %s\n", i+1, nonReq.Type, nonReq.Description)
		fmt.Printf("     Quote: \"%s\"\n", nonReq.Quote)
		fmt.Printf("     ID: %s\n\n", nonReq.ID)
	}

	totalItems := len(structure.Requests) + len(structure.GeneralFeedback) + len(structure.NonRequests)
	fmt.Printf("Total items extracted: %d\n", totalItems)

	return nil
}

func outputEmailStructureJSON(structure *internal.EmailStructure) error {
	result := map[string]interface{}{
		"requests":         structure.Requests,
		"general_feedback": structure.GeneralFeedback,
		"non_requests":     structure.NonRequests,
		"summary": map[string]int{
			"requests":         len(structure.Requests),
			"general_feedback": len(structure.GeneralFeedback),
			"non_requests":     len(structure.NonRequests),
			"total":            len(structure.Requests) + len(structure.GeneralFeedback) + len(structure.NonRequests),
		},
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func outputValidationResultsJSON(results []ValidationResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(results)
}

func getSampleEmailContent() string {
	return `Hi there,

I hope this email finds you well. I wanted to reach out about making some updates to my job search preferences.

First, I'd like to add remote work options to my search criteria. The commute to downtown has become quite challenging, and I think I'd be more productive working from home.

Also, could you please update my salary range? I'm now looking for positions in the $85,000 to $115,000 range based on my increased experience.

One more thing - I'm interested in adding "product manager" roles to my target positions, in addition to the software engineering roles I'm already searching for.

By the way, I wanted to mention that the job recommendations have been excellent lately! I've had three interviews this month from positions you recommended.

Thanks for all your help!

Best regards,
Alex Johnson`
}

// ValidationTestCase represents a test case for email parser validation
type ValidationTestCase struct {
	Name           string
	Email          *inbox.Email
	ExpectedCounts ExpectedCounts
}

// ExpectedCounts represents expected parsing results
type ExpectedCounts struct {
	Requests  int
	Feedback  int
	NonReq    int
	Tolerance int // Allow ±tolerance difference
}

// ValidationResult represents the result of a validation test
type ValidationResult struct {
	Name      string                   `json:"name"`
	Success   bool                     `json:"success"`
	Error     string                   `json:"error,omitempty"`
	StartTime time.Time                `json:"start_time"`
	Duration  time.Duration            `json:"duration"`
	Structure *internal.EmailStructure `json:"structure,omitempty"`
}

func getValidationTestCases() []ValidationTestCase {
	return []ValidationTestCase{
		{
			Name: "Simple Request",
			Email: &inbox.Email{
				ID:      "val-001",
				From:    "user@example.com",
				Subject: "Add remote work",
				Body:    "Hi, please add remote work options to my job search. Thanks!",
				Date:    time.Now(),
			},
			ExpectedCounts: ExpectedCounts{
				Requests:  1,
				Feedback:  0,
				NonReq:    2,
				Tolerance: 1,
			},
		},
		{
			Name: "Mixed Content",
			Email: &inbox.Email{
				ID:      "val-002",
				From:    "user@example.com",
				Subject: "Updates and feedback",
				Body: `Hello,
				
I need to update my salary range to $80k-$120k and add Austin, TX to my locations.

Also, your recent job recommendations have been much better! Keep it up.

Thanks,
User`,
				Date: time.Now(),
			},
			ExpectedCounts: ExpectedCounts{
				Requests:  2,
				Feedback:  1,
				NonReq:    2,
				Tolerance: 1,
			},
		},
		{
			Name: "Pure Feedback",
			Email: &inbox.Email{
				ID:      "val-003",
				From:    "user@example.com",
				Subject: "Great service!",
				Body: `Hi,
				
Just wanted to say thank you for the excellent job matching service. I found my dream job through your platform!

Best regards,
Happy User`,
				Date: time.Now(),
			},
			ExpectedCounts: ExpectedCounts{
				Requests:  0,
				Feedback:  1,
				NonReq:    2,
				Tolerance: 1,
			},
		},
	}
}

func validateTestCase(tc ValidationTestCase, structure *internal.EmailStructure) bool {
	tolerance := tc.ExpectedCounts.Tolerance

	requestDiff := abs(len(structure.Requests) - tc.ExpectedCounts.Requests)
	feedbackDiff := abs(len(structure.GeneralFeedback) - tc.ExpectedCounts.Feedback)
	nonReqDiff := abs(len(structure.NonRequests) - tc.ExpectedCounts.NonReq)

	return requestDiff <= tolerance && feedbackDiff <= tolerance && nonReqDiff <= tolerance
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func batchTestEmailParser(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Setup logging
	logLevel := slog.LevelWarn
	if testVerbose {
		logLevel = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Check for OpenAI API key
	openAIAPIKey := os.Getenv("OPENAI_API_KEY")
	if openAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}

	// Create Firestore client
	fs, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer fs.Close()

	// Create email parser
	parser := internal.NewOpenAIEmailParser(logger, fs)

	// Determine directory
	directory := "."
	if len(args) > 0 {
		directory = args[0]
	}

	// Find all .txt files
	pattern := fmt.Sprintf("%s/*.txt", directory)
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no .txt files found in directory: %s", directory)
	}

	// Sort files for consistent ordering
	sort.Strings(files)

	fmt.Printf("📧 Found %d email files in %s\n", len(files), directory)
	fmt.Printf("🧪 Starting batch email parser tests...\n\n")

	var results []BatchTestResult
	successCount := 0

	for i, file := range files {
		filename := filepath.Base(file)
		fmt.Printf("[%d/%d] Testing: %s\n", i+1, len(files), filename)

		result := BatchTestResult{
			Filename:  filename,
			FilePath:  file,
			Success:   false,
			StartTime: time.Now(),
		}

		// Read email content
		content, err := os.ReadFile(file)
		if err != nil {
			result.Error = fmt.Sprintf("Failed to read file: %v", err)
			fmt.Printf("  ❌ FAILED: %s\n", result.Error)
			results = append(results, result)
			continue
		}

		// Extract subject from first line if it starts with "Subject:"
		lines := strings.Split(string(content), "\n")
		subject := "Test email"
		body := string(content)

		if len(lines) > 0 && strings.HasPrefix(lines[0], "Subject:") {
			subject = strings.TrimPrefix(lines[0], "Subject:")
			subject = strings.TrimSpace(subject)
			if len(lines) > 1 {
				body = strings.Join(lines[1:], "\n")
				body = strings.TrimSpace(body)
			}
		}

		// Create test email
		email := &inbox.Email{
			ID:      fmt.Sprintf("batch-test-%d", i),
			From:    "batch.test@example.com",
			Subject: subject,
			Body:    body,
			Date:    time.Now(),
		}

		// Parse the email
		structure, err := parser.ParseEmail(ctx, email)
		result.Duration = time.Since(result.StartTime)

		if err != nil {
			result.Error = err.Error()
			fmt.Printf("  ❌ FAILED: %v\n", err)
		} else {
			result.Success = true
			result.Structure = structure
			successCount++

			requests := len(structure.Requests)
			feedback := len(structure.GeneralFeedback)
			nonReq := len(structure.NonRequests)
			total := requests + feedback + nonReq

			result.Summary = BatchTestSummary{
				Requests:    requests,
				Feedback:    feedback,
				NonRequests: nonReq,
				Total:       total,
			}

			fmt.Printf("  ✅ PASSED: %d requests, %d feedback, %d non-requests (%d total)\n",
				requests, feedback, nonReq, total)
		}

		results = append(results, result)
	}

	// Output summary
	successRate := float64(successCount) / float64(len(files)) * 100

	fmt.Printf("\n📊 BATCH TEST SUMMARY\n")
	fmt.Printf("====================\n")
	fmt.Printf("Total files: %d\n", len(files))
	fmt.Printf("Passed: %d\n", successCount)
	fmt.Printf("Failed: %d\n", len(files)-successCount)
	fmt.Printf("Success rate: %.1f%%\n", successRate)

	if testJSON {
		return outputBatchResultsJSON(results)
	}

	// Show failed tests if any
	if successCount < len(files) {
		fmt.Printf("\nFailed tests:\n")
		for _, result := range results {
			if !result.Success {
				fmt.Printf("  - %s: %s\n", result.Filename, result.Error)
			}
		}
	}

	// Show top performers
	fmt.Printf("\nTop results by total items extracted:\n")
	sort.Slice(results, func(i, j int) bool {
		return results[i].Summary.Total > results[j].Summary.Total
	})

	for i, result := range results {
		if i >= 5 || !result.Success { // Show top 5 successful results
			break
		}
		fmt.Printf("  %d. %s: %d items (%d req, %d feedback, %d non-req)\n",
			i+1, result.Filename, result.Summary.Total,
			result.Summary.Requests, result.Summary.Feedback, result.Summary.NonRequests)
	}

	if successRate < 70.0 {
		return fmt.Errorf("batch test success rate too low: %.1f%% (expected ≥70%%)", successRate)
	}

	fmt.Printf("\n🎉 Batch testing completed successfully!\n")
	return nil
}

// BatchTestResult represents the result of testing a single email file
type BatchTestResult struct {
	Filename  string                   `json:"filename"`
	FilePath  string                   `json:"file_path"`
	Success   bool                     `json:"success"`
	Error     string                   `json:"error,omitempty"`
	StartTime time.Time                `json:"start_time"`
	Duration  time.Duration            `json:"duration"`
	Structure *internal.EmailStructure `json:"structure,omitempty"`
	Summary   BatchTestSummary         `json:"summary"`
}

// BatchTestSummary provides a summary of parsing results
type BatchTestSummary struct {
	Requests    int `json:"requests"`
	Feedback    int `json:"feedback"`
	NonRequests int `json:"non_requests"`
	Total       int `json:"total"`
}

func outputBatchResultsJSON(results []BatchTestResult) error {
	output := map[string]interface{}{
		"batch_test_results": results,
		"summary":            calculateBatchSummary(results),
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func calculateBatchSummary(results []BatchTestResult) map[string]interface{} {
	totalFiles := len(results)
	successCount := 0
	totalRequests := 0
	totalFeedback := 0
	totalNonRequests := 0
	totalItems := 0

	for _, result := range results {
		if result.Success {
			successCount++
			totalRequests += result.Summary.Requests
			totalFeedback += result.Summary.Feedback
			totalNonRequests += result.Summary.NonRequests
			totalItems += result.Summary.Total
		}
	}

	successRate := float64(successCount) / float64(totalFiles) * 100

	return map[string]interface{}{
		"total_files":        totalFiles,
		"successful_files":   successCount,
		"failed_files":       totalFiles - successCount,
		"success_rate":       successRate,
		"total_requests":     totalRequests,
		"total_feedback":     totalFeedback,
		"total_non_requests": totalNonRequests,
		"total_items":        totalItems,
		"avg_items_per_email": func() float64 {
			if successCount > 0 {
				return float64(totalItems) / float64(successCount)
			}
			return 0
		}(),
	}
}
