# Talent Rodeo CLI

A comprehensive command line interface for Talent Rodeo operations.

## Building

```bash
go build -o trcli ./cmd/cli
```

## Global Flags

- `--json`: Output in JSON format (available for most commands)
- `--env <file>`: Path to environment file (default: ".env")
- `--verbose`: Enable verbose output

## Commands Overview

### User Jobs (`userjobs`)

Manage user jobs, which are job records assigned to specific users with ratings, status, and notes.

```bash
trcli userjobs [command]
```

**Available Commands:**
- `list <user-id>`: List all jobs for a specific user
- `update [user-id] [--all]`: Update user job ratings from JobMatchAnalysis
- `followup <user-id>`: Show or dump follow-up questions for user jobs
- `update <user-id> --id <job-id>`: Update a specific user job's status or notes
- `delete <user-id> [--id <job-id>] [--prefix <prefix>]`: Delete user job(s)

**Key Flags:**
- `--json`: Output in JSON format
- `--all`: Update ratings for all users
- `--dump`: Dump all follow-up questions, one per line
- `--id`: Job ID for updates/deletion
- `--status`: Update job status (new, saved, applied, archived)
- `--notes`: Update notes for job
- `--prefix`: Prefix for bulk deletion

### Job Details (`job-details`, `job`)

Manage job details, job listings, and process related jobs (SimilarJobs and PeopleAlsoViewed).

```bash
trcli job-details [command]
trcli job [command]  # alias
```

**Available Commands:**
- `listings [job-listing-id] [--all]`: View job listings (basic job info from search results)
- `job [job-id] [--all]`: View job details (comprehensive job data including SimilarJobs and PeopleAlsoViewed)
- `delete-listings --prefix <prefix>`: Delete job listings by prefix
- `delete-details --prefix <prefix>`: Delete job details by prefix
- `delete-user-jobs --prefix <prefix>`: Delete user jobs by prefix across all users
- `upsert`: Upsert job details to vector database (Pinecone)
- `process-related`: **Process SimilarJobs and PeopleAlsoViewed for a job detail**

**Key Flags:**
- `--all`: List all items instead of a specific one
- `--limit`: Maximum number of items to return (default: 100)
- `--offset`: Number of items to skip (default: 0)
- `--prefix`: Prefix for bulk deletion
- `--user`, `-u`: User ID to associate jobs with (for process-related)
- `--job-id`, `-j`: Job detail ID to process related jobs for
- `--criteria-id`, `-c`: Search criteria ID to associate related jobs with
- `--dry-run`: Show what would be processed without actually doing it

### Users (`users`)

Manage users in the system.

```bash
trcli users [command]
```

**Available Commands:**
- `list`: List all users
- `create`: Create a new user
- `delete`: Delete a user by ID, email, or prefix
- `import`: Import users from a file
- `subscribe`: Subscribe/unsubscribe user to search criteria
- `subscription`: Manage user subscriptions
- `timezone`: Update user timezone

**Key Flags:**
- `--limit`: Maximum number of users to return (default: 100)
- `--offset`: Number of users to skip (default: 0)
- `--user-id`, `--email`, `--prefix`: User identification options
- `--name`, `--phone`: User creation options

### Search (`search`)

Manage job searches and search criteria.

```bash
trcli search [command]
```

**Available Commands:**
- `queue --criteria-id <criteria-id>`: Queue a new job search with saved criteria

**Key Flags:**
- `--criteria-id`, `-c`: ID of saved search criteria to use

### Search Criteria (`criteria`)

Manage search criteria for job searches.

```bash
trcli criteria [command]
```

**Available Commands:**
- `list`: List all search criteria
- `create`: Create new search criteria
- `update`: Update existing search criteria
- `delete`: Delete search criteria

### Feedback (`feedback`)

Manage and analyze user feedback on job listings.

```bash
trcli feedback [command]
```

**Available Commands:**
- `process`: Process all unprocessed user feedback
- `process-user <user-id>`: Process feedback for a specific user
- `stats`: Show feedback statistics
- `cleanup-orphaned`: Clean up feedback for deleted users
- `list-user <user-id>`: List all feedback for a specific user
- `show <feedback-id>`: Show detailed feedback information
- `list-job <job-id>`: List all feedback for a specific job
- `user-stats <user-id>`: Show comprehensive user feedback statistics
- `search-date <user-id> <start-date> <end-date>`: Search feedback by date range
- `export <user-id>`: Export feedback data for analysis

**Key Flags:**
- `--format`: Export format (json or csv)

### Nudge (`nudge`)

Manage nudge emails and email campaigns.

```bash
trcli nudge [command]
```

**Available Commands:**
- `list`: List predicted nudges for users who will receive nudges in the future
- `stats`: Show statistics about nudge emails that have been sent
- `test-email`: Generate test emails for preview

**Key Flags:**
- `--type`: Email type (trial_welcome, trial_expectations, trial_engagement, trial_conversion, no_resume, no_call, five_star_matches)
- `--user-id`: User ID for real data
- `--test-data`: Use synthetic test data
- `--format`: Output format (plain, html, both, json)

### Appointments (`appointment`)

Manage user appointments and call processing.

```bash
trcli appointment [command]
```

**Available Commands:**
- `list`: List appointments
- `create --user <user-id>`: Create appointment for a user
- `latest`: Show latest appointment
- `reprocess`: Reprocess a completed appointment
- `send-recap`: Send recap email for an appointment

**Key Flags:**
- `--user`, `-u`: User ID
- `--appointment`, `-a`: Appointment ID
- `--completed`: Only show completed appointments
- `--send-recap`: Send recap email after reprocessing
- `--all`: Process all users

### Resume (`resume`)

Manage user resumes.

