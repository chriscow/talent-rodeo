package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"talent-rodeo/internal"
)

func TestShowProfilePage(t *testing.T) {
	ts := newTestServer(t, "profile")
	defer ts.Cleanup()

	tests := []struct {
		name            string
		isAuthenticated bool
		setupUser       func(*internal.User) // Function to set up user state
		wantCode        int
		wantBody        string
	}{
		{
			name:            "Unauthenticated Access Redirects to Home",
			isAuthenticated: false,
			setupUser:       nil,
			wantCode:        http.StatusSeeOther,
			wantBody:        "",
		},
		{
			name:            "New User Shows Resume Upload",
			isAuthenticated: true,
			setupUser: func(u *internal.User) {
				internal.UpdateUserStatus(context.Background(), ts.app.fs, u, internal.UserStatusPendingResumeUpload)
			},
			wantCode: http.StatusOK,
			wantBody: "Upload Resume",
		},
		{
			name:            "Processing Resume Shows Processing Page",
			isAuthenticated: true,
			setupUser: func(u *internal.User) {
				internal.UpdateUserStatus(context.Background(), ts.app.fs, u, internal.UserStatusProcessingResume)
			},
			wantCode: http.StatusOK,
			wantBody: "processing",
		},
		{
			name:            "Resume Uploaded Shows Schedule Page",
			isAuthenticated: true,
			setupUser: func(u *internal.User) {
				internal.UpdateUserStatus(context.Background(), ts.app.fs, u, internal.UserStatusResumeUploaded)
			},
			wantCode: http.StatusOK,
			wantBody: "Schedule",
		},
		{
			name:            "Call Scheduled Shows Waiting Page",
			isAuthenticated: true,
			setupUser: func(u *internal.User) {
				internal.UpdateUserStatus(context.Background(), ts.app.fs, u, internal.UserStatusCallScheduled)
				// Create a test appointment
				internal.CreateTestAppointment(t, ts.app.fs, u)
			},
			wantCode: http.StatusSeeOther,
			wantBody: "",
		},
		{
			name:            "Error State Resets to Resume Upload",
			isAuthenticated: true,
			setupUser: func(u *internal.User) {
				internal.UpdateUserStateWithError(context.Background(), ts.app.fs, u, "test error")
			},
			wantCode: http.StatusOK,
			wantBody: "Upload Resume",
		},
		{
			name:            "Unknown State Resets to Resume Upload",
			isAuthenticated: true,
			setupUser: func(u *internal.User) {
				internal.UpdateUserStatus(context.Background(), ts.app.fs, u, 999) // Invalid state
			},
			wantCode: http.StatusSeeOther,
			wantBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var code int
			var _, body string

			if tt.isAuthenticated {
				if tt.setupUser != nil {
					tt.setupUser(ts.user1)
				}
				code, _, body = ts.authenticatedGet(t, "/onboarding")
			} else {
				code, _, body = ts.get(t, "/onboarding")
			}

			if code != tt.wantCode {
				t.Errorf("want %d; got %d", tt.wantCode, code)
			}

			if tt.wantBody != "" && !strings.Contains(strings.ToLower(body), strings.ToLower(tt.wantBody)) {
				t.Errorf("want body to contain %q", tt.wantBody)
			}
		})
	}
}

func TestShowWaitingCallPage(t *testing.T) {
	ts := newTestServer(t, "waiting-call")
	defer ts.Cleanup()

	tests := []struct {
		name            string
		isAuthenticated bool
		setupUser       func(*internal.User)
		wantCode        int
		wantBody        string
		wantRedirect    string
	}{
		{
			name:            "Shows Waiting Page with Active Appointment",
			isAuthenticated: true,
			setupUser: func(u *internal.User) {
				internal.UpdateUserStatus(context.Background(), ts.app.fs, u, internal.UserStatusCallScheduled)
				internal.CreateTestAppointment(t, ts.app.fs, u)
			},
			wantCode: http.StatusOK,
			wantBody: "calling you shortly",
		},
		{
			name:            "Redirects to Profile When No Active Appointment",
			isAuthenticated: true,
			setupUser: func(u *internal.User) {
				// Clean up any existing appointments first
				internal.DeleteTestAppointments(t, ts.app.fs, ts.user1.ID)
				internal.UpdateUserStatus(context.Background(), ts.app.fs, u, internal.UserStatusCallScheduled)
				// Don't create an appointment
			},
			wantCode:     http.StatusSeeOther,
			wantRedirect: "/onboarding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up appointments before each test
			internal.DeleteTestAppointments(t, ts.app.fs, ts.user1.ID)

			var code int
			var headers http.Header
			var body string

			if tt.setupUser != nil {
				tt.setupUser(ts.user1)
			}
			// First get the profile page which should redirect to dashboard
			code, headers, _ = ts.authenticatedGet(t, "/onboarding")
			if code != http.StatusSeeOther {
				t.Errorf("want redirect %d; got %d", http.StatusSeeOther, code)
			}
			if headers.Get("Location") != "/dashboard" {
				t.Errorf("want redirect to /dashboard; got %s", headers.Get("Location"))
			}

			// Then follow the redirect to get the dashboard page
			code, headers, body = ts.authenticatedGet(t, "/dashboard")

			if code != tt.wantCode {
				t.Errorf("want %d; got %d", tt.wantCode, code)
			}

			if tt.wantRedirect != "" {
				if headers.Get("Location") != tt.wantRedirect {
					t.Errorf("want redirect to %s; got %s", tt.wantRedirect, headers.Get("Location"))
				}
			} else if tt.wantBody != "" && !strings.Contains(strings.ToLower(body), strings.ToLower(tt.wantBody)) {
				t.Errorf("want body to contain %q", tt.wantBody)
			}
		})
	}
}
