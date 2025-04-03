# Testing Mode Documentation

## Overview
The application includes a testing mode that allows simulation of phone calls and webhooks without making actual API calls. This is particularly useful for development and testing of the call flow without incurring costs or requiring actual phone infrastructure.

## Enabling Testing Mode
Testing mode is enabled by setting the `-testing` flag in `@main.go`, which sets the `TESTING_MODE` environment variable to "true".

## Features

### Call Simulation
When in testing mode:
1. The `CreateCall` function returns simulated call data from JSON files
2. The `GetCall` function returns simulated call status data
3. Webhook calls are simulated with realistic transcripts and timing

### Test Data Location
Test data is stored in the `testdata/vapi/` directory and includes:
- `end-of-call-report.json`: Contains the simulated end-of-call report data
- Other webhook payloads are saved in `testdata/webhooks/` when `SAVE_WEBHOOK_PAYLOADS=true`

### Webhook Simulation
- Simulated webhooks are triggered using the `X-Vapi-Simulate` header
- Current supported simulation: `X-Vapi-Simulate: end-of-call`
- The system waits 5 seconds before sending the end-of-call webhook to simulate a realistic call duration

## Debug Logging
Comprehensive debug logging has been implemented throughout the webhook handling process:
- Request details (method, URL, headers)
- Test mode status
- Request body size and content
- Webhook message parsing
- End-of-call report processing
- Appointment and user status updates

## Testing Flow Reset
To run through the testing flow again:
1. Reset the user's state in Firestore:
   - Set the user's `status` field to `"2"` (UserStatusPendingResumeUpload)
   - This can be done manually in the Firestore console or using the reset button (when in testing mode)
2. Navigate back to the profile page
3. The call simulation can now be triggered again

## Implementation Details
The testing mode implementation is primarily in:
- `cmd/web/handlers.vapi.webhooks.go`: Webhook handling and test data injection
- `internal/vapi/client.go`: Test mode checks in API client functions
- `testdata/vapi/`: Test data JSON files

## Best Practices
1. Always use the testing mode for development to avoid unnecessary API calls
2. Save new webhook payloads by enabling `SAVE_WEBHOOK_PAYLOADS=true`
3. Keep test data files up to date with production payload formats
4. Use debug logging to validate the testing flow 