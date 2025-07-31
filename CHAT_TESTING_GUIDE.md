# Chat Completion Testing Guide

This guide covers the unified chat completion testing framework for validating the hybrid completion system.

## 🚀 Quick Start

The framework is already integrated into the CLI. Here's how to use it:

```bash
# Get help and see all options
./cli chat-test --help

# Run all scenarios once
./cli chat-test

# Run all scenarios multiple times for reliability testing
./cli chat-test --iterations 3

# Run specific scenario (use filename only)
./cli chat-test --scenario basic_conversation.json

# Run with verbose output
./cli chat-test --verbose

# Output results in JSON format
./cli chat-test --json-output
```

## 📋 Available Commands

### Run All Scenarios
```bash
# Run all scenarios once
./cli chat-test --scenario all

# Or simply
./cli chat-test
```

### List Available Scenarios

To see all available scenarios without running them:
```bash
# List all scenario files
find testdata/conversations -name "*.json" | sort

# Or with descriptions
ls -la testdata/conversations/*/
```

### Available Test Scenarios

**Complete Scenarios** (should reach completion):
- `complete/soft_completion_engaged.json` - Soft completion with high engagement
- `complete/successful_comprehensive.json` - Comprehensive successful conversation
- `complete/successful_minimal.json` - Minimal successful conversation
- `complete/user_initiated_end.json` - User initiated early end

**Incomplete Scenarios** (should not complete yet):
- `incomplete/early_conversation.json` - Early stage conversation
- `incomplete/missing_deal_breakers.json` - Missing key information

**Edge Cases** (unusual scenarios):
- `edge/max_attempts_escalation.json` - Maximum attempts reached
- `edge/user_confused.json` - User confused and needs guidance

**Sample Scenarios** (demo conversations):
- `sample/basic_conversation.json` - Basic demo conversation

### Run Specific Scenarios
```bash
# Test specific scenario (use just the filename, not the full path)
./cli chat-test --scenario successful_comprehensive.json

# Test with different models
./cli chat-test --openai-model gpt-4o

# Test with multiple iterations for reliability
./cli chat-test --scenario successful_comprehensive.json --iterations 5

# Test the sample scenario
./cli chat-test --scenario basic_conversation.json --iterations 3
```

**Important**: Use only the filename (e.g., `basic_conversation.json`), not the full path (e.g., `sample/basic_conversation.json`). The CLI will find the file in any subdirectory.

## 🔍 Understanding Results

The framework tests the **hybrid completion system** by:

1. **Loading conversation scenarios** from JSON files
2. **Running them with real OpenAI API calls** (not mocked)
3. **Measuring completion consistency** across multiple runs
4. **Analyzing tool call patterns** and completion types
5. **Detecting anomalies** in LLM behavior

### Sample Output
```
=== LLM Conversation Completion Test Results ===

Model: gpt-4o
Total Scenarios: 9
Total Tests: 9
Overall Success Rate: 88.9%

Results by Category:
- Complete Scenarios: 3/3 passed (100.0%)
- Incomplete Scenarios: 2/3 passed (66.7%)
- Edge Case Scenarios: 3/3 passed (100.0%)

Individual Test Results:

Scenario: Successful Comprehensive Conversation
  ✅ Iteration 1: Expected complete, Got complete (1.2s)
    Tools Called: mark_topic_progress, check_completion_readiness, complete_chat

Scenario: Early Conversation  
  ✅ Iteration 1: Expected incomplete, Got incomplete (0.8s)
    Tools Called: mark_topic_progress, check_completion_readiness
```

## 🎯 What Each Test Validates

### **Hard Completion** (`complete/`)
- All required information gathered with high quality
- Should call: `mark_topic_progress`, `check_completion_readiness`, `complete_chat`
- Expected result: Conversation ends with complete status

### **Soft Completion** (`complete/`)
- Sufficient information for basic job matching
- Should call: `mark_topic_progress`, `check_completion_readiness`, `complete_chat`
- Expected result: Conversation ends with acceptable information

### **Escalated Completion** (`edge/`)
- Maximum attempts reached, complete with available info
- Should call: `check_completion_readiness`, `complete_chat`
- Expected result: Conversation ends despite incomplete information

### **Incomplete** (`incomplete/`)
- Not enough information gathered yet
- Should call: `mark_topic_progress`, `check_completion_readiness`
- Expected result: Conversation continues, no completion

## 📊 Key Metrics

- **Success Rate**: % of tests that produced expected completion type
- **Tool Call Consistency**: How reliably the LLM calls the same tools
- **Completion Consistency**: Whether the same completion type is reached
- **Execution Time**: How long each test takes to complete

## 🔧 Adding New Test Scenarios

Test scenarios are JSON files in `testdata/conversations/` with this structure:

```json
{
  "name": "Your Test Name",
  "description": "What this test validates",
  "expected_outcome": "complete|incomplete|soft_complete|escalated_complete", 
  "expected_tool_calls": ["mark_topic_progress", "check_completion_readiness"],
  "conversation_messages": [
    {
      "role": "system",
      "content": "System prompt with checklist status"
    },
    {
      "role": "assistant", 
      "content": "Hi! I'm Shirely from Talent Rodeo..."
    },
    {
      "role": "user",
      "content": "User response"
    }
  ],
  "checklist_state": {
    "overall_status": "in_progress",
    "completed_topics": ["introduction", "search_strategy"],
    "high_quality_topics": ["search_strategy"],
    "message_count": 8,
    "completion_attempts": 0
  }
}
```

### Valid Completion Types
- `complete` - Hard completion (all requirements met)
- `soft_complete` - Soft completion (sufficient information)
- `escalated_complete` - Escalated completion (max attempts reached)
- `incomplete` - Should not complete yet

