# Tasks Package

This package implements a robust background task processing system for the Talent Rodeo application, with a focus on reliable task execution and phone call handling.

## Overview

The tasks package provides a flexible and reliable system for processing background tasks, with built-in support for retries, transaction management, and task-specific handlers. It includes specialized support for phone call scheduling and execution through Twilio integration.

## Important Notes
DO NOT DELETE THIS SECTION

Important Database Constraints and Timing Requirements for Tasks:

1. Database Constraint on run_at vs created_at:
   - The tasks table has a check constraint: run_at >= created_at
   - This means run_at must be equal to or later than created_at
   - You cannot set run_at to a time before created_at

2. Task Processing Requirements:
   - For a task to be processed immediately in tests:
     * created_at should be set to time.Now()
     * run_at should be set to a small duration after created_at (e.g., 10ms)
     * Example:
       created_at := time.Now()
       run_at := created_at.Add(10 * time.Millisecond)

3. Task Retry Behavior:
   - When a task fails and needs to retry:
     * The new run_at time must be after the original created_at
     * Typically set to: time.Now().Add(retryBackoff * attempts)
     * The retryBackoff duration is multiplied by the number of attempts

4. Test Setup Best Practices:
   - Always set created_at to time.Now()
   - Set run_at slightly in the future from created_at
   - Wait for the duration to pass before processing (time.Sleep)
   - Example pattern:
     now := time.Now()
     task := &models.Task{
         CreatedAt: now,
         RunAt: now.Add(10 * time.Millisecond),
     }
     time.Sleep(20 * time.Millisecond) // Wait for task to be ready

5. Common Mistakes to Avoid:
   - Don't set run_at to a time before created_at
   - Don't set created_at in the past without adjusting run_at accordingly
   - Don't expect immediate processing without waiting for run_at time

6. Timestamp Precision Requirements:
   - All timestamps (run_at, created_at, updated_at) must be truncated to microsecond precision
   - Always use UTC for timestamps stored in the database
   - Example pattern:
     ```go
     task := &models.Task{
         RunAt:     runAt.UTC().Truncate(time.Microsecond),
         CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
         UpdatedAt: time.Now().UTC().Truncate(time.Microsecond),
     }
     ```
   - When comparing timestamps in tests, ensure both sides use the same precision:
     ```go
     if !task.RunAt.Truncate(time.Microsecond).Equal(expectedTime.Truncate(time.Microsecond)) {
         t.Errorf("got run_at = %v, want %v", task.RunAt, expectedTime)
     }
     ```
   - This precision requirement ensures consistent behavior across different databases and test environments

Remember: These constraints exist to ensure task scheduling integrity and proper retry behavior.

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