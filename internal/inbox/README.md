# Inbox Package

The `inbox` package provides functionality for reading emails from various sources (Gmail API, IMAP) and processing them through configurable handlers.

## Features

- **Multiple Inbox Sources**: Support for Gmail API and IMAP protocols
- **Periodic Processing**: Configurable intervals for checking new emails
- **Handler Interface**: Pluggable email processing with custom handlers
- **Settings Management**: Cloudflare KV integration for runtime configuration
- **Time-based Controls**: Configurable processing hours and pause functionality

## Architecture

The package follows the same patterns as other processors in the codebase:

- `Processor`: Main orchestrator that manages periodic inbox checking
- `InboxReader`: Interface for different email sources (Gmail, IMAP)
- `EmailHandler`: Interface for processing individual emails
- Settings stored in Cloudflare KV for runtime configuration

## Usage

### Basic Setup

```go
// Configure Gmail reader
config := inbox.InboxConfig{
    CredentialsJSON: credentialsData,
    TokenPath:       "/path/to/token.json",
}

reader, err := inbox.NewInboxReader(config, logger)
if err != nil {
    return err
}

// Create email handler
handler := inbox.EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) error {
    // Process the email
    log.Printf("Processing email: %s", email.Subject)
    return nil
})

// Create and start processor
processor := inbox.NewProcessor(reader, handler, logger)
if err := processor.Start(ctx); err != nil {
    return err
}

// Stop when done
defer processor.Stop()
```

### Gmail API Setup

1. Create a Google Cloud project and enable Gmail API
2. Create OAuth2 credentials and download the JSON file
3. Configure the inbox reader with credentials:

```go
config := inbox.InboxConfig{
    CredentialsJSON: credentialsJSON,
    TokenPath:       "token.json", // Will be created on first auth
}
```

### Custom Email Handler

```go
type MyEmailHandler struct {
    db     *firestore.Client
    logger *slog.Logger
}

func (h *MyEmailHandler) HandleEmail(ctx context.Context, email *inbox.Email) error {
    // Custom processing logic
    if strings.Contains(email.Subject, "feedback") {
        return h.processFeedbackEmail(ctx, email)
    }
    return nil
}
```

## Configuration

Settings are managed via Cloudflare KV with the key `inbox-processor`:

```go
type ProcessorSettings struct {
    Paused    bool          // Pause processing
    Interval  time.Duration // Check interval
    StartHour int           // Processing start hour (UTC)
    EndHour   int           // Processing end hour (UTC)
    MaxEmails int           // Max emails per check
}
```

## Email Structure

```go
type Email struct {
    ID          string            // Unique email ID
    ThreadID    string            // Email thread ID
    From        string            // Sender address
    To          []string          // Recipients
    Subject     string            // Email subject
    Body        string            // Plain text body
    HTML        string            // HTML body
    Date        time.Time         // Email date
    Labels      []string          // Email labels (Gmail)
    IsRead      bool              // Read status
    Headers     map[string]string // Email headers
    Attachments []Attachment      // File attachments
}
```

## Integration with Task System

The inbox processor can work with the existing task system:

```go
handler := inbox.EmailHandlerFunc(func(ctx context.Context, email *inbox.Email) error {
    // Queue task for email processing
    task := &internal.Task{
        Type: "process_email",
        Data: map[string]interface{}{
            "email_id": email.ID,
            "subject":  email.Subject,
            "from":     email.From,
            "body":     email.Body,
        },
    }
    return taskProcessor.QueueTask(ctx, task)
})
```