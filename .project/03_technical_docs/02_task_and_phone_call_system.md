# Task Processing System

The task processing system in Talent Rodeo provides a robust background task execution framework for handling asynchronous operations. It is implemented in the `internal/tasks` package.

## Overview

The task processing system allows for the scheduling and execution of background tasks with support for:
- Automatic retries
- Transaction safety
- Status tracking
- Error handling
- Pluggable handlers

## Core Components

### Task Data Structure (`internal/task.go`)

The `Task` struct represents a background job to be processed:

```go
type Task struct {
    ID          int64           `db:"id" json:"id"`
    Type        string          `db:"type" json:"type"`
    Payload     json.RawMessage `db:"payload" json:"payload"`
    Status      string          `db:"status" json:"status"`
    Attempts    int             `db:"attempts" json:"attempts"`
    MaxAttempts int             `db:"max_attempts" json:"max_attempts"`
    RunAt       time.Time       `db:"run_at" json:"run_at"`
    Error       string          `db:"error" json:"error"`
    CreatedAt   time.Time       `db:"created_at" json:"created_at"`
    UpdatedAt   time.Time       `db:"updated_at" json:"updated_at"`
}
```

Task statuses include:
- `pending`: Task is waiting to be processed
- `running`: Task is currently being processed
- `complete`: Task completed successfully
- `failed`: Task failed and may be retried
- `cancelled`: Task was manually cancelled
- `retrying`: Task is scheduled for retry

### Task Processor (`internal/tasks/processor.go`)

The `Processor` is the core component responsible for:
- Polling the database for pending tasks
- Dispatching tasks to the appropriate handlers
- Managing task state
- Handling errors and retries
- Transaction management

### Task Handler Interface

Handlers implement the `TaskHandler` interface:

```go
type TaskHandler interface {
    ProcessTask(ctx context.Context, task *models.Task) error
    TaskType() string
}
```

## Key Task Handlers

### Search Scheduler

Located in `internal/tasks/handlers/search_scheduler.go`, this handler manages job search scheduling with different frequencies based on user subscription tiers:

- Free tier: Searches every 72 hours (3 days)
- Mid tier: Searches every 24 hours (daily)
- Top tier: Searches every 6 hours (4 times daily)

The handler also enforces page limits:
- Free tier: Maximum 3 pages of search results
- Mid tier: Maximum 50 pages of search results
- Top tier: Maximum 200 pages of search results

### NATS Message Scheduler

Located in `internal/tasks/handlers/message_scheduler.go`, this handler is a critical component in the system's asynchronous communication architecture. It:

- Handles delayed message publishing to NATS JetStream
- Provides structured message data and metadata support
- Enables scheduling of messages for future delivery
- Works with a standard payload structure (`NATSMessagePayload`)

The scheduler handles specific responsibilities:
- Validation of message payload (subject, data)
- Storing task ID in message metadata for tracing
- Formatting messages in a standardized envelope
- Publishing to appropriate NATS subjects

#### Message Structure
```go
// NATSMessagePayload represents the data needed to publish a NATS message
type NATSMessagePayload struct {
    Subject  string            `json:"subject"`  // The subject to publish to
    Data     json.RawMessage   `json:"data"`     // The data to publish
    Metadata map[string]string `json:"metadata"` // Optional metadata for the message
}

// MessageData wraps the actual message data with metadata
type MessageData struct {
    Data     json.RawMessage   `json:"data"`
    Metadata map[string]string `json:"metadata"`
}
```

### Automated Call Scheduling

The system uses a combination of components to implement automated outbound calling:

1. **Appointment Database**: Stores scheduled call appointments with timestamps
2. **Task Processor**: Monitors database and processes tasks at scheduled times
3. **NATS Message Scheduler**: Publishes call requests to NATS topics
4. **Voice Handler**: Subscribes to NATS topics and initiates Twilio calls

#### Call Scheduling Workflow

The detailed workflow for scheduling and executing calls is:

1. **User Interface**:
   - User submits the appointment form in `profile-schedule-phone-call.html`
   - Form posts to `/profile/schedule/appointment` endpoint
   - The form includes datetime selection and timezone information

2. **API Handler**:
   - `createAppointment` in `cmd/web/handlers.schedule.go` processes the request
   - Performs validation:
     - Verifies appointment is in the future (at least 5 minutes)
     - Ensures appointment is not too far in the future (max 14 days)
     - Validates appointment is during business hours (9 AM - 8 PM)
     - Checks that user has a valid phone number
   - Creates appointment record in database
   - Updates user onboarding state to indicate call is scheduled