### Valid Topics
- `introduction` - Initial greeting and rapport
- `warm_up` - Getting user comfortable
- `deal_breakers` - Things to avoid
- `search_strategy` - Role, skills, preferences
- `work_environment` - Company size, culture, etc.
- `location_preferences` - Remote, hybrid, location
- `validation` - Confirming understanding
- `final_confirmation` - Final check before completion

## 🧪 Development Workflow

### 1. Write Your Test Scenario
```bash
# Create new scenario file
touch testdata/conversations/complete/my_new_test.json
```

### 2. Test Your Scenario
```bash
# Test specific scenario
./cli chat-test --scenario complete/my_new_test.json

# Test multiple times for consistency
./cli chat-test --scenario complete/my_new_test.json --iterations 5
```

### 3. Validate Results
- Check if expected completion type is reached
- Verify tool calls match expectations
- Ensure consistent behavior across runs

### 4. Debug Issues
```bash
# Run with verbose output
./cli chat-test --scenario complete/my_new_test.json --verbose

# Check JSON structure
python -m json.tool testdata/conversations/complete/my_new_test.json
```

## 📈 Interpreting Results

### ✅ **Good Results** (Success Rate > 80%)
- **Consistent completion types**: Same outcome across runs
- **Expected tool calls**: LLM calls the right tools
- **Reasonable timing**: Tests complete in 1-3 seconds

### ⚠️ **Warning Signs** (Success Rate 60-80%)
- **Inconsistent behavior**: Different outcomes between runs
- **Missing tool calls**: LLM doesn't call expected tools
- **Slow execution**: Tests take longer than expected

### ❌ **Issues** (Success Rate < 60%)
- **Wrong completion type**: LLM makes incorrect completion decisions
- **Tool call failures**: LLM fails to call tools properly
- **Timeout errors**: Tests exceed time limits

### Common Fixes
1. **Refine conversation flow**: Make user responses more natural
2. **Adjust checklist state**: Ensure it matches conversation content
3. **Update system prompt**: Provide clearer completion criteria
4. **Add more context**: Help LLM understand conversation progress

## 🛠️ Advanced Usage

### CI/CD Integration
```bash
# Run tests in CI pipeline
./cli chat-test --json-output > test_results.json

# Check for failures
if [ $? -ne 0 ]; then
  echo "Chat completion tests failed"
  exit 1
fi
```

### Performance Monitoring
```bash
# Monitor consistency over time
./cli chat-test --iterations 10 --json-output | jq '.overall_success_rate'

# Track specific scenarios
./cli chat-test --scenario complete/successful_comprehensive.json --iterations 20
```

### Model Comparison
```bash
# Test with different models
./cli chat-test --openai-model gpt-4o --iterations 5
./cli chat-test --openai-model gpt-4o-mini --iterations 5
```

## 🔍 Troubleshooting

### Common Issues

**"OPENAI_API_KEY not set"**
- Ensure API key is in `.env` file
- Check that `.env` file is in project root

**"No scenarios found"**
- Verify `testdata/conversations/` directory exists
- Check that scenario files are valid JSON

**"High failure rate"**
- Review conversation scenarios for realism
- Check if expected outcomes match conversation content
- Verify checklist state is appropriate

**"Inconsistent results"**
- Run more iterations to identify patterns
- Check if LLM behavior varies by time of day
- Consider if conversation scenario is ambiguous

### Debug Steps
1. **Check scenario structure**: Validate JSON format
2. **Review conversation flow**: Ensure realistic user responses
3. **Verify expected outcome**: Match outcome to conversation content
4. **Test individual components**: Run single iteration with verbose output
5. **Compare with working examples**: Use `complete/successful_comprehensive.json` as reference

## 📚 Best Practices

### Writing Good Test Scenarios
1. **Natural conversation flow**: Use realistic, conversational language
2. **Progressive information gathering**: Build information gradually
3. **Clear expected outcomes**: Match expectation to conversation content
4. **Appropriate checklist state**: Align with conversation progress

### Maintaining Test Suite
1. **Regular testing**: Run tests after code changes
2. **Monitor consistency**: Track success rates over time
3. **Update scenarios**: Keep scenarios current with system changes
4. **Document changes**: Note why scenarios were added/modified

### Performance Optimization
1. **Batch testing**: Run multiple scenarios together
2. **Parallel execution**: Use multiple API keys if available
3. **Efficient scenarios**: Keep conversations focused and concise
4. **Smart scheduling**: Run expensive tests during off-peak hours

## 🎯 Framework Architecture

The testing framework consists of:

1. **Integration Tests** (`internal/chat/completion_*_test.go`)
   - Real OpenAI API integration
   - Consistency measurement across multiple runs
   - Tool call tracking and validation

2. **CLI Interface** (`cmd/cli/cmd/cli_appointment.go`)
   - User-friendly command interface
   - Scenario management and execution
   - Result reporting and analysis

3. **Test Data** (`testdata/conversations/`)
   - JSON scenario files organized by category
   - Realistic conversation flows
   - Expected outcomes and tool calls

This framework validates that the hybrid completion system works reliably with real LLM interactions, ensuring consistent and appropriate completion decisions across various conversation scenarios.

## 🚀 Next Steps

1. **Run the existing tests**: `./cli chat-test`
2. **Create your own scenarios**: Add files to `testdata/conversations/`
3. **Monitor consistency**: Run with multiple iterations
4. **Integrate with CI/CD**: Add to automated testing pipeline
5. **Track performance**: Monitor success rates over time

The framework is production-ready and provides comprehensive validation of the hybrid completion system! 🎉