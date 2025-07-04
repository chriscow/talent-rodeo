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

## Gmail Token Management

The Gmail integration requires two types of credentials:

1. **Application Credentials** (`GMAIL_READER_CREDENTIALS`): Your app's OAuth2 credentials from Google Cloud Console (base64 encoded)
2. **User Authorization Token**: OAuth token proving user has authorized your app to access their Gmail

### Initial Gmail Setup

#### Step 1: Create Google Cloud Credentials

1. **Go to Google Cloud Console**: https://console.cloud.google.com/
2. **Create/select a project**
3. **Enable Gmail API**: 
   - APIs & Services → Library 
   - Search "Gmail API" → Enable
4. **Create OAuth2 credentials**:
   - APIs & Services → Credentials → Create Credentials → OAuth 2.0 Client IDs
   - Application type: **Desktop application**
   - Name it (e.g., "Gmail Reader")
5. **Download JSON**: Click download button next to your OAuth client
6. **Encode credentials**: `cat credentials.json | base64` and save this value as `GMAIL_READER_CREDENTIALS`

#### Step 2: Generate Initial OAuth Token

```bash
# Set your application credentials
export GMAIL_READER_CREDENTIALS="your_base64_encoded_app_credentials"

# Run CLI setup to authorize Gmail access
go run cmd/cli/cli_main.go gmail setup

# Follow the prompts:
# 1. Open the provided URL in your browser
# 2. Sign in and authorize the application
# 3. Copy the authorization code from the page
# 4. Paste it into the terminal
```

This creates `gmail_token.json` in your project root.

### Regenerating Expired Tokens

When you see this error:
```
oauth2: "invalid_grant" "Token has been expired or revoked."
```

#### Method 1: Using CLI Tool (Recommended)

```bash
# Navigate to project root
cd /path/to/talent-rodeo

# Regenerate token
go run cmd/cli/cli_main.go gmail setup

# Test the new token
go run cmd/cli/cli_main.go gmail test --max 5
```

#### Method 2: Using Web Server Flag

```bash
# Run server in Gmail setup mode
go run cmd/web/main.go --gmail-setup
```

#### Method 3: Production Token Regeneration

For production environments, regenerate locally then deploy:

```bash
# 1. Regenerate token locally
go run cmd/cli/cli_main.go gmail setup

# 2. Encode token for deployment (IMPORTANT: use tr -d '\n' to remove line breaks)
ENCODED_TOKEN=$(cat gmail_token.json | base64 | tr -d '\n')

# 3. Verify the encoding looks correct (should be one long line with no spaces)
echo "Token length: ${#ENCODED_TOKEN}"
echo "First 50 chars: ${ENCODED_TOKEN:0:50}..."

# 4. Set as environment variable (Fly.io example)
fly secrets set GMAIL_TOKEN_BASE64="$ENCODED_TOKEN"

# 5. Restart application
fly machines restart

# 6. Verify Gmail is working
curl -s https://your-app.fly.dev/health/gmail | jq .
```

**⚠️ Important Notes:**
- **Always use `tr -d '\n'`** to remove line breaks from base64 output
- The encoded token should be one continuous string with no spaces or line breaks
- If you get "illegal base64 data" errors, the encoding was corrupted - regenerate it

### Testing Gmail Integration

```bash
# Test connection and fetch recent emails
go run cmd/cli/cli_main.go gmail test --max 5

# List unread emails in JSON format
go run cmd/cli/cli_main.go gmail list --json --max 10

# Check health endpoint (if server is running)
curl http://localhost:2304/health/gmail
```

### Health Check Endpoints

Monitor Gmail integration status:

- `/health` - Overall application health including Gmail
- `/health/gmail` - Gmail-specific health check

**Expected Responses:**

Gmail working:
```json
{
  "service": "gmail",
  "status": "healthy", 
  "message": "Gmail connection successful",
  "timestamp": "2025-07-04T17:00:00Z"
}
```

Token expired:
```json
{
  "service": "gmail",
  "status": "unhealthy",
  "message": "unhealthy: oauth2: \"invalid_grant\" \"Token has been expired or revoked.\"",
  "timestamp": "2025-07-04T17:00:00Z"
}
```

### Environment Variables

| Variable | Required | Purpose | Example |
|----------|----------|---------|---------|
| `GMAIL_READER_CREDENTIALS` | Yes | App OAuth2 credentials (base64) | `eyJjbGllbnRfaWQi...` |
| `GMAIL_TOKEN_BASE64` | Optional | User auth token (base64) | `eyJhY2Nlc3NfdG9rZW4i...` |
| `GMAIL_TOKEN_PATH` | Optional | Custom token file path | `/app/custom_token.json` |

### Troubleshooting

**"Gmail credentials not configured"**
- Set `GMAIL_READER_CREDENTIALS` environment variable
- Ensure the value is properly base64 encoded

**"Gmail token not found"**
- Run `go run cmd/cli/cli_main.go gmail setup` to create token
- Or set `GMAIL_TOKEN_BASE64` for deployed applications

**"invalid_grant" OAuth error**
- Token has expired - regenerate using steps above
- Ensure OAuth client is configured for "Desktop application"
- Check that redirect URI is set to `urn:ietf:wg:oauth:2.0:oob`