3. **Message Scheduling**:
   - Creates a task with NATS message payload
   - The task type is "nats_message" which is handled by the MessageScheduler
   - The task is scheduled to run at the appointment time
   - The payload includes:
     - Topic: `handlers.TopicVoiceInput`
     - Data: Phone number, appointment ID, user ID, and metadata
     - Metadata: Source information and appointment ID for tracing

4. **Task Processing**:
   - When the appointment time arrives, the task processor executes the task
   - The MessageScheduler handler processes the task and publishes to NATS
   - The message is published to the `rodeo.voice.input` topic

5. **Voice Handler**:
   - The Voice Handler in `internal/handlers/voice.go` subscribes to the topic
   - When it receives a message, it extracts phone number and metadata
   - It uses the Twilio client to initiate the actual phone call
   - After the call is made, it publishes results back to NATS on `rodeo.voice.result`
   - It updates the appointment status based on call results

#### Error Handling and Resilience

The system implements several layers of error handling and resilience:

1. **Input Validation**:
   - Client-side validation in the scheduling form
   - Server-side validation in the createAppointment handler
   - Timezone-aware business hours validation
   - Phone number format validation

2. **Error Handling**:
   - Structured error responses to the client
   - Appointment status updates on failures
   - Graceful handling of database errors
   - Retry mechanisms for transient failures

3. **Observability**:
   - Structured logging throughout the call lifecycle
   - Metrics for call scheduling success rates and errors
   - Traceability via consistent appointment ID across components
   - Error aggregation for monitoring

4. **Resilience Features**:
   - Task processor provides automatic retries for failed tasks
   - NATS JetStream ensures reliable message delivery
   - Transaction safety for database operations
   - Graceful handling of service disruptions

### Call Message Structure

The voice call message published to NATS has the following structure:

```go
// Voice call message structure
{
    "to": "+15551234567",             // Phone number to call
    "appointment_id": 123,            // Database ID of the appointment
    "user_id": 456,                   // User ID associated with the call
    "metadata": {
        "appointment_id": "123",      // String version for easier processing
        "scheduled_time": "2023-11-15T14:30:00Z",  // ISO8601 format
        "source": "appointment-scheduler"          // Component that initiated the call
    }
}
```

The Voice Handler implements:
- Processing incoming NATS messages with call requests
- Initiating calls via the Twilio client
- Handling errors during the call process
- Publishing call results back to NATS
- Structured logging throughout the process

### Phone Call Handler

Manages Twilio-based phone call scheduling, status monitoring, and failure recovery.

## Database Integration

Tasks are stored in the PostgreSQL database with functions in `internal/database/task.go` for:
- Creating tasks
- Updating task status
- Querying pending tasks
- Managing task retries

Important database constraints:
- The `run_at` timestamp must be greater than or equal to `created_at`
- All timestamps must be in UTC with microsecond precision

## Task Lifecycle

1. **Creation**:
   - Task is created with a specific type and payload
   - Status is set to `pending`
   - `run_at` time is set to when the task should execute

2. **Processing**:
   - Processor queries for pending tasks with `run_at` <= current time
   - Task status is updated to `running`
   - The appropriate handler is selected based on task type
   - Handler processes the task within a database transaction

3. **Completion/Failure**:
   - On success, status is updated to `complete`
   - On failure, error is recorded and status is set to `failed`
   - If retries are available, status is set to `retrying` and `run_at` is updated with backoff

## Communication Patterns

The task system uses two primary communication patterns:

1. **Direct Database Access**:
   - Used for task status tracking and management
   - Provides strong consistency and transaction support
   - Used for scheduling and monitoring task execution

2. **NATS Messaging**:
   - Used for service-to-service communication
   - Provides decoupling between task scheduling and execution
   - Enables specialized handlers for different message types
   - Used particularly for Twilio call integration

## Best Practices

- Tasks should be idempotent (can be safely retried)
- Transaction safety ensures database consistency
- Handlers should handle timeouts gracefully
- Error contexts should be preserved for debugging
- Task payloads should be small and serializable

## Files

- Core processor: `internal/tasks/processor.go`
- Task data structure: `internal/task.go`
- Database operations: `internal/database/task.go`
- Handler implementations: `internal/tasks/handlers/`
- Message scheduler: `internal/tasks/handlers/message_scheduler.go`
- Voice handler: `internal/handlers/voice.go`
- Tests: `internal/tasks/processor_test.go`
- Message scheduler tests: `internal/tasks/handlers/message_scheduler_test.go` 