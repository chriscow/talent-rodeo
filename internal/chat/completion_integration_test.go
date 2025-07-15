//go:build integration

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"talent-rodeo/internal"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConversationData represents the structure of test conversation files
type TestConversationData struct {
	Name                 string
	Description          string
	ExpectedOutcome      string
	ExpectedToolCalls    []string
	ConversationMessages []ConversationMessage
	ChecklistState       ExpectedChecklistState
}

// ConversationMessage represents a single message in the test conversation
type ConversationMessage struct {
	Role    string
	Content string
}

// ExpectedChecklistState represents the expected final state of the checklist
type ExpectedChecklistState struct {
	OverallStatus      string
	CompletedTopics    []string
	InProgressTopics   []string
	PendingTopics      []string
	HighQualityTopics  []string
	MessageCount       int
	CompletionAttempts int
}

// CompletionTestResult represents the result of a single completion test run
type CompletionTestResult struct {
	TestName           string
	RunNumber          int
	Success            bool
	CompletionStatus   *internal.CompletionStatus
	ToolCallsExecuted  []string
	CompletionAchieved bool
	ExecutionTime      time.Duration
	ErrorMessage       string
	AIResponse         string
}

// ConsistencyAnalysis represents the analysis of multiple test runs
type ConsistencyAnalysis struct {
	TestName                string
	TotalRuns               int
	SuccessfulRuns          int
	SuccessRate             float64
	ToolCallConsistency     map[string]int
	CompletionConsistency   map[string]int
	AverageExecutionTime    time.Duration
	CompletionScoreVariance float64
	Results                 []CompletionTestResult
}

// CompletionTestFramework provides the framework for running completion consistency tests
type CompletionTestFramework struct {
	chatService ChatService
	logger      *slog.Logger
	testDataDir string
}

// NewCompletionTestFramework creates a new completion test framework
func NewCompletionTestFramework(chatService ChatService, logger *slog.Logger) *CompletionTestFramework {
	// Find the testdata directory
	testDataDir := filepath.Join("../../testdata", "conversations")

	return &CompletionTestFramework{
		chatService: chatService,
		logger:      logger,
		testDataDir: testDataDir,
	}
}

// LoadConversationData loads test conversation data from JSON files
func (f *CompletionTestFramework) LoadConversationData(filename string) (*TestConversationData, error) {
	filepath := filepath.Join(f.testDataDir, filename)

	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read test data file %s: %w", filename, err)
	}

	var conversationData TestConversationData
	if err := json.Unmarshal(data, &conversationData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal test data: %w", err)
	}

	return &conversationData, nil
}

// RunConsistencyTest runs a conversation test multiple times and analyzes consistency
func (f *CompletionTestFramework) RunConsistencyTest(ctx context.Context, testData *TestConversationData, runs int) (*ConsistencyAnalysis, error) {
	f.logger.Info("Starting consistency test", "test_name", testData.Name, "runs", runs)

	analysis := &ConsistencyAnalysis{
		TestName:              testData.Name,
		TotalRuns:             runs,
		ToolCallConsistency:   make(map[string]int),
		CompletionConsistency: make(map[string]int),
		Results:               make([]CompletionTestResult, 0, runs),
	}

	var totalExecutionTime time.Duration
	var completionScores []float64

	for i := 0; i < runs; i++ {
		f.logger.Info("Running test iteration", "test_name", testData.Name, "run", i+1)

		result := f.runSingleTest(ctx, testData, i+1)
		analysis.Results = append(analysis.Results, result)

		if result.Success {
			analysis.SuccessfulRuns++
			totalExecutionTime += result.ExecutionTime

			// Track tool call consistency
			for _, toolCall := range result.ToolCallsExecuted {
				analysis.ToolCallConsistency[toolCall]++
			}

			// Track completion consistency
			if result.CompletionStatus != nil {
				completionType := f.getCompletionType(result.CompletionStatus)
				analysis.CompletionConsistency[completionType]++
				completionScores = append(completionScores, result.CompletionStatus.CompletionScore)
			}
		}
	}

	// Calculate final metrics
	analysis.SuccessRate = float64(analysis.SuccessfulRuns) / float64(analysis.TotalRuns)
	if analysis.SuccessfulRuns > 0 {
		analysis.AverageExecutionTime = totalExecutionTime / time.Duration(analysis.SuccessfulRuns)
	}

	// Calculate completion score variance
	if len(completionScores) > 1 {
		analysis.CompletionScoreVariance = f.calculateVariance(completionScores)
	}

	f.logger.Info("Consistency test completed",
		"test_name", testData.Name,
		"success_rate", analysis.SuccessRate,
		"avg_execution_time", analysis.AverageExecutionTime,
		"score_variance", analysis.CompletionScoreVariance,
	)

	return analysis, nil
}

