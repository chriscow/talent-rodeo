package inbox

import "time"

// ResponseType categorizes the type of response to send to the user.
// It determines the overall tone and content structure of the response.
type ResponseType int

const (
	// ResponseTypeConfirmation indicates all requested actions were completed successfully
	ResponseTypeConfirmation ResponseType = iota
	// ResponseTypePartial indicates some actions were completed, others were escalated
	ResponseTypePartial
	// ResponseTypeEscalation indicates nothing was handled and everything was escalated to admin
	ResponseTypeEscalation
)

// String returns the string representation of the ResponseType
func (rt ResponseType) String() string {
	switch rt {
	case ResponseTypeConfirmation:
		return "confirmation"
	case ResponseTypePartial:
		return "partial"
	case ResponseTypeEscalation:
		return "escalation"
	default:
		return "unknown"
	}
}

// IsValid checks if the ResponseType is a valid value
func (rt ResponseType) IsValid() bool {
	switch rt {
	case ResponseTypeConfirmation, ResponseTypePartial, ResponseTypeEscalation:
		return true
	default:
		return false
	}
}

// EmailResponse represents a generated response to a user's email.
// It contains all the information needed to send an appropriate reply.
type EmailResponse struct {
	// Type categorizes the response (confirmation, partial, escalation)
	Type ResponseType
	// Subject line for the response email
	Subject string
	// Body contains the plain text version of the response
	Body string
	// HTMLBody contains the HTML version of the response
	HTMLBody string
	// Summaries contains descriptions of actions taken by handlers
	Summaries []string
	// EscalatedItems contains descriptions of items that could not be handled
	EscalatedItems []string
	// GeneratedAt timestamp when the response was generated
	GeneratedAt time.Time
}

// IsValid validates that the EmailResponse has required fields
func (er *EmailResponse) IsValid() error {
	if er.Subject == "" {
		return ErrMissingSubject
	}
	if er.Body == "" && er.HTMLBody == "" {
		return ErrMissingBody
	}
	if !er.Type.IsValid() {
		return ErrInvalidResponseType
	}
	return nil
}

// HasContent returns true if the response has actionable content
func (er *EmailResponse) HasContent() bool {
	return len(er.Summaries) > 0 || len(er.EscalatedItems) > 0
}

// HandledCount returns the number of items that were successfully handled
func (er *EmailResponse) HandledCount() int {
	return len(er.Summaries)
}

// EscalatedCount returns the number of items that were escalated
func (er *EmailResponse) EscalatedCount() int {
	return len(er.EscalatedItems)
}