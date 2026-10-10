package internal

import (
	"testing"
	"time"
)

func TestRuntimeModeCanMatch(t *testing.T) {
	now := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	started := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	tests := []struct {
		name string
		mode RuntimeMode
		user *User
		want bool
	}{
		{name: "production subscriber", user: &User{ID: "owner", SubscriptionEnd: &future}, want: true},
		{name: "production expired", user: &User{ID: "owner", SubscriptionEnd: &started}},
		{name: "personal confirmed", mode: RuntimeMode{Personal: true, UserID: "owner"}, user: &User{ID: "owner", Status: UserStatusJobSearchScheduled, PersonalSearchStartedAt: &started}, want: true},
		{name: "personal other user", mode: RuntimeMode{Personal: true, UserID: "owner"}, user: &User{ID: "other", Status: UserStatusJobSearchScheduled, PersonalSearchStartedAt: &started}},
		{name: "personal future activation", mode: RuntimeMode{Personal: true, UserID: "owner"}, user: &User{ID: "owner", Status: UserStatusJobSearchScheduled, PersonalSearchStartedAt: &future}},
		{name: "personal unconfirmed", mode: RuntimeMode{Personal: true, UserID: "owner"}, user: &User{ID: "owner", Status: UserStatusResumeUploaded, PersonalSearchStartedAt: &started}},
		{name: "personal missing activation", mode: RuntimeMode{Personal: true, UserID: "owner"}, user: &User{ID: "owner", Status: UserStatusJobSearchScheduled}},
		{name: "personal empty owner", mode: RuntimeMode{Personal: true}, user: &User{ID: "owner", Status: UserStatusJobSearchScheduled, PersonalSearchStartedAt: &started}},
		{name: "nil user", mode: RuntimeMode{Personal: true, UserID: "owner"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.CanMatch(tt.user, now); got != tt.want {
				t.Errorf("CanMatch(%s) = %t, want %t", tt.name, got, tt.want)
			}
		})
	}
}
