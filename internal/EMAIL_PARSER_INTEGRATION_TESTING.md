# EmailParser Integration Testing

This document describes the comprehensive integration testing suite for the EmailParser system, which validates LLM-first email parsing with real API calls.

## Overview

The EmailParser integration tests validate:
- **Prompt effectiveness** with real LLM responses
- **Parsing accuracy** across different email types and complexity levels
- **Structured output quality** and consistency
- **Cost tracking functionality** integration
- **Edge case handling** and robustness

## Test Suites

### 1. Real LLM Integration Test (`TestEmailParser_RealLLMIntegration`)

**Purpose**: Validates parsing accuracy with real LLM calls across comprehensive email scenarios.

**Test Cases**:
- **Simple job search request** - Basic remote work preference
- **Complex multi-request email** - Multiple updates with feedback
- **Complaint with mixed content** - Negative feedback + actionable requests
- **Pure feedback email** - Positive feedback with no requests
- **Edge case - very short email** - Minimal content parsing

**Validation**:
- Request/feedback/non-request categorization accuracy
- Content extraction quality (descriptions, quotes, types)
- ID assignment and uniqueness
- Structured output compliance
- Custom semantic validation per test case

### 2. Prompt Stress Test (`TestEmailParser_PromptStressTest`)

**Purpose**: Tests parser robustness with challenging content and edge cases.

**Stress Scenarios**:
- **Very long emails** - Repetitive content with buried requests
- **Special characters & formatting** - Unicode, emojis, symbols
- **Code snippets** - Technical content with programming examples
- **Ambiguous content** - Unclear requests requiring interpretation

**Success Criteria**:
- ≥75% success rate across stress tests
- Graceful handling of challenging content
- Maintained structure quality standards

## Running the Tests

### Prerequisites

1. **Environment Setup**:
   ```bash
   # Set OpenAI API key
   export OPENAI_API_KEY="your-openai-key"
   
   # Or add to .env file
   echo "OPENAI_API_KEY=your-key" >> .env
   ```

2. **Dependencies**:
   - Real OpenAI API access
   - Firestore emulator (for cost tracking)
   - Network connectivity

### Test Execution

```bash
# Run all integration tests
go test -tags=integration -run TestEmailParser -v ./internal

# Run specific integration test
go test -tags=integration -run TestEmailParser_RealLLMIntegration -v ./internal

# Run stress tests only
go test -tags=integration -run TestEmailParser_PromptStressTest -v ./internal

# Run with timeout for long operations
go test -tags=integration -run TestEmailParser -timeout=10m -v ./internal
```

### Skipping in CI/Short Mode

Tests automatically skip when:
- `testing.Short()` is true (CI environment)
- `OPENAI_API_KEY` environment variable not set
- Network connectivity issues

```bash
# Skip integration tests in CI
go test -short ./internal
```

## Test Output Analysis

### Successful Test Output

```
=== RUN   TestEmailParser_RealLLMIntegration/simple_job_search_request
    Parsing email: simple job search request
    Email body: Hi, I would like to add remote work options...
    Parsing results:
      Requests: 1 (expected 1)
      Feedback: 0 (expected 0)
      Non-requests: 2 (expected 2)
      Request 1: job_search - Add remote work options (ID: req_1)
        Quote: add remote work options to my job search
--- PASS: TestEmailParser_RealLLMIntegration/simple_job_search_request

=== INTEGRATION TEST SUMMARY ===
Total emails parsed: 5
Total requests extracted: 10
Total feedback items: 6
Total non-requests: 11
Parse errors: 0
Average requests per email: 2.00
Average feedback per email: 1.20
```

### Quality Metrics

The tests track several quality indicators:

1. **Parsing Accuracy**: Request/feedback categorization matches expectations (±1 tolerance)
2. **Structure Quality**: All items have required fields (ID, Type, Description, Quote)
3. **ID Uniqueness**: No duplicate IDs within parsed structure
4. **Content Extraction**: Quotes contain exact text from original email
5. **Semantic Validation**: Custom checks for specific content types

### Error Analysis

Common issues and debugging:

```bash
# API key issues
ERROR: OPENAI_API_KEY not set, skipping real LLM integration test

# Rate limiting
ERROR: Failed to parse email: API request failed with status 429

# Timeout issues
ERROR: context deadline exceeded

# Structure validation failures
ERROR: Item 3 missing Quote
ERROR: Duplicate ID found: req_1
```

## Cost Considerations

### API Usage

Integration tests make real LLM API calls:
- **5 test emails** in main integration test
- **4 stress test emails** in prompt stress test
- **~50-100 tokens per email** for parsing
- **Total cost**: ~$0.01-0.05 per full test run

### Cost Optimization

- Tests use **DeepSeekChat** model for cost efficiency
- **8192 token limit** prevents runaway costs
- **Timeout protection** prevents hanging requests
- **Skip mechanisms** prevent accidental CI costs

## Integration with Development Workflow

### Pre-commit Testing

```bash
# Quick validation before commit
go test -tags=integration -run TestEmailParser_RealLLMIntegration/simple -v ./internal
```

### Prompt Development Workflow

1. **Modify prompt** in `email_parser.go`
2. **Run integration tests** to validate changes
3. **Analyze output quality** and accuracy metrics
4. **Iterate on prompt** based on test results
5. **Commit** when tests pass consistently

### Quality Gates

Integration tests serve as quality gates for:
- **Prompt modifications** - Ensure changes don't break parsing
- **Model updates** - Validate new LLM models work correctly
- **Feature additions** - Test new email types and scenarios
- **Production readiness** - Verify system handles real-world content

## Extending the Test Suite

### Adding New Test Cases

```go
{
    name: "your_new_scenario",
    email: &inbox.Email{
        ID:      "test-new-001",
        From:    "user@example.com",
        Subject: "Test subject",
        Body:    "Your test email content...",
        Date:    time.Now(),
    },
    expectRequests: 2,
    expectFeedback: 1,
    expectNonReq:   1,
    validateFunc: func(t *testing.T, structure *EmailStructure) {
        // Custom validation logic
        // Check for specific content extraction
    },
},
```

### Custom Validation Functions

Add scenario-specific validation:
- Check for specific keywords in descriptions
- Validate quote accuracy
- Verify request type categorization
- Test edge case handling

### Performance Benchmarks

Add performance tracking:
```go
func BenchmarkEmailParser_RealLLM(b *testing.B) {
    // Benchmark parsing performance
    // Track response times and accuracy
}
```

## Troubleshooting

### Common Issues

1. **Rate Limiting**: Space out test runs, use different API keys
2. **Inconsistent Results**: LLM responses vary; use tolerance ranges
3. **Cost Concerns**: Monitor API usage, set usage alerts
4. **Network Issues**: Ensure stable internet, increase timeouts

### Debug Mode

Enable detailed logging:
```bash
export LOG_LEVEL=debug
go test -tags=integration -run TestEmailParser -v ./internal
```

### Manual Validation

For debugging specific cases:
```go
// Add temporary logging in test
t.Logf("Raw LLM response: %+v", structure)
t.Logf("Email content: %s", email.Body)
```

## Production Integration

### Monitoring Metrics

Use integration test patterns for production monitoring:
- **Parsing success rates**
- **Average items extracted per email**
- **Response time tracking**
- **Cost per email processed**

### Quality Assurance

Regular integration test runs help ensure:
- **Prompt effectiveness** remains high
- **Model updates** don't break functionality
- **Edge cases** are handled gracefully
- **Performance** stays within bounds

The integration testing suite provides comprehensive validation for the EmailParser system's real-world effectiveness and reliability.