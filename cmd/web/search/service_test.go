package search

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search"
	"testing"
	"time"

	"talent-rodeo/internal/search/providers/scrapingdog"

	"cloud.google.com/go/firestore"
	"github.com/joho/godotenv"
)

func init() {
	// Load environment variables from the project root .env file
	if err := godotenv.Overload("../../../.env"); err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
	}
}

// QueuedSearch represents a search request queued for processing
type QueuedSearch struct {
	UserID   string          `json:"user_id"`
	Criteria search.Criteria `json:"criteria"`
}

// mockProvider implements JobProvider for testing
type mockProvider struct {
	queuedSearches []QueuedSearch
	shouldError    bool
}

type SearchQueue interface {
	QueueSearch(ctx context.Context, userID string, criteria search.Criteria) error
}

func (m *mockProvider) QueueSearch(ctx context.Context, userID string, criteria search.Criteria) error {
	if m.shouldError {
		return fmt.Errorf("mock provider error")
	}
	m.queuedSearches = append(m.queuedSearches, QueuedSearch{UserID: userID, Criteria: criteria})
	return nil
}

func (m *mockProvider) GetQueueDepth(ctx context.Context) (int, error) {
	return len(m.queuedSearches), nil
}

// mockMailer implements Mailer for testing
type mockMailer struct {
	sentMails []struct {
		recipient string
		data      any
		patterns  []string
	}
}

// Send(recipient string, data any, patterns ...string) error
// see `internal/smtp` - it is designed to use a template from assets/emails
func (m *mockMailer) Send(recipient string, data any, patterns ...string) error {
	m.sentMails = append(m.sentMails, struct {
		recipient string
		data      any
		patterns  []string
	}{recipient, data, patterns})
	return nil
}

func TestNewService(t *testing.T) {
	fs := &firestore.Client{}
	logger := slog.Default()
	queue := &mockProvider{}
	mailer := &mockMailer{}

	svc := NewService(fs, logger, queue, mailer, "test@example.com", nil)

	if svc == nil {
		t.Fatal("NewService returned nil")
	}

	if svc.fs != fs {
		t.Error("firestore client not properly set")
	}

	if svc.queue != queue {
		t.Error("queue not properly set")
	}

	if svc.mailer != mailer {
		t.Error("mailer not properly set")
	}

	if svc.done == nil {
		t.Error("done channel not initialized")
	}
}

