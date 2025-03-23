package auth

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/sessions"
)

// SCSAdapter adapts scs.SessionManager to implement gorilla/sessions.Store interface
type SCSAdapter struct {
	scs *scs.SessionManager
}

// NewSCSAdapter creates a new adapter for scs.SessionManager
func NewSCSAdapter(scs *scs.SessionManager) *SCSAdapter {
	return &SCSAdapter{scs: scs}
}

// Get returns a cached session.
func (s *SCSAdapter) Get(r *http.Request, name string) (*sessions.Session, error) {
	session := sessions.NewSession(s, name)
	// Load data from SCS context into gorilla session
	for _, key := range s.scs.Keys(r.Context()) {
		if val := s.scs.Get(r.Context(), key); val != nil {
			session.Values[key] = val
		}
	}
	return session, nil
}

// New creates and returns a new session.
func (s *SCSAdapter) New(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.NewSession(s, name), nil
}

// Save persists session to the underlying store implementation.
func (s *SCSAdapter) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	// Save data from gorilla session into SCS context
	for key, val := range session.Values {
		if keyStr, ok := key.(string); ok {
			s.scs.Put(r.Context(), keyStr, val)
		}
	}
	return nil
}
