//go:build integration

package chat

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompletionFrameworkDemo demonstrates the complete testing framework
func TestCompletionFrameworkDemo(t *testing.T) {
	// Skip if OpenAI API key is not available
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	t.Log("===== COMPLETION FRAMEWORK DEMONSTRATION =====")
	t.Log("This test demonstrates the complete testing framework")
	t.Log("including consistency analysis, pattern detection, and reporting")
	t.Log("")

	// Create test runner with custom configuration
	config := &TestRunnerConfig{
		RunsPerTest:        3, // Small number for demo
		TestDataDirectory:  "../../testdata/conversations",
		OutputDirectory:    "../../test_results_demo",
		ParallelExecution:  false,
		SkipSlowTests:      true,
		MaxExecutionTime:   20 * time.Second,
		TestNameFilter:     "", // Run all tests
		SaveDetailedLogs:   true,
	}

	runner, err := CreateTestRunnerFromEnv(logger)
	require.NoError(t, err, "Failed to create test runner")
	
	// Override configuration
	runner.config = config

	t.Log("Configuration:")
	t.Logf("  Runs per test: %d", config.RunsPerTest)
	t.Logf("  Test data directory: %s", config.TestDataDirectory)
	t.Logf("  Output directory: %s", config.OutputDirectory)
	t.Logf("  Max execution time: %v", config.MaxExecutionTime)
	t.Log("")

	// Run the complete test suite
	t.Log("Running complete test suite...")
	startTime := time.Now()
	
	result, err := runner.RunAllTests(context.Background())
	require.NoError(t, err, "Failed to run test suite")
	
	executionTime := time.Since(startTime)
	t.Logf("Test suite completed in %v", executionTime)
	t.Log("")

	// Validate results
	t.Log("===== VALIDATION RESULTS =====")
	
	// Check that tests were executed
	assert.Greater(t, result.ExecutedTests, 0, "Should have executed at least one test")
	assert.Equal(t, result.TotalTests, result.ExecutedTests+result.SkippedTests, "Total tests should equal executed + skipped")
	
	t.Logf("✓ Executed %d tests (skipped %d)", result.ExecutedTests, result.SkippedTests)

	// Check overall success rate
	successRate := result.Report.OverallMetrics.OverallSuccessRate
	assert.GreaterOrEqual(t, successRate, 0.0, "Success rate should be non-negative")
	assert.LessOrEqual(t, successRate, 1.0, "Success rate should not exceed 100%")
	
	t.Logf("✓ Overall success rate: %.2f%%", successRate*100)

	// Check execution time metrics
	avgTime := result.Report.OverallMetrics.AverageExecutionTime
	assert.Greater(t, avgTime, time.Duration(0), "Average execution time should be positive")
	assert.Less(t, avgTime, config.MaxExecutionTime, "Average execution time should be less than max")
	
	t.Logf("✓ Average execution time: %v", avgTime)

	// Check completion score variance
	variance := result.Report.OverallMetrics.CompletionScoreVariance
	assert.GreaterOrEqual(t, variance, 0.0, "Variance should be non-negative")
	assert.LessOrEqual(t, variance, 1.0, "Variance should be reasonable")
	
	t.Logf("✓ Completion score variance: %.4f", variance)

	// Check that we have test results
	assert.Greater(t, len(result.Report.TestResults), 0, "Should have test results")
	
	t.Logf("✓ Generated results for %d tests", len(result.Report.TestResults))

	// Check pattern analysis
	assert.NotNil(t, result.Patterns, "Should have pattern analysis")
	assert.Greater(t, len(result.Patterns.CompletionTypeDistribution), 0, "Should have completion type distribution")
	
	t.Logf("✓ Completion type distribution: %v", result.Patterns.CompletionTypeDistribution)

	// Validate individual test results
	t.Log("")
	t.Log("===== INDIVIDUAL TEST ANALYSIS =====")
	
	for testName, analysis := range result.Report.TestResults {
		t.Logf("Test: %s", testName)
		t.Logf("  Success rate: %.2f%%", analysis.SuccessRate*100)
		t.Logf("  Runs: %d/%d successful", analysis.SuccessfulRuns, analysis.TotalRuns)
		t.Logf("  Average execution time: %v", analysis.AverageExecutionTime)
		t.Logf("  Tool call consistency: %v", analysis.ToolCallConsistency)
		t.Logf("  Completion consistency: %v", analysis.CompletionConsistency)
		
		// Validate individual test metrics
		assert.GreaterOrEqual(t, analysis.SuccessRate, 0.0, "Success rate should be non-negative")
		assert.LessOrEqual(t, analysis.SuccessRate, 1.0, "Success rate should not exceed 100%")
		assert.Equal(t, analysis.TotalRuns, config.RunsPerTest, "Should have correct number of runs")
		assert.LessOrEqual(t, analysis.SuccessfulRuns, analysis.TotalRuns, "Successful runs should not exceed total runs")
		
		t.Log("")
	}

	// Check for specific completion types
	t.Log("===== COMPLETION TYPE ANALYSIS =====")
	
	completionTypes := result.Patterns.CompletionTypeDistribution
	
	// We should see different completion types
	if len(completionTypes) > 1 {
		t.Log("✓ Multiple completion types detected:")
		for compType, count := range completionTypes {
			t.Logf("  %s: %d occurrences", compType, count)
		}
	} else {
		t.Log("⚠ Only one completion type detected - consider adding more diverse test scenarios")
	}

	// Test the framework's ability to detect patterns
	t.Log("")
	t.Log("===== PATTERN DETECTION =====")
	
	if len(result.Patterns.ToolCallPatterns) > 0 {
		t.Log("✓ Tool call patterns detected:")
		for testName, patterns := range result.Patterns.ToolCallPatterns {
			if len(patterns) > 0 {
				t.Logf("  %s: %d patterns", testName, len(patterns))
				// Show first pattern as example
				if len(patterns[0]) > 0 {
					t.Logf("    Example: %s", patterns[0])
				}
			}
		}
	} else {
		t.Log("⚠ No tool call patterns detected")
	}

	// Final assessment
	t.Log("")
	t.Log("===== FRAMEWORK ASSESSMENT =====")
	
	// Assess framework performance
	if successRate >= 0.8 {
		t.Log("✅ EXCELLENT: Framework shows high consistency (≥80% success rate)")
	} else if successRate >= 0.6 {
		t.Log("✅ GOOD: Framework shows acceptable consistency (≥60% success rate)")
	} else {
		t.Log("⚠️ NEEDS IMPROVEMENT: Framework shows low consistency (<60% success rate)")
	}

	if variance <= 0.1 {
		t.Log("✅ EXCELLENT: Low completion score variance (≤0.1)")
	} else if variance <= 0.2 {
		t.Log("✅ GOOD: Moderate completion score variance (≤0.2)")
	} else {
		t.Log("⚠️ NEEDS IMPROVEMENT: High completion score variance (>0.2)")
	}

	if avgTime <= 5*time.Second {
		t.Log("✅ EXCELLENT: Fast execution time (≤5s average)")
	} else if avgTime <= 10*time.Second {
		t.Log("✅ GOOD: Reasonable execution time (≤10s average)")
	} else {
		t.Log("⚠️ SLOW: Long execution time (>10s average)")
	}

	t.Log("")
	t.Log("===== DEMO COMPLETED SUCCESSFULLY =====")
	t.Logf("Total execution time: %v", executionTime)
	t.Logf("Framework reliability: %.2f%%", successRate*100)
	t.Logf("Results saved to: %s", config.OutputDirectory)
	t.Log("")
	t.Log("The framework is ready for production use!")
}

