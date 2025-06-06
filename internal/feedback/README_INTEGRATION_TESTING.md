# Integration Testing for Feedback System

This document describes how to run integration tests for the feedback processing system, including tests with real LLM calls and real Firestore data.

## Overview

The feedback system has three types of integration tests:

1. **TestFeedbackProcessingIntegration** - Tests with mock data and real LLM calls
2. **TestStatementUpdateWithRealLLM** - Tests statement updates with real LLM calls
3. **TestFeedbackProcessingWithRealFirestore** - Tests with real user data from Firestore (NEW)

## Prerequisites

### Environment Variables

Set the following environment variables for LLM integration:

```bash
export OPENAI_API_KEY="your-openai-api-key"
# OR
export DEEPSEEK_API_KEY="your-deepseek-api-key"
```

For Firestore integration (TestFeedbackProcessingWithRealFirestore):

```bash
export FIREBASE_PROJECT_ID="your-project-id"
export FIREBASE_CREDENTIALS="your-base64-encoded-credentials"
export TEST_USER_ID="real-user-id-from-firestore"
```

## Running Integration Tests

### All Integration Tests

```bash
go test -tags=integration ./internal/feedback/... -v
```

### Individual Tests

**Test with mock data and real LLM:**
```bash
go test -tags=integration -run TestFeedbackProcessingIntegration ./internal/feedback/ -v
```

**Test statement updates only:**
```bash
go test -tags=integration -run TestStatementUpdateWithRealLLM ./internal/feedback/ -v
```

**Test with real Firestore data:**
```bash
export TEST_USER_ID="user123"  # Replace with actual user ID
go test -tags=integration -run TestFeedbackProcessingWithRealFirestore ./internal/feedback/ -v
```

## TestFeedbackProcessingWithRealFirestore Details

This test allows you to test the feedback processing pipeline with actual user data from your Firestore database without saving any changes back to the database.

### What it does:

1. **Connects to real Firestore** using your credentials
2. **Loads actual user feedback** from the database
3. **Gets current user state** (statement and steering query) for comparison
4. **Processes feedback through the LLM pipeline** using real AI calls
5. **Displays results** without saving them back to Firestore
6. **Shows before/after comparisons** of statements and steering queries

### Usage:

```bash
# Set the user ID you want to test with
export TEST_USER_ID="actual-user-id-from-your-database"

# Run the test
go test -tags=integration -run TestFeedbackProcessingWithRealFirestore ./internal/feedback/ -v
```

### Example Output:

```
=== RUN   TestFeedbackProcessingWithRealFirestore
TestFeedbackProcessingWithRealFirestore: integration_test.go:37: Testing feedback processing for user: user123
TestFeedbackProcessingWithRealFirestore: integration_test.go:65: Found 3 unprocessed feedback items
TestFeedbackProcessingWithRealFirestore: integration_test.go:66: Feedback 1: Work remotely with flexible hours (liked: true, job: job456)
TestFeedbackProcessingWithRealFirestore: integration_test.go:66: Feedback 2: Strong proficiency in Go programming (liked: true, job: job456)  
TestFeedbackProcessingWithRealFirestore: integration_test.go:66: Feedback 3: Must be available for daily in-office meetings (liked: false, job: job789)
TestFeedbackProcessingWithRealFirestore: integration_test.go:82: Processing feedback for user user123...

=== PROCESSING RESULTS ===
TestFeedbackProcessingWithRealFirestore: integration_test.go:87: Processing completed successfully
TestFeedbackProcessingWithRealFirestore: integration_test.go:88: Feedback count: 3

=== STATEMENT COMPARISON ===
TestFeedbackProcessingWithRealFirestore: integration_test.go:94: Current statement:
I'm a software engineer with 5 years of experience in Python and web development.

TestFeedbackProcessingWithRealFirestore: integration_test.go:99: New statement:
I'm a software engineer with 5 years of experience in Python and web development. I strongly prefer remote work opportunities and flexible arrangements that allow me to work from home. I'm particularly interested in roles involving Go programming and backend development. I want to avoid positions that require daily in-office meetings or rigid on-site requirements.

=== STEERING QUERY COMPARISON ===
TestFeedbackProcessingWithRealFirestore: integration_test.go:105: Current steering query - Move toward: software engineering python web development
TestFeedbackProcessingWithRealFirestore: integration_test.go:106: Current steering query - Force away: 

TestFeedbackProcessingWithRealFirestore: integration_test.go:111: New steering query - Move toward: software engineering python web development remote work golang backend
TestFeedbackProcessingWithRealFirestore: integration_test.go:112: New steering query - Force away: office required in-person meetings

=== TEST COMPLETED ===
TestFeedbackProcessingWithRealFirestore: integration_test.go:116: No changes were saved to Firestore
```

### Safety Features:

- **Read-only mode**: Uses a `readOnlyFeedbackStore` wrapper that captures updates without saving them
- **No data modification**: Your Firestore data remains unchanged
- **Clear logging**: Shows exactly what would be changed
- **Error isolation**: Failures don't affect your production data

### Use Cases:

- **Testing with real user data** to validate the feedback processing pipeline
- **Debugging feedback analysis** with actual user preferences
- **Validating LLM output quality** with real-world scenarios
- **Performance testing** with realistic data volumes
- **Quality assurance** before deploying changes

## Timeouts and Performance

- **Mock data tests**: 2 minute timeout
- **Statement update tests**: 1 minute timeout
- **Real Firestore tests**: 5 minute timeout (allows for larger datasets)

Integration tests make real API calls and can be slow. They are automatically skipped in:
- Short test mode (`go test -short`)
- CI environments (when `CI` environment variable is set)

## Troubleshooting

### Common Issues:

1. **"TEST_USER_ID environment variable not set"**
   - Set the environment variable to a real user ID from your Firestore database

2. **"No unprocessed feedback found"**
   - The user has no unprocessed feedback items
   - Try a different user ID with recent feedback

3. **Firestore connection errors**
   - Check your `FIREBASE_PROJECT_ID` and `FIREBASE_CREDENTIALS`
   - Ensure credentials have read access to the feedback collections

4. **LLM API errors**
   - Check your API keys are set correctly
   - Verify you have sufficient API credits

5. **Timeout errors**
   - Large datasets may need longer timeouts
   - Consider testing with users who have fewer feedback items

### Debug Tips:

- Use `-v` flag for verbose output
- Check logs for detailed processing information
- Verify user exists and has feedback data before running the test
- Test with users who have varying amounts of feedback

## API Keys and Credits

Integration tests make real LLM API calls which consume credits:
- **DeepSeekChat**: Typically $0.001-0.01 per test run
- **OpenAI GPT-4**: Typically $0.01-0.10 per test run

Monitor your API usage when running integration tests frequently.

## Development Workflow

1. **Unit Tests First**: Always run unit tests before integration tests
   ```bash
   go test ./internal/feedback/... -v
   ```

2. **Integration Tests**: Run when you need to verify real LLM behavior
   ```bash
   go test -tags=integration ./internal/feedback/... -v
   ```

3. **Full Build**: Ensure everything compiles
   ```bash
   go build -o /tmp/talent-rodeo ./cmd/web
   ```

## Phase 2 Completion Criteria

✅ **Integration Tests Created**
✅ **Real LLM Testing Framework**
✅ **End-to-End Pipeline Testing**
🚧 **Daily Batch Processing** (Next step)
🚧 **Production Monitoring** (Next step)

The integration testing framework is now complete and ready for Phase 2 validation! 