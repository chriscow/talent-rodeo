# Feedback System Analysis

## Overview

The `internal/feedback` package implements an LLM-powered feedback analysis chain that processes user job feedback to dynamically update their search preferences (steering queries). This system represents a sophisticated approach to learning from user interactions and improving job recommendation relevance over time.

## System Architecture

### Core Components

1. **LLM Processor** (`llm_processor.go`) - Central orchestrator 
2. **Analysis Chain** (`feedback_analyzer.go`) - Semantic feedback analysis
3. **Query Update Chain** (`query_updater.go`) - Steering query refinement
4. **Validation Chain** (`validation_chain.go`) - Query validation
5. **Feedback Store** (`feedback_store.go`) - Data access abstraction
6. **LLM Client** (`llm_client.go`) - AI integration interface
7. **Processor** (`feedback_processor.go`) - Scheduled batch processing

### Data Flow

```
User Feedback → Analysis Chain → Query Update Chain → Validation Chain → Updated Steering Query
```

The system operates as a three-phase pipeline:

1. **Phase 1 - Semantic Analysis**: Extract themes and patterns from user likes/dislikes
2. **Phase 2 - Query Updates**: Modify steering query based on discovered themes
3. **Phase 3 - Validation**: Ensure updated query is coherent and valid

## Key Types and Data Structures

### Core Feedback Processing Types

- **`UserFeedback`** - Individual like/dislike on job components
  - Fields: `JobID`, `ItemID`, `ItemText`, `Liked`, `ProcessedAt`
  - Granular feedback on specific job aspects (responsibilities, skills, requirements)

- **`SteeringQuery`** - User's search preferences in semantic vector space
  - Fields: `Queries`, `MoveToward`, `MoveAway`, `ForceAway`, `ExperienceLevel`, `WorkTypes`
  - Used for semantic search and job ranking

- **`SemanticFeedbackAnalysis`** - LLM analysis output
  - Fields: `LikedThemes`, `DislikedThemes`, `ConfidenceScore`, `TrendingPatterns`, `Reasoning`
  - Structured representation of user preference patterns

### Supporting Types

- **`SemanticTheme`** - Individual preference themes
- **`FeedbackData`** - Prepared data for LLM analysis
- **`ProcessorSettings`** - Background processing configuration

## Implementation Status

### ✅ **PHASE 1 COMPLETE** - Foundation & Testing (Week 1)

**Core Infrastructure:**
1. **Data Models**: All core types well-defined and documented
2. **Database Integration**: Complete Firestore integration with proper error handling
3. **LLM Integration**: Structured LLM calls using the minds library with DeepSeekChat
4. **Statement Update Chain**: ✅ **NEW PRIMARY FUNCTION** - Updates user job preference statements
5. **Mock Implementations**: ✅ **FIXED** - Complete `mockLLMClient` with all required methods
6. **Validation Chain**: ✅ **IMPLEMENTED** - Basic validation with proper error handling
7. **Feedback Store**: ✅ **ENHANCED** - Added statement CRUD operations (`GetUserStatement`, `SetUserStatement`)

**Prompt Templates:**
- `feedback-analysis-semantic.v001.txt` - Semantic analysis (✅ Template syntax fixed)
- `feedback-query-update.v001.txt` - Query refinement (✅ Template syntax fixed)  
- `statement-update.v001.txt` - ✅ **NEW** - Statement update with clear guidelines

**Testing & Quality:**
- ✅ **Complete Test Coverage**: All tests passing (`go test ./internal/feedback/... -v`)
- ✅ **Build Verification**: Clean compilation (`go build -o /tmp/talent-rodeo ./cmd/web`)
- ✅ **Type Safety**: Structured LLM responses with proper Go type validation

**Updated Architecture:**
```
User Feedback → Analysis Chain → Statement Update Chain (PRIMARY) → Query Update Chain (LEGACY) → Validation Chain
```

### 🚧 **READY FOR PHASE 2** - Integration & Real Testing

