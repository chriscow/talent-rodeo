package search

import (
	"talent-rodeo/internal"
	"testing"
)

func TestPersonalSchedulerMissingSettingsRemainPaused(t *testing.T) {
	scheduler := &Scheduler{mode: internal.RuntimeMode{Personal: true, UserID: "owner"}}
	if err := scheduler.RefreshSettings(); err == nil {
		t.Fatal("RefreshSettings() error = nil, want unavailable settings")
	}
	if !scheduler.Paused() {
		t.Fatal("Paused() = false after missing settings, want true")
	}
}
