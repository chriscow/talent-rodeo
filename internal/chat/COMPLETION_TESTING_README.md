# Hybrid Completion Testing Framework

## Overview

This framework provides comprehensive testing for the hybrid conversation completion system, enabling **"approval framework"** testing where the same conversation is run multiple times with real LLMs to measure consistency and reliability.

## Key Features

- **Real LLM Integration**: Tests with actual OpenAI GPT-4 calls, not mocks
- **Consistency Analysis**: Runs conversations multiple times to measure reliability
- **Tool Call Tracking**: Monitors which tools are called and how consistently
- **Completion Type Testing**: Tests hard, soft, and escalated completion scenarios
- **Pattern Analysis**: Identifies trends in completion behavior
- **Performance Benchmarking**: Measures execution times and variance

## Architecture

```
TestRunner
├── CompletionTestFramework  # Core testing logic
├── TestDataLoader          # Loads conversation JSON files
├── AnalysisReporter        # Generates reports and metrics
└── CompletionPatternAnalyzer # Analyzes completion patterns
```

## Running Tests

### Prerequisites

1. **Environment Variables**:
   ```bash
   export OPENAI_API_KEY="your-openai-key"
   export FIRESTORE_EMULATOR_HOST="localhost:8200"
   ```

2. **Test Data**: Conversation test files in `testdata/conversations/`

### Basic Usage

```bash
# Run all integration tests
go test -tags=integration ./internal/chat -v

# Run specific test
go test -tags=integration ./internal/chat -v -run TestHybridCompletionConsistency

# Run with custom configuration
TEST_RUNS_PER_TEST=5 go test -tags=integration ./internal/chat -v
```

### Advanced Usage

```go
// Create custom test runner
runner, err := CreateTestRunnerFromEnv(logger)
if err != nil {
    log.Fatal(err)
}

// Run all tests
result, err := runner.RunAllTests(ctx)
if err != nil {
    log.Fatal(err)
}

// Run specific test
analysis, err := runner.RunSpecificTest(ctx, "complete/successful_comprehensive.json")
```

## Test Data Format

Test conversations are stored as JSON files in `testdata/conversations/`:

```json
{
  "name": "Test Name",
  "description": "Test description",
  "expected_outcome": "complete|incomplete|escalated_complete|soft_complete",
  "expected_tool_calls": ["mark_topic_progress", "check_completion_readiness"],
  "conversation_messages": [
    {
      "role": "user|assistant|system",
      "content": "Message content"
    }
  ],
  "checklist_state": {
    "overall_status": "completed|in_progress|ready_for_completion",
    "completed_topics": ["topic1", "topic2"],
    "message_count": 20,
    "completion_attempts": 1
  }
}
```

## Test Categories

### 1. Consistency Tests (`TestHybridCompletionConsistency`)
- Runs same conversation multiple times
- Measures tool call consistency
- Analyzes completion decision reliability
- Tracks execution time variance

### 2. Scenario Tests (`TestHybridCompletionScenarios`)
- Tests specific completion types
- Validates completion logic
- Checks metrics calculation
- Verifies guidance generation

### 3. Integration Tests
- End-to-end conversation flows
- FastForward integration
- WebSocket simulation
- Real LLM tool calling

## Output and Analysis

### Console Output
```
===== COMPLETION CONSISTENCY TEST REPORT =====
Generated at: 2025-01-15T10:30:00Z
Total tests: 5

===== OVERALL METRICS =====
Overall success rate: 85.00%
Average execution time: 2.3s
Completion score variance: 0.0234

===== INDIVIDUAL TEST RESULTS =====
Test: complete/successful_comprehensive
  Success rate: 100.00%
  Tool call consistency: {"mark_topic_progress": 5, "complete_chat": 5}
  Completion consistency: {"hard": 5}
```

### Generated Files
- `completion_consistency_report.json` - Comprehensive test results
- `completion_patterns.json` - Pattern analysis
- `{test_name}_detailed.json` - Individual test details

## Test Configuration

```go
config := &TestRunnerConfig{
    RunsPerTest:        5,                    // Runs per test scenario
    TestDataDirectory:  "../../testdata/conversations",
    OutputDirectory:    "../../test_results",
    ParallelExecution:  false,               // Sequential execution
    SkipSlowTests:      false,               // Run all tests
    MaxExecutionTime:   30 * time.Second,   // Per-test timeout
    TestNameFilter:     "",                  // Filter tests by name
    SaveDetailedLogs:   true,                // Save detailed results
}
```

## Adding New Tests

1. **Create Test Data**: Add JSON file to `testdata/conversations/`
2. **Run Tests**: Framework auto-discovers new files
3. **Analyze Results**: Review consistency and pattern reports

### Example Test Data Creation

```json
{
  "name": "New Test Scenario",
  "description": "Test description",
  "expected_outcome": "soft_complete",
  "expected_tool_calls": ["mark_topic_progress", "check_completion_readiness"],
  "conversation_messages": [
    // Add conversation messages here
  ],
  "checklist_state": {
    // Define expected final state
  }
}
```

## Interpreting Results

### Success Rates
- **>90%**: Excellent consistency
- **80-90%**: Good consistency  
- **70-80%**: Acceptable consistency
- **<70%**: Investigate issues

### Completion Score Variance
- **<0.05**: Very consistent
- **0.05-0.10**: Moderately consistent
- **>0.10**: High variance (investigate)

### Tool Call Consistency
- Shows which tools are called reliably
- Identifies missing or extra tool calls
- Helps optimize tool usage patterns

## Common Issues and Solutions

### Low Success Rate
- Check OpenAI API key and rate limits
- Verify test data format
- Review conversation setup

### High Variance
- Increase number of runs per test
- Check for non-deterministic behavior
- Review completion logic

### Tool Call Inconsistency
- Verify tool definitions
- Check system prompt effectiveness
- Review conversation flow

## Framework Extension

The framework is designed to be extensible:

1. **New Metrics**: Add to `CompletionTestResult`
2. **New Analysis**: Extend `CompletionPatternAnalyzer`
3. **New Test Types**: Add to `CompletionTestFramework`
4. **New Outputs**: Extend `AnalysisReporter`

## Performance Considerations

- **Rate Limits**: OpenAI API has rate limits
- **Parallel Execution**: Currently disabled for reliability
- **Memory Usage**: Large test suites may consume significant memory
- **Execution Time**: Full test suite takes 10-15 minutes

## Best Practices

1. **Start Small**: Begin with 3-5 runs per test
2. **Incremental Testing**: Test changes with specific scenarios first
3. **Monitor Trends**: Track consistency over time
4. **Document Changes**: Update test data when logic changes
5. **Review Patterns**: Analyze completion patterns for insights

## Troubleshooting

### Common Error Messages

**"OPENAI_API_KEY not set"**
- Set the environment variable before running tests

**"Failed to load conversation data"**
- Check JSON file format and path
- Verify file permissions

**"Test timeout exceeded"**
- Increase `MaxExecutionTime` in config
- Check for API response delays

### Debug Mode

```bash
# Enable debug logging
SLOG_LEVEL=DEBUG go test -tags=integration ./internal/chat -v
```

## Future Enhancements

- **Multi-LLM Testing**: Test with different models
- **Parallel Execution**: Safe parallel test execution
- **Performance Profiling**: Detailed performance analysis
- **Regression Detection**: Automated regression testing
- **CI/CD Integration**: Automated test runs

---

This framework provides the foundation for reliable, consistent testing of the hybrid completion system with real LLM interactions.