func TestShouldSearch(t *testing.T) {
	now := time.Now()
	threeDaysAgo := now.Add(-73 * time.Hour)
	oneDayAgo := now.Add(-25 * time.Hour)
	fourHoursAgo := now.Add(-5 * time.Hour)

	tests := []struct {
		name string
		user internal.User
		want bool
	}{
		{
			name: "nil search",
			user: internal.User{ID: "1", Search: nil},
			want: false,
		},
		{
			name: "never searched or queued",
			user: internal.User{
				ID:     "2",
				Search: &internal.UserSearch{},
			},
			want: true,
		},
		{
			name: "currently queued",
			user: internal.User{
				ID: "3",
				Search: &internal.UserSearch{
					QueuedAt: &now,
				},
			},
			want: false,
		},
		{
			name: "completed nil but not queued",
			user: internal.User{
				ID: "4",
				Search: &internal.UserSearch{
					CompletedAt: nil,
					QueuedAt:    nil,
				},
			},
			want: true,
		},
		{
			name: "free tier needs search",
			user: internal.User{
				ID:   "5",
				Tier: internal.UserTierFree,
				Search: &internal.UserSearch{
					CompletedAt: &threeDaysAgo,
				},
			},
			want: true,
		},
		{
			name: "pro tier needs search",
			user: internal.User{
				ID:   "6",
				Tier: internal.UserTierPro,
				Search: &internal.UserSearch{
					CompletedAt: &oneDayAgo,
				},
			},
			want: true,
		},
		{
			name: "enterprise tier needs search",
			user: internal.User{
				ID:   "7",
				Tier: internal.UserTierEnterprise,
				Search: &internal.UserSearch{
					CompletedAt: &fourHoursAgo,
				},
			},
			want: true,
		},
		{
			name: "free tier too recent",
			user: internal.User{
				ID:   "8",
				Tier: internal.UserTierFree,
				Search: &internal.UserSearch{
					CompletedAt: &now,
				},
			},
			want: false,
		},
		{
			name: "pro tier too recent",
			user: internal.User{
				ID:   "9",
				Tier: internal.UserTierPro,
				Search: &internal.UserSearch{
					CompletedAt: &now,
				},
			},
			want: false,
		},
		{
			name: "enterprise tier too recent",
			user: internal.User{
				ID:   "10",
				Tier: internal.UserTierEnterprise,
				Search: &internal.UserSearch{
					CompletedAt: &now,
				},
			},
			want: false,
		},
	}

	svc := NewService(nil, slog.Default(), nil, nil, "test@example.com", nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.shouldSearch(&tt.user)
			if got != tt.want {
				t.Errorf("shouldSearch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPerformSearch(t *testing.T) {
	// Set testing mode to true to ensure only 1 page is queued
	original := os.Getenv("TESTING_MODE")
	os.Setenv("TESTING_MODE", "true")
	defer os.Setenv("TESTING_MODE", original)

	tests := []struct {
		name        string
		user        internal.User
		providerErr bool
		wantErr     bool
	}{
		{
			name: "successful search",
			user: internal.User{
				ID: "1",
				Search: &internal.UserSearch{
					Criteria: search.Criteria{
						Keywords: []string{"golang"},
					},
				},
			},
			providerErr: false,
			wantErr:     false,
		},
		{
			name: "provider error",
			user: internal.User{
				ID: "2",
				Search: &internal.UserSearch{
					Criteria: search.Criteria{
						Keywords: []string{"golang"},
					},
				},
			},
			providerErr: true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockProvider{shouldError: tt.providerErr}
			mailer := &mockMailer{}
			svc := NewService(nil, slog.Default(), provider, mailer, "test@example.com", nil)

			err := svc.enqueueSearch(context.Background(), &tt.user)
			if (err != nil) != tt.wantErr {
				t.Errorf("performSearch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				depth, err := provider.GetQueueDepth(context.Background())
				if err != nil {
					t.Fatalf("GetQueueDepth() error = %v", err)
				}
				if depth != 1 {
					t.Errorf("expected 1 queued search, got %d", depth)
					return
				}

				// check if this is the mock provider to check this
				got := provider.queuedSearches[0]
				if got.UserID != tt.user.ID {
					t.Errorf("queued search userID = %v, want %v", got.UserID, tt.user.ID)
				}
			}
		})
	}
}

func TestNotifyAdmin(t *testing.T) {
	user := &internal.User{
		ID:    "1",
		Email: "test@example.com",
		Search: &internal.UserSearch{
			Criteria: search.Criteria{
				Keywords: []string{"golang"},
			},
		},
	}
	testErr := fmt.Errorf("test error")

	mailer := &mockMailer{}
	svc := NewService(nil, slog.Default(), nil, mailer, "test@example.com", nil)

	svc.notifyAdmin(context.Background(), user, testErr)

	if len(mailer.sentMails) != 1 {
		t.Fatalf("expected 1 sent mail, got %d", len(mailer.sentMails))
	}

	got := mailer.sentMails[0]

	// Verify the template name
	if got.patterns[0] != "search-error.html" {
		t.Errorf("wrong template used = %q, want %q", got.patterns[0], "search-error.html")
	}

	// Verify the template data
	data, ok := got.data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected template data to be map[string]interface{}, got %T", got.data)
	}

	// Test scalar values
	scalarTests := []struct {
		key  string
		want interface{}
	}{
		{"UserID", user.ID},
		{"UserEmail", user.Email},
		{"Error", testErr.Error()},
	}

	for _, tt := range scalarTests {
		if got := data[tt.key]; got != tt.want {
			t.Errorf("template data[%q] = %v, want %v", tt.key, got, tt.want)
		}
	}

	// Verify Criteria separately
	gotCriteria, ok := data["Criteria"].(search.Criteria)
	if !ok {
		t.Errorf("Criteria not of expected type, got %T", data["Criteria"])
	} else {
		// Compare relevant fields of Criteria
		if !reflect.DeepEqual(gotCriteria.Keywords, user.Search.Criteria.Keywords) {
			t.Errorf("Criteria.Keywords = %v, want %v",
				gotCriteria.Keywords, user.Search.Criteria.Keywords)
		}
		// Add other Criteria field comparisons as needed
	}

	// Verify timestamp exists and is in correct format
	if ts, ok := data["Timestamp"].(string); !ok {
		t.Error("Timestamp not found in template data or not a string")
	} else {
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("Timestamp not in RFC3339 format: %v", err)
		}
	}
}

// setupFirestore creates a Firestore client for testing and ensures cleanup
// This function is crucial for setting up the Firestore environment needed for the tests.
// It skips the test if the necessary environment variables are not set.
func setupFirestore(t *testing.T) *firestore.Client {
	t.Helper()

	// Skip if we're not in an environment with Firebase credentials
	if os.Getenv("FIREBASE_PROJECT_ID") == "" || os.Getenv("FIREBASE_CREDENTIALS") == "" {
		t.Skip("Skipping test: FIREBASE_PROJECT_ID and FIREBASE_CREDENTIALS must be set")
	}

	ctx := context.Background()
	fs, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}

	return fs
}

