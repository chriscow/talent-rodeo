# Onboarding Package

This package manages the user onboarding workflow for the Talent Rodeo application, tracking the progression of users through various onboarding steps.

## Overview

The onboarding package defines the structure and logic for managing user onboarding state, including step progression, timing tracking, and state management. It implements a linear workflow from initial signup through job search initiation.

## Files

### types.go
Core onboarding types and logic including:
- Step enumeration and constants
- State management structure
- Progress tracking
- Step transition logic
- Timing management

Key features:
- Well-defined onboarding steps
- State tracking
- Progress validation
- Timestamp management
- Database mapping

## Key Features

1. Onboarding Steps
   - Initial state
   - Resume upload
   - Phone call scheduling
   - Phone call completion
   - Preferences extraction
   - Job search initiation
   - Process completion

2. State Management
   - Current step tracking
   - Start time tracking
   - Completion time tracking
   - Step update timing
   - Progress validation

3. Step Progression
   - Linear workflow
   - Step validation
   - Next step determination
   - Progress tracking
   - Completion checking

## Usage

State management:
```go
// Check onboarding status
state := &onboarding.State{
    CurrentStep: onboarding.StepInitial,
    StartedAt:   &now,
}

// Check completion
if state.IsComplete() {
    // Handle completion
}

// Check progress
if state.IsInProgress() {
    // Handle in-progress state
}

// Get next step
nextStep := state.NextStep()
```

Step progression:
```go
// Progress through steps
switch state.CurrentStep {
case onboarding.StepInitial:
    // Handle initial step
case onboarding.StepResumeUploaded:
    // Handle resume upload
case onboarding.StepPhoneCallScheduled:
    // Handle call scheduling
}
```

## Database Integration

The package is designed for database persistence:
- `db` tags for column mapping
- Nullable timestamp fields
- Step string representation
- State tracking fields

## Workflow Steps

1. Initial (`StepInitial`)
   - Starting state
   - User registration complete
   - Ready for resume upload

2. Resume Upload (`StepResumeUploaded`)
   - Resume received
   - Document processed
   - Ready for call scheduling

3. Phone Call Scheduling (`StepPhoneCallScheduled`)
   - Call time selected
   - Reminder set
   - Awaiting call

4. Phone Call Completion (`StepPhoneCallCompleted`)
   - Call conducted
   - Recording saved
   - Ready for processing

5. Preferences Extraction (`StepPreferencesExtracted`)
   - Call analyzed
   - Preferences identified
   - Ready for job search

6. Job Search (`StepJobSearchStarted`)
   - Search criteria set
   - Initial search performed
   - Matches being found

7. Complete (`StepComplete`)
   - Process finished
   - Regular operations begin
   - Ongoing job matching

## State Validation

The package provides several validation methods:
- Completion checking
- Progress validation
- Step sequencing
- Timestamp validation
- State consistency checks 