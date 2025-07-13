# Inbox Email Processing System

This system implements a comprehensive email-based feedback and adjustment
system that allows users to send requests via email, which are automatically
processed using AI to update their job search preferences and generate
appropriate responses.

## System Overview

The email processing system consists of three main layers:

1. **Email Infrastructure** (`internal/inbox/`) - Handles email reception, response generation, and escalation
2. **Central Dispatch** (`internal/inbox_dispatch.go`) - Orchestrates email routing to specialized handlers
3. **Specialized Handlers** (`internal/feedback/`, `internal/email_parser.go`) - Process specific types of email content

## Complete Architecture

### Email Infrastructure Layer (`internal/inbox/`)

**Email Reception:**
- `gmail_reader.go` - Gmail API integration for reading emails
- `imap_reader.go` - IMAP server integration for email reading
- `inbox_processor.go` - Main email processing pipeline coordinator
- `inbox.go` - Core inbox functionality and email handling
- `types.go` - Core data structures (Email, HandlingResult, etc.)

**Response System:**
- `email_responder.go` - AI-powered response generation using OpenAI
- `email_response.go` - Response data structures and formatting
- `email_sender.go` - Response delivery system
- `escalation_service.go` - Admin escalation for unhandled emails
- `errors.go` - Email processing error definitions

### Central Dispatch Layer (`internal/inbox_dispatch.go`)

**InboxDispatch** - Central orchestrator that:
- Maintains handler registry (`map[string]EmailHandler`)
- Routes emails to appropriate handlers
- Handles panic recovery and error logging
- Supports two operational modes:
  - **Basic Mode**: Handler processing only
  - **Enhanced Mode**: Adds AI response generation + admin escalation
- Provides comprehensive logging and metrics

### Email Parsing Layer (`internal/email_parser.go`)

**NEW: LLM-First Email Structure Analysis**
- `EmailParser` interface - Converts raw emails into structured format
- `openAIEmailParser` - OpenAI implementation with cost tracking
- `EmailStructure` - Categorized email content:
  - `Requests` - Actionable items (job search changes, account updates)
  - `GeneralFeedback` - Human feedback requiring admin review
  - `NonRequests` - Greetings, signatures, non-actionable content
- **Cost Tracking** - Allocates LLM usage costs to user accounts
- **Comprehensive Testing** - Integration, validation, and unit tests

### Specialized Handlers

**Search Adjustment Handler** (`internal/feedback/inbox_feedback.go`):
- `InboxFeedbackHandler` - Processes job search adjustment requests
- `OpenAIClient` - Interface for LLM operations (analysis and statement updates)
- `UserService` - Interface for user data operations
- **Features**:
  - Email analysis for search adjustment detection
  - Statement updates using LLM merging
  - User lookup by email address
  - Admin notifications for adjustments

### Key System Features

- **AI-Powered Analysis**: Uses OpenAI GPT-4o-mini and GPT-4 for email understanding
- **Structured Processing**: Categorizes email content into actionable and non-actionable items
- **Cost Management**: Tracks and allocates LLM usage costs to user accounts
- **Response Generation**: Automatically generates contextual replies to users
- **Admin Escalation**: Routes unhandled content to administrators
- **Comprehensive Logging**: Detailed tracking throughout the processing pipeline
- **Error Recovery**: Panic recovery and graceful error handling
- **Testability**: Interface-based design with extensive mocking

## Complete Email Processing Flow

### Enhanced Dispatch Workflow (Production)
```
Email Received → Email Parser → Handler Registry → AI Response → Admin Escalation
      ↓              ↓              ↓              ↓              ↓
   Gmail/IMAP → Structure Analysis → Route to Handlers → Generate Reply → Escalate Unhandled
      ↓              ↓              ↓              ↓              ↓
   Email Object → EmailStructure → HandlingResult → Response Email → Admin Notification
```

### Detailed Step-by-Step Flow

