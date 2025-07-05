# Talent Rodeo CLI

A command line interface for Talent Rodeo operations.

## Building

```bash
go build -o trcli ./cmd/cli
```

## Commands

### Firestore Tools (fs)

The `fs` namespace contains commands for managing data in the Firestore database.

```bash
trcli fs [command]
```

#### Users

```bash
trcli fs users [command]
```

Available Commands:
- `list`: List all users
- `delete`: Delete a user by ID, email, or prefix
- `list-geoids`: List all geographic location IDs

Flags:
- `--pretty`: Pretty print JSON output
- `--limit`: Maximum number of users to return (default: 100)
- `--offset`: Number of users to skip (default: 0)
- `--id`: User ID for deletion
- `--email`: User email for deletion
- `--prefix`: Prefix for bulk deletion

#### Jobs

```bash
trcli fs jobs [command]
```

Available Commands:
- `list`: List jobs for a specific user
- `update`: Update job status or notes
- `delete`: Delete a job by ID or prefix
- `listings`: List all job listings
- `details`: List all job details
- `delete-listings`: Delete job listings by prefix
- `delete-details`: Delete job details by prefix
- `delete-user-jobs`: Delete user jobs by prefix across all users

Flags:
- `--user`: User ID (required for user-specific operations)
- `--id`: Job ID for updates/deletion
- `--status`: Update job status (viewed, saved, applied, archived)
- `--notes`: Update notes for job
- `--prefix`: Prefix for bulk deletion
- `--limit`: Maximum number of items to return (default: 100)
- `--offset`: Number of items to skip (default: 0)
- `--pretty`: Pretty print JSON output

#### Webhooks

```bash
trcli fs webhooks [command]
```

Available Commands:
- `list`: List all webhook data
- `delete`: Delete webhook(s) by ID or prefix

Flags:
- `--pretty`: Pretty print JSON output
- `--id-only`: Only output document IDs (for list command)
- `--id`: Webhook ID to delete
- `--prefix`: Delete all webhooks with this prefix
- `--all`: Delete all webhooks in the collection
- `--force`: Skip confirmation prompt when using --all

#### Test Data

```bash
trcli fs test-data
```

Generates test data for development and testing.

Flags:
- `--users`: Number of test users to create (default: 5)
- `--jobs`: Number of test jobs to create per user (default: 10)
- `--prefix`: Prefix for test data IDs (default: "test-")

### Nudge Tools

The `nudge` namespace contains commands for managing and analyzing nudge emails.

```bash
trcli nudge [command]
```

Available Commands:
- `list`: List predicted nudges for users who will receive nudges in the future
- `stats`: Show statistics about nudge emails that have been sent

#### List Predicted Nudges

```bash
trcli nudge list
```

Analyzes all users and predicts when they will receive nudge emails based on their current state and the nudge system settings. The output shows:

- **ID**: First 5 characters of the user ID
- **Name**: User's name (truncated if long)
- **Email**: User's email address (truncated if long)
- **Sub Days**: Days remaining until subscription expires (or "EXPIRED" if past due, "N/A" if no subscription)
- **Nudge Type**: The type of nudge they'll receive next
- **When**: Date and time the nudge will be sent
- **Days**: Number of days until the nudge is sent
- **Reason**: Brief explanation of why they're eligible

The prediction logic follows the same rules as the actual nudge processor:
1. **No Resume**: Users who signed up but haven't uploaded a resume (after 2 days)
2. **No Call**: Users who uploaded a resume but haven't scheduled a call (after 3 days)
3. **Five Star Matches**: Users with actual 5-star job matches that haven't been sent (after 2 days)
4. **Trial Emails**: Users in trial period receiving sequence emails (Days 1, 2, 3, 6)

The system verifies that five-star match users actually have high-scoring jobs (≥4.5 rating) created within the last 90 days that haven't been included in previous nudges. This ensures only users who will actually receive emails are shown.

Factors in rate limiting (7 days between nudges), per-type limits, and user activity restrictions.

#### Show Nudge Statistics

```bash
trcli nudge stats
```

Displays statistics about nudge emails that have already been sent, including:
- Total users who have received nudges
- Count of each nudge type sent
- Breakdown by trial email sequence

## Examples

1. List all users with pretty printing:
```bash
trcli fs users list --pretty
```

2. Delete a user by ID:
```bash
trcli fs users delete --id <user-id>
```

3. List jobs for a user:
```bash
trcli fs jobs list --user <user-id> --pretty
```

4. Update job status:
```bash
trcli fs jobs update --user <user-id> --id <job-id> --status viewed
```

5. Generate test data:
```bash
trcli fs test-data --users 3 --jobs 5 --prefix "dev-"
```

6. Delete test data:
```bash
trcli fs users delete --prefix "dev-"
```

7. List predicted nudges:
```bash
trcli nudge list
```

8. Show nudge statistics:
```bash
trcli nudge stats
```

## Environment Variables

The CLI requires proper configuration of Firebase credentials. Make sure either:
- A `.env` file exists in the project root with `FIREBASE_CREDENTIALS`
- The `FIREBASE_CREDENTIALS` environment variable is set in your system 