// getProvider returns either a mock provider or real ScrapingDog provider based on test flags
func getProvider(t *testing.T, fs *firestore.Client) search.JobQueue {
	t.Helper()

	if testing.Short() {
		return &mockProvider{}
	}

	// For integration tests, use the real ScrapingDog provider
	apiKey := os.Getenv("SCRAPINGDOG_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: SCRAPINGDOG_API_KEY environment variable not set")
	}

	provider := scrapingdog.NewProvider(apiKey, scrapingdog.WithFirestore(fs, slog.Default()))
	if err := provider.Validate(context.Background()); err != nil {
		t.Fatalf("Failed to validate ScrapingDog provider: %v", err)
	}

	return provider
}

// TestServiceLifecycle tests the lifecycle of the search service.
// It verifies that the service can start, process searches, and stop correctly.
// The test uses a mock provider to simulate the search queue.
//
// Fixes made:
//   - The Firestore query in checkUsersForSearch was modified to ensure it correctly retrieves users.
//   - The original query was filtering users based on the existence of 'search.error' being an empty string,
//     which caused newly created users (without this field) to be excluded from the results.
//   - The query was changed to simply retrieve all users, allowing the service to process users correctly.
//   - Additional logging was added to track the number of users retrieved and their eligibility for search.
func TestServiceLifecycle(t *testing.T) {
	fs := setupFirestore(t)
	defer fs.Close()
	defer internal.DeleteTestData(t, fs, "test-")

	ctx := context.Background()
	// Create test user
	t.Log("Creating test user...")
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+1234567890", false)
	t.Logf("Created test user: %s", user.ID)
	search := &internal.UserSearch{
		Criteria: search.Criteria{
			Keywords: []string{"golang"},
		},
	}
	t.Log("Updating test user search criteria...")
	if err := internal.UpdateUserSearch(ctx, fs, user.ID, search); err != nil {
		t.Fatalf("Failed to update test user search: %v", err)
	}
	t.Log("Updated test user search criteria")

	// Use short durations for testing
	testInterval := 50 * time.Millisecond

	tests := []struct {
		name          string
		startService  bool
		stopAfterTime time.Duration
		wantTicks     int
	}{
		{
			name:          "start and immediate stop",
			startService:  true,
			stopAfterTime: 0,
			wantTicks:     0,
		},
		{
			name:          "start and run for one tick",
			startService:  true,
			stopAfterTime: testInterval + (10 * time.Millisecond),
			wantTicks:     1,
		},
		{
			name:          "service not started",
			startService:  false,
			stopAfterTime: 0,
			wantTicks:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := os.Getenv("TESTING_MODE")
			os.Setenv("TESTING_MODE", "true")
			defer os.Setenv("TESTING_MODE", orig)

			t.Logf("Starting test case: %s", tt.name)
			provider := getProvider(t, fs)
			svc := NewService(fs, slog.Default(), provider, nil, "test@example.com", nil)

			if tt.startService {
				t.Log("Starting service...")
				svc.ticker = time.NewTicker(testInterval)
				err := svc.Start()
				if err != nil {
					t.Fatalf("Start() error = %v", err)
				}
				t.Log("Service started successfully")
			}

			t.Logf("Sleeping for stopAfterTime: %v", tt.stopAfterTime)
			time.Sleep(tt.stopAfterTime)
			t.Log("Stopping service...")
			svc.Stop()
			t.Log("Service stopped")

			if tt.startService && tt.wantTicks > 0 {
				t.Log("Waiting for search processing to complete...")
				time.Sleep(100 * time.Millisecond)
				searchCount, err := provider.GetQueueDepth(context.Background())
				if err != nil {
					t.Fatalf("GetQueueDepth() error = %v", err)
				}
				t.Logf("Found %d queued searches", searchCount)
				if searchCount == 0 {
					t.Error("expected search processing to occur")
				}
			}
		})
	}
}

