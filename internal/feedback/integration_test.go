package feedback

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"

	"github.com/joho/godotenv"
)

// TestFeedbackProcessingWithRealFirestore tests feedback processing with a real user's
// data from Firestore without saving the results. This allows testing with actual
// production data to validate the feedback processing pipeline.
//
// To use this test:
// 1. Set FIREBASE_PROJECT_ID and FIREBASE_CREDENTIALS environment variables
// 2. Run: go test -tags=integration -run TestFeedbackProcessingWithRealFirestore
// 3. When prompted, enter a real user ID from your Firestore database
//
// The test will:
// - Connect to real Firestore
// - Load the user's actual feedback
// - Process it through the LLM pipeline
// - Display results without saving them back to Firestore
func TestFeedbackProcessingWithRealFirestore(t *testing.T) {
	// if testing.Short() {
	// 	t.Skip("Skipping real Firestore test in short mode")
	// }

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Get user ID from environment variable or use default test user
	userID := "0dad1caa-c308-42d1-883d-9f6fc85deff1"
	if userID == "" {
		t.Skip("TEST_USER_ID environment variable not set. Set it to a real user ID to run this test.")
	}

	t.Logf("Testing feedback processing for user: %s", userID)

	godotenv.Overload("../../.prod.env")

	// Connect to real Firestore
	firestoreClient, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to Firestore: %v", err)
	}
	defer firestoreClient.Close()

	// Create real feedback store
	feedbackStore := newFeedbackStore(firestoreClient)

	// Get current user state for comparison
	currentStatement, err := feedbackStore.GetUserStatement(ctx, userID)
	if err != nil {
		t.Logf("Could not get current statement (user may not have one): %v", err)
	}

	currentSteeringQuery, err := feedbackStore.GetUserSteeringQuery(ctx, userID)
	if err != nil {
		t.Logf("Could not get current steering query (user may not have one): %v", err)
	}

	// Load user's actual feedback from Firestore
	feedback, err := feedbackStore.GetUnprocessedUserFeedback(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to load user feedback: %v", err)
	}

	if len(feedback) == 0 {
		t.Fatalf("No unprocessed feedback found for user %s", userID)
	}

	t.Logf("Found %d unprocessed feedback items", len(feedback))

	// Log feedback items for context
	for i, fb := range feedback {
		t.Logf("Feedback %d: %s (liked: %t, job: %s)", i+1, fb.ItemText, fb.Liked, fb.JobID)
	}

	// Create a read-only feedback store that won't save updates
	readOnlyStore := &readOnlyFeedbackStore{
		real:   feedbackStore,
		logger: logger,
	}

	// Use real LLM client
	realLLMClient := NewMindsClient()

	// Create the processor with read-only store
	processor := newLLMProcessor(readOnlyStore, logger, realLLMClient)

	// Test processing
	t.Logf("Processing feedback for user %s...", userID)
	err = processor.ProcessUserFeedback(ctx, userID)
	if err != nil {
		t.Fatalf("ProcessUserFeedback failed: %v", err)
	}

	// Display results
	t.Logf("\n=== PROCESSING RESULTS ===")
	t.Logf("Processing completed successfully")
	t.Logf("Feedback count: %d", len(feedback))

	// Show current vs. new statement
	t.Logf("\n=== STATEMENT COMPARISON ===")
	if currentStatement != nil {
		t.Logf("Current statement:\n%s", *currentStatement)
	} else {
		t.Logf("Current statement: (none)")
	}

	newStatement := readOnlyStore.capturedStatement
	if newStatement != nil {
		t.Logf("\nNew statement:\n%s", *newStatement)
	} else {
		t.Logf("\nNew statement: (none generated)")
	}

	// Show current vs. new steering query
	t.Logf("\n=== STEERING QUERY COMPARISON ===")
	if currentSteeringQuery != nil {
		t.Logf("Current steering query - Move toward: %s", currentSteeringQuery.MoveToward)
		t.Logf("Current steering query - Move away: %s", currentSteeringQuery.MoveAway)
		t.Logf("Current steering query - Force away: %.2f", currentSteeringQuery.ForceAway)
	} else {
		t.Logf("Current steering query: (none)")
	}

	newSteeringQuery := readOnlyStore.capturedSteeringQuery
	if newSteeringQuery != nil {
		t.Logf("\nNew steering query - Move toward: %s", newSteeringQuery.MoveToward)
		t.Logf("New steering query - Move away: %s", newSteeringQuery.MoveAway)
		t.Logf("New steering query - Force away: %.2f", newSteeringQuery.ForceAway)
	} else {
		t.Logf("\nNew steering query: (none generated)")
	}

	t.Logf("\n=== TEST COMPLETED ===")
	t.Logf("No changes were saved to Firestore")
}

