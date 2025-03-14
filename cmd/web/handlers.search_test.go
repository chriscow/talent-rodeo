package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"talent-rodeo/internal/services/search"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestStartJobSearch(t *testing.T) {
	t.Skip("Not fully implemented")

	tests := []struct {
		name          string
		setupAuth     func(ts *testServer) *http.Request
		setupDB       func(ts *testServer)
		wantCode      int
		wantBody      string
		checkMessages bool
		wantMsgCount  int
		validateNATS  func(t *testing.T, msgs []jetstream.Msg) error
	}{
		{
			name: "Unauthenticated user redirects to login",
			setupAuth: func(ts *testServer) *http.Request {
				return httptest.NewRequest("POST", "/profile/search/start", nil)
			},
			wantCode: http.StatusFound,
			wantBody: "",
		},
		{
			name: "Invalid candidate ID returns error",
			setupAuth: func(ts *testServer) *http.Request {
				user := ts.createCandidateUser(999, "test@example.com")
				return ts.authenticatedRequest(t, "POST", "/profile/search/start", user, nil)
			},
			wantCode: http.StatusOK,
			wantBody: "Sorry, we are having a problem with our system",
		},
		{
			name: "No skills returns empty arrays in messages",
			setupAuth: func(ts *testServer) *http.Request {
				user := ts.createCandidateUser(3, "test@example.com")
				// Create candidate record but no skills
				ts.app.db.CreateCandidate(context.Background(), user.ID)
				return ts.authenticatedRequest(t, "POST", "/profile/search/start", user, nil)
			},
			wantCode:      http.StatusOK,
			checkMessages: true,
			wantMsgCount:  3, // Free tier gets 3 pages
			validateNATS: func(t *testing.T, msgs []jetstream.Msg) error {
				for i, msg := range msgs {
					var input search.SearchInput
					if err := json.Unmarshal(msg.Data(), &input); err != nil {
						return err
					}
					if len(input.MustHaveSkills) != 0 {
						t.Errorf("Expected empty must-have skills, got %v", input.MustHaveSkills)
					}
					if len(input.PreferredSkills) != 0 {
						t.Errorf("Expected empty preferred skills, got %v", input.PreferredSkills)
					}
					if input.Page != i+1 {
						t.Errorf("Expected page %d, got %d", i+1, input.Page)
					}
				}
				return nil
			},
		},
		{
			name: "Skills are correctly categorized in messages",
			setupAuth: func(ts *testServer) *http.Request {
				user := ts.createCandidateUser(4, "test@example.com")
				candidateID, err := ts.app.db.CreateCandidate(context.Background(), user.ID)
				if err != nil {
					t.Fatal(err)
				}

				// Add test skills with different preferences
				skills := []struct {
					name string
					pref int
				}{
					{"golang", 5},     // Must have
					{"python", 5},     // Must have
					{"docker", 4},     // Preferred
					{"kubernetes", 4}, // Preferred
					{"java", 3},       // Not included
				}

				for _, s := range skills {
					skill, err := ts.app.db.AddCandidateSkill(context.Background(), candidateID, s.name)
					if err != nil {
						t.Fatal(err)
					}
					// Update the preference value
					updates := map[string]struct {
						PreferenceValue int
						IsSearchTerm    bool
					}{
						skill.ID: {
							PreferenceValue: s.pref,
							IsSearchTerm:    s.pref > 3,
						},
					}
					if err := ts.app.db.UpdateSkillPreferences(context.Background(), candidateID, updates); err != nil {
						t.Fatal(err)
					}
				}

				return ts.authenticatedRequest(t, "POST", "/profile/search/start", user, nil)
			},
			wantCode:      http.StatusOK,
			checkMessages: true,
			wantMsgCount:  3, // Free tier gets 3 pages
			validateNATS: func(t *testing.T, msgs []jetstream.Msg) error {
				for i, msg := range msgs {
					var input search.SearchInput
					if err := json.Unmarshal(msg.Data(), &input); err != nil {
						return err
					}

					// Check must-have skills
					wantMustHave := []string{"golang", "python"}
					if !stringSliceEqual(input.MustHaveSkills, wantMustHave) {
						t.Errorf("Expected must-have skills %v, got %v", wantMustHave, input.MustHaveSkills)
					}

					// Check preferred skills
					wantPreferred := []string{"docker", "kubernetes"}
					if !stringSliceEqual(input.PreferredSkills, wantPreferred) {
						t.Errorf("Expected preferred skills %v, got %v", wantPreferred, input.PreferredSkills)
					}

					// Check page number
					if input.Page != i+1 {
						t.Errorf("Expected page %d, got %d", i+1, input.Page)
					}
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)

			// Set up test environment
			if tt.setupDB != nil {
				tt.setupDB(ts)
			}

			// Create request
			req := tt.setupAuth(ts)
			w := httptest.NewRecorder()

			// Execute request
			ts.app.routes().ServeHTTP(w, req)

			// Check response code
			if w.Code != tt.wantCode {
				t.Errorf("want %d; got %d", tt.wantCode, w.Code)
			}

			// Check response body if specified
			s := w.Body.String()
			if tt.wantBody != "" && !strings.Contains(s, tt.wantBody) {
				t.Errorf("want body to contain %q; got %q", tt.wantBody, w.Body.String())
			}

			// Check NATS messages if required
			if tt.checkMessages {
				// Create consumer to read messages
				consumer, err := ts.app.jetstream.CreateOrUpdateConsumer(
					context.Background(),
					"rodeo",
					jetstream.ConsumerConfig{
						Name:          "test-consumer",
						FilterSubject: "rodeo.search.input",
						DeliverPolicy: jetstream.DeliverAllPolicy,
					},
				)
				if err != nil {
					t.Fatal(err)
				}

				// Read messages with timeout
				msgs := make([]jetstream.Msg, 0)
				for i := 0; i < tt.wantMsgCount; i++ {
					batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(time.Second))
					if err != nil {
						t.Fatal(err)
					}
					msg := <-batch.Messages()
					if msg == nil {
						t.Fatal("Expected message but got nil")
					}
					msgs = append(msgs, msg)
					msg.Ack()
				}

				// Validate message count
				if len(msgs) != tt.wantMsgCount {
					t.Errorf("Expected %d messages, got %d", tt.wantMsgCount, len(msgs))
				}

				// Run custom validation if provided
				if tt.validateNATS != nil {
					if err := tt.validateNATS(t, msgs); err != nil {
						t.Error(err)
					}
				}
			}
		})
	}
}

// Helper function to compare string slices ignoring order
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]bool)
	for _, str := range a {
		seen[str] = true
	}
	for _, str := range b {
		if !seen[str] {
			return false
		}
	}
	return true
}