// TestConcurrentProcessing verifies that the search service can handle multiple users
// concurrently without race conditions or duplicate processing. Specifically it tests:
//  1. Multiple users can be processed in the same tick interval
//  2. Each user is processed exactly once (no duplicate processing)
//  3. The service correctly handles concurrent access to the search queue
//  4. The service maintains data consistency when processing multiple users
//
// Test Flow:
//  1. Creates two test users with identical search criteria
//  2. Starts the service with a short tick interval
//  3. Lets the service process one tick
//  4. Verifies that:
//     - All users were processed (correct count)
//     - No user was processed more than once (using seen map)
//     - The service maintained data consistency
func TestConcurrentProcessing(t *testing.T) {
	fs := setupFirestore(t)
	defer fs.Close()
	defer internal.DeleteTestData(t, fs, "test-")

	orig := os.Getenv("TESTING_MODE")
	os.Setenv("TESTING_MODE", "true")
	defer os.Setenv("TESTING_MODE", orig)

	ctx := context.Background()
	// Create test users
	users := []*internal.User{
		internal.CreateTestUser(t, fs, "Test User 1", "test1@example.com", "+1234567890", false),
		internal.CreateTestUser(t, fs, "Test User 2", "test2@example.com", "+1234567891", false),
	}

	// Update users with search criteria
	search := &internal.UserSearch{
		Criteria: search.Criteria{
			Keywords: []string{"golang"},
		},
	}
	for _, user := range users {
		if err := internal.UpdateUserSearch(ctx, fs, user.ID, search); err != nil {
			t.Fatalf("Failed to update test user search: %v", err)
		}
	}

	provider := &mockProvider{}
	svc := NewService(fs, slog.Default(), provider, nil, "test@example.com", nil)

	// Use a short tick interval
	svc.ticker = time.NewTicker(50 * time.Millisecond)
	err := svc.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Let the service process one tick
	time.Sleep(75 * time.Millisecond)
	svc.Stop()

	// Create a map of our test user IDs for quick lookup
	testUsers := make(map[string]bool)
	for _, user := range users {
		testUsers[user.ID] = true
	}

	// Verify all users were processed
	processedTestUsers := 0
	seen := make(map[string]bool)

	for _, search := range provider.queuedSearches {
		// Only count our test users
		if testUsers[search.UserID] {
			processedTestUsers++
			if seen[search.UserID] {
				t.Errorf("user %s was processed multiple times", search.UserID)
			}
			seen[search.UserID] = true
		}
	}

	if processedTestUsers != len(users) {
		t.Errorf("got %d processed test users, want %d", processedTestUsers, len(users))
	}
}