1. **Integration Tests**: Need end-to-end testing with real LLM calls
2. **Daily Processing**: Ready to implement scheduled batch processing  
3. **Monitoring**: Ready to add metrics collection for analysis quality

### 📋 **DEFERRED TO LATER PHASES**

1. **Advanced Validation**: LLM-powered validation logic (keeping basic validation for now)
2. **Performance Optimization**: Caching and batch processing improvements
3. **Error Recovery**: Advanced error handling and recovery strategies

## Prompt Engineering

The system uses sophisticated prompt templates:

### Semantic Analysis Prompt
- Extracts themes with confidence scores and keywords
- Identifies temporal patterns in preferences
- Provides reasoning for analysis decisions
- Outputs structured JSON for type safety

### Query Update Prompt
- Semantically merges new preferences with existing queries
- Maintains coherence across multiple preference dimensions
- Preserves user intent while incorporating feedback

## Technical Integration

### LLM Configuration
- **Model**: DeepSeekChat for cost-effectiveness
- **Structured Responses**: Type-safe JSON outputs using Go generics
- **Token Limits**: 8192 for analysis, 2048 for query updates
- **Library**: Custom `github.com/chriscow/minds/tools` wrapper

### Database Design
- **Firestore Collections**: `user_feedback`, `users` (for steering queries)
- **Processing State**: `ProcessedAt` timestamp for incremental processing
- **Batch Operations**: Efficient bulk updates for large feedback volumes

### Background Processing
- **Scheduling**: 24-hour intervals with configurable time windows
- **Resilience**: Error logging with Sentry integration, continues on individual failures
- **Settings**: Cloudflare KV for dynamic configuration
- **Concurrency**: Processes multiple users in parallel safely

## Architectural Strengths

1. **Separation of Concerns**: Clear boundaries between analysis, updates, and validation
2. **Testability**: Interface-based design enables easy mocking
3. **Scalability**: Batch processing with configurable limits
4. **Type Safety**: Structured LLM responses with Go type validation
5. **Flexibility**: Template-based prompts allow iteration without code changes
6. **Observability**: Comprehensive logging throughout the pipeline

## Current Limitations

1. **Validation Gap**: The validation chain needs actual LLM-powered coherence checking
2. **Testing Gap**: Missing mock implementations prevent proper unit testing
3. **Error Recovery**: Limited ability to recover from partial failures
4. **Feedback Quality**: No analysis of LLM output quality or consistency
5. **Performance**: No caching of similar feedback patterns

## Questions for Clarification

1. **Feedback Granularity**: How granular should the ItemID be? Currently supports job components like "resp-1", "skill-2" - is this the intended level?

2. **Processing Frequency**: Is daily processing optimal, or should it be more/less frequent based on user activity?

3. **Validation Strategy**: Should validation be LLM-powered (expensive but thorough) or rule-based (fast but limited)?

4. **Error Handling**: What should happen when LLM analysis fails - retry, skip, or revert to default behavior?

5. **User Feedback Volume**: What's the expected volume of feedback per user? This affects batch sizing and processing strategies.

6. **Prompt Evolution**: How should prompt templates be versioned and migrated when improved?

## ✅ **PHASE 1 COMPLETED SUCCESSFULLY**

**What We Built:**
- Complete statement update chain as the primary function
- Comprehensive test coverage with proper mocks
- Fixed all template syntax issues
- Enhanced feedback store with statement operations
- Basic validation with good error handling
- All tests passing and code building successfully

**Key Architectural Decisions:**
1. **Statement Updates First**: Prioritized over steering query updates
2. **Structured LLM Calls**: Using `tools.StructuredAsk` for type safety
3. **Simple Templates**: Removed conflicting JSON examples
4. **Dual Update**: Statement (primary) + steering query (legacy support)
5. **Graceful Degradation**: Statement updates succeed even if steering query fails

