# Cascade Regeneration System Analysis

## What This Feature Actually Does

The cascade regeneration system implements a "branch-and-replace" strategy for updating user statement versions when systems are updated with new prompts or logic. This is **NOT** about manually overriding prompts - it's about applying system updates across the dependency chain.

## Core Concept

### The Problem Being Solved
When a system like the feedback processor or transcript analyzer gets updated with improved prompts or logic, existing user statements were generated with the old version. The cascade system allows us to "replay" the statement generation process with the new system logic while preserving the original source data.

### The Solution: System-Aware Cascade Regeneration
Each statement version is tied to a specific system that generated it:
- **transcript** system generates initial statements from call transcripts
- **feedback** system enhances statements based on user feedback  
- **inbox** system processes email-based statement adjustments
- **followup** system integrates question/answer data

When cascading, each version is regenerated using **that system's current prompt**, not an arbitrary prompt passed by the user.

## How It Works

### 1. System Prompt Resolution
The cascade regeneration directly uses the prompts that the actual systems use:

```go
switch version.GeneratedBy {
case "transcript":
    promptToUse = "transcript.user-statement.v001.txt"
case "feedback":
    promptToUse = "statement-update.v001.txt"
default:
    // Skip unsupported systems
    result.Status = "skipped"
    result.Error = "unsupported generation method"
}
```

**Supported Systems:**
- **transcript**: Uses `transcript.user-statement.v001.txt` (matches `internal/transcript/transcript_processor.go:976`)
- **feedback**: Uses `statement-update.v001.txt` (matches `internal/feedback/statement_updater.go:53` and `internal/feedback/llm_processor.go:194`)

**Unsupported Systems:**
- **inbox**: No actual implementation exists that creates statement versions
- **followup**: No actual implementation exists that creates statement versions

### 2. Dependency Chain Building
`BuildDependencyChain()` identifies all versions that depend on a starting version by analyzing the version sequence. Given a starting version, it returns all subsequent versions that would be affected by changes to that version.

### 3. Cascade Regeneration Process
For each version in the dependency chain:

1. **Determine System Prompt**: Use the system's current prompt (not user-provided)
2. **Extract Source Data**: Retrieve the original data used to generate this version
3. **Regenerate**: Call the appropriate generation function with current prompt
4. **Replace In-Place**: Update the existing version record rather than creating new ones
5. **Maintain Chain**: Ensure the next version uses this newly regenerated version as input

### 4. Replace vs. Create New
This is a **replace** strategy, not **create new**:
- Version IDs stay the same (`user123_v4` remains `user123_v4`)
- Only the content, prompt references, and metadata are updated
- Dependency relationships are preserved
- No version number gaps are created

## CLI Interface

### Current Commands
```bash
# Cascade from specific version for one user
trcli statements regenerate --user user123 --version v4

# Regenerate everything for one user (with confirmation)
trcli statements regenerate --user user123

# Regenerate everything for all users (with confirmation)  
trcli statements regenerate --all

# Dry run to preview changes
trcli statements regenerate --user user123 --dry-run
```

### What Each Command Does

#### Single User, Specific Version
```bash
trcli statements regenerate --user user123 --version v4
```
1. Finds version `user123_v4` and all subsequent versions (`v5`, `v6`, etc.)
2. Regenerates `v4` using the **transcript system's current prompt**
3. Regenerates `v5` using the **feedback system's current prompt** with `v4`'s new content as input
4. Regenerates `v6` using the **followup system's current prompt** with `v5`'s new content as input
5. Updates User.Statement field to match the final version

#### Single User, All Versions
```bash
trcli statements regenerate --user user123
```
1. Prompts for confirmation (destructive operation)
2. Finds the first version for the user (`v1`)
3. Performs cascade regeneration starting from `v1`
4. Effectively regenerates the entire statement history with current system logic

#### All Users
```bash
trcli statements regenerate --all
```
1. Prompts for confirmation (very destructive operation)
2. Finds all users with statement versions
3. For each user, regenerates from their first version
4. Effectively updates the entire database with current system logic

## Use Cases

### 1. System Updates
When the transcript analysis prompt is improved:
```bash
# Apply the new transcript logic to all users
trcli statements regenerate --all
```

### 2. User-Specific Issues
When a specific user's statement chain has issues:
```bash
# Regenerate from version 3 forward with current system logic
trcli statements regenerate --user user123 --version v3
```

### 3. Testing System Changes
Before deploying prompt changes:
```bash
# Test what would happen with current prompts
trcli statements regenerate --user test_user --dry-run
```

## Data Flow Example

**Original State:**
- `user123_v1`: Generated by transcript system with `transcript.user-statement.v001.txt`
- `user123_v2`: Generated by feedback system with `statement-update.v001.txt` 
- `user123_v3`: Generated by followup system with `followup-integration.v001.txt`

**After Cascade from v1:**
- `user123_v1`: Regenerated with transcript system's **current** prompt
- `user123_v2`: Regenerated with feedback system's **current** prompt using new v1 content
- `user123_v3`: Regenerated with followup system's **current** prompt using new v2 content

**Key Point:** Each version uses **its system's current prompt**, not the original prompt or a user-specified prompt.

## Why No `--prompt` Flag

The `--prompt` flag was fundamentally flawed because:

1. **Systems own their prompts**: Each system (transcript, feedback, etc.) defines its own prompt
2. **Prompts are system-specific**: You can't use a feedback prompt for transcript generation
3. **Current prompts are authoritative**: The goal is to apply current system logic, not arbitrary prompts
4. **Chain integrity**: Each version must use its appropriate system's logic

The correct behavior is for the system to automatically determine the right prompt for each version based on which system generated it.

## Implementation Details

### Database Schema
- `SystemType`: Identifies which system generated the version
- `PromptFile`: Updated to reflect the current prompt used during regeneration
- `TransformationSteps`: Tracks each regeneration with cascade metadata
- `SourceData`: Preserved from original to enable regeneration

### Error Handling
- Source data validation (can't regenerate without original data)
- Prompt compatibility checking  
- System validation (prompt must match generation method)
- Transaction safety (all-or-nothing updates)

### Reporting
- Detailed reports showing which versions were processed
- Success/failure tracking per version
- Dry-run mode for safe testing
- Progress tracking for large operations

## Security and Safety

### Confirmation Prompts
- Required for destructive operations (all versions, all users)
- Clear warnings about scope of changes
- Dry-run mode always available

### Atomic Operations
- Uses Firestore transactions
- All-or-nothing updates per user
- Rollback on any failure

### Audit Trail
- Complete transformation history preserved
- Source data maintained for future regenerations
- Detailed logging of all operations

## Performance Considerations

### Parallel Processing
- Each user can be processed independently
- Within a user, versions must be processed sequentially (dependency chain)

### Resource Usage
- LLM API calls for each version regeneration
- Cost tracking per operation
- Token usage monitoring

### Scalability
- Designed for background processing
- Progress reporting for long operations
- Resumable operations (future enhancement)

This system enables the platform to evolve its AI logic while maintaining user data integrity and providing a clear path for applying improvements across the entire user base.