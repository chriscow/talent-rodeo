package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"talent-rodeo/internal"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func TestShowDashboard(t *testing.T) {
	ts := newTestServer(t, "test-show-dashboard")
	defer ts.Cleanup()

	tests := []struct {
		name         string
		setupContext func(context.Context) context.Context
		setupUser    func() *internal.User
		setupJobs    func() []*internal.UserJob
		setupAppt    func() *internal.Appointment
		wantStatus   int
		wantRedirect string
	}{
		{
			name: "no user in context redirects to home",
			setupContext: func(ctx context.Context) context.Context {
				return ctx
			},
			setupUser: func() *internal.User {
				return nil
			},
			wantStatus:   http.StatusSeeOther,
			wantRedirect: "/",
		},
		{
			name: "empty user ID redirects to home",
			setupContext: func(ctx context.Context) context.Context {
				return context.WithValue(ctx, contextKey("user"), &internal.User{})
			},
			setupUser: func() *internal.User {
				return &internal.User{}
			},
			wantStatus:   http.StatusSeeOther,
			wantRedirect: "/",
		},
		{
			name: "unscheduled user redirects to profile",
			setupContext: func(ctx context.Context) context.Context {
				user := &internal.User{
					ID:     "test-user",
					Status: internal.UserStatusUnknown,
				}
				return context.WithValue(ctx, contextKey("user"), user)
			},
			setupUser: func() *internal.User {
				return &internal.User{
					ID:     "test-user",
					Status: internal.UserStatusUnknown,
				}
			},
			wantStatus:   http.StatusSeeOther,
			wantRedirect: "/onboarding",
		},
		{
			name: "call scheduled user with no jobs shows appointment",
			setupContext: func(ctx context.Context) context.Context {
				user := &internal.User{
					ID:     "test-user",
					Status: internal.UserStatusCallScheduled,
				}
				return context.WithValue(ctx, contextKey("user"), user)
			},
			setupUser: func() *internal.User {
				return &internal.User{
					ID:     "test-user",
					Status: internal.UserStatusCallScheduled,
				}
			},
			setupJobs: func() []*internal.UserJob {
				return []*internal.UserJob{}
			},
			setupAppt: func() *internal.Appointment {
				return &internal.Appointment{
					ID:            "test-appt",
					ScheduledTime: time.Now().Add(24 * time.Hour),
					Status:        internal.AppointmentStatusScheduled,
					Type:          internal.AppointmentTypeVoice,
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "user with jobs shows job categories",
			setupContext: func(ctx context.Context) context.Context {
				user := &internal.User{
					ID:     "test-user",
					Status: internal.UserStatusCallScheduled,
				}
				return context.WithValue(ctx, contextKey("user"), user)
			},
			setupUser: func() *internal.User {
				return &internal.User{
					ID:     "test-user",
					Status: internal.UserStatusCallScheduled,
				}
			},
			setupJobs: func() []*internal.UserJob {
				now := time.Now()
				return []*internal.UserJob{
					{
						ID:        "job1",
						Status:    "new",
						JobRef:    &firestore.DocumentRef{},
						CreatedAt: now,
						UpdatedAt: now,
					},
					{
						ID:        "job2",
						Status:    "viewed",
						JobRef:    &firestore.DocumentRef{},
						CreatedAt: now,
						UpdatedAt: now,
					},
				}
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))

			app := ts.app
			app.logger = logger

			req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)

			ctx, err := app.sessionManager.Load(req.Context(), "")
			if err != nil {
				t.Fatal(err)
			}
			req = req.WithContext(ctx)

			req = req.WithContext(tt.setupContext(req.Context()))

			if user := tt.setupUser(); user != nil {
				app.sessionManager.Put(req.Context(), "UserID", user.ID)
				req = contextSetAuthenticatedUser(req, user)
				
				// For users with actual IDs, update the database
				if user.ID != "" && user.Status != internal.UserStatusUnknown {
					// Use the existing test user but modify it for this test
					testUser := ts.user1
					testUser.Status = user.Status
					
					// Update the user in the database to match test expectations
					err := internal.UpdateUserStatus(req.Context(), app.fs, testUser, user.Status)
					if err != nil {
						t.Fatalf("Failed to update test user status: %v", err)
					}
					
					// Set up appointment if provided
					if tt.setupAppt != nil {
						appt := tt.setupAppt()
						appt.UserID = testUser.ID
						err := internal.CreateAppointment(req.Context(), app.fs, testUser.ID, appt)
						if err != nil {
							t.Fatalf("Failed to create test appointment: %v", err)
						}
					}
					
					// Update the authenticated user to point to the test user
					app.sessionManager.Put(req.Context(), "UserID", testUser.ID)
					req = contextSetAuthenticatedUser(req, testUser)
				}
			}

			w := httptest.NewRecorder()
			app.showDashboard(w, req)

			if got := w.Code; got != tt.wantStatus {
				t.Errorf("showDashboard() status = %v, want %v", got, tt.wantStatus)
			}

			if tt.wantRedirect != "" {
				if got := w.Header().Get("Location"); got != tt.wantRedirect {
					t.Errorf("showDashboard() redirect = %v, want %v", got, tt.wantRedirect)
				}
			}
		})
	}
}
