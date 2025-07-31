//go:build integration

package chat

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"talent-rodeo/internal"
)

// Additional utility functions for the testing framework
// Main type definitions are in completion_test_runner.go

// Extended LoadAllConversations with additional filtering
func (l *TestDataLoader) LoadAllConversationsWithFilter(filter func(*TestConversationData) bool) (map[string]*TestConversationData, error) {
	allConversations, err := l.LoadAllConversations()
	if err != nil {
		return nil, err
	}

	filtered := make(map[string]*TestConversationData)
	for filename, conversation := range allConversations {
		if filter(conversation) {
			filtered[filename] = conversation
		}
	}

	return filtered, nil
}

// ValidateConversationData validates the structure of conversation test data
func (l *TestDataLoader) ValidateConversationData(data *TestConversationData) []string {
	var issues []string

	if data.Name == "" {
		issues = append(issues, "missing name")
	}

	if data.Description == "" {
		issues = append(issues, "missing description")
	}

	if len(data.ConversationMessages) == 0 {
		issues = append(issues, "no conversation messages")
	}

	if len(data.ExpectedToolCalls) == 0 {
		issues = append(issues, "no expected tool calls")
	}

	// Validate conversation flow
	hasUser := false
	hasAssistant := false
	for _, msg := range data.ConversationMessages {
		if msg.Role == "user" {
			hasUser = true
		}
		if msg.Role == "assistant" {
			hasAssistant = true
		}
	}

	if !hasUser {
		issues = append(issues, "no user messages")
	}
	if !hasAssistant {
		issues = append(issues, "no assistant messages")
	}

	return issues
}

// GetConversationStats returns statistics about conversation test data
func (l *TestDataLoader) GetConversationStats() (*ConversationStats, error) {
	conversations, err := l.LoadAllConversations()
	if err != nil {
		return nil, err
	}

	stats := &ConversationStats{
		TotalConversations: len(conversations),
		OutcomeDistribution: make(map[string]int),
		ToolCallDistribution: make(map[string]int),
		MessageCountDistribution: make(map[string]int),
	}

	for _, conv := range conversations {
		// Count outcomes
		stats.OutcomeDistribution[conv.ExpectedOutcome]++

		// Count tool calls
		for _, tool := range conv.ExpectedToolCalls {
			stats.ToolCallDistribution[tool]++
		}

		// Count message ranges
		msgCount := len(conv.ConversationMessages)
		switch {
		case msgCount <= 10:
			stats.MessageCountDistribution["1-10"]++
		case msgCount <= 20:
			stats.MessageCountDistribution["11-20"]++
		case msgCount <= 30:
			stats.MessageCountDistribution["21-30"]++
		default:
			stats.MessageCountDistribution["31+"]++
		}
	}

	return stats, nil
}

// ConversationStats represents statistics about the conversation test data
type ConversationStats struct {
	TotalConversations      int            `json:"total_conversations"`
	OutcomeDistribution     map[string]int `json:"outcome_distribution"`
	ToolCallDistribution    map[string]int `json:"tool_call_distribution"`
	MessageCountDistribution map[string]int `json:"message_count_distribution"`
}

// Extended GenerateReport with additional metrics
func (r *AnalysisReporter) GenerateReportWithTrends(analyses map[string]*ConsistencyAnalysis, previousReport *TestReport) *TestReport {
	currentReport := r.GenerateReport(analyses)

	if previousReport != nil {
		// Add trend analysis
		currentReport.OverallMetrics.MostConsistentToolCalls = r.calculateTrends(currentReport, previousReport)
	}

	return currentReport
}

// calculateTrends calculates trends between current and previous reports
func (r *AnalysisReporter) calculateTrends(current, previous *TestReport) []string {
	// Simple trend calculation - this could be expanded
	trends := []string{}

	if current.OverallMetrics.OverallSuccessRate > previous.OverallMetrics.OverallSuccessRate {
		trends = append(trends, "success_rate_improving")
	} else if current.OverallMetrics.OverallSuccessRate < previous.OverallMetrics.OverallSuccessRate {
		trends = append(trends, "success_rate_declining")
	}

	if current.OverallMetrics.AverageExecutionTime < previous.OverallMetrics.AverageExecutionTime {
		trends = append(trends, "performance_improving")
	} else if current.OverallMetrics.AverageExecutionTime > previous.OverallMetrics.AverageExecutionTime {
		trends = append(trends, "performance_declining")
	}

	return trends
}

// SaveReportWithTimestamp saves the report with a timestamp
func (r *AnalysisReporter) SaveReportWithTimestamp(report *TestReport, baseFilename string) error {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.json", baseFilename, timestamp)
	return r.SaveReport(report, filename)
}

// CompareReports compares two test reports and returns a comparison
func (r *AnalysisReporter) CompareReports(current, previous *TestReport) *ReportComparison {
	return &ReportComparison{
		CurrentReport:  current,
		PreviousReport: previous,
		SuccessRateDiff: current.OverallMetrics.OverallSuccessRate - previous.OverallMetrics.OverallSuccessRate,
		ExecutionTimeDiff: current.OverallMetrics.AverageExecutionTime - previous.OverallMetrics.AverageExecutionTime,
		VarianceDiff: current.OverallMetrics.CompletionScoreVariance - previous.OverallMetrics.CompletionScoreVariance,
		GeneratedAt: time.Now(),
	}
}