func TestErrorRecovery(t *testing.T) {
	fs := setupFirestore(t)
	defer fs.Close()
	defer internal.DeleteTestData(t, fs, "test-")

	if !testing.Short() {
		t.Skip("Skipping error recovery test for real provider")
	}

	orig := os.Getenv("TESTING_MODE")
	os.Setenv("TESTING_MODE", "true")
	defer os.Setenv("TESTING_MODE", orig)

	ctx := context.Background()
	// Create test user with valid search criteria
	user := internal.CreateTestUser(t, fs, "Test User", "test@example.com", "+1234567890", false)

	// Initialize search criteria directly in the user object
	user.Search = &internal.UserSearch{
		Criteria: search.Criteria{
			Keywords: []string{"golang"},
			Location: "remote",
		},
	}

	// Update the user in Firestore
	if err := internal.UpdateUser(ctx, fs, user.ID, map[string]any{
		"search": user.Search,
	}); err != nil {
		t.Fatalf("Failed to update user search: %v", err)
	}

	mailer := &mockMailer{}
	provider := &mockProvider{shouldError: true} // Force error case
	svc := NewService(fs, slog.Default(), provider, mailer, "admin@example.com", nil)

	// Test that service handles error properly
	err := svc.enqueueSearch(ctx, user)
	if err == nil {
		t.Fatal("expected error from enqueueSearch, got nil")
	}

	// Verify admin notification was sent
	if len(mailer.sentMails) != 1 {
		t.Fatal("expected admin notification email")
	}

	// Verify email was sent to correct admin address
	if mailer.sentMails[0].recipient != "admin@example.com" {
		t.Errorf("wrong admin email recipient: got %s, want admin@example.com", mailer.sentMails[0].recipient)
	}

	// Test service can handle subsequent searches
	provider.shouldError = false // Reset error state
	err = svc.enqueueSearch(ctx, user)
	if err != nil {
		t.Errorf("unexpected error after recovery: %v", err)
	}

	// Verify searches were queued after recovery
	// In testing mode, we expect 1 page per keyword
	expectedSearches := len(user.Search.Criteria.Keywords) // One search per keyword in testing mode
	if len(provider.queuedSearches) != expectedSearches {
		t.Errorf("wrong number of queued searches after recovery: got %d, want %d", len(provider.queuedSearches), expectedSearches)
	}

	// Verify each queued search has the correct user ID and one keyword
	for _, qs := range provider.queuedSearches {
		if qs.UserID != user.ID {
			t.Errorf("wrong user ID in queued search: got %s, want %s", qs.UserID, user.ID)
		}
		if len(qs.Criteria.Keywords) != 1 {
			t.Errorf("wrong number of keywords in queued search: got %d, want 1", len(qs.Criteria.Keywords))
		}
		found := false
		for _, k := range user.Search.Criteria.Keywords {
			if k == qs.Criteria.Keywords[0] {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected keyword in queued search: %s", qs.Criteria.Keywords[0])
		}
	}
}

func TestSearchFrequencyEdgeCases(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		user internal.User
		want bool
	}{
		{
			name: "enterprise tier frequency",
			user: internal.User{
				ID:   "1",
				Tier: "enterprise",
				Search: &internal.UserSearch{
					CompletedAt: &now,
				},
			},
			want: false,
		},
		{
			name: "invalid tier",
			user: internal.User{
				ID:   "2",
				Tier: "invalid",
				Search: &internal.UserSearch{
					CompletedAt: &now,
				},
			},
			want: false, // Should default to free tier frequency
		},
		{
			name: "time exactly at frequency boundary",
			user: internal.User{
				ID: "3",
				Search: &internal.UserSearch{
					CompletedAt: func() *time.Time {
						t := now.Add(-FreeTierFrequency)
						return &t
					}(),
				},
			},
			want: true,
		},
	}

	svc := NewService(nil, slog.Default(), nil, nil, "test@example.com", nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.shouldSearch(&tt.user)
			if got != tt.want {
				t.Errorf("shouldSearch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestEnqueueSearch(t *testing.T) {
	// Save original env and restore after test
	originalTestingMode := os.Getenv("TESTING_MODE")
	defer os.Setenv("TESTING_MODE", originalTestingMode)

	tests := []struct {
		name          string
		user          *internal.User
		testingMode   string
		wantNumPages  int
		wantError     bool
		errorContains string
	}{
		{
			name: "free tier user - 3 pages",
			user: &internal.User{
				ID:   "1",
				Tier: internal.UserTierFree,
				Search: &internal.UserSearch{
					Criteria: search.Criteria{
						Keywords:   []string{"software", "engineer"},
						Location:   "remote",
						PageNumber: 1,
					},
				},
			},
			testingMode:  "false",
			wantNumPages: 6, // 3 pages * 2 keywords = 6 total searches
		},
		{
			name: "pro tier user - 10 pages",
			user: &internal.User{
				ID:   "2",
				Tier: internal.UserTierPro,
				Search: &internal.UserSearch{
					Criteria: search.Criteria{
						Keywords:   []string{"software", "engineer"},
						Location:   "remote",
						PageNumber: 1,
					},
				},
			},
			testingMode:  "false",
			wantNumPages: 20, // 10 pages * 2 keywords = 20 total searches
		},
		{
			name: "testing mode - 1 page regardless of tier",
			user: &internal.User{
				ID:   "3",
				Tier: internal.UserTierPro,
				Search: &internal.UserSearch{
					Criteria: search.Criteria{
						Keywords:   []string{"software", "engineer"},
						Location:   "remote",
						PageNumber: 1,
					},
				},
			},
			testingMode:  "true",
			wantNumPages: 2, // 1 page * 2 keywords = 2 total searches
		},
		{
			name: "error case - nil search",
			user: &internal.User{
				ID:     "4",
				Tier:   internal.UserTierFree,
				Search: nil,
			},
			testingMode:   "false",
			wantError:     true,
			errorContains: "nil search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set testing mode for this test case
			orig := os.Getenv("TESTING_MODE")
			os.Setenv("TESTING_MODE", tt.testingMode)
			defer os.Setenv("TESTING_MODE", orig)

			// Create mock dependencies
			mockQueue := &mockProvider{}
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			mailer := &mockMailer{}

			// Create service
			svc := NewService(nil, logger, mockQueue, mailer, "admin@example.com", nil)

			// Call enqueueSearch
			err := svc.enqueueSearch(context.Background(), tt.user)

			// Check error cases
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
					return
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			// Check success cases
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify number of queued searches
			if got := len(mockQueue.queuedSearches); got != tt.wantNumPages {
				t.Errorf("wrong number of queued searches: got %d, want %d", got, tt.wantNumPages)
			}

			// Verify each queued search has the correct page number and keyword
			seenPages := make(map[int]bool)
			seenKeywords := make(map[string]bool)
			for _, qs := range mockQueue.queuedSearches {
				if qs.UserID != tt.user.ID {
					t.Errorf("wrong user ID: got %s, want %s", qs.UserID, tt.user.ID)
				}

				// Check location is preserved
				if qs.Criteria.Location != tt.user.Search.Criteria.Location {
					t.Errorf("wrong location: got %s, want %s", qs.Criteria.Location, tt.user.Search.Criteria.Location)
				}

				// Each search should have exactly one keyword from the original list
				if len(qs.Criteria.Keywords) != 1 {
					t.Errorf("expected exactly one keyword per search, got %d", len(qs.Criteria.Keywords))
				} else {
					keyword := qs.Criteria.Keywords[0]
					found := false
					for _, k := range tt.user.Search.Criteria.Keywords {
						if k == keyword {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("unexpected keyword: %s", keyword)
					}
					seenKeywords[keyword] = true
				}

				seenPages[qs.Criteria.PageNumber] = true
			}

			// Verify we saw all expected pages
			expectedPages := numPagesForTier(tt.user.Tier, tt.testingMode == "true")
			if len(seenPages) != expectedPages {
				t.Errorf("wrong number of unique pages: got %d, want %d", len(seenPages), expectedPages)
			}

			// Verify we saw all keywords
			if len(seenKeywords) != len(tt.user.Search.Criteria.Keywords) {
				t.Errorf("wrong number of unique keywords: got %d, want %d", len(seenKeywords), len(tt.user.Search.Criteria.Keywords))
			}
		})
	}
}

// Helper function to determine number of pages based on tier and testing mode
func numPagesForTier(tier internal.UserTier, isTestingMode bool) int {
	if isTestingMode {
		return TestingModeNumPages
	}
	switch tier {
	case internal.UserTierPro:
		return ProTierNumPages
	default:
		return FreeTierNumPages
	}
}
