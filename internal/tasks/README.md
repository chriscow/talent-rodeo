# Tasks Package

This package implements a robust background task processing system for the Talent Rodeo application, with a focus on reliable task execution and phone call handling.

## Overview

The tasks package provides a flexible and reliable system for processing background tasks, with built-in support for retries, transaction management, and task-specific handlers. It includes specialized support for phone call scheduling and execution through Twilio integration.

## Files

### processor.go
Core task processing functionality:
- Task queue management
- Handler registration
- Transaction handling
- Retry logic
- State management

### processor_test.go
Comprehensive tests for:
- Task processing
- Error handling
- Retry behavior
- Transaction management
- Handler integration

## Directories

### handlers/
Task-specific handlers including:
- Phone call management
- Twilio integration
- Call scheduling
- Status tracking
- Error handling

## Key Features

1. Task Processing
   - Asynchronous execution
   - Automatic retries
   - Transaction safety
   - Status tracking
   - Error handling

2. Phone Call Management
   - Call scheduling
   - Status monitoring
   - Twilio integration
   - Timezone handling
   - Failure recovery

3. Handler System
   - Pluggable handlers
   - Type safety
   - Context support
   - Configuration management
   - Logging integration

## Usage

Initialize task processor:
```go
processor := tasks.NewProcessor(db, logger)
processor.RegisterHandler(phoneCallHandler)
processor.Start(ctx)
```

Schedule a phone call:
```go
params := handlers.ScheduleCallParams{
    PhoneNumber: "+1234567890",
    Message:     "Hello, this is your reminder call",
    ScheduledAt: time.Now().Add(1 * time.Hour),
    TimeZone:    "America/New_York",
}
err := phoneCallHandler.ScheduleCall(ctx, params)
```

## Task Lifecycle

1. Task Creation
   - Task type assignment
   - Payload serialization
   - Schedule setting
   - Initial state setup

2. Task Processing
   - Handler selection
   - Transaction start
   - Status updates
   - Error handling
   - Retry management

3. Task Completion
   - Status finalization
   - Transaction commit
   - Resource cleanup
   - Logging
   - Metrics update

## Handler Implementation

Implement TaskHandler interface:
```go
type TaskHandler interface {
    ProcessTask(ctx context.Context, task *models.Task) error
    TaskType() string
}
```

Example handler:
```go
type CustomHandler struct {
    // Dependencies
}

func (h *CustomHandler) TaskType() string {
    return "custom_task"
}

func (h *CustomHandler) ProcessTask(ctx context.Context, task *models.Task) error {
    // Implementation
}
```

## Error Handling

The package provides comprehensive error handling:
- Task-level errors
- Transaction errors
- Handler errors
- Retry management
- Status tracking

## Best Practices

1. Task Design
   - Keep tasks idempotent
   - Include necessary context
   - Handle partial failures
   - Plan for retries
   - Log key events

2. Handler Implementation
   - Validate inputs
   - Handle timeouts
   - Clean up resources
   - Report progress
   - Maintain state

3. Error Management
   - Use appropriate status codes
   - Include error context
   - Log failures
   - Track metrics
   - Monitor retries 