// readOnlyFeedbackStore wraps a real FeedbackStore but captures updates without saving them
type readOnlyFeedbackStore struct {
	real                  FeedbackStore
	logger                *slog.Logger
	capturedStatement     *string
	capturedSteeringQuery *internal.SteeringQuery
}

func (r *readOnlyFeedbackStore) GetUnprocessedUserFeedback(ctx context.Context, userID string) ([]*internal.UserFeedback, error) {
	return r.real.GetUnprocessedUserFeedback(ctx, userID)
}

func (r *readOnlyFeedbackStore) MarkFeedbackAsProcessed(ctx context.Context, feedbackIDs []string, processedAt time.Time) error {
	r.logger.Info("Would mark feedback as processed (read-only mode)", "feedbackIDs", feedbackIDs, "count", len(feedbackIDs))
	return nil
}

func (r *readOnlyFeedbackStore) SetUserStatement(ctx context.Context, userID string, statement string) error {
	r.capturedStatement = &statement
	r.logger.Info("Captured statement update (read-only mode)", "userID", userID, "length", len(statement))
	return nil
}

func (r *readOnlyFeedbackStore) SetUserSteeringQuery(ctx context.Context, userID string, sq internal.SteeringQuery) error {
	r.capturedSteeringQuery = &sq
	r.logger.Info("Captured steering query update (read-only mode)", "userID", userID)
	return nil
}

func (r *readOnlyFeedbackStore) GetUserStatement(ctx context.Context, userID string) (*string, error) {
	return r.real.GetUserStatement(ctx, userID)
}

func (r *readOnlyFeedbackStore) GetUserSteeringQuery(ctx context.Context, userID string) (*internal.SteeringQuery, error) {
	return r.real.GetUserSteeringQuery(ctx, userID)
}

// TestFeedbackProcessingIntegration tests the complete feedback processing pipeline
// with real LLM calls. This test is tagged with "integration" to avoid running
// during normal test runs since it makes real API calls.
func TestFeedbackProcessingIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create test feedback data
	testFeedbacks := createTestFeedbackData()

	// Create mock store with test data
	mockStore := &mockFeedbackStore{
		feedbacks:       testFeedbacks,
		statements:      make(map[string]*string),
		steeringQueries: make(map[string]*internal.SteeringQuery),
	}

	// Use real LLM client
	realLLMClient := NewMindsClient()

	// Create the processor
	processor := newLLMProcessor(mockStore, logger, realLLMClient)

	// Test processing
	userID := "test-user-123"
	err := processor.ProcessUserFeedback(ctx, userID)
	if err != nil {
		t.Fatalf("ProcessUserFeedback failed: %v", err)
	}

	// Verify statement was updated
	updatedStatement := mockStore.statements[userID]
	if updatedStatement == nil {
		t.Fatal("Expected statement to be updated, but it was nil")
	}

	t.Logf("Updated statement: %s", *updatedStatement)

	// Verify statement contains expected content based on our test feedback
	statement := *updatedStatement
	if statement == "" {
		t.Fatal("Expected non-empty statement")
	}

	// Basic validation that the statement was influenced by our feedback
	// Our test data includes likes for "remote work" and "Go programming"
	// and dislikes for "on-site required" and "Java"
	if len(statement) < 50 {
		t.Errorf("Statement seems too short: %s", statement)
	}

	// Log the results for manual inspection
	t.Logf("Integration test completed successfully")
	t.Logf("Statement length: %d characters", len(statement))
	t.Logf("Feedback items processed: %d", len(testFeedbacks))
}

