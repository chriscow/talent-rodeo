package scrapingdog

import (
	"context"
	"talent-rodeo/internal"
	"testing"
)

func TestPersonalProviderMissingSettingsRemainPaused(t *testing.T) {
	provider := &Provider{mode: internal.RuntimeMode{Personal: true, UserID: "owner"}}
	settings, err := provider.getWorkerSettings(context.Background())
	if err != nil {
		t.Fatalf("getWorkerSettings() error = %v", err)
	}
	if settings == nil || !settings.Paused {
		t.Fatalf("getWorkerSettings() = %+v, want paused", settings)
	}
}