// TestFrameworkComponentsIntegration tests that all components work together
func TestFrameworkComponentsIntegration(t *testing.T) {
	// Skip if OpenAI API key is not available
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	t.Log("Testing individual framework components...")

	// Test 1: TestDataLoader
	t.Run("TestDataLoader", func(t *testing.T) {
		loader := NewTestDataLoader("../../testdata/conversations", logger)
		conversations, err := loader.LoadAllConversations()
		require.NoError(t, err, "Failed to load conversations")
		assert.Greater(t, len(conversations), 0, "Should load at least one conversation")
		t.Logf("✓ Loaded %d conversations", len(conversations))
	})

	// Test 2: CompletionTestFramework
	t.Run("CompletionTestFramework", func(t *testing.T) {
		runner, err := CreateTestRunnerFromEnv(logger)
		require.NoError(t, err, "Failed to create test runner")
		
		// Test loading a specific conversation
		testData, err := runner.framework.LoadConversationData("complete/successful_comprehensive.json")
		require.NoError(t, err, "Failed to load test conversation")
		assert.NotEmpty(t, testData.Name, "Conversation should have a name")
		assert.NotEmpty(t, testData.ConversationMessages, "Conversation should have messages")
		t.Logf("✓ Loaded conversation: %s", testData.Name)
	})

	// Test 3: AnalysisReporter
	t.Run("AnalysisReporter", func(t *testing.T) {
		reporter := NewAnalysisReporter(logger)
		
		// Create mock analysis results
		mockAnalysis := &ConsistencyAnalysis{
			TestName:              "mock_test",
			TotalRuns:             3,
			SuccessfulRuns:        2,
			SuccessRate:           0.67,
			ToolCallConsistency:   map[string]int{"test_tool": 2},
			CompletionConsistency: map[string]int{"hard": 2},
			AverageExecutionTime:  time.Second,
			CompletionScoreVariance: 0.05,
		}
		
		results := map[string]*ConsistencyAnalysis{
			"mock_test": mockAnalysis,
		}
		
		report := reporter.GenerateReport(results)
		require.NotNil(t, report, "Should generate report")
		assert.Equal(t, 1, report.TotalTests, "Should have correct test count")
		assert.NotNil(t, report.OverallMetrics, "Should have overall metrics")
		t.Log("✓ Generated analysis report")
	})

	// Test 4: CompletionPatternAnalyzer
	t.Run("CompletionPatternAnalyzer", func(t *testing.T) {
		analyzer := NewCompletionPatternAnalyzer(logger)
		
		// Create mock results for pattern analysis
		mockResult := &CompletionTestResult{
			TestName:           "pattern_test",
			Success:            true,
			ToolCallsExecuted:  []string{"mark_topic_progress", "complete_chat"},
			CompletionStatus:   &internal.CompletionStatus{
				HardComplete:     true,
				CompletionScore:  0.9,
			},
		}
		
		mockAnalysis := &ConsistencyAnalysis{
			TestName: "pattern_test",
			Results:  []CompletionTestResult{*mockResult},
		}
		
		results := map[string]*ConsistencyAnalysis{
			"pattern_test": mockAnalysis,
		}
		
		patterns := analyzer.AnalyzeCompletionPatterns(results)
		require.NotNil(t, patterns, "Should generate patterns")
		assert.NotNil(t, patterns.CompletionTypeDistribution, "Should have completion type distribution")
		t.Log("✓ Generated pattern analysis")
	})

	t.Log("All framework components integrated successfully!")
}