**"illegal base64 data" error**
- The `GMAIL_TOKEN_BASE64` environment variable is corrupted
- **Solution**: Re-encode the token properly:
  ```bash
  # Generate clean base64 encoding without line breaks
  ENCODED_TOKEN=$(cat gmail_token.json | base64 | tr -d '\n')
  fly secrets set GMAIL_TOKEN_BASE64="$ENCODED_TOKEN"
  fly machines restart
  ```

**"Gmail connection failed"**
- Check network connectivity
- Verify Gmail API is enabled in Google Cloud Console
- Ensure OAuth client has correct configuration

### Token Security

- **Development**: Token stored as local file (`gmail_token.json`)
- **Production**: Token stored as base64 environment variable (`GMAIL_TOKEN_BASE64`)
- **File permissions**: Token files created with `0600` permissions (read/write for owner only)
- **Automatic refresh**: OAuth2 library handles token refresh automatically using refresh_token

## Monitoring and Alerting

### How to Know When Token Regeneration is Needed

You'll know the Gmail token needs regeneration when you see these signs:

**1. Application Logs:**
```
oauth2: "invalid_grant" "Token has been expired or revoked."
```

**2. Health Check Failures:**
```bash
curl https://talent-rodeo.fly.dev/health/gmail
# Returns status: "unhealthy" with oauth2 error
```

**3. Sentry Alerts:**
- Gmail authentication errors are automatically reported to Sentry
- Look for alerts tagged with `component: inbox_processor`

### Monitoring Options

#### Option 1: Sentry Monitoring (Recommended)
Gmail errors are automatically sent to Sentry with these tags:
- `component: inbox_processor`
- `error_type: gmail_authentication`

**Setup Sentry Alerts:**
1. Go to your Sentry project
2. Create a new Alert Rule
3. Set conditions:
   - `event.tags.component:inbox_processor`
   - `event.message` contains `"invalid_grant"`
4. Configure notifications (email, Slack, etc.)

#### Option 2: Health Check Monitoring
Monitor the health endpoint with external services:

**Uptime Robot / Pingdom:**
- URL: `https://talent-rodeo.fly.dev/health/gmail`
- Alert when status code is not 200
- Check frequency: Every 5-15 minutes

**Script example:**
```bash
#!/bin/bash
# Check Gmail health and send alert if failed
HEALTH_STATUS=$(curl -s https://talent-rodeo.fly.dev/health/gmail | jq -r '.status')

if [ "$HEALTH_STATUS" != "healthy" ]; then
    # Send notification (email, Slack, etc.)
    echo "Gmail integration unhealthy: $HEALTH_STATUS" | mail -s "Gmail Token Alert" admin@yourcompany.com
fi
```

#### Option 3: Fly.io Monitoring
Fly.io provides built-in monitoring capabilities:

**1. Set up Fly.io Health Checks:**
```toml
# In fly.toml
[http_service.checks.gmail]
  grace_period = "10s"
  interval = "30s"
  method = "GET"
  path = "/health/gmail"
  protocol = "http"
  timeout = "5s"
  headers = {}
```

**2. Configure Fly.io Notifications:**
```bash
# Set up webhook notifications
fly apps config set --app talent-rodeo WEBHOOK_URL="your-slack-webhook-url"
```

#### Option 4: Application-Level Monitoring
The application already includes comprehensive error reporting:

**Automatic Sentry Reporting:**
- All Gmail errors are automatically sent to Sentry
- Includes context like user ID, operation, and error details
- Uses structured tags for easy filtering

**Email Notifications:**
- Server errors automatically trigger email notifications
- Configure via `--notifications-email` flag
- Uses template in `assets/emails/error-notification.html`

### Token Refresh Schedule

**OAuth2 tokens typically expire:**
- Access tokens: 1 hour (automatically refreshed)
- Refresh tokens: 6 months to 1 year (manual regeneration needed)

**Recommended monitoring frequency:**
- Health checks: Every 5-15 minutes
- Proactive token regeneration: Every 3-6 months
- Emergency response: Monitor Sentry for immediate alerts

### Emergency Response Playbook

**When you receive a Gmail token expiration alert:**

1. **Immediate Response (< 5 minutes):**
   ```bash
   # Quickly regenerate token locally
   go run cmd/cli/cli_main.go gmail setup
   ```

2. **Deploy Fix (< 10 minutes):**
   ```bash
   # Encode and deploy new token
   ENCODED_TOKEN=$(cat gmail_token.json | base64 | tr -d '\n')
   fly secrets set GMAIL_TOKEN_BASE64="$ENCODED_TOKEN"
   fly machines restart
   ```

3. **Verify Fix (< 15 minutes):**
   ```bash
   # Confirm Gmail is working
   curl -s https://talent-rodeo.fly.dev/health/gmail | jq .
   ```

4. **Post-Incident:**
   - Check Sentry for any related errors
   - Review logs for impact duration
   - Update monitoring if needed

### Proactive Token Management

**Set up calendar reminders:**
- Monthly health check review
- Quarterly token regeneration (preventive)
- Annual OAuth client credential review

**Monitor token expiration proactively:**
```bash
# Check token expiration locally
go run cmd/cli/cli_main.go gmail test --max 1
# If this fails, regenerate immediately
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