1. **Email Reception** (`internal/inbox/`)
   - `gmail_reader.go` or `imap_reader.go` receives email
   - `inbox_processor.go` validates and normalizes email data
   - Creates `Email` struct with ID, From, Subject, Body

2. **Optional: Structured Parsing** (`internal/email_parser.go`)
   - `ParseEmail()` analyzes content with OpenAI GPT-4
   - Categorizes into `Requests`, `GeneralFeedback`, `NonRequests`
   - Records LLM cost to user account via `RecordLLMCost()`

3. **Central Dispatch** (`internal/inbox_dispatch.go`)
   - `InboxDispatch.HandleEmail()` orchestrates processing
   - Loops through registered handlers in `map[string]EmailHandler`
   - Collects `HandlingResult` from each handler
   - Handles panic recovery with detailed logging

4. **Handler Processing** (e.g., `internal/feedback/inbox_feedback.go`)
   - `HandleEmail()` analyzes email for specific content type
   - For search adjustments: OpenAI analysis → user lookup → statement update
   - Returns `HandlingResult{Handled: bool, Summary: string, Metadata: map}`

5. **Response Generation** (`internal/inbox/email_responder.go`)
   - `GenerateResponse()` creates AI-powered replies based on handling results
   - Uses handling summaries to craft contextual responses
   - Supports multiple response types (confirmation, error, escalation)

6. **Response Delivery** (`internal/inbox/email_sender.go`)
   - `SendResponse()` delivers replies to original email sender
   - Uses SMTP mailer with template rendering

7. **Admin Escalation** (`internal/inbox/escalation_service.go`)
   - `SendEscalation()` notifies admin of unhandled content
   - Includes original email and unprocessed items

## Integration Setup

### Enhanced Mode (Full AI Pipeline)
```go
import (
    "talent-rodeo/internal"
    "talent-rodeo/internal/feedback" 
    "talent-rodeo/internal/inbox"
)

// Setup email infrastructure
responder := inbox.NewEmailResponder(openAIClient, logger)
sender := inbox.NewResponseSender(mailer, logger)
escalationService := inbox.NewEscalationService(adminEmail, mailer, logger)

// Create enhanced dispatcher
dispatcher := internal.NewEnhancedInboxDispatch(responder, sender, escalationService, logger)

// Register email parser (optional but recommended)
emailParser := internal.NewOpenAIEmailParser(openAIAPIKey, logger, firestoreClient)

// Register search adjustment handler
feedbackHandler := feedback.NewInboxFeedbackHandler(
    feedback.NewOpenAIClient(openAIAPIKey, logger),
    firestoreClient,
    mailer, 
    adminEmail,
    logger,
)
dispatcher.Register("search-feedback", feedbackHandler)

// Register other handlers as needed
dispatcher.Register("account-updates", accountHandler)
dispatcher.Register("support-requests", supportHandler)
```

### Basic Mode (Handler Processing Only)
```go
// Fallback when AI features unavailable
dispatcher := internal.NewInboxDispatch()
dispatcher.Register("search-feedback", feedbackHandler)
// No AI responses or escalation
```

## Environment Variables

### Required
- `OPENAI_API_KEY` - Required for all AI features (email parsing, analysis, statement updates, response generation)
- `FIREBASE_PROJECT_ID` - Firestore project for user data and cost tracking
- `FIREBASE_CREDENTIALS` - Firestore service account credentials

### Optional  
- `ADMIN_EMAIL` - Admin email for escalation notifications when emails can't be handled automatically
- `SEND_FEEDBACK_ADJUSTMENTS` - Set to "true" to enable admin notifications for search adjustments
- `BASE_URL` - Application base URL for email templates and links

### Email Infrastructure
- Gmail: `GMAIL_CREDENTIALS`, `GMAIL_TOKEN`
- IMAP: `IMAP_SERVER`, `IMAP_USERNAME`, `IMAP_PASSWORD`
- SMTP: Mailer configuration for response delivery

## File-by-File Documentation

### Core Email Infrastructure (`internal/inbox/`)