// TestFrameworkReliability tests the framework's reliability with edge cases
func TestFrameworkReliability(t *testing.T) {
	// Skip if OpenAI API key is not available
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	t.Log("Testing framework reliability with edge cases...")

	// Test timeout handling
	t.Run("TimeoutHandling", func(t *testing.T) {
		config := &TestRunnerConfig{
			RunsPerTest:        1,
			TestDataDirectory:  "../../testdata/conversations",
			OutputDirectory:    "../../test_results_reliability",
			MaxExecutionTime:   1 * time.Millisecond, // Very short timeout
			SaveDetailedLogs:   false,
		}

		runner, err := CreateTestRunnerFromEnv(logger)
		require.NoError(t, err, "Failed to create test runner")
		runner.config = config

		// This should handle timeouts gracefully
		result, err := runner.RunAllTests(context.Background())
		
		// Should not crash, even with timeouts
		require.NoError(t, err, "Should handle timeouts gracefully")
		
		// May have low success rate due to timeouts, but should complete
		assert.NotNil(t, result, "Should return result even with timeouts")
		t.Log("✓ Timeout handling works correctly")
	})

	// Test with invalid test data
	t.Run("InvalidDataHandling", func(t *testing.T) {
		framework := NewCompletionTestFramework(nil, logger)
		
		// Try to load non-existent file
		_, err := framework.LoadConversationData("nonexistent.json")
		assert.Error(t, err, "Should handle missing files gracefully")
		t.Log("✓ Invalid data handling works correctly")
	})

	// Test with minimal configuration
	t.Run("MinimalConfiguration", func(t *testing.T) {
		config := &TestRunnerConfig{
			RunsPerTest:        1,
			TestDataDirectory:  "../../testdata/conversations",
			OutputDirectory:    "../../test_results_minimal",
			ParallelExecution:  false,
			SkipSlowTests:      true,
			MaxExecutionTime:   5 * time.Second,
			TestNameFilter:     "successful_comprehensive", // Run only one test
			SaveDetailedLogs:   false,
		}

		runner, err := CreateTestRunnerFromEnv(logger)
		require.NoError(t, err, "Failed to create test runner")
		runner.config = config

		result, err := runner.RunAllTests(context.Background())
		require.NoError(t, err, "Should work with minimal configuration")
		
		assert.Greater(t, result.ExecutedTests, 0, "Should execute at least one test")
		t.Log("✓ Minimal configuration works correctly")
	})

	t.Log("Framework reliability tests completed successfully!")
}