package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
)

// OnboardingStep represents the current step in the onboarding process
type OnboardingStep string

const (
	// Onboarding step constants
	OnboardingStepResumeUpload      OnboardingStep = "resume_upload"
	OnboardingStepResumeProcessing  OnboardingStep = "resume_processing"
	OnboardingStepCommunicationChoice OnboardingStep = "communication_choice"
	OnboardingStepActiveConversation OnboardingStep = "active_conversation"
	OnboardingStepCompleted         OnboardingStep = "completed"
)

// OnboardingSession tracks the user's progress through the onboarding process
// This is stored in the session and persists across page refreshes
type OnboardingSession struct {
	Step            OnboardingStep    `json:"step"`
	UserID          string           `json:"user_id"`
	ResumeID        string           `json:"resume_id,omitempty"`
	AppointmentID   string           `json:"appointment_id,omitempty"`
	ConversationType string          `json:"conversation_type,omitempty"` // "voice" or "chat"
	FormData        map[string]interface{} `json:"form_data,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// OnboardingSessionManager handles session-based onboarding state management
type OnboardingSessionManager struct {
	sessionManager *scs.SessionManager
}

// NewOnboardingSessionManager creates a new onboarding session manager
func NewOnboardingSessionManager(sessionManager *scs.SessionManager) *OnboardingSessionManager {
	return &OnboardingSessionManager{
		sessionManager: sessionManager,
	}
}

// GetSession retrieves the current onboarding session from the HTTP request
func (osm *OnboardingSessionManager) GetSession(r *http.Request) (*OnboardingSession, error) {
	sessionData := osm.sessionManager.GetString(r.Context(), "onboarding_session")
	if sessionData == "" {
		return nil, nil // No session exists
	}

	var session OnboardingSession
	if err := json.Unmarshal([]byte(sessionData), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal onboarding session: %w", err)
	}

	return &session, nil
}

// SetSession stores the onboarding session in the HTTP request context
func (osm *OnboardingSessionManager) SetSession(r *http.Request, session *OnboardingSession) error {
	session.UpdatedAt = time.Now()
	
	sessionData, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal onboarding session: %w", err)
	}

	osm.sessionManager.Put(r.Context(), "onboarding_session", string(sessionData))
	return nil
}

// CreateSession creates a new onboarding session for the user
func (osm *OnboardingSessionManager) CreateSession(r *http.Request, userID string, initialStep OnboardingStep) (*OnboardingSession, error) {
	now := time.Now()
	session := &OnboardingSession{
		Step:      initialStep,
		UserID:    userID,
		FormData:  make(map[string]interface{}),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := osm.SetSession(r, session); err != nil {
		return nil, fmt.Errorf("failed to create onboarding session: %w", err)
	}

	return session, nil
}

// UpdateStep updates the current step in the onboarding session
func (osm *OnboardingSessionManager) UpdateStep(r *http.Request, step OnboardingStep) error {
	session, err := osm.GetSession(r)
	if err != nil {
		return fmt.Errorf("failed to get session for step update: %w", err)
	}
	
	if session == nil {
		return fmt.Errorf("no onboarding session found")
	}

	session.Step = step
	return osm.SetSession(r, session)
}

// SetFormData stores form data in the session for later use
func (osm *OnboardingSessionManager) SetFormData(r *http.Request, key string, value interface{}) error {
	session, err := osm.GetSession(r)
	if err != nil {
		return fmt.Errorf("failed to get session for form data: %w", err)
	}
	
	if session == nil {
		return fmt.Errorf("no onboarding session found")
	}

	if session.FormData == nil {
		session.FormData = make(map[string]interface{})
	}

	session.FormData[key] = value
	return osm.SetSession(r, session)
}

// GetFormData retrieves form data from the session
func (osm *OnboardingSessionManager) GetFormData(r *http.Request, key string) (interface{}, bool) {
	session, err := osm.GetSession(r)
	if err != nil || session == nil {
		return nil, false
	}

	if session.FormData == nil {
		return nil, false
	}

	value, exists := session.FormData[key]
	return value, exists
}

// SetResumeID stores the resume ID in the session
func (osm *OnboardingSessionManager) SetResumeID(r *http.Request, resumeID string) error {
	session, err := osm.GetSession(r)
	if err != nil {
		return fmt.Errorf("failed to get session for resume ID: %w", err)
	}
	
	if session == nil {
		return fmt.Errorf("no onboarding session found")
	}

	session.ResumeID = resumeID
	return osm.SetSession(r, session)
}

// SetAppointmentID stores the appointment ID in the session
func (osm *OnboardingSessionManager) SetAppointmentID(r *http.Request, appointmentID string) error {
	session, err := osm.GetSession(r)
	if err != nil {
		return fmt.Errorf("failed to get session for appointment ID: %w", err)
	}
	
	if session == nil {
		return fmt.Errorf("no onboarding session found")
	}

	session.AppointmentID = appointmentID
	return osm.SetSession(r, session)
}

// SetConversationType stores the conversation type (voice/chat) in the session
func (osm *OnboardingSessionManager) SetConversationType(r *http.Request, conversationType string) error {
	session, err := osm.GetSession(r)
	if err != nil {
		return fmt.Errorf("failed to get session for conversation type: %w", err)
	}
	
	if session == nil {
		return fmt.Errorf("no onboarding session found")
	}

	session.ConversationType = conversationType
	return osm.SetSession(r, session)
}

// ClearSession removes the onboarding session from storage
func (osm *OnboardingSessionManager) ClearSession(r *http.Request) error {
	osm.sessionManager.Remove(r.Context(), "onboarding_session")
	return nil
}

// GetOrCreateSession gets the existing session or creates a new one
func (osm *OnboardingSessionManager) GetOrCreateSession(r *http.Request, userID string) (*OnboardingSession, error) {
	session, err := osm.GetSession(r)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing session: %w", err)
	}

	if session != nil {
		return session, nil
	}

	// Create new session based on user status
	// This will be determined by the current user's UserStatus
	initialStep := OnboardingStepResumeUpload // Default starting step
	return osm.CreateSession(r, userID, initialStep)
}

// ValidateStep checks if the current step is valid for progression
func (osm *OnboardingSessionManager) ValidateStep(ctx context.Context, session *OnboardingSession) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	// Validate step progression logic
	switch session.Step {
	case OnboardingStepResumeUpload:
		// Always valid as starting step
		return nil
	case OnboardingStepResumeProcessing:
		// Must have resume ID
		if session.ResumeID == "" {
			return fmt.Errorf("resume processing step requires resume ID")
		}
		return nil
	case OnboardingStepCommunicationChoice:
		// Must have processed resume
		if session.ResumeID == "" {
			return fmt.Errorf("communication choice step requires processed resume")
		}
		return nil
	case OnboardingStepActiveConversation:
		// Must have appointment and conversation type
		if session.AppointmentID == "" || session.ConversationType == "" {
			return fmt.Errorf("active conversation step requires appointment and conversation type")
		}
		return nil
	case OnboardingStepCompleted:
		// Must have completed conversation
		if session.AppointmentID == "" {
			return fmt.Errorf("completed step requires appointment")
		}
		return nil
	default:
		return fmt.Errorf("invalid onboarding step: %s", session.Step)
	}
}