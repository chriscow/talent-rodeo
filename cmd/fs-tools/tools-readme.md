# Firestore Tools

A command-line utility for managing users, jobs, and related data in the Talent Rodeo application's Firestore database.

## Building

```bash
go build -o fs-tools ./cmd/fs-tools
```

## Commands

### Users

```bash
fs-tools users [command]
```

Available Commands:
- `list`: List all users
- `delete`: Delete a user by ID
- `delete-by-email`: Delete a user by email
- `delete-by-prefix`: Delete users with IDs starting with a prefix

Flags:
- `--limit`: Maximum number of users to return (default: 100)
- `--offset`: Number of users to skip (default: 0)
- `--pretty`: Pretty print JSON output
- `--id`: User ID for deletion
- `--email`: User email for deletion
- `--prefix`: Prefix for bulk deletion

### Jobs

```bash
fs-tools jobs [command]
```

Available Commands:
- `list`: List jobs for a specific user
- `update`: Update job status or notes
- `delete`: Delete a job by ID
- `delete-listings`: Delete job listings by prefix
- `delete-details`: Delete job details by prefix
- `delete-user-jobs`: Delete user jobs by prefix across all users
- `listings`: List all job listings
- `details`: List all job details

Flags:
- `--user`: User ID (required for user-specific operations)
- `--id`: Job ID for updates/deletion
- `--status`: Update job status (viewed, saved, applied, rejected)
- `--notes`: Update notes for job
- `--prefix`: Prefix for bulk deletion
- `--limit`: Maximum number of items to return (default: 100)
- `--offset`: Number of items to skip (default: 0)
- `--pretty`: Pretty print JSON output

## Examples

1. List all users with pretty printing:
```bash
fs-tools users list --pretty
```

2. Delete a user by ID:
```bash
fs-tools users delete --id <user-id>
```

3. Delete users by prefix:
```bash
fs-tools users delete-by-prefix --prefix "test-"
```

4. List jobs for a user:
```bash
fs-tools jobs list --user <user-id> --pretty
```

5. Update job status:
```bash
fs-tools jobs update --user <user-id> --id <job-id> --status viewed
```

6. Add notes to a job:
```bash
fs-tools jobs update --user <user-id> --id <job-id> --notes "Interesting position"
```

7. List all job listings with pagination:
```bash
fs-tools jobs listings --limit 50 --offset 100 --pretty
```

8. Delete job listings by prefix:
```bash
fs-tools jobs delete-listings --prefix "company123"
```

9. Delete job details by prefix:
```bash
fs-tools jobs delete-details --prefix "company123"
```

10. Delete user jobs by prefix across all users:
```bash
fs-tools jobs delete-user-jobs --prefix "company123"
```

## Output Formats

### User Output
```json
{
  "id": "string",
  "email": "string",
  "name": "string",
  "phone": "string",
  "is_super_user": boolean,
  "created_at": "timestamp",
  "state": {
    "status": "string",
    "started": "timestamp"
  }
}
```

### Job Output
```json
{
  "id": "string",
  "status": "string",
  "viewed_at": "timestamp",
  "saved_at": "timestamp",
  "applied_at": "timestamp",
  "rejected_at": "timestamp",
  "notes": "string",
  "search_hash": "string",
  "created_at": "timestamp",
  "updated_at": "timestamp",
  "job_ref": "string"
}
```

## Environment Variables

The tool requires proper configuration of Firebase credentials. Make sure either:
- A `.env` file exists in the project root with `FIREBASE_CREDENTIALS`
- The `FIREBASE_CREDENTIALS` environment variable is set in your system

## Error Handling

The tool will exit with status code 1 and display an error message if:
- Required flags are missing
- Firebase credentials are invalid
- Firestore operations fail
- Invalid parameters are provided
- Resources are not found

## Pagination

Most list commands support pagination through the `--limit` and `--offset` flags:
- `--limit`: Controls how many items to return per page
- `--offset`: Controls how many items to skip

When there might be more results available, the tool will indicate how to view the next page. 