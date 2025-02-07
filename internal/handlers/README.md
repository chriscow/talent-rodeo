# Handler Patterns

This document outlines the consistent patterns used across handlers in the `internal/handlers` package.

## Handler Structure

### Core Components
Each handler follows a consistent structure with these components:

1. **Constants and Types**
   - Topic constants defined at the top (e.g., `TopicSkillsExtracted`)
   - System prompts when LLM is used
   - Clear type definitions with JSON tags and descriptions

2. **Handler Struct**
   ```go
   type HandlerName struct {
       ctx      context.Context
       js       jetstream.JetStream
       llm      minds.ContentGenerator  // if LLM is needed
       logger   *slog.Logger
       consumer jetstream.Consumer
   }
   ```

3. **Constructor Pattern**
   ```go
   func NewHandlerName(ctx context.Context, js jetstream.JetStream, ...) (*HandlerName, error) {
       consumer, err := getConsumer(ctx, js, streamName, consumerName, topicName)
       if err != nil {
           return nil, fmt.Errorf("failed to get consumer: %w", err)
       }
       return &HandlerName{...}, nil
   }
   ```

### Message Processing Flow

1. **Start Method**
   - Runs in an infinite loop until context is cancelled
   - Handles errors gracefully without stopping the handler
   - Logs start/stop events

2. **Message Processing**
   - Uses `processNextMessage` for message fetching
   - Implements `handleMessage` for business logic
   - Proper error handling and logging at each step

3. **Error Handling**
   - Wraps errors with context using `fmt.Errorf("failed to...: %w", err)`
   - Logs errors with appropriate context
   - Graceful error recovery to continue processing

4. **Logging**
   - Verbose debug logging for state tracking
   - Consistent log levels (Info for lifecycle, Debug for operations, Error for issues)
   - Structured logging with relevant fields

## Testing Patterns

### Test Structure

1. **Test Setup**
   - Uses table-driven tests
   - Common test harness for handler setup
   - Mock implementations for external dependencies

2. **Test Cases**
   ```go
   func TestHandlerName(t *testing.T) {
       tests := []struct {
           name     string
           input    InputType
           want     OutputType
           wantErr  bool
       }{
           // test cases...
       }
       
       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               got, err := handler.Process(tt.input)
               if (err != nil) != tt.wantErr {
                   t.Errorf("unexpected error: %v", err)
               }
               if !reflect.DeepEqual(got, tt.want) {
                   t.Errorf("got %v, want %v", got, tt.want)
               }
           })
       }
   }
   ```

3. **Integration Tests**
   - Separate `_integration_test.go` files
   - Tests full message flow through NATS
   - Verifies end-to-end functionality

4. **Mock Implementations**
   - Interface-based design for external dependencies
   - Mock implementations in test files
   - Consistent mock patterns across handlers

### Testing Best Practices

1. **Coverage**
   - Tests for happy path and error cases
   - Boundary condition testing
   - Error injection testing

2. **Assertions**
   - Uses standard library testing
   - Follows "got, want" pattern
   - Clear error messages with context

3. **Test Helpers**
   - Common setup/teardown functions
   - Shared test utilities
   - Clear documentation of helper functions

## Common Interfaces

Handlers implement these common interfaces:

```go
type Handler interface {
    Start(context.Context) error
    Close() error
}
```

## Message Flow

1. **Input**
   - Messages received from NATS JetStream
   - JSON unmarshaling into typed structs
   - Validation of input data

2. **Processing**
   - Business logic in separate methods
   - Clear separation of concerns
   - Error handling at each step

3. **Output**
   - Results published back to NATS
   - Consistent JSON marshaling
   - Proper message acknowledgment 