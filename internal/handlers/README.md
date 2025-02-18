# Handler Patterns

This document outlines the consistent patterns used across handlers in the `internal/handlers` package.

## Handler Structure

### Core Components

1. **Generic Handler**
   - Provides common functionality for message handling
   - Manages NATS JetStream consumers and message processing
   - Handles context cancellation and error recovery
   ```go
   type GenericHandler struct {
       js     jetstream.JetStream
       logger *slog.Logger
   }
   ```

2. **Specific Handlers**
   - Implement business logic for specific message types
   - Use GenericHandler for message processing
   - Define message types and constants
   ```go
   type SpecificHandler struct {
       db      *database.DB        // if database access needed
       llm     minds.ContentGenerator  // if LLM is needed
       logger  *slog.Logger
       generic *GenericHandler
   }
   ```

3. **Message Types**
   - Clear type definitions with JSON tags and descriptions
   - Input/Output structs for each handler
   - Validation and business rules

### Message Processing Flow

1. **Handler Registration**
   ```go
   func NewSpecificHandler(ctx context.Context, js jetstream.JetStream, ...) (*SpecificHandler, error) {
       handler := &SpecificHandler{
           generic: NewGenericHandler(js, logger),
           // ... other dependencies
       }
       
       // Register message handlers
       err := handler.generic.Register(ctx, streamName, subject, durable, handler.handleMessage)
       if err != nil {
           return nil, fmt.Errorf("failed to register handler: %w", err)
       }
       
       return handler, nil
   }
   ```

2. **Message Processing**
   - Implement `handleMessage` callback for business logic
   - Process messages with proper error handling
   - Use structured logging for debugging
   ```go
   func (h *SpecificHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
       // Unmarshal message
       // Process business logic
       // Return error if processing fails
   }
   ```

3. **Error Handling**
   - Wraps errors with context using `fmt.Errorf("failed to...: %w", err)`
   - Logs errors with appropriate context
   - Messages are automatically terminated on error or acknowledged on success

## Testing Patterns

### Test Structure

1. **Handler Tests**
   - Test specific handler business logic
   - Mock dependencies (database, LLM, etc.)
   - Table-driven tests for various scenarios

2. **Test Cases**
   ```go
   func TestSpecificHandler_HandleMessage(t *testing.T) {
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
               // Setup handler with mocks
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
   - Test full message flow through NATS
   - Verify end-to-end functionality
   - Test consumer creation and message processing

### Testing Best Practices

1. **Coverage**
   - Test happy path and error cases
   - Test message handling timeouts
   - Test context cancellation

2. **Assertions**
   - Use standard library testing
   - Follow "got, want" pattern
   - Clear error messages with context

3. **Test Helpers**
   - Mock message implementations
   - Common setup/teardown functions
   - Shared test utilities

## Message Flow

1. **Registration**
   - Handler registers with GenericHandler
   - Consumer created/updated for subject
   - Message processing goroutine started

2. **Processing**
   - Messages fetched with timeout
   - Business logic executed
   - Automatic acknowledgment handling

3. **Cleanup**
   - Context cancellation handled
   - Resources cleaned up properly
   - Error recovery and backoff implemented 