# Transcript Processor

The transcript processor is responsible for processing completed phone call appointments, analyzing the conversation, and generating personalized job search criteria and preferences based on the discussion.

## Overview

When a phone call ends, the system processes the transcript to extract work preferences and generate a steering query that will be used to find relevant job opportunities.

## Workflow

1. **Call Completion**
   ```
   Phone call ends 
   → Webhook received 
   → ProcessCompletedCall triggered
   → Appointment marked as "processing"
   ```

2. **Processing Pipeline**
   ```
   Get transcript from VAPI call
   → Analyze work location preferences  
   → Generate steering query
   → Create or reuse search criteria
   → Subscribe user to search criteria
   → Queue immediate search (if new criteria)
   → Refresh existing jobs (if existing criteria)
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

## Data Storage

The transcript processor stores processed data directly on the User model:

- `User.WorkPreferences`: Work location preferences extracted from the transcript
- `User.SteeringQuery`: Generated steering query containing:
  - `Query`: The main job title, skill, or programming language to search for
  - `MoveToward`: Concept to move search results toward (optional)
  - `MoveAway`: Concept to move search results away from (optional)  
  - `RerankingQuery`: Comprehensive sentence combining all concepts
  - `ForceToward`/`ForceAway`: Steering force values (0.0-1.0)

The processor also creates or reuses `SearchCriteria` objects that combine the steering query with geographical constraints.

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

The processor uses LLM models (primarily DeepSeek) to analyze the transcript in two stages:

1. **Work Location Analysis** (`transcript-030-work-location.txt`)
   - Analyzes work location preferences from the transcript
   - Matches user preferences to supported geographic location IDs
   - Extracts data within `<final_determinations>` tags
   - Stores result in `User.WorkPreferences`

2. **Steering Query Generation** (`transcript-010-steering-query.txt`)
   - Creates a structured steering query from the conversation
   - Identifies main search terms (job titles, skills, programming languages)
   - Determines concepts to move toward/away from
   - Generates a comprehensive reranking query
   - Uses DeepSeekReasoner model for analysis
   - Returns JSON structure with Query, MoveToward, MoveAway, and RerankingQuery fields
   - Stores result in `User.SteeringQuery`

## Search Integration

After processing the transcript:

1. **Search Criteria Creation**: Combines steering query with geographic constraints
2. **User Subscription**: Subscribes user to the search criteria for ongoing updates  
3. **Immediate Search**: If new criteria, queues an immediate job search using ScrapingDog
4. **Job Refresh**: If existing criteria, refreshes user's job results from Pinecone
5. **Real-time Updates**: Uses Pusher to send status updates and redirect to dashboard

## Error Handling

- All errors are captured in Sentry with full context
- Failed processing sets appointment status to `AppointmentStatusFailed`
- Errors are logged with structured logging (user-id, appointment-id)
- User receives real-time notifications via Pusher
- Failed appointments can be recovered and reprocessed

## Dependencies

- **Firestore**: User data and search criteria storage
- **VAPI**: Call transcripts and conversation data
- **DeepSeek/OpenAI**: LLM models for transcript analysis
- **Sentry**: Error tracking and monitoring
- **Pusher**: Real-time user notifications
- **ScrapingDog**: Job search execution
- **Pinecone**: Vector search and job matching

## Usage

To process a completed appointment:

```go
ProcessCompletedCall(hub, fs, psh, queue, logger, dispatch, user, appointment)
```

To recover interrupted processing on service startup:

```go
err := RecoverProcessingAppointments(hub, fs, psh, logger, dispatch, queue)
if err != nil {
    // Handle error
}
```

## Key Functions

- `ProcessCompletedCall`: Main processing function for completed calls
- `RecoverProcessingAppointments`: Recovers interrupted processing sessions
- `getWorkPreferences`: Extracts work location preferences using LLM
- `generateSteeringQuery`: Creates steering query from transcript
- `getOrCreateSearchCriteria`: Creates or retrieves existing search criteria
- `userNotifier`: Provides real-time updates during job search execution 