package subscription

import (
	"talent-rodeo/internal"
	"testing"
)

func TestPersonalSubscriptionMissingSettingsRemainPaused(t *testing.T) {
	processor := &Processor{mode: internal.RuntimeMode{Personal: true, UserID: "owner"}}
	if err := processor.RefreshSettings(); err == nil {
		t.Fatal("RefreshSettings() error = nil, want unavailable settings")
	}
	if !processor.Paused() {
		t.Fatal("Paused() = false after missing settings, want true")
	}
}