// ReportComparison represents a comparison between two test reports
type ReportComparison struct {
	CurrentReport     *TestReport   `json:"current_report"`
	PreviousReport    *TestReport   `json:"previous_report"`
	SuccessRateDiff   float64       `json:"success_rate_diff"`
	ExecutionTimeDiff time.Duration `json:"execution_time_diff"`
	VarianceDiff      float64       `json:"variance_diff"`
	GeneratedAt       time.Time     `json:"generated_at"`
}

// Extended pattern analysis with anomaly detection
func (a *CompletionPatternAnalyzer) AnalyzeCompletionPatternsWithAnomalies(results map[string]*ConsistencyAnalysis) *ExtendedPatternReport {
	basicReport := a.AnalyzeCompletionPatterns(results)

	extendedReport := &ExtendedPatternReport{
		CompletionPatternReport: *basicReport,
		Anomalies:              []string{},
		Insights:               []string{},
	}

	// Detect anomalies
	for testName, analysis := range results {
		if analysis.SuccessRate < 0.5 {
			extendedReport.Anomalies = append(extendedReport.Anomalies, 
				fmt.Sprintf("Low success rate in %s: %.2f%%", testName, analysis.SuccessRate*100))
		}

		if analysis.CompletionScoreVariance > 0.2 {
			extendedReport.Anomalies = append(extendedReport.Anomalies, 
				fmt.Sprintf("High variance in %s: %.4f", testName, analysis.CompletionScoreVariance))
		}
	}

	// Generate insights
	if len(basicReport.CompletionTypeDistribution) > 0 {
		mostCommon := ""
		maxCount := 0
		for compType, count := range basicReport.CompletionTypeDistribution {
			if count > maxCount {
				maxCount = count
				mostCommon = compType
			}
		}
		extendedReport.Insights = append(extendedReport.Insights,
			fmt.Sprintf("Most common completion type: %s (%d occurrences)", mostCommon, maxCount))
	}

	return extendedReport
}

// ExtendedPatternReport includes additional analysis
type ExtendedPatternReport struct {
	CompletionPatternReport
	Anomalies []string `json:"anomalies"`
	Insights  []string `json:"insights"`
}

// MockChatService provides a mock implementation for testing
type MockChatService struct {
	logger *slog.Logger
}

// NewMockChatService creates a new mock chat service
func NewMockChatService(logger *slog.Logger) *MockChatService {
	return &MockChatService{
		logger: logger,
	}
}

// Implement ChatService interface methods
func (m *MockChatService) StartSession(ctx context.Context, userID string, req StartChatRequest) (*StartChatResponse, error) {
	return &StartChatResponse{}, nil
}

func (m *MockChatService) EndSession(ctx context.Context, userID, appointmentID string) error {
	return nil
}

func (m *MockChatService) ProcessUserMessage(ctx context.Context, userID, appointmentID, message string) error {
	return nil
}

func (m *MockChatService) StreamAIResponse(ctx context.Context, userID, appointmentID string, msgChan chan<- StreamMessage) error {
	return nil
}

func (m *MockChatService) GetHistory(ctx context.Context, userID, appointmentID string) ([]internal.ChatMessage, error) {
	return []internal.ChatMessage{}, nil
}

func (m *MockChatService) FastForwardChat(ctx context.Context, userID, appointmentID string) error {
	m.logger.Info("FastForward called", "user_id", userID, "appointment_id", appointmentID)
	return nil
}

func (m *MockChatService) SaveAppointment(ctx context.Context, appointment *internal.Appointment) error {
	return nil
}

// TestConfigManager manages test configuration
type TestConfigManager struct {
	configs map[string]*TestRunnerConfig
	logger  *slog.Logger
}

// NewTestConfigManager creates a new test configuration manager
func NewTestConfigManager(logger *slog.Logger) *TestConfigManager {
	return &TestConfigManager{
		configs: make(map[string]*TestRunnerConfig),
		logger:  logger,
	}
}

// AddConfig adds a named configuration
func (m *TestConfigManager) AddConfig(name string, config *TestRunnerConfig) {
	m.configs[name] = config
}

// GetConfig retrieves a named configuration
func (m *TestConfigManager) GetConfig(name string) (*TestRunnerConfig, bool) {
	config, exists := m.configs[name]
	return config, exists
}

// GetPresetConfigs returns common preset configurations
func (m *TestConfigManager) GetPresetConfigs() map[string]*TestRunnerConfig {
	return map[string]*TestRunnerConfig{
		"quick": {
			RunsPerTest:        2,
			TestDataDirectory:  "../../testdata/conversations",
			OutputDirectory:    "../../test_results_quick",
			SkipSlowTests:      true,
			MaxExecutionTime:   10 * time.Second,
			SaveDetailedLogs:   false,
		},
		"comprehensive": {
			RunsPerTest:        10,
			TestDataDirectory:  "../../testdata/conversations",
			OutputDirectory:    "../../test_results_comprehensive",
			SkipSlowTests:      false,
			MaxExecutionTime:   60 * time.Second,
			SaveDetailedLogs:   true,
		},
		"development": {
			RunsPerTest:        3,
			TestDataDirectory:  "../../testdata/conversations",
			OutputDirectory:    "../../test_results_dev",
			SkipSlowTests:      true,
			MaxExecutionTime:   20 * time.Second,
			TestNameFilter:     "comprehensive",
			SaveDetailedLogs:   true,
		},
	}
}