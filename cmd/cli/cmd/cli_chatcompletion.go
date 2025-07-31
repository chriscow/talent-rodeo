package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/chat"
	"time"

	"github.com/chriscow/vapi-go"
	"github.com/sashabaranov/go-openai"
	"github.com/spf13/cobra"
)

func init() {
	// Add conversation-completion-test command
	println("Registering conversation-completion-test command")
	RootCmd.AddCommand(conversationTestCmd)

	// Add flags
	conversationTestCmd.Flags().StringP("test-data-dir", "d", "testdata/conversations/", "Directory containing conversation test scenarios")
	conversationTestCmd.Flags().StringP("scenario", "s", "all", "Specific scenario to test, or 'all' for all scenarios")
	conversationTestCmd.Flags().IntP("iterations", "i", 1, "Number of times to test each scenario for reliability")
	conversationTestCmd.Flags().StringP("openai-model", "m", "gpt-4o", "OpenAI model to use for testing")
	conversationTestCmd.Flags().BoolP("json-output", "j", false, "Output results in JSON format")
}

var conversationTestCmd = &cobra.Command{
	Use:   "chat-test",
	Short: "Test LLM conversation completion reliability",
	Long: `Test the LLM's ability to reliably call the complete_chat tool when conversations 
have gathered sufficient information. Uses various conversation scenarios stored as test data.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		testDataDir, _ := cmd.Flags().GetString("test-data-dir")
		scenario, _ := cmd.Flags().GetString("scenario")
		iterations, _ := cmd.Flags().GetInt("iterations")
		model, _ := cmd.Flags().GetString("openai-model")
		jsonOutput, _ := cmd.Flags().GetBool("json-output")

		// Initialize logger
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))

		if verbose {
			logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))
		}

		// Create test runner
		tester := &ConversationCompletionTester{
			logger:      logger,
			testDataDir: testDataDir,
			model:       model,
			jsonOutput:  jsonOutput,
		}

		// Run tests
		results, err := tester.RunTests(context.Background(), scenario, iterations)
		if err != nil {
			return fmt.Errorf("failed to run conversation tests: %w", err)
		}

		// Output results
		if jsonOutput {
			return tester.OutputJSON(results)
		} else {
			return tester.OutputConsole(results)
		}
	},
}

// ConversationTestScenario represents a single test scenario
type ConversationTestScenario struct {
	Name                 string                         `json:"name"`
	Description          string                         `json:"description"`
	ExpectedOutcome      string                         `json:"expected_outcome"`    // "complete", "incomplete", "edge"
	ExpectedToolCalls    []string                       `json:"expected_tool_calls"` // Expected tool names
	ConversationMessages []openai.ChatCompletionMessage `json:"conversation_messages"`
	ChecklistState       map[string]any                 `json:"checklist_state"`
}

// ConversationTestResult represents the result of testing a scenario
type ConversationTestResult struct {
	Scenario        string   `json:"scenario"`
	Expected        string   `json:"expected"`
	Actual          string   `json:"actual"`
	Success         bool     `json:"success"`
	ToolCalls       []string `json:"tool_calls"`
	ExecutionTimeMs int64    `json:"execution_time_ms"`
	Error           string   `json:"error,omitempty"`
	Iteration       int      `json:"iteration"`
}

// ConversationTestSummary represents the overall test results
type ConversationTestSummary struct {
	TestRun struct {
		Timestamp      time.Time `json:"timestamp"`
		Model          string    `json:"model"`
		TotalScenarios int       `json:"total_scenarios"`
		TotalTests     int       `json:"total_tests"`
		SuccessRate    float64   `json:"success_rate"`
	} `json:"test_run"`
	Results []ConversationTestResult `json:"results"`
	Summary struct {
		CompleteScenarios   TestCategoryResult `json:"complete_scenarios"`
		IncompleteScenarios TestCategoryResult `json:"incomplete_scenarios"`
		EdgeCaseScenarios   TestCategoryResult `json:"edge_case_scenarios"`
	} `json:"summary"`
}

type TestCategoryResult struct {
	Total  int     `json:"total"`
	Passed int     `json:"passed"`
	Rate   float64 `json:"rate"`
}

// ConversationCompletionTester manages the testing process
type ConversationCompletionTester struct {
	logger      *slog.Logger
	testDataDir string
	model       string
	jsonOutput  bool
}

// RunTests executes all conversation completion tests
func (t *ConversationCompletionTester) RunTests(ctx context.Context, scenarioFilter string, iterations int) (*ConversationTestSummary, error) {
	t.logger.Info("Starting LLM conversation completion tests",
		"test_data_dir", t.testDataDir,
		"scenario_filter", scenarioFilter,
		"iterations", iterations,
		"model", t.model)

	// Load test scenarios
	scenarios, err := t.loadTestScenarios(scenarioFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to load test scenarios: %w", err)
	}

	t.logger.Info("Loaded test scenarios", "count", len(scenarios))

	// Initialize services
	chatService, cleanup, err := t.initializeChatService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chat service: %w", err)
	}
	defer cleanup()

	// Run tests
	var allResults []ConversationTestResult

	for _, scenario := range scenarios {
		for i := 0; i < iterations; i++ {
			t.logger.Debug("Running test scenario", "scenario", scenario.Name, "iteration", i+1)

			result := t.testScenario(ctx, chatService, scenario, i+1)
			allResults = append(allResults, result)
		}
	}

	// Generate summary
	summary := t.generateSummary(allResults)

	return summary, nil
}

// loadTestScenarios loads test scenarios from JSON files
func (t *ConversationCompletionTester) loadTestScenarios(scenarioFilter string) ([]ConversationTestScenario, error) {
	var scenarios []ConversationTestScenario

	// Check if test data directory exists
	if _, err := os.Stat(t.testDataDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("test data directory does not exist: %s", t.testDataDir)
	}

	// Walk through test data directory
	err := filepath.Walk(t.testDataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process JSON files
		if !strings.HasSuffix(path, ".json") {
			return nil
		}

		// Apply scenario filter
		if scenarioFilter != "all" {
			if !strings.Contains(filepath.Base(path), scenarioFilter) {
				return nil
			}
		}

		// Load scenario
		scenario, err := t.loadScenario(path)
		if err != nil {
			t.logger.Warn("Failed to load scenario", "path", path, "error", err)
			return nil // Continue with other scenarios
		}

		scenarios = append(scenarios, *scenario)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk test data directory: %w", err)
	}

	return scenarios, nil
}

// loadScenario loads a single test scenario from a JSON file
func (t *ConversationCompletionTester) loadScenario(path string) (*ConversationTestScenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read scenario file: %w", err)
	}

	var scenario ConversationTestScenario
	if err := json.Unmarshal(data, &scenario); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scenario: %w", err)
	}

	return &scenario, nil
}

// initializeChatService creates a chat service for testing
func (t *ConversationCompletionTester) initializeChatService(ctx context.Context) (chat.ChatService, func(), error) {
	// Initialize Firestore client
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Firestore client: %w", err)
	}

	// Initialize OpenAI client
	openAIClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	// Create service dependencies
	serviceDeps := &internal.ServiceDeps{
		FS:     fs,
		Logger: t.logger,
		// Add other dependencies as needed
	}

	// Create chat service
	chatService := chat.NewService(fs, openAIClient, serviceDeps, t.logger)

	cleanup := func() {
		fs.Close()
	}

	return chatService, cleanup, nil
}

// testScenario tests a single conversation scenario
func (t *ConversationCompletionTester) testScenario(ctx context.Context, chatService chat.ChatService, scenario ConversationTestScenario, iteration int) ConversationTestResult {
	startTime := time.Now()

	result := ConversationTestResult{
		Scenario:  scenario.Name,
		Expected:  scenario.ExpectedOutcome,
		Iteration: iteration,
	}

	// Create test user and appointment
	testUser, testAppointment, err := t.createTestData(ctx, chatService)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to create test data: %v", err)
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		return result
	}

	// Defer cleanup
	defer func() {
		// TODO: Clean up test data
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
	}()

	// Test the conversation by simulating the final AI response generation
	toolCalls, actualOutcome, err := t.simulateConversationCompletion(ctx, chatService, testUser, testAppointment, scenario)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to simulate conversation: %v", err)
		return result
	}

	result.Actual = actualOutcome
	result.ToolCalls = toolCalls

	// Determine success based on expected vs actual outcome
	result.Success = t.evaluateResult(scenario, result.Actual, result.ToolCalls)

	return result
}

// createTestData creates a test user and appointment for the scenario
func (t *ConversationCompletionTester) createTestData(ctx context.Context, chatService chat.ChatService) (*internal.User, *internal.Appointment, error) {
	// Create a test user
	userID := fmt.Sprintf("test-conversation-%d", time.Now().UnixNano())
	user := &internal.User{
		ID:     userID,
		Name:   "Test User",
		Email:  "test@example.com",
		Status: internal.UserStatusCallStarted,
	}

	// For this test, we don't need to persist the user to the database
	// We'll work with in-memory objects

	// Create a test appointment
	appointment := &internal.Appointment{
		ID:                    fmt.Sprintf("test-appointment-%d", time.Now().UnixNano()),
		UserID:                userID,
		Type:                  internal.AppointmentTypeChat,
		Status:                internal.AppointmentStatusRunning,
		ChatMessages:          []internal.ChatMessage{},
		ConversationChecklist: internal.InitializeConversationChecklist(),
	}

	return user, appointment, nil
}

// simulateConversationCompletion simulates the final AI response generation with conversation context
func (t *ConversationCompletionTester) simulateConversationCompletion(ctx context.Context, chatService chat.ChatService, user *internal.User, appointment *internal.Appointment, scenario ConversationTestScenario) ([]string, string, error) {
	t.logger.Debug("Simulating conversation completion with real OpenAI integration", "scenario", scenario.Name)

	// Create OpenAI client
	openAIClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	// Build assistant overrides for the conversation context
	assistantOverrides, err := internal.BuildAssistantOverrides(ctx, user, nil, t.logger)
	if err != nil {
		t.logger.Warn("Failed to build assistant overrides, using defaults", "error", err)
	}

	// Build conversation messages with checklist context
	messages := t.buildConversationMessages(scenario, assistantOverrides)

	// Get all available tools
	toolRegistry := internal.NewLLMToolRegistry("chat", t.logger)
	t.registerTestTools(toolRegistry)
	tools := toolRegistry.GetAllTools()

	// Create OpenAI request with tools
	req := openai.ChatCompletionRequest{
		Model:      t.model,
		Messages:   messages,
		Tools:      tools,
		ToolChoice: "auto",
		Stream:     false, // Use non-streaming for easier testing
	}

	t.logger.Debug("Sending request to OpenAI", "model", t.model, "message_count", len(messages), "tool_count", len(tools))

	// Send request to OpenAI
	resp, err := openAIClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create chat completion: %w", err)
	}

	// Analyze the response for tool calls
	var toolCalls []string
	var outcome string

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]

		// Check if any tools were called
		if len(choice.Message.ToolCalls) > 0 {
			for _, toolCall := range choice.Message.ToolCalls {
				toolCalls = append(toolCalls, toolCall.Function.Name)
			}

			// Determine outcome based on tool calls
			hasCompleteChat := false
			hasOverrideCompletion := false

			for _, toolName := range toolCalls {
				if toolName == "complete_chat" {
					hasCompleteChat = true
				}
				if toolName == "override_completion" {
					hasOverrideCompletion = true
				}
			}

			if hasCompleteChat || hasOverrideCompletion {
				outcome = "complete"
			} else {
				outcome = "incomplete"
			}
		} else {
			// No tool calls made - continuing conversation
			outcome = "incomplete"
		}

		t.logger.Debug("OpenAI response analyzed",
			"tool_calls", toolCalls,
			"outcome", outcome,
			"content_length", len(choice.Message.Content))
	}

	return toolCalls, outcome, nil
}

// buildConversationMessages builds the OpenAI messages from the test scenario
func (t *ConversationCompletionTester) buildConversationMessages(scenario ConversationTestScenario, assistantOverrides *vapi.Assistant) []openai.ChatCompletionMessage {
	var messages []openai.ChatCompletionMessage

	// Use the scenario's conversation messages as the base
	for _, msg := range scenario.ConversationMessages {
		messages = append(messages, msg)
	}

	// If we have checklist state information, update the system message to include it
	if len(scenario.ChecklistState) > 0 {
		// Find the system message and enhance it with checklist status
		for i, msg := range messages {
			if msg.Role == openai.ChatMessageRoleSystem {
				// Add checklist information to system prompt
				checklistInfo := t.buildChecklistInfo(scenario.ChecklistState)
				messages[i].Content = msg.Content + "\n\n" + checklistInfo
				break
			}
		}
	}

	return messages
}

// buildChecklistInfo creates checklist status information for the system prompt
func (t *ConversationCompletionTester) buildChecklistInfo(checklistState map[string]any) string {
	// Extract key information from checklist state
	status := "in_progress"
	if s, ok := checklistState["overall_status"].(string); ok {
		status = s
	}

	messageCount := 0
	if mc, ok := checklistState["message_count"].(float64); ok {
		messageCount = int(mc)
	}

	completedTopics := []string{}
	if ct, ok := checklistState["completed_topics"].([]interface{}); ok {
		for _, topic := range ct {
			if str, ok := topic.(string); ok {
				completedTopics = append(completedTopics, str)
			}
		}
	}

	return fmt.Sprintf(`
CURRENT CHECKLIST STATUS:
- Overall Status: %s
- Message Count: %d/100
- Completed Topics: %s
- Ready for Completion: %v

INSTRUCTIONS: Analyze this conversation carefully. If sufficient information has been gathered and the conversation naturally concludes, call the 'complete_chat' tool. If more information is needed, continue the conversation by asking relevant questions and use 'mark_topic_progress' to track what you learn.`,
		status,
		messageCount,
		strings.Join(completedTopics, ", "),
		status == "ready_for_completion")
}

// registerTestTools registers the chat tools with the tool registry
func (t *ConversationCompletionTester) registerTestTools(registry internal.LLMToolRegistry) {
	// Register the main chat tools that we want to test
	registry.RegisterTool("complete_chat", chat.NewCompleteChatTool(t.logger))
	registry.RegisterTool("get_checklist_status", chat.NewGetChecklistStatusTool(t.logger))
	registry.RegisterTool("mark_topic_progress", chat.NewMarkTopicProgressTool(t.logger))
	registry.RegisterTool("check_completion_readiness", chat.NewCheckCompletionReadinessTool(t.logger))
	registry.RegisterTool("override_completion", chat.NewOverrideCompletionTool(t.logger))
	registry.RegisterTool("validate_information_quality", chat.NewValidateInformationQualityTool(t.logger))
}

// evaluateResult determines if the test result matches expectations
func (t *ConversationCompletionTester) evaluateResult(scenario ConversationTestScenario, actualOutcome string, actualToolCalls []string) bool {
	// Check if outcome matches expectation
	if actualOutcome != scenario.ExpectedOutcome {
		return false
	}

	// Check if expected tool calls were made
	if len(scenario.ExpectedToolCalls) > 0 {
		expectedMap := make(map[string]bool)
		for _, tool := range scenario.ExpectedToolCalls {
			expectedMap[tool] = true
		}

		for _, tool := range actualToolCalls {
			if !expectedMap[tool] {
				// Unexpected tool call
				return false
			}
		}

		// Check if all expected tools were called
		for _, expectedTool := range scenario.ExpectedToolCalls {
			found := false
			for _, actualTool := range actualToolCalls {
				if actualTool == expectedTool {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}

// generateSummary creates a summary of all test results
func (t *ConversationCompletionTester) generateSummary(results []ConversationTestResult) *ConversationTestSummary {
	summary := &ConversationTestSummary{}

	// Initialize test run info
	summary.TestRun.Timestamp = time.Now()
	summary.TestRun.Model = t.model
	summary.TestRun.TotalTests = len(results)
	summary.Results = results

	// Count unique scenarios
	scenarioMap := make(map[string]bool)
	for _, result := range results {
		scenarioMap[result.Scenario] = true
	}
	summary.TestRun.TotalScenarios = len(scenarioMap)

	// Calculate success rates by category
	completeTests := []ConversationTestResult{}
	incompleteTests := []ConversationTestResult{}
	edgeTests := []ConversationTestResult{}

	for _, result := range results {
		switch result.Expected {
		case "complete":
			completeTests = append(completeTests, result)
		case "incomplete":
			incompleteTests = append(incompleteTests, result)
		case "edge":
			edgeTests = append(edgeTests, result)
		}
	}

	summary.Summary.CompleteScenarios = t.calculateCategoryResult(completeTests)
	summary.Summary.IncompleteScenarios = t.calculateCategoryResult(incompleteTests)
	summary.Summary.EdgeCaseScenarios = t.calculateCategoryResult(edgeTests)

	// Calculate overall success rate
	totalPassed := 0
	for _, result := range results {
		if result.Success {
			totalPassed++
		}
	}
	summary.TestRun.SuccessRate = float64(totalPassed) / float64(len(results))

	return summary
}

// calculateCategoryResult calculates success rate for a category of tests
func (t *ConversationCompletionTester) calculateCategoryResult(results []ConversationTestResult) TestCategoryResult {
	if len(results) == 0 {
		return TestCategoryResult{}
	}

	passed := 0
	for _, result := range results {
		if result.Success {
			passed++
		}
	}

	return TestCategoryResult{
		Total:  len(results),
		Passed: passed,
		Rate:   float64(passed) / float64(len(results)),
	}
}

// OutputConsole outputs results to console in human-readable format
func (t *ConversationCompletionTester) OutputConsole(summary *ConversationTestSummary) error {
	fmt.Printf("\n=== LLM Conversation Completion Test Results ===\n\n")
	fmt.Printf("Model: %s\n", summary.TestRun.Model)
	fmt.Printf("Total Scenarios: %d\n", summary.TestRun.TotalScenarios)
	fmt.Printf("Total Tests: %d\n", summary.TestRun.TotalTests)
	fmt.Printf("Overall Success Rate: %.1f%%\n\n", summary.TestRun.SuccessRate*100)

	// Category breakdown
	fmt.Printf("Results by Category:\n")
	fmt.Printf("- Complete Scenarios: %d/%d passed (%.1f%%)\n",
		summary.Summary.CompleteScenarios.Passed,
		summary.Summary.CompleteScenarios.Total,
		summary.Summary.CompleteScenarios.Rate*100)
	fmt.Printf("- Incomplete Scenarios: %d/%d passed (%.1f%%)\n",
		summary.Summary.IncompleteScenarios.Passed,
		summary.Summary.IncompleteScenarios.Total,
		summary.Summary.IncompleteScenarios.Rate*100)
	fmt.Printf("- Edge Case Scenarios: %d/%d passed (%.1f%%)\n\n",
		summary.Summary.EdgeCaseScenarios.Passed,
		summary.Summary.EdgeCaseScenarios.Total,
		summary.Summary.EdgeCaseScenarios.Rate*100)

	// Individual results
	fmt.Printf("Individual Test Results:\n")
	currentScenario := ""
	for _, result := range summary.Results {
		if result.Scenario != currentScenario {
			fmt.Printf("\nScenario: %s\n", result.Scenario)
			currentScenario = result.Scenario
		}

		status := "✅"
		if !result.Success {
			status = "❌"
		}

		fmt.Printf("  %s Iteration %d: Expected %s, Got %s (%dms)\n",
			status, result.Iteration, result.Expected, result.Actual, result.ExecutionTimeMs)

		if result.Error != "" {
			fmt.Printf("    Error: %s\n", result.Error)
		}

		if len(result.ToolCalls) > 0 {
			fmt.Printf("    Tools Called: %s\n", strings.Join(result.ToolCalls, ", "))
		}
	}

	return nil
}

// OutputJSON outputs results in JSON format
func (t *ConversationCompletionTester) OutputJSON(summary *ConversationTestSummary) error {
	jsonData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	fmt.Println(string(jsonData))
	return nil
}
