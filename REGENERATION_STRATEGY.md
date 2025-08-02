# Statement Version Cascade Regeneration Strategy

## Overview

This document outlines the implementation strategy for **cascade regeneration** - the ability to regenerate a statement version and automatically propagate those changes forward through all subsequent versions in the dependency chain.

## Current State vs Target State

### Current Implementation ✅
- **Single version regeneration**: Can recreate any specific version with a new prompt
- **Source data preservation**: Complete historical data for regeneration
- **Prompt compatibility checking**: Validates prompt versions before regeneration
- **Version history tracking**: Full audit trail of all changes

### Target Implementation 🎯
- **Cascade regeneration**: Regenerate a version and all subsequent dependent versions
- **Branch-and-replace logic**: Replace existing versions in the chain rather than creating new ones
- **Dependency-aware regeneration**: Understanding of version dependencies
- **System-aware prompt resolution**: Use each system's current prompt during cascade

## Understanding Statement Version Dependencies

### Version Chain Example
```
transcript(v1) → inbox(v2) → inbox(v3) → feedback(v4) → followup(v5) → feedback(v6)
     ↓              ↓          ↓           ↓            ↓             ↓
  [prompt.v001] [prompt.v001] [prompt.v001] [prompt.v001] [prompt.v001] [prompt.v001]
```

### Regeneration Scenario
When the **feedback prompt** is improved and we want to regenerate from v4 forward:

```
Original Chain:
transcript(v1) → inbox(v2) → inbox(v3) → feedback(v4) → followup(v5) → feedback(v6)

After Cascade Regeneration:
transcript(v1) → inbox(v2) → inbox(v3) → feedback(v4-NEW) → followup(v5-NEW) → feedback(v6-NEW)
                                            ↓                    ↓               ↓
                                       [prompt.v002]       [prompt.v001]   [prompt.v002]
```

## System-Aware Prompt Resolution

Each system defines its **current prompt** as a constant in the code:

### Current System Prompts
- **Transcript System**: `transcript.user-statement.v001.txt` (in `transcript_processor.go:976`)
- **Feedback System**: Uses prompts from `feedback/statement_updater.go`
- **Inbox System**: Uses prompts from `feedback/inbox_feedback.go` 
- **Followup System**: Uses prompts from `followup/` (when implemented)

### Prompt Resolution Strategy
During cascade regeneration, each system should use **its current prompt**, not necessarily the new prompt being applied:

1. **Target Version** (e.g., v4): Uses the new prompt specified by admin
2. **Subsequent Versions** (e.g., v5, v6): Use their respective system's current prompt
3. **Source Data**: Original transformation data is preserved and reused
4. **Base Statement**: Each version uses the newly regenerated previous version as input

## Implementation Strategy

### 1. Core Cascade Function

```go
// RegenerateCascade regenerates a version and all subsequent dependent versions
func RegenerateCascade(ctx context.Context, fs *FirestoreClient, versionID string, newPrompt string, dryRun bool) (*CascadeReport, error) {
    // 1. Load the target version and validate
    // 2. Build dependency chain forward from target version
    // 3. For each version in chain:
    //    a. Determine system type and current prompt
    //    b. Regenerate using appropriate prompt and previous version as base
    //    c. Replace existing version (not create new one)
    // 4. Update User.Statement field with final version
    // 5. Return comprehensive report
}
```

### 2. System Prompt Resolution

```go
// SystemPromptResolver resolves current prompts for each system
type SystemPromptResolver struct{}

func (r *SystemPromptResolver) GetCurrentPrompt(generatedBy string) string {
    switch generatedBy {
    case "transcript":
        return "transcript.user-statement.v001.txt"
    case "feedback":
        return "statement-update.v001.txt" 
    case "inbox":
        return "inbox-feedback.v001.txt"
    case "followup":
        return "followup-integration.v001.txt"
    default:
        return "" // Error case
    }
}
```

### 3. Version Dependency Chain

```go
// BuildDependencyChain builds the forward dependency chain from a starting version
func BuildDependencyChain(ctx context.Context, fs *FirestoreClient, userID string, fromVersionID string) ([]*UserStatementVersion, error) {
    // 1. Get all versions for user ordered by version number
    // 2. Find starting position in chain
    // 3. Return slice of all subsequent versions
}
```

### 4. Enhanced CLI Commands

```bash
# New cascade regeneration command
go run ./cmd/cli statements regenerate-cascade --user user123 --from-version user123_v4 --prompt feedback-v002.txt

# Options:
--dry-run           # Preview what would be regenerated
--stop-at-version   # Stop cascade at specific version  
--use-system-prompts # Use each system's current prompt (default: true)
--force-prompt      # Force same prompt for all versions in cascade
```