```bash
trcli resume [command]
```

**Available Commands:**
- `create <user-id> <resume-file>`: Create a resume for a user
- `list <user-id>`: List resumes for a user
- `annotate`: Annotate a resume with AI analysis
- `delete`: Delete a resume

**Key Flags:**
- `--user`, `-u`: User ID
- `--resume`, `-r`: Resume ID

### Payments (`payments`)

Manage payments and billing.

```bash
trcli payments [command]
```

**Available Commands:**
- `list`: List payments
- `webhooks`: List processed webhooks
- `upgrade`: Upgrade user subscription
- `reset`: Reset user subscription

### Pinecone (`pinecone`)

Manage vector database operations.

```bash
trcli pinecone [command]
```

**Available Commands:**
- `delete-all`: Delete all records from vector database
- `similarity`: Search for jobs using cosine similarity

**Key Flags:**
- `--dense`: Use dense index
- `--sparse`: Use sparse index
- `--namespace`: Pinecone namespace
- `--user`: User ID for similarity search

### Generate (`generate`)

Generate test data and AI content.

```bash
trcli generate [command]
```

**Available Commands:**
- `job-desc`: Generate job descriptions
- `steering-query`: Generate steering queries
- `statement`: Generate user statements

### Webhooks (`webhooks`)

Manage webhook data.

```bash
trcli webhooks [command]
```

**Available Commands:**
- `list`: List all webhook data
- `delete`: Delete webhook(s) by ID or prefix

**Key Flags:**
- `--id`: Webhook ID
- `--prefix`: Webhook prefix for bulk operations
- `--all`: Delete all webhooks
- `--force`: Skip confirmation prompts

### Gmail (`gmail`)

Manage Gmail integration.

```bash
trcli gmail [command]
```

**Available Commands:**
- `setup`: Setup Gmail OAuth
- `test`: Test Gmail connectivity
- `list`: List Gmail messages

### Geographic IDs (`geoids`)

Manage geographic location IDs.

```bash
trcli geoids [command]
```

**Available Commands:**
- `list`: List all geographic location IDs
- `search`: Search for geographic IDs

### Test Data (`test-data`)

Generate test data for development.

```bash
trcli test-data
```

**Key Flags:**
- `--users`: Number of test users to create (default: 5)
- `--jobs`: Number of test jobs to create per user (default: 10)
- `--prefix`: Prefix for test data IDs (default: "test-")

## Examples

### Processing SimilarJobs and PeopleAlsoViewed

```bash
# Process related jobs for a specific job detail
trcli job process-related --user user123 --job-id scrapingdog:12345 --criteria-id criteria456

# Dry run to see what would be processed
trcli job process-related --user user123 --job-id scrapingdog:12345 --criteria-id criteria456 --dry-run
```

### User Jobs Management

```bash
# List all jobs for a user
trcli userjobs list user123 --json

# Update job ratings for all users
trcli userjobs update --all

# Update job ratings for a specific user
trcli userjobs update user123

# Get follow-up questions for a user
trcli userjobs followup user123 --dump

# Update a specific job's status
trcli userjobs update user123 --id job456 --status applied

# Delete a specific job
trcli userjobs delete user123 --id job456

# Delete jobs by prefix
trcli userjobs delete user123 --prefix "test-"
```

### Job Details Management

```bash
# List job details with SimilarJobs and PeopleAlsoViewed
trcli job job --all --limit 20

# View a specific job detail
trcli job job scrapingdog:12345 --json

# List basic job listings
trcli job listings --all --limit 50

# Delete test job data
trcli job delete-details --prefix "test-"
```

### User Management

```bash
# List all users
trcli users list --json

# Create a new user
trcli users create --name "John Doe" --email "john@example.com"

# Delete a user by ID
trcli users delete --user-id user123

# Generate test data
trcli test-data --users 3 --jobs 5 --prefix "dev-"
```

### Search Operations

```bash
# Queue a job search with saved criteria
trcli search queue --criteria-id golang-developer-103644278

# List predicted nudges
trcli nudge list

# Show nudge statistics
trcli nudge stats
```

### Feedback Analysis

```bash
# Process all unprocessed feedback
trcli feedback process

# Show feedback statistics
trcli feedback stats

# Export user feedback data
trcli feedback export user123 --format csv
```

## Environment Variables

The CLI requires proper configuration of environment variables:

- `FIREBASE_CREDENTIALS`: Firebase service account credentials
- `SCRAPINGDOG_API_KEY`: ScrapingDog API key for job fetching
- `PINECONE_API_KEY`: Pinecone API key for vector operations
- `OPENAI_API_KEY`: OpenAI API key for AI operations

Make sure your `.env` file contains these variables, or set them in your system environment.

## Common Workflows

### 1. Processing Related Jobs Workflow

```bash
# 1. Find a job with good ratings
trcli job job --all --limit 20 | grep -A 5 -B 5 "scrapingdog:"

# 2. Process its related jobs
trcli job process-related --user user123 --job-id scrapingdog:12345 --criteria-id criteria456

# 3. Check the results
trcli userjobs list user123 --json
```

### 2. User Feedback Analysis Workflow

```bash
# 1. Process all feedback
trcli feedback process

# 2. Show statistics
trcli feedback stats

# 3. Analyze specific user
trcli feedback user-stats user123

# 4. Export data for analysis
trcli feedback export user123 --format csv > user123_feedback.csv
```

### 3. Data Cleanup Workflow

```bash
# 1. Delete test job data
trcli job delete-details --prefix "test-"
trcli job delete-listings --prefix "test-"

# 2. Clean up test user jobs
trcli job delete-user-jobs --prefix "test-"

# 3. Remove test users
trcli users delete --prefix "test-"
``` 