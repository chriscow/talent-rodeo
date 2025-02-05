# Task Processing System

This package implements a robust task processing system with a focus on scheduled outbound phone calls using Twilio.

## Architecture

### Core Components

1. **Task Processor**
   - Manages background task execution
   - Handles retries and error recovery
   - Provides transaction support
   - Implements task scheduling
   - Debug logging throughout

2. **Task Handlers**
   - Implement specific task types
   - Each handler focuses on one task type
   - Handlers are registered with the processor

### Outbound Calling

The system implements outbound calling through the `PhoneCallHandler` which:
- Schedules calls for future execution
- Manages Twilio integration
- Handles call status tracking
- Provides retry logic for failed calls

#### Making an Outbound Call

```go
params := handlers.ScheduleCallParams{
    PhoneNumber: "+1234567890",
    Message:     "Your interview is scheduled for tomorrow",
    ScheduledAt: time.Now().Add(1 * time.Hour),
    TimeZone:    "America/New_York",
}

err := phoneCallHandler.ScheduleCall(ctx, params)
```

#### Task Flow
1. Call is scheduled via `ScheduleCall`
2. Task is created with status `TaskStatusPending`
3. Processor picks up task at scheduled time
4. Task status changes to `TaskStatusRunning`
5. Twilio API call is made
6. Status is monitored for up to 30 seconds
7. Task completes or retries based on call outcome

### Testing

The package provides both mock and real implementations:

1. **Mock Testing**
   ```go
   // Use MockTwilioClient for unit tests
   mockClient := handlers.NewMockTwilioClient()
   handler := handlers.NewPhoneCallHandler(mockClient, logger, config)
   ```

2. **Integration Testing**
   ```go
   // Use RealTwilioClient for integration tests
   realClient := handlers.NewRealTwilioClient(
       os.Getenv("TWILIO_ACCOUNT_SID"),
       os.Getenv("TWILIO_AUTH_TOKEN"),
       os.Getenv("TWILIO_FROM_NUMBER"),
   )
   handler := handlers.NewPhoneCallHandler(realClient, logger, config)
   ```

### Required Environment Variables
For real Twilio calls:
- `TWILIO_ACCOUNT_SID`: Your Twilio account SID
- `TWILIO_AUTH_TOKEN`: Your Twilio auth token
- `TWILIO_FROM_NUMBER`: Your Twilio phone number
- `TWILIO_WEBHOOK_URL`: Base URL for call webhooks

For testing:
- `TWILIO_TEST_FROM_NUMBER`: Test from number
- `TWILIO_TEST_TO_NUMBER`: Test to number

### Error Handling
- Failed calls are retried up to `MaxAttempts` times
- Each retry includes exponential backoff
- Detailed error logging for debugging
- Transaction support prevents task state corruption 