| File | Purpose | Key Functions |
|------|---------|---------------|
| `types.go` | Core data structures | `Email`, `HandlingResult`, `ResponseType` |
| `gmail_reader.go` | Gmail API integration | `ReadEmails()`, token management |
| `imap_reader.go` | IMAP server integration | `Connect()`, `ReadEmails()` |
| `inbox_processor.go` | Email processing pipeline | `ProcessInbox()`, email validation |
| `email_responder.go` | AI response generation | `GenerateResponse()` with OpenAI |
| `email_response.go` | Response data structures | `EmailResponse`, formatting helpers |
| `email_sender.go` | Response delivery | `SendResponse()` via SMTP |
| `escalation_service.go` | Admin escalation | `SendEscalation()`, unhandled item processing |
| `errors.go` | Error definitions | Email processing error types |

### Central Dispatch (`internal/`)

| File | Purpose | Key Functions |
|------|---------|---------------|
| `inbox_dispatch.go` | Handler orchestration | `HandleEmail()`, handler registry, panic recovery |
| `email_parser.go` | LLM-based email parsing | `ParseEmail()`, structured content analysis |

### Search Feedback Handler (`internal/feedback/`)

| File | Purpose | Key Functions |
|------|---------|---------------|
| `inbox_feedback.go` | Search adjustment processing | `HandleEmail()`, user statement updates |
| `inbox_feedback_test.go` | Comprehensive test suite | Handler testing with mocks |

### Testing Files

| File | Purpose | Coverage |
|------|---------|----------|
| `inbox_dispatch_test.go` | Dispatch system testing | Handler registry, error scenarios |
| `email_parser_test.go` | Parser unit tests | Content categorization |
| `email_parser_integration_test.go` | Parser integration tests | Real OpenAI API calls |
| `email_parser_validation_test.go` | Parser validation tests | Edge cases, malformed content |
| `inbox_feedback_test.go` | Feedback handler tests | Search adjustment scenarios |

## Cost Tracking Integration

The system includes comprehensive LLM cost tracking:

```go
// Automatic cost allocation in email_parser.go:98-110
user, found, err := GetUserByEmail(ctx, p.fs, email.From)
if found {
    err := RecordLLMCost(ctx, p.fs, &user.ID, string(tools.GPT41), 
                        resp, "email_parsing", nil)
}
```

**Cost Categories:**
- `email_parsing` - Structured content analysis
- `email_analysis` - Search request detection  
- `statement_update` - User statement modifications
- `response_generation` - AI reply creation

**Cost Tracking Location:** `internal/cost_tracking.go`

## Testing Strategy

### Test Coverage by Component

**Email Infrastructure Tests:**
```bash
# Core inbox functionality
go test ./internal/inbox -v

# Email dispatch system  
go test ./internal -run "TestInboxDispatch" -v

# Email parser (structured analysis)
go test ./internal -run "TestEmailParser" -v
```

**Feedback Handler Tests:**
```bash
# Comprehensive feedback handler testing
go test ./internal/feedback -run "TestInboxFeedbackHandler" -v

# Email analysis and extraction
go test ./internal/feedback -run "TestExtractEmailAddress|TestAnalyzeEmail" -v
```

**Integration Tests:**
```bash
# Real OpenAI API integration tests (requires OPENAI_API_KEY)
go test ./internal -run "TestEmailParserIntegration" -v
go test ./internal/feedback -run "TestInboxIntegration" -v
```

### Test Categories

| Test Type | Files | Coverage |
|-----------|-------|----------|
| **Unit Tests** | `*_test.go` | Email parsing, handler logic, utilities |
| **Integration Tests** | `*_integration_test.go` | Real API calls, end-to-end flows |
| **Validation Tests** | `*_validation_test.go` | Edge cases, malformed data |
| **Mock Tests** | Uses `test_mocks.go` | Isolated component testing |

### Key Test Scenarios

