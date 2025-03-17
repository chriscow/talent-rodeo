# Task Handlers

This directory contains specialized handlers for processing various types of background tasks in the Talent Rodeo platform. Each handler implements the `TaskHandler` interface and provides task-specific functionality.

## Handler Overview

### SearchScheduler
- **File**: `search_scheduler.go`
- **Purpose**: Manages automated job searches for candidates based on their subscription tier
- **Key Features**:
  - Tier-based scheduling (Free, Mid, Top) with different search frequencies
  - Page limit enforcement based on subscription tier
  - Automatic rescheduling of recurring searches
  - NATS message publishing for search execution
  - Transaction-safe task processing

### NATSMessageScheduler
- **File**: `message_scheduler.go`
- **Purpose**: Schedules and publishes messages to NATS subjects at specified times
- **Key Features**:
  - Delayed message publishing
  - Structured message data with metadata
  - Error handling and logging
  - JetStream integration

### PhoneCallHandler
- **File**: `phone_call.go`
- **Purpose**: Manages scheduled phone calls through Twilio integration
- **Key Features**:
  - Call scheduling based on user timezone
  - TwiML template support
  - Onboarding step tracking
  - Call status management
  - Error handling and retries

## Common Interfaces

### TaskProcessor
```go
type TaskProcessor interface {
    EnqueueTask(ctx context.Context, task *internal.Task) error
}
```

### Publisher
```go
type Publisher interface {
    Publish(ctx context.Context, subject string, data []byte) error
}
```

### TwilioClient
```go
type TwilioClient interface {
    MakeCall(to, from, twiml string) (string, error)
    GetCallStatus(sid string) (string, error)
}
```

## Handler Implementation Guide

To implement a new task handler:

1. Create a new file in the handlers directory
2. Implement the `TaskHandler` interface:
   ```go
   type TaskHandler interface {
       ProcessTask(ctx context.Context, task *internal.Task) error
       TaskType() string
   }
   ```
3. Define a task-specific payload structure
4. Implement constructor and helper methods
5. Add comprehensive tests

## Testing Approach

Each handler includes comprehensive test files:
- Unit tests for core functionality
- Integration tests with databases
- Edge case handling
- Tier-specific behavior (for SearchScheduler)
- Mock implementations for external dependencies

## Best Practices

1. **Transaction Safety**: Always process tasks within a transaction when updating database state
2. **Error Handling**: Implement proper error handling with appropriate retries
3. **Logging**: Use structured logging with task context for debugging
4. **Idempotency**: Design handlers to be idempotent to handle duplicate task processing
5. **Testing**: Test with realistic data and edge cases
6. **Mocking**: Use interfaces for external dependencies to facilitate testing 