// runSingleTest executes a single test run with the given conversation data
func (f *CompletionTestFramework) runSingleTest(ctx context.Context, testData *TestConversationData, runNumber int) CompletionTestResult {
	startTime := time.Now()

	result := CompletionTestResult{
		TestName:          testData.Name,
		RunNumber:         runNumber,
		ToolCallsExecuted: make([]string, 0),
	}

	// Create a test user and appointment
	user := f.createTestUser()
	appointment := f.createTestAppointment(user.ID)

	// Initialize the conversation with the test data
	if err := f.initializeConversation(ctx, appointment, testData); err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to initialize conversation: %v", err)
		result.ExecutionTime = time.Since(startTime)
		return result
	}

	// Use FastForward to set up the conversation state
	if err := f.chatService.FastForwardChat(ctx, user.ID, appointment.ID); err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to fast forward chat: %v", err)
		result.ExecutionTime = time.Since(startTime)
		return result
	}

	// Test the completion status check
	completionStatus := internal.GetHybridCompletionStatus(appointment.ConversationChecklist)
	result.CompletionStatus = completionStatus

	// Simulate tool calls that the LLM would make
	toolCalls := f.simulateLLMToolCalls(ctx, user, appointment, testData)
	result.ToolCallsExecuted = toolCalls

	// Check if completion was achieved
	result.CompletionAchieved = completionStatus.CanComplete

	// Mark as successful if no errors occurred
	result.Success = result.ErrorMessage == ""
	result.ExecutionTime = time.Since(startTime)

	return result
}

// createTestUser creates a test user for the test run
func (f *CompletionTestFramework) createTestUser() *internal.User {
	return &internal.User{
		ID:       fmt.Sprintf("test-user-%d", time.Now().UnixNano()),
		Email:    "test@example.com",
		Name:     "Test User",
		Status:   internal.UserStatusActive,
		Timezone: "America/New_York",
	}
}

