# Transcript Processor

The transcript processor is responsible for processing completed phone call appointments, analyzing the conversation, and generating job search criteria based on the discussion.

## Overview

When a phone call ends, the system processes the transcript through several stages to extract meaningful information and create personalized job search criteria.

## Workflow

1. **Call Completion**
   ```
   Phone call ends 
   → Webhook received 
   → ProcessCompletedAppointment triggered
   → Appointment marked as "processing"
   ```

2. **Processing Pipeline**
   ```
   Get transcript 
   → Extract data 
   → Resolve preferences 
   → Analyze work location preferences 
   → Generate search keywords 
   → Create user statement 
   → Generate search criteria
   ```

3. **Final States**
   - Success: Status set to `AppointmentStatusCompleted`
   - Failure: Status set to `AppointmentStatusFailed`

## State Management

The processor uses a state machine to track progress:

- `AppointmentStatusScheduled`: Initial state
- `AppointmentStatusRunning`: Call is in progress
- `AppointmentStatusProcessing`: Transcript is being processed
- `AppointmentStatusCompleted`: Processing successful
- `AppointmentStatusFailed`: Processing failed
- `AppointmentStatusCancelled`: Appointment cancelled
- `AppointmentStatusOther`: Other states

## Prompt Results Storage

The transcript processor saves all intermediate prompt results to the appointment's `ExtractedData` field for analysis and debugging:

- `ExtractedData.TranscriptExtract`: Initial data extraction from the transcript
- `ExtractedData.ResolvedPreferences`: Candidate preferences derived from the extracted data
- `ExtractedData.WorkPreferences`: Analysis of work location preferences
- `ExtractedData.KeywordAnalysis`: Search phrases and keywords generated from preferences
- `ExtractedData.UserStatement`: Generated summary of the candidate's job search criteria

The `ExtractedData` structure ensures all the processing steps are properly organized and stored. To retrieve these prompt results:

```go
results, err := transcript.GetPromptResults(ctx, FirestoreClient, userID, appointmentID)
```

## Recovery Mechanism

The service implements a robust recovery mechanism to handle interruptions during processing:

1. **State Tracking**
   - Appointments in "processing" state are tracked in Firestore
   - Each state change is persisted immediately

2. **Recovery Process**
   ```go
   Service starts up
   → Find all appointments in "processing" state
   → Reprocess each appointment from the beginning
   → Update final state based on processing result
   ```

3. **Implementation**
   - `RecoverProcessingAppointments`: Finds and reprocesses interrupted appointments
   - Safe to reprocess: The operation is idempotent
   - Errors during recovery are logged but don't stop other recoveries

## LLM Processing Steps

The processor uses GPT-4 to analyze the transcript through several stages:

1. **Data Extraction** (`transcript-010-extract.txt`)
   - Extracts key information from raw transcript

2. **Preference Resolution** (`transcript-020-resolved-prefs.txt`)
   - Resolves and normalizes user preferences

3. **Work Location Analysis** (`transcript-030-work-location.txt`)
   - Analyzes work location preferences
   - Matches to known location IDs

4. **Search Keyword Generation** (`transcript-040-search.txt`)
   - Generates relevant job search keywords

5. **User Statement Creation** (`transcript-050-user-statement.txt`)
   - Creates a comprehensive user statement

6. **Search Criteria Generation** (`transcript-060-final.txt`)
   - Generates structured search criteria

## Error Handling

- All errors are captured in Sentry
- Failed processing is marked in the appointment status
- Errors are logged with context for debugging
- User is notified of processing status

## Dependencies

- Firestore: State and data storage
- OpenAI GPT-4: Transcript analysis
- Sentry: Error tracking
- VAPI: Call transcripts

## Usage

To process a completed appointment:

```go
err := ProcessCompletedAppointment(hub, fs, logger, appointment)
if err != nil {
    // Handle error
}
```

To recover interrupted processing on service startup:

```go
err := RecoverProcessingAppointments(hub, fs, logger)
if err != nil {
    // Handle error
}
``` 