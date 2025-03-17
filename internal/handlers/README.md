# Handler Patterns

This document outlines the consistent patterns used across handlers in the `internal/handlers` package.

## Handlers Overview

The `internal/handlers` package contains handlers for various message types processed through NATS JetStream. The package follows a consistent pattern for message handling, with specializations for LLM-based processing and job search functionality.

### Key Handlers

| Handler | Purpose | File |
|---------|---------|------|
| GenericHandler | Base handler providing common NATS functionality | `generic_handler.go` |
| SearchHandler | Processes job search requests | `search.go` |
| AssessmentHandler | Analyzes assessment data with LLM | `assessment.go` |
| CultureHandler | Processes culture fit data with LLM | `culture.go` |
| CompensationHandler | Handles compensation requirements with LLM | `compensation.go` |
| ExperienceHandler | Processes experience data with LLM | `experience.go` |
| PreferencesHandler | Handles job preferences with LLM | `preferences.go` |
| RequirementsHandler | Processes job requirements with LLM | `requirements.go` |
| SkillsHandler | Analyzes skills data with LLM | `skills.go` |
| WorkplaceHandler | Processes workplace preferences with LLM | `workplace.go` |
| VoiceHandler | Manages voice-related functionality | `voice.go` |

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

## Testing Tools and Patterns

### Test Harness

The package provides a robust test harness in `test_harness.go` to simplify testing handlers:

```go
// SendAndReceiveMessage is a generic test harness function that publishes an input message of type T
// to a given subject and waits for a response message of type R from an existing consumer
func SendAndReceiveMessage[T any, R any](
    t *testing.T, 
    ctx context.Context, 
    js jetstream.JetStream, 
    streamName string, 
    publishSubject, 
    responseSubject string, 
    input T, 
    fetchTimeout time.Duration, 
    consumerName string
) (R, error)
```

Key features:
- Generic types for input and output messages
- Automatic consumer creation and configuration
- Message marshaling and unmarshaling
- Timeout handling
- Detailed test logging

Usage example:
```go
result, err := SendAndReceiveMessage(
    t, ctx, js, "RODEO", 
    "rodeo.skills.input", "rodeo.skills.processed",
    inputMessage, 5*time.Second, "test-consumer"
)
```

### Common Test Utilities

The `common_test.go` file provides several critical testing utilities:

1. **testEnv - Test Environment**
   ```go
   type testEnv struct {
       db       *database.DB
       js       jetstream.JetStream
       nc       *nats.Conn
       llm      *mockLLM
       logger   *slog.Logger
       ctx      context.Context
       cancel   context.CancelFunc
       cleanups []func()
   }
   ```
   
   - Provides a complete test environment with all dependencies
   - Automatically sets up NATS server, JetStream, mock LLM
   - Handles cleanup with defer env.cleanup()
   - Created with `env := newTestEnv(t)`

2. **mockLLM - LLM Mock**
   ```go
   type mockLLM struct {
       responses map[string]string
       queue     []string
       index     int
   }
   ```
   
   Key functions:
   - `SetResponse(response string)` - Sets single response
   - `SetResponses(responses []string)` - Sets multiple responses in sequence
   - Implements full `minds.ContentGenerator` interface

3. **Helper Functions**
   - `deleteConsumers` - Cleans up test consumers
   - `deleteStreams` - Removes test streams
   - Various setup and teardown utilities

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
   - Examples: `search_integration_test.go`, `voice_integration_test.go`

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

The SearchHandler uses a different pattern as it processes job search requests rather than LLM content.

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

### Search Handler Pattern

The SearchHandler follows a different pattern:

1. **Job Search Processing**
   - Consumes messages from `rodeo.search.input`
   - Uses the configured search provider to perform job searches
   - Constructs search criteria from input message
   - Scores job results based on relevance to candidate requirements
   - Publishes results to `rodeo.search.result`

2. **Implementation**
   ```go
   func (h *SearchHandler) handleMessage(ctx context.Context, msg jetstream.Msg) error {
       // Unmarshal search input
       // Construct search criteria
       // Perform search
       // Score results
       // Publish results
   }
   ```

3. **Testing**
   - Integration tests cover end-to-end search flow
   - Unit tests validate scoring and criteria construction
   - Mock search providers for repeatable tests 