# Rodeo Package

This package implements the core recruiting automation functionality for the Talent Rodeo application, managing the entire candidate interaction workflow from resume analysis to automated interviews.

## Overview

The rodeo package provides a sophisticated system for automated recruiting, including conversation management, candidate assessment, and workflow orchestration. It uses AI-powered interactions to conduct and analyze candidate interviews.

## Files

### types.go
Core data structures including:
- Conversation stages and state management
- Candidate information model
- Recruiter turn handling
- Goal tracking
- Engagement metrics

### workflow.go
Workflow orchestration including:
- Conversation workflow initialization
- Resume processing
- Interview planning
- Status tracking
- Workflow persistence

### plan.go
Interview planning functionality:
- Conversation strategy generation
- Question sequencing
- Topic coverage planning
- Goal setting
- Progress tracking

### chat.go
Chat interaction handling:
- Conversation management
- Response generation
- Context maintenance
- State transitions
- Engagement tracking

### resume.go
Resume processing functionality:
- Resume parsing
- Skill extraction
- Experience analysis
- Qualification assessment
- Format handling

### call.go
Call management functionality:
- Call scheduling
- Duration tracking
- Goal progression
- Context management
- Call state handling

### tools.go & tools.candidate-info.go
Utility functions for:
- Candidate data processing
- Information extraction
- Data validation
- Tool integration
- Helper functions

### keywords.go
Keyword management for:
- Skill identification
- Topic extraction
- Pattern matching
- Relevance scoring
- Term normalization

### status.go
Status management for:
- Progress tracking
- State updates
- Error handling
- Event notification
- Status broadcasting

## Key Features

1. Conversation Management
   - Multi-stage conversation flow
   - Context-aware responses
   - Goal-oriented progression
   - Engagement tracking
   - Dynamic adaptation

2. Candidate Assessment
   - Skill evaluation
   - Experience verification
   - Preference discovery
   - Cultural fit assessment
   - Red flag detection

3. Workflow Automation
   - Resume analysis
   - Interview planning
   - Response generation
   - Progress tracking
   - Status management

## Usage

Initialize a workflow:
```go
err := rodeo.StartWorkflow(ctx, resumeText, statusHandler)
```

Create a recruiter instance:
```go
recruiter := rodeo.NewRecruiter()
recruiter.InitializeCall()
```

Handle conversation turns:
```go
state := &ConversationState{
    Stage: ConversationStageInitialContact,
    Candidate: CandidateInfo{...},
}

// Process candidate response
turn := RecruiterTurn{
    KnownFacts: []string{...},
    EngagementLevel: EngagementLevelHigh,
}
```

## Conversation Stages

1. Planning Contact
   - Initial preparation
   - Resume review
   - Strategy development
   - Goal setting

2. Initial Contact
   - Introduction
   - Rapport building
   - Basic verification
   - Agenda setting

3. Discovery
   - Background exploration
   - Skill assessment
   - Experience verification
   - Preference discovery

4. Deep Dive
   - Technical assessment
   - Scenario exploration
   - Motivation understanding
   - Cultural fit evaluation

5. Follow Up
   - Clarification
   - Additional questions
   - Preference refinement
   - Next steps planning

6. Closing
   - Summary
   - Action items
   - Timeline setting
   - Next steps confirmation

## State Management

The package maintains comprehensive state tracking:
- Conversation progress
- Candidate information
- Known facts
- Inferred preferences
- Engagement metrics
- Goal completion
- Call context

## Integration Points

1. AI Integration
   - LLM-powered responses
   - Content generation
   - Context analysis
   - Pattern recognition

2. Database Integration
   - State persistence
   - Candidate records
   - Progress tracking
   - Metrics storage

3. External Services
   - Resume processing
   - Skill verification
   - Background checks
   - Communication channels 