// createTestAppointment creates a test appointment for the test run
func (f *CompletionTestFramework) createTestAppointment(userID string) *internal.Appointment {
	return &internal.Appointment{
		ID:                    fmt.Sprintf("test-appointment-%d", time.Now().UnixNano()),
		UserID:                userID,
		Type:                  internal.AppointmentTypeChat,
		Status:                internal.AppointmentStatusRunning,
		ConversationChecklist: internal.InitializeConversationChecklist(),
		ChatMessages:          make([]internal.ChatMessage, 0),
		ScheduledTime:         time.Now(),
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
}

// initializeConversation sets up the conversation with test data
func (f *CompletionTestFramework) initializeConversation(ctx context.Context, appointment *internal.Appointment, testData *TestConversationData) error {
	// Convert test messages to internal format
	for _, msg := range testData.ConversationMessages {
		if msg.Role == "user" || msg.Role == "assistant" {
			chatMessage := internal.ChatMessage{
				ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
				Role:      msg.Role,
				Content:   msg.Content,
				Timestamp: time.Now(),
			}
			appointment.ChatMessages = append(appointment.ChatMessages, chatMessage)
		}
	}

	// Update message count in checklist
	appointment.ConversationChecklist.MessageCount = len(appointment.ChatMessages)

	// Set up initial checklist state based on test data
	if testData.ChecklistState.CompletionAttempts > 0 {
		appointment.ConversationChecklist.CompletionAttempts = testData.ChecklistState.CompletionAttempts
	}

	return nil
}

// simulateLLMToolCalls simulates the tool calls that an LLM would make
func (f *CompletionTestFramework) simulateLLMToolCalls(ctx context.Context, user *internal.User, appointment *internal.Appointment, testData *TestConversationData) []string {
	var toolCalls []string

	// Simulate mark_topic_progress calls for completed topics
	for _, topic := range testData.ChecklistState.CompletedTopics {
		toolCalls = append(toolCalls, "mark_topic_progress")

		// Find the topic in the checklist and mark it as completed
		for i, item := range appointment.ConversationChecklist.Items {
			if item.ID == topic {
				appointment.ConversationChecklist.Items[i].Status = internal.ChecklistItemStatusCompleted
				appointment.ConversationChecklist.Items[i].Quality = internal.ChecklistItemQualityMedium

				// Mark high quality topics
				for _, hqTopic := range testData.ChecklistState.HighQualityTopics {
					if hqTopic == topic {
						appointment.ConversationChecklist.Items[i].Quality = internal.ChecklistItemQualityHigh
						break
					}
				}
				break
			}
		}
	}

	// Simulate get_checklist_status call
	toolCalls = append(toolCalls, "get_checklist_status")

	// Simulate check_completion_readiness call
	toolCalls = append(toolCalls, "check_completion_readiness")

	// Simulate complete_chat call if expected
	if testData.ExpectedOutcome == "complete" {
		toolCalls = append(toolCalls, "complete_chat")
	}

	return toolCalls
}

// getCompletionType determines the type of completion achieved
func (f *CompletionTestFramework) getCompletionType(status *internal.CompletionStatus) string {
	if status.HardComplete {
		return "hard"
	} else if status.SoftComplete {
		return "soft"
	} else if status.CanComplete {
		return "escalated"
	}
	return "incomplete"
}

// calculateVariance calculates the variance of completion scores
func (f *CompletionTestFramework) calculateVariance(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}

	// Calculate mean
	var sum float64
	for _, score := range scores {
		sum += score
	}
	mean := sum / float64(len(scores))

	// Calculate variance
	var variance float64
	for _, score := range scores {
		variance += (score - mean) * (score - mean)
	}

	return variance / float64(len(scores))
}

// Integration tests for the hybrid completion system
func TestHybridCompletionConsistency(t *testing.T) {
	// Skip if OpenAI API key is not available
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	// Set up test environment
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Create OpenAI client
	openAIClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	// Create mock service dependencies
	serviceDeps := &internal.ServiceDeps{
		Logger: logger,
	}

	// Create chat service
	chatService := NewService(nil, openAIClient, serviceDeps, logger)

	// Create test framework
	framework := NewCompletionTestFramework(chatService, logger)

	// Test scenarios
	testCases := []struct {
		filename    string
		description string
		runs        int
	}{
		{
			filename:    "complete/successful_comprehensive.json",
			description: "Comprehensive successful conversation",
			runs:        3, // Start with smaller number for faster testing
		},
		{
			filename:    "incomplete/early_conversation.json",
			description: "Early stage conversation",
			runs:        3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Load test data
			testData, err := framework.LoadConversationData(tc.filename)
			require.NoError(t, err, "Failed to load test data")

			// Run consistency test
			analysis, err := framework.RunConsistencyTest(ctx, testData, tc.runs)
			require.NoError(t, err, "Failed to run consistency test")

			// Validate results
			assert.Greater(t, analysis.SuccessRate, 0.6, "Success rate should be above 60%")
			assert.Greater(t, analysis.SuccessfulRuns, 0, "Should have at least one successful run")
			assert.LessOrEqual(t, analysis.CompletionScoreVariance, 0.1, "Completion score variance should be low")

			// Log results for analysis
			t.Logf("Test Results for %s:", tc.description)
			t.Logf("  Success Rate: %.2f%%", analysis.SuccessRate*100)
			t.Logf("  Average Execution Time: %v", analysis.AverageExecutionTime)
			t.Logf("  Completion Score Variance: %.4f", analysis.CompletionScoreVariance)
			t.Logf("  Tool Call Consistency: %+v", analysis.ToolCallConsistency)
			t.Logf("  Completion Consistency: %+v", analysis.CompletionConsistency)
		})
	}
}

