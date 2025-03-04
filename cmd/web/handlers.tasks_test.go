package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/database"
)

func TestShowTasksPage(t *testing.T) {
	ts := newTestServer(t)

	tests := []struct {
		name     string
		user     *database.User
		wantCode int
	}{
		{
			name:     "unauthenticated user",
			user:     nil,
			wantCode: http.StatusFound,
		},
		{
			name:     "non-admin user",
			user:     ts.createCandidateUser(3, "test@example.com"),
			wantCode: http.StatusFound,
		},
		{
			name:     "admin user",
			user:     ts.createAdminUser(4, "admin2@example.com"),
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/admin/tasks"

			req := ts.authenticatedRequest(t, "GET", path, tt.user, nil)
			if tt.user != nil {
				req = contextSetAuthenticatedUser(req, tt.user)
			}

			w := httptest.NewRecorder()
			ts.app.routes().ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("want %d; got %d", tt.wantCode, w.Code)
			}
		})
	}
}

func TestGetTasksPartial(t *testing.T) {
	ts := newTestServer(t)

	// Create some test tasks
	tasks := []*internal.Task{
		{
			Type:    "test_task_1",
			Payload: json.RawMessage(`{"test": "data1"}`),
			Status:  internal.TaskStatusPending,
			RunAt:   time.Now().Add(time.Hour),
		},
		{
			Type:    "test_task_2",
			Payload: json.RawMessage(`{"test": "data2"}`),
			Status:  internal.TaskStatusRunning,
			RunAt:   time.Now().Add(2 * time.Hour),
		},
	}

	for _, task := range tasks {
		err := ts.app.db.CreateTask(context.Background(), task)
		if err != nil {
			t.Fatalf("failed to create test task: %v", err)
		}
	}

	tests := []struct {
		name     string
		user     *database.User
		path     string
		wantCode int
	}{
		{
			name:     "unauthenticated user",
			user:     nil,
			path:     "/admin/tasks/partial",
			wantCode: http.StatusFound,
		},
		{
			name:     "non-admin user",
			user:     ts.createCandidateUser(3, "test@example.com"),
			path:     "/admin/tasks/partial",
			wantCode: http.StatusFound,
		},
		{
			name:     "admin user - no filters",
			user:     ts.createAdminUser(4, "admin2@example.com"),
			path:     "/admin/tasks/partial",
			wantCode: http.StatusOK,
		},
		{
			name:     "admin user - with status filter",
			user:     ts.createAdminUser(5, "admin3@example.com"),
			path:     "/admin/tasks/partial?status=pending",
			wantCode: http.StatusOK,
		},
		{
			name:     "admin user - with pagination",
			user:     ts.createAdminUser(6, "admin4@example.com"),
			path:     "/admin/tasks/partial?page=1&size=10",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ts.authenticatedRequest(t, "GET", tt.path, tt.user, nil)
			if tt.user != nil {
				req = contextSetAuthenticatedUser(req, tt.user)
			}

			w := httptest.NewRecorder()
			ts.app.routes().ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("want %d; got %d", tt.wantCode, w.Code)
			}
		})
	}
}