## Database Schema Updates

### Version Dependency Tracking

```go
type UserStatementVersion struct {
    // ... existing fields ...
    
    // New fields for cascade regeneration
    DependsOnVersionID    *string `json:"depends_on_version_id,omitempty"`    // Previous version this depends on
    DependentVersionIDs   []string `json:"dependent_version_ids,omitempty"`   // Versions that depend on this one
    SystemType           string   `json:"system_type"`                        // transcript, feedback, inbox, followup
    IsReplacedVersion    bool     `json:"is_replaced_version"`               // True if this version was replaced during cascade
}
```

### Cascade Operation Tracking

```go
type CascadeOperation struct {
    ID                   string                  `json:"id"`
    UserID              string                  `json:"user_id"`
    InitiatedAt         time.Time               `json:"initiated_at"`
    CompletedAt         *time.Time              `json:"completed_at,omitempty"`
    TriggerVersionID    string                  `json:"trigger_version_id"`     // Version that started the cascade
    TriggerPrompt       string                  `json:"trigger_prompt"`         // New prompt applied to trigger version
    VersionsAffected    []string                `json:"versions_affected"`      // All version IDs that were regenerated
    DryRun              bool                    `json:"dry_run"`
    Status              string                  `json:"status"`                 // "in_progress", "completed", "failed"
    Error               *string                 `json:"error,omitempty"`
}
```

## Admin Interface Updates

### 1. Cascade Regeneration Modal
- **Trigger Version Selection**: Choose which version to start cascade from
- **Prompt Selection**: Choose new prompt for trigger version
- **Cascade Options**: 
  - Stop at specific version
  - Use system prompts vs force same prompt
  - Dry run preview
- **Impact Preview**: Show which versions will be affected

### 2. Version History Enhancements
- **Dependency Visualization**: Show parent/child relationships between versions
- **Cascade History**: Show previous cascade operations
- **Version Comparison**: Compare before/after cascade results

## Implementation Phases

### Phase 1: Core Cascade Infrastructure
1. **Dependency Chain Builder**: Function to build forward dependency chains
2. **System Prompt Resolver**: Centralized prompt resolution for each system
3. **Cascade Regeneration Engine**: Core logic for replacing versions in sequence
4. **Cascade Report**: Comprehensive reporting of what was changed

### Phase 2: CLI Integration
1. **New CLI Commands**: `regenerate-cascade` command with full options
2. **Enhanced Progress Reporting**: Show cascade progress across multiple versions
3. **Dry Run Validation**: Complete preview of cascade effects

### Phase 3: Admin Interface Integration
1. **Cascade Modal**: User-friendly interface for initiating cascades
2. **Dependency Visualization**: Visual representation of version chains
3. **Cascade History**: Track and display previous cascade operations

### Phase 4: Advanced Features
1. **Selective Cascade**: Skip certain versions in the chain
2. **Rollback Capability**: Undo cascade operations
3. **Batch Cascade**: Apply same prompt improvements to multiple users

## Error Handling & Rollback Strategy

### Transaction Safety
- **Atomic Operations**: Each version replacement is atomic
- **Rollback Points**: Save original versions before replacement
- **Failure Recovery**: If cascade fails midway, rollback to original state

### Validation Checks
- **Prompt Compatibility**: Validate all prompts before starting cascade
- **Source Data Integrity**: Ensure all versions have valid source data
- **Dependency Validation**: Verify dependency chain is intact

## Testing Strategy

### Unit Tests
- **Dependency Chain Building**: Test chain construction logic
- **System Prompt Resolution**: Test prompt resolution for each system
- **Single Version Replacement**: Test atomic version replacement

### Integration Tests  
- **End-to-End Cascade**: Test complete cascade regeneration flow
- **Rollback Scenarios**: Test failure recovery and rollback
- **Multi-System Cascades**: Test cascades across different systems

### Performance Tests
- **Large Cascade Operations**: Test performance with long version chains
- **Concurrent Cascades**: Test multiple simultaneous cascade operations

## Migration Considerations

### Existing Data
- **Version Dependency Mapping**: Analyze existing versions to build dependency relationships
- **System Type Classification**: Classify existing versions by system type
- **Backward Compatibility**: Ensure existing regeneration still works

### Deployment Strategy
- **Feature Flag**: Deploy behind feature flag for gradual rollout
- **Admin-Only Initial**: Limit to admin users initially
- **Monitoring**: Comprehensive monitoring of cascade operations

This cascade regeneration strategy provides the exact branching and forward-propagation behavior you described, while maintaining the robust versioning and audit trail capabilities of the current system.