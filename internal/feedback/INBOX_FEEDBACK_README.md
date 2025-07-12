# Inbox Feedback Processor

This module implements an email-based feedback system that allows users to send
search adjustment requests via email, which are automatically processed to
update their job search statements.

## Architecture

### Core Components

1. **InboxFeedbackHandler** - Main handler that implements the `EmailHandler` interface
2. **OpenAIClient** - Interface for LLM operations (email analysis and statement updates)  
3. **UserService** - Interface for user data operations (lookup and statement updates)

### Key Features

- **Email Analysis**: Uses OpenAI GPT-4o-mini to determine if emails contain search adjustment requests
- **Statement Updates**: Merges email requests with existing user statements using LLM
- **User Lookup**: Finds users by email address from the email "From" field
- **Error Handling**: Graceful error handling with detailed logging
- **Testability**: Interface-based design with comprehensive mocks

## Integration

### Setup

```go
import "talent-rodeo/internal/feedback"

// During application startup - Enhanced dispatch with AI response capabilities
openAIClient := inbox.NewOpenAIClient(openAIAPIKey, logger)
responder := inbox.NewEmailResponder(openAIClient, logger)
sender := inbox.NewResponseSender(mailer, logger)
escalationService := inbox.NewEscalationService(notificationEmail, mailer, logger)

dispatcher := internal.NewEnhancedInboxDispatch(responder, sender, escalationService, logger)

// Register the feedback handler
feedbackOpenAIClient := feedback.NewOpenAIClient(openAIAPIKey, logger)
inboxFeedbackHandler := feedback.NewInboxFeedbackHandler(
    feedbackOpenAIClient,
    firestoreClient,
    mailer,
    notificationEmail,
    logger,
)

err := dispatcher.Register("inbox-feedback", inboxFeedbackHandler)
if err != nil {
    log.Fatal("Failed to register inbox feedback handler:", err)
}
```

**Fallback for missing OpenAI API key:**
```go
// If OPENAI_API_KEY is not set, falls back to basic dispatch without AI features
if openAIAPIKey == "" {
    dispatcher := internal.NewInboxDispatch()
    // Basic handler registration only - no AI response generation
}
```

### Environment Variables

- `OPENAI_API_KEY` - Required for email analysis, statement updates, and AI response generation
- `ADMIN_EMAIL` - Admin email for escalation notifications when emails can't be handled automatically
- `SEND_FEEDBACK_ADJUSTMENTS` - Set to "true" to enable email notifications for search adjustments

## Usage

Once registered with the enhanced inbox dispatch system, the handler will automatically:

1. Receive new emails from the dispatcher
2. Analyze each email to determine if it contains search adjustment requests
3. Look up the user by their email address
4. Update the user's statement using LLM to merge the request with existing preferences
5. Save the updated statement to Firestore
6. Send email notification (if enabled) with adjustment summary

**Enhanced dispatch features (when using `NewEnhancedInboxDispatch`):**
- **AI-generated responses**: Automatically sends contextual replies to users based on handling results
- **Admin escalation**: Forwards unhandled emails to admin for manual review
- **Comprehensive logging**: Detailed tracking of email processing workflow

## Email Processing Flow

**Enhanced Dispatch Workflow:**
```
Email Received → Handler Processing → AI Response Generation → Send User Reply → Admin Escalation (if needed)
     ↓
   Analysis (OpenAI) → User Lookup → Statement Update (OpenAI) → Save to DB → Handling Result
```

**Basic Dispatch Workflow (fallback):**
```
Email Received → Analysis (OpenAI) → User Lookup → Statement Update (OpenAI) → Save to DB → Email Notification (if enabled)
```

## Testing

The module includes comprehensive tests covering:

- Email address extraction from various formats
- Search request detection (positive and negative cases)
- User lookup scenarios (found/not found)
- Statement updates (with and without existing statements)
- Error handling for API failures
- Email notification functionality (enabled/disabled scenarios)
- Integration testing with real OpenAI API calls

Run tests:
```bash
go test ./internal/feedback -run "TestInboxFeedbackHandler|TestExtractEmailAddress|TestAnalyzeEmail" -v
```

## Example Email Processing

**Input Email:**
```
From: john.doe@example.com
Subject: Update my job search
Body: I would like to add remote work options and focus on senior-level positions instead of entry-level roles.
```

**Processing:**
1. OpenAI analysis identifies this as a search adjustment request
2. User lookup finds user with email "john.doe@example.com"  
3. Current statement: "I am looking for entry-level software engineering positions in San Francisco"
4. OpenAI updates statement: "I am looking for senior-level software engineering positions with remote work options"
5. Updated statement saved to user's profile
6. Email notification sent to admin (if `SEND_FEEDBACK_ADJUSTMENTS=true`)

## Error Scenarios

- **Unknown user**: Email from unregistered user → logged and skipped
- **OpenAI API failure**: Temporary API issues → error logged, processing skipped
- **Invalid email format**: Malformed emails → error logged, processing skipped
- **Statement save failure**: Database issues → error logged, retry logic could be added

## Code Quality Improvements

The codebase has been enhanced for better maintainability and clarity:

### **Documentation**
- Complete Go-style documentation for all exported functions and types
- Detailed comments explaining error conditions and return values
- Clear interface documentation with usage examples

### **Function Decomposition**
The complex functions have been broken down into smaller, single-purpose functions:

- `extractUserEmail()` - Validates and extracts email addresses from email headers
- `lookupUser()` - Handles user lookup with proper error handling
- `buildUserName()` - Constructs user display names with fallback logic

### **Enhanced Testing**
- **13 test functions** covering all scenarios including new helper functions
- Comprehensive edge case testing for email extraction and user name building
- Error condition testing for user lookup scenarios
- Maintained 100% test coverage for new functions

### **Improved Error Handling**
- More descriptive error messages with context
- Better separation of concerns for error scenarios
- Graceful handling of missing or malformed data

## Dependencies

- `github.com/sashabaranov/go-openai` - OpenAI API client
- `internal/user.go` - User data operations
- `internal/inbox_dispatch.go` - Enhanced email dispatch system with AI responses and escalation
- `internal/inbox/` - Email processing components (EmailResponder, ResponseSender, EscalationService)
- Standard library for testing (no external test frameworks)