**Email Parser Tests:**
- Content categorization (Requests vs GeneralFeedback vs NonRequests)
- Cost tracking integration
- Malformed email handling
- Empty content scenarios

**Dispatch System Tests:**
- Handler registration/deregistration
- Panic recovery and error handling
- Enhanced vs basic mode functionality
- Response generation and escalation

**Feedback Handler Tests:**
- Search adjustment detection (positive/negative cases)
- User lookup scenarios (found/not found/error)
- Statement updates with OpenAI integration
- Email notification functionality
- Error handling for API failures

## Example: Complete Email Processing

### Input Email
```
From: john.doe@example.com
Subject: Update my job search preferences
Body: Hi! I would like to add remote work options and focus on senior-level positions instead of entry-level roles. Also, the job recommendations have been helpful but could be more accurate. Thanks!
```

### Step-by-Step Processing

**1. Email Parser Analysis** (`internal/email_parser.go`)
```json
{
  "requests": [
    {
      "id": "req_1",
      "type": "job_search", 
      "description": "User wants remote work and senior positions",
      "quote": "I would like to add remote work options and focus on senior-level positions instead of entry-level roles"
    }
  ],
  "general_feedback": [
    {
      "id": "feedback_2", 
      "type": "feedback",
      "description": "Feedback about job recommendation accuracy",
      "quote": "the job recommendations have been helpful but could be more accurate"
    }
  ],
  "non_requests": [
    {
      "id": "non_req_3",
      "type": "greeting", 
      "description": "Email greeting",
      "quote": "Hi!"
    }
  ]
}
```

**2. Handler Processing** (`internal/feedback/inbox_feedback.go`)
- Search adjustment detected: ✅ 
- User lookup: `john.doe@example.com` → Found user ID `user_123`
- Current statement: `"I am looking for entry-level software engineering positions in San Francisco"`
- OpenAI statement update: `"I am looking for senior-level software engineering positions with remote work options"`
- Statement saved to Firestore
- Returns: `HandlingResult{Handled: true, Summary: "Updated search preferences: remote work and senior positions"}`

**3. AI Response Generation** (`internal/inbox/email_responder.go`)
```
Subject: Re: Update my job search preferences
Body: Hi John! I've successfully updated your job search preferences to include remote work options and focus on senior-level positions. Your updated preferences are now active and will be used for future job recommendations. Thanks for your feedback!
```

**4. Admin Escalation** (`internal/inbox/escalation_service.go`)
- Unhandled item: General feedback about recommendation accuracy
- Admin notification sent with original email and unprocessed feedback

### Result Summary
- ✅ Search preferences updated automatically
- ✅ User receives confirmation email  
- ✅ Admin notified about feedback requiring review
- 💰 LLM costs tracked to user account

## Error Scenarios & Handling

| Scenario | Behavior | Recovery |
|----------|----------|----------|
| **Unknown User** | Log warning, skip processing | Continue with other emails |
| **OpenAI API Failure** | Log error, return unhandled result | Retry with exponential backoff |
| **Firestore Connection Error** | Log error, fail gracefully | Connection retry logic |
| **Malformed Email** | Log error, skip processing | Continue processing queue |
| **Handler Panic** | Catch panic, log stack trace | Continue with next handler |

## System Dependencies

### Core Libraries
- `github.com/sashabaranov/go-openai` - OpenAI API client for LLM operations
- `github.com/chriscow/minds/tools` - Structured LLM queries with cost tracking
- Standard Go library for HTTP, JSON, context handling

### Internal Dependencies  
- `internal/firestore.go` - Firestore client and user operations
- `internal/cost_tracking.go` - LLM usage cost allocation
- `internal/smtp/` - Email delivery infrastructure
- `internal/sentry.go` - Error tracking and monitoring

### External Services
- **OpenAI API** - GPT-4 and GPT-4o-mini for content analysis
- **Firestore** - User data and cost tracking storage
- **Gmail API** - Email reception (optional, can use IMAP)
- **SMTP Provider** - Email response delivery