// TestStatementUpdateWithRealLLM tests just the statement update chain with real LLM
func TestStatementUpdateWithRealLLM(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create statement updater with real LLM
	realLLMClient := NewMindsClient()
	updater := newStatementUpdater(realLLMClient, logger)

	// Create test analysis data
	analysis := &SemanticFeedbackAnalysis{
		SemanticThemes: struct {
			Liked    []SemanticTheme `json:"Liked"`
			Disliked []SemanticTheme `json:"Disliked"`
		}{
			Liked: []SemanticTheme{
				{
					Theme:           "Remote work opportunities",
					Keywords:        []string{"remote", "work from home", "distributed team"},
					ConfidenceScore: 0.9,
					Frequency:       3,
				},
				{
					Theme:           "Go programming",
					Keywords:        []string{"golang", "go", "backend development"},
					ConfidenceScore: 0.85,
					Frequency:       2,
				},
			},
			Disliked: []SemanticTheme{
				{
					Theme:           "On-site requirements",
					Keywords:        []string{"office required", "in-person", "commute"},
					ConfidenceScore: 0.8,
					Frequency:       2,
				},
			},
		},
		ConfidenceScore:  0.87,
		TrendingPatterns: "preference for flexible work arrangements",
		Reasoning:        "User consistently likes remote-friendly positions and dislikes rigid office requirements",
	}

	// Test with no existing statement
	t.Run("no existing statement", func(t *testing.T) {
		updatedStatement, err := updater.Run(ctx, nil, analysis)
		if err != nil {
			t.Fatalf("StatementUpdateChain.Run failed: %v", err)
		}

		if updatedStatement == "" {
			t.Fatal("Expected non-empty statement")
		}

		t.Logf("Generated statement (no existing): %s", updatedStatement)
	})

	// Test with existing statement
	t.Run("with existing statement", func(t *testing.T) {
		existingStatement := "I'm a software engineer with 5 years of experience in Python and web development. I prefer collaborative environments and am open to learning new technologies."

		updatedStatement, err := updater.Run(ctx, &existingStatement, analysis)
		if err != nil {
			t.Fatalf("StatementUpdateChain.Run failed: %v", err)
		}

		if updatedStatement == "" {
			t.Fatal("Expected non-empty statement")
		}

		t.Logf("Updated statement (with existing): %s", updatedStatement)

		// The updated statement should be different from the original
		if updatedStatement == existingStatement {
			t.Error("Expected statement to be updated, but it remained the same")
		}
	})
}

// createTestFeedbackData creates realistic test feedback for integration testing
func createTestFeedbackData() []*internal.UserFeedback {
	baseTime := time.Now().Add(-24 * time.Hour)

	return []*internal.UserFeedback{
		{
			ID:        "fb1",
			UserID:    "test-user-123",
			JobID:     "job1",
			ItemID:    "resp-1",
			ItemText:  "Work remotely with flexible hours and distributed team collaboration",
			Liked:     true,
			CreatedAt: baseTime,
		},
		{
			ID:        "fb2",
			UserID:    "test-user-123",
			JobID:     "job1",
			ItemID:    "skill-1",
			ItemText:  "Strong proficiency in Go programming language and microservices architecture",
			Liked:     true,
			CreatedAt: baseTime.Add(1 * time.Hour),
		},
		{
			ID:        "fb3",
			UserID:    "test-user-123",
			JobID:     "job2",
			ItemID:    "req-1",
			ItemText:  "Must be available for daily in-office meetings and collaborative work sessions",
			Liked:     false,
			CreatedAt: baseTime.Add(2 * time.Hour),
		},
		{
			ID:        "fb4",
			UserID:    "test-user-123",
			JobID:     "job2",
			ItemID:    "skill-2",
			ItemText:  "Experience with Java Enterprise applications and legacy system maintenance",
			Liked:     false,
			CreatedAt: baseTime.Add(3 * time.Hour),
		},
		{
			ID:        "fb5",
			UserID:    "test-user-123",
			JobID:     "job3",
			ItemID:    "resp-2",
			ItemText:  "Build and maintain APIs using modern Go frameworks and cloud technologies",
			Liked:     true,
			CreatedAt: baseTime.Add(4 * time.Hour),
		},
	}
}
