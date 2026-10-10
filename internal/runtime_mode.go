package internal

import "time"

// RuntimeMode scopes a web process to production or an explicitly selected owner.
// Its zero value preserves regular production behavior.
type RuntimeMode struct {
	Personal bool
	UserID   string
}

// AllowsUser reports whether the process may act on userID.
func (m RuntimeMode) AllowsUser(userID string) bool {
	return !m.Personal || (m.UserID != "" && userID == m.UserID)
}

// CanMatch reports whether the user is eligible for job matching now.
// Personal eligibility requires confirmed preferences and does not change billing.
func (m RuntimeMode) CanMatch(user *User, now time.Time) bool {
	if user == nil || !m.AllowsUser(user.ID) {
		return false
	}
	if !m.Personal {
		return user.SubscriptionEnd != nil && user.SubscriptionEnd.After(now)
	}
	if user.PersonalSearchStartedAt == nil || user.PersonalSearchStartedAt.After(now) {
		return false
	}
	switch user.Status {
	case UserStatusJobSearchScheduled, UserStatusJobSearchStarted, UserStatusJobSearchComplete:
		return true
	default:
		return false
	}
}
