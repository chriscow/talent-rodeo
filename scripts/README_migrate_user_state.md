# User State Migration Script

## Overview

This script migrates User documents in Firestore from the old nested `State` structure to the new flat structure.

## Migration Details

### Old Structure
```json
{
  "ID": "user-id-123",
  "Name": "John Doe",
  "State": {
    "Status": 5,
    "Started": "2024-01-15T10:30:00Z",
    "Error": "optional error message"
  }
}
```

### New Structure
```json
{
  "ID": "user-id-123", 
  "Name": "John Doe",
  "Status": 5,
  "StatusUpdatedAt": "2024-01-15T10:30:00Z",
  "Error": "optional error message"
}
```

## Field Mapping

- `State.Status` → `Status` (top-level)
- `State.Started` → `StatusUpdatedAt` (top-level)
- `State.Error` → `Error` (top-level)
- `State` field is deleted

## Usage

### Dry Run (Safe - No Changes Made)
```bash
python scripts/migrate_user_state_to_top_level.py
```

### Apply Migration (Modifies Data)
```bash
python scripts/migrate_user_state_to_top_level.py --apply
```

## Safety Features

1. **Dry Run by Default**: Script runs in dry-run mode unless `--apply` flag is used
2. **Confirmation Prompt**: When using `--apply`, script asks for confirmation
3. **Skip Already Migrated**: Automatically detects and skips users that have already been migrated
4. **Error Handling**: Gracefully handles errors and continues processing other users
5. **Detailed Logging**: Provides verbose output about what changes are being made

## Prerequisites

1. Python 3.11+ (use `.venv/bin/python`)
2. Required environment variables in `.env`:
   - `FIREBASE_PROJECT_ID`
   - `FIREBASE_CREDENTIALS_FILE`
3. Required Python packages (already in requirements.txt):
   - `google-cloud-firestore`
   - `firebase-admin`
   - `python-dotenv`

## Running the Migration

1. **First, run a dry run to see what will be changed:**
   ```bash
   .venv/bin/python scripts/migrate_user_state_to_top_level.py
   ```

2. **Review the output carefully**

3. **If everything looks correct, apply the migration:**
   ```bash
   .venv/bin/python scripts/migrate_user_state_to_top_level.py --apply
   ```

## Example Output

```
Found 150 user documents
User abc-123: Moving Status 5 to top level
User abc-123: Moving Started to StatusUpdatedAt
DRY RUN - Would update User abc-123 with: {'Status': 5, 'StatusUpdatedAt': datetime(2024, 1, 15, 10, 30), 'State': <DELETE_FIELD>}

Migration Summary:
  Migrated: 145
  Already migrated: 3
  No State field: 2
  Errors: 0
  Total processed: 150
```

## Rollback

This migration removes the `State` field, so rollback would require:
1. A database backup (recommended before running)
2. A reverse migration script (not provided)

## Template Updates

The following templates have been updated to use the new structure:
- `assets/templates/pages/admin/subscriptions.html`
- `assets/templates/pages/admin/user-dashboard.html` 
- `assets/templates/pages/profile-resume-processing.html`

All references to `{{.State.Status.String}}` have been changed to `{{.Status.String}}`, etc. 