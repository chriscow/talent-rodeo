# Command Line Tools

## UserJobs Tool (`cmd/userjobs`)

A command-line utility for managing user jobs and search data in the Talent Rodeo application.

### Building

```bash
go build -o userjobs-tool ./cmd/userjobs
```

### Usage

```bash
./userjobs-tool [flags]
```

### Flags

- `--user string`: User ID to check (required for most operations)
- `--list`: List jobs for user
- `--pretty`: Pretty print JSON output
- `--status string`: Update job status (viewed, saved, applied, rejected)
- `--job string`: Job ID for updates
- `--notes string`: Update notes for job
- `--create-test`: Create test user and job data
- `--check-search`: Check user's search status

### Examples

1. Create test data:
```bash
./userjobs-tool --create-test
```

2. List jobs for a user (with pretty printing):
```bash
./userjobs-tool --user <user-id> --list --pretty
```

3. Check search status for a user:
```bash
./userjobs-tool --user <user-id> --check-search
```

4. Update job status:
```bash
./userjobs-tool --user <user-id> --job <job-id> --status viewed
```

5. Add notes to a job:
```bash
./userjobs-tool --user <user-id> --job <job-id> --notes "Interesting position"
```

### Output Format

#### Job Listing Output
When using `--list`, jobs are returned in JSON format with the following fields:
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

#### Search Status Output
When using `--check-search`, search information is returned in JSON format with the following fields:
```json
{
  "queued_at": "timestamp",
  "completed_at": "timestamp",
  "error": "string",
  "keywords": ["string"],
  "location": "string",
  "job_count": number
}
```

### Environment Variables

The tool requires proper configuration of Firebase credentials. Make sure either:
- A `.env` file exists in the project root with `FIREBASE_CREDENTIALS`
- The `FIREBASE_CREDENTIALS` environment variable is set in your system

### Error Handling

The tool will exit with status code 1 and display an error message if:
- Required flags are missing
- Firebase credentials are invalid
- Firestore operations fail
- Invalid job status is provided
- User or job is not found 