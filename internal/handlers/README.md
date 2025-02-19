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

4. **Mock Implementations**
   - `mockLLM` in `common_test.go` provides a reusable LLM mock
   - Supports both single and queued responses
   - Implements full `minds.ContentGenerator` interface
   ```go
   // Example usage:
   env := newTestEnv(t)
   env.llm.SetResponse("mock response")
   // or
   env.llm.SetResponses([]string{"response1", "response2"})
   ```

5. **Test Environment**
   - `testEnv` struct provides a complete test environment
   - Includes NATS, JetStream, mock LLM, and logger
   - Handles cleanup automatically
   - Example:
   ```go
   env := newTestEnv(t)
   defer env.cleanup()
   
   handler := NewGenericHandler(env.js, env.logger, env.llm)
   ```

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

## LLM Processing Pattern

Many handlers in this package follow a consistent pattern for LLM-based content processing. The following handlers implement this pattern:

- AssessmentHandler
- CultureHandler
- ExperienceHandler
- PreferencesHandler
- RequirementsHandler
- SkillsHandler
- WorkplaceHandler
- CompensationHandler

The one that does not is: SearchHandler

### Common Pattern Structure

1. **Schema Definition**
   - Each handler defines a Go struct that serves as both the schema for LLM output and the message type
   - Structs use JSON tags and description tags to guide LLM output
   ```go
   type ExampleSchema struct {
       Field1 string   `json:"field1" description:"Description of what this field means"`
       Field2 []string `json:"field2" description:"Description of what this array contains"`
   }
   ```

2. **System Prompt**
   - Each handler defines a static prompt string that guides the LLM's analysis
   - Prompts are constants defined at the package level
   ```go
   const examplePrompt = `You are an expert analyzer. Your task is to...`
   ```

3. **LLM Processor Interface**
   ```go
   type LLMProcessor interface {
       // GetSystemPrompt returns the system prompt for LLM processing
       GetSystemPrompt() string
       // GetResponseSchema returns the schema for LLM response
       GetResponseSchema() (*minds.ResponseSchema, error)
       // GetOutputTopic returns the topic to publish LLM processed results
       GetOutputTopic() string
   }
   ```

4. **Message Processing Flow**
   ```go
   func (h *Handler) extract(ctx context.Context, input string) (*Schema, error) {
       // 1. Create response schema
       schema, err := h.GetResponseSchema()
       if err != nil {
           return nil, fmt.Errorf("failed to create schema: %w", err)
       }
       
       // 2. Construct messages array with system prompt and user input
       messages := minds.Messages{
           {Role: minds.RoleSystem, Content: h.GetSystemPrompt()},
           {Role: minds.RoleUser, Content: input},
       }
       
       // 3. Create request with schema
       request := minds.NewRequest(messages, minds.WithResponseSchema(*schema))
       
       // 4. Generate content using LLM
       result, err := h.llm.GenerateContent(ctx, request)
       if err != nil {
           return nil, fmt.Errorf("failed to generate content: %w", err)
       }
       
       // 5. Parse JSON response into schema struct
       var output Schema
       if err := json.Unmarshal([]byte(result.String()), &output); err != nil {
           return nil, fmt.Errorf("failed to unmarshal response: %w", err)
       }
       
       return &output, nil
   }
   ```

### Error Handling

The LLM processing pattern includes comprehensive error handling:

1. **Schema Creation Errors**
   - Errors from `minds.NewResponseSchema` are propagated through the interface
   - Each handler can provide context-specific error messages
   - Schema validation errors are caught early in the process

2. **LLM Processing Errors**
   - Content generation errors are wrapped with context
   - Response parsing errors include details about the schema
   - All errors are logged with appropriate context

3. **Message Processing**
   - Failed messages are terminated with error context
   - Successful processing is acknowledged
   - Error recovery follows NATS JetStream patterns

### Implementation Example

```go
type ExampleHandler struct {
    llm     minds.ContentGenerator
    logger  *slog.Logger
    generic *GenericHandler
}

func (h *ExampleHandler) GetSystemPrompt() string {
    return "You are an expert analyzer..."
}

func (h *ExampleHandler) GetResponseSchema() (*minds.ResponseSchema, error) {
    return minds.NewResponseSchema(
        "ExampleSchema",
        "Example schema for LLM processing",
        ExampleSchema{},
    )
}

func (h *ExampleHandler) GetOutputTopic() string {
    return "rodeo.example.processed"
}
```

### Planned GenericHandler Enhancement

To centralize this pattern, we should modify `GenericHandler` to support LLM processing:

1. **Interface Requirements**
   ```go
   type LLMProcessor interface {
       GetSystemPrompt() string
       GetResponseSchema() interface{}
   }
   ```

2. **GenericHandler Updates**
   - Add LLM field to GenericHandler
   - Add methods for LLM processing
   - Support both raw message handling and LLM processing

3. **Implementation Plan**
   a. Update GenericHandler struct to include LLM
   b. Add new registration method for LLM handlers
   c. Add helper methods for schema creation and content generation
   d. Modify existing handlers to use new interface
   e. Update tests to cover new functionality

4. **Migration Strategy**
   - Create new methods alongside existing ones
   - Gradually migrate handlers to new pattern
   - Maintain backward compatibility
   - Add comprehensive tests for new functionality

This enhancement will reduce code duplication and standardize LLM processing across all handlers. 