// TestHybridCompletionScenarios tests specific completion scenarios
func TestHybridCompletionScenarios(t *testing.T) {
	// Skip if OpenAI API key is not available
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Create a test framework
	framework := NewCompletionTestFramework(nil, logger)

	// Test different completion scenarios
	scenarios := []struct {
		name         string
		setupFunc    func(*internal.ConversationChecklist)
		expectedType string
	}{
		{
			name: "Hard Completion",
			setupFunc: func(checklist *internal.ConversationChecklist) {
				// Mark all required items as completed with high quality
				for i := range checklist.Items {
					if checklist.Items[i].Required {
						checklist.Items[i].Status = internal.ChecklistItemStatusCompleted
						checklist.Items[i].Quality = internal.ChecklistItemQualityHigh
					}
				}
			},
			expectedType: "hard",
		},
		{
			name: "Soft Completion",
			setupFunc: func(checklist *internal.ConversationChecklist) {
				// Mark 75% of required items as completed
				checklist.MessageCount = 25
				requiredCount := 0
				for i := range checklist.Items {
					if checklist.Items[i].Required {
						requiredCount++
					}
				}
				completedCount := 0
				target := int(float64(requiredCount) * 0.75)
				for i := range checklist.Items {
					if checklist.Items[i].Required && completedCount < target {
						checklist.Items[i].Status = internal.ChecklistItemStatusCompleted
						checklist.Items[i].Quality = internal.ChecklistItemQualityMedium
						completedCount++
					}
				}
			},
			expectedType: "soft",
		},
		{
			name: "Escalated Completion",
			setupFunc: func(checklist *internal.ConversationChecklist) {
				// Set up escalated completion scenario
				checklist.CompletionAttempts = internal.MaxCompletionAttempts
				checklist.MessageCount = 20
				// Mark some items as completed
				for i := 0; i < 3 && i < len(checklist.Items); i++ {
					if checklist.Items[i].Required {
						checklist.Items[i].Status = internal.ChecklistItemStatusCompleted
						checklist.Items[i].Quality = internal.ChecklistItemQualityMedium
					}
				}
			},
			expectedType: "escalated",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Create test checklist
			checklist := internal.InitializeConversationChecklist()
			scenario.setupFunc(checklist)

			// Test hybrid completion status
			status := internal.GetHybridCompletionStatus(checklist)
			actualType := framework.getCompletionType(status)

			assert.Equal(t, scenario.expectedType, actualType,
				"Expected completion type %s, got %s", scenario.expectedType, actualType)

			// Validate completion score
			assert.GreaterOrEqual(t, status.CompletionScore, 0.0, "Completion score should be non-negative")
			assert.LessOrEqual(t, status.CompletionScore, 1.0, "Completion score should not exceed 1.0")

			// Validate guidance
			assert.NotEmpty(t, status.Guidance, "Guidance should not be empty")

			t.Logf("Scenario: %s", scenario.name)
			t.Logf("  Completion Type: %s", actualType)
			t.Logf("  Completion Score: %.2f", status.CompletionScore)
			t.Logf("  Can Complete: %v", status.CanComplete)
			t.Logf("  Guidance: %s", status.Guidance)
		})
	}
}