## ✅ **PHASE 2 COMPLETED SUCCESSFULLY - Integration & Real Testing**

**What We Built:**
1. **Integration Testing Framework** (`integration_test.go`) - Complete end-to-end testing with real LLM calls
2. **Metrics Collection System** (`metrics.go`) - Comprehensive processing metrics and monitoring
3. **Enhanced LLM Processor** - Integrated metrics collection throughout the pipeline
4. **Documentation** (`README_INTEGRATION_TESTING.md`) - Complete testing and usage documentation

**Integration Testing Features:**
- Real LLM testing with DeepSeekChat
- End-to-end pipeline validation
- Realistic test data scenarios  
- Statement update verification
- API key management and CI skipping

**Monitoring & Metrics:**
- Processing duration tracking
- Success/failure rate monitoring
- Analysis confidence scoring
- Statement/query update status
- Structured logging for production debugging
- Daily processing summaries

## 📋 **READY FOR PHASE 3 - Production Deployment & Daily Processing**

**Next Steps:**
1. **Daily Batch Processing**: Deploy scheduled processing to production
2. **Production Monitoring**: Enable real-time metrics collection
3. **Performance Optimization**: Monitor and optimize processing times
4. **Error Alerting**: Set up alerts for processing failures
5. **Scale Testing**: Verify system handles expected user volumes

**Phase 3 Goal**: Deploy to production with reliable daily processing and monitoring.

The integration testing and monitoring framework is complete and ready for production! 🚀

## CLI Commands

The feedback system includes comprehensive CLI commands for management and troubleshooting:

### Available Commands

```bash
# Process all unprocessed feedback (daily batch job)
cli feedback process

# Process feedback for a specific user
cli feedback process-user [userID]

# Show feedback statistics and unprocessed counts
cli feedback stats

# Clean up orphaned feedback for deleted users
cli feedback cleanup-orphaned
```

### Command Details

**`cli feedback process`**
- Processes all unprocessed user feedback across the system
- Designed to be run as a daily batch job
- Uses enhanced LLM processor with metrics collection
- Handles errors gracefully and continues processing other users
- Automatically manages orphaned feedback for deleted users

**`cli feedback process-user [userID]`**
- Processes feedback for a specific user
- Useful for testing or targeted processing
- Shows detailed processing logs and metrics
- Example: `cli feedback process-user 58d3e019-9db5-4791-bcfd-8cd5ab9b575a`

**`cli feedback stats`**
- Displays comprehensive feedback statistics
- Shows total unprocessed feedback counts
- Breaks down by liked/disliked items
- Lists unique users and jobs with feedback
- Supports `--json` flag for structured output
- Shows top users by feedback volume

**`cli feedback cleanup-orphaned`**
- Identifies and cleans up feedback for deleted users
- Marks orphaned feedback as processed to prevent infinite retries
- Prevents feedback processing errors caused by deleted users
- Safe to run multiple times (idempotent operation)
- Logs all cleanup actions for auditing

### Error Handling & Resilience

The CLI commands include robust error handling:

- **Deleted User Handling**: Automatically detects and handles feedback for deleted users
- **Graceful Degradation**: Individual user failures don't stop batch processing
- **Orphaned Data Cleanup**: Prevents infinite retry loops for deleted users
- **Detailed Logging**: Comprehensive logging for troubleshooting and monitoring
- **Metrics Collection**: Processing duration, success rates, and confidence scores

### Usage Examples

```bash
# Daily processing (production use)
cli feedback process

# Check system health
cli feedback stats --json

# Clean up after user deletions
cli feedback cleanup-orphaned

# Debug specific user issues
cli feedback process-user abc123

# Get help for any command
cli feedback --help
cli feedback process --help
```

### Integration with Production

- Commands designed for production automation
- Compatible with cron jobs and scheduled tasks
- Structured logging for monitoring systems
- Exit codes indicate success/failure for automation
- JSON output option for programmatic processing

The CLI provides complete operational control over the feedback processing system! 