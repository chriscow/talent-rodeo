package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestShowDashboard(t *testing.T) {
	ts := newTestServer(t, "dashboard")
	defer ts.Cleanup()

	tests := []struct {
		name            string
		isAuthenticated bool
		wantCode        int
		wantBody        string
	}{
		{
			name:            "Unauthenticated Access Redirects to Home",
			isAuthenticated: false,
			wantCode:        http.StatusSeeOther, // 303 redirect
			wantBody:        "",
		},
		{
			name:            "Authenticated Access Shows Dashboard",
			isAuthenticated: true,
			wantCode:        http.StatusOK,
			wantBody:        "<h1>Dashboard</h1>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var code int
			var _, body string

			if tt.isAuthenticated {
				code, _, body = ts.authenticatedGet(t, "/dashboard")
			} else {
				code, _, body = ts.get(t, "/dashboard")
			}

			if code != tt.wantCode {
				t.Errorf("want %d; got %d", tt.wantCode, code)
			}

			if tt.wantBody != "" && !strings.Contains(body, tt.wantBody) {
				t.Errorf("want body to contain %q", tt.wantBody)
			}
		})
	}
}
