package internal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/chriscow/minds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SystemPromptResolver was removed - prompts are now determined
// by actual system logic in the cascade regeneration code

// mockStatementRegenerator is a simple mock implementation for testing
type mockStatementRegenerator struct {
	regenerationText string
	shouldFail       bool
}

func (m *mockStatementRegenerator) GenerateStatement(ctx context.Context, version *UserStatementVersion) (string, error) {
	if m.shouldFail {
		return "", fmt.Errorf("mock regeneration failed")
	}
	return m.regenerationText, nil
}

// createTestStatementRegeneratorRegistry creates a test registry with mock regenerators
func createTestStatementGeneratorRegistry(t *testing.T, client *FirestoreClient) *StatementGeneratorRegistry {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	registry := NewStatementGeneratorRegistry(logger)

	// Register mock regenerators for testing
	transcriptMock := &mockStatementRegenerator{
		regenerationText: "Mock regenerated transcript statement",
		shouldFail:       false,
	}
	registry.Register("transcript", transcriptMock)

	feedbackMock := &mockStatementRegenerator{
		regenerationText: "Mock regenerated feedback statement",
		shouldFail:       false,
	}
	registry.Register("feedback", feedbackMock)

	return registry
}

func TestBuildDependencyChain(t *testing.T) {
	ctx := context.Background()
	client, err := NewTestFirestoreClient(ctx)
	require.NoError(t, err)
	defer client.Close()

	// Clean up test data
	testPrefix := "test-cascade-"
	DeleteTestData(t, client, testPrefix)
	t.Cleanup(func() {
		DeleteTestData(t, client, testPrefix)
	})

	// Create test user
	timestamp := time.Now().UnixNano()
	uniqueEmail := fmt.Sprintf("test-cascade-%d@example.com", timestamp)
	user := CreateTestUser(t, client, "Test Cascade User", uniqueEmail, "+14251234567", false)
	userID := user.ID

	// Create a chain of versions: transcript -> feedback (only actual systems)
	transcriptVersion := createTestStatementVersion(t, ctx, client, userID, 1, "transcript", "transcript.user-statement.v001.txt")
	feedbackVersion := createTestStatementVersion(t, ctx, client, userID, 2, "feedback", "statement-update.v001.txt")

	tests := []struct {
		name          string
		fromVersionID string
		expectedCount int
		expectedFirst string
		expectedLast  string
	}{
		{
			name:          "Chain from transcript version",
			fromVersionID: transcriptVersion.ID,
			expectedCount: 2,
			expectedFirst: transcriptVersion.ID,
			expectedLast:  feedbackVersion.ID,
		},
		{
			name:          "Chain from feedback version",
			fromVersionID: feedbackVersion.ID,
			expectedCount: 1,
			expectedFirst: feedbackVersion.ID,
			expectedLast:  feedbackVersion.ID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, err := BuildDependencyChain(ctx, client, userID, tt.fromVersionID)
			require.NoError(t, err)
			assert.Len(t, chain, tt.expectedCount)

			if len(chain) > 0 {
				assert.Equal(t, tt.expectedFirst, chain[0].ID)
				assert.Equal(t, tt.expectedLast, chain[len(chain)-1].ID)
			}
		})
	}
}

func TestBuildDependencyChainErrors(t *testing.T) {
	ctx := context.Background()
	client, err := NewTestFirestoreClient(ctx)
	require.NoError(t, err)
	defer client.Close()

	// Clean up test data
	testPrefix := "test-cascade-errors-"
	DeleteTestData(t, client, testPrefix)
	t.Cleanup(func() {
		DeleteTestData(t, client, testPrefix)
	})

	timestamp := time.Now().UnixNano()
	uniqueEmail := fmt.Sprintf("test-cascade-errors-%d@example.com", timestamp)
	user := CreateTestUser(t, client, "Test Cascade Errors User", uniqueEmail, "+14251234567", false)
	userID := user.ID

	tests := []struct {
		name          string
		userID        string
		fromVersionID string
		expectError   bool
	}{
		{
			name:          "Empty userID",
			userID:        "",
			fromVersionID: "test_v1",
			expectError:   true,
		},
		{
			name:          "Empty fromVersionID",
			userID:        userID,
			fromVersionID: "",
			expectError:   true,
		},
		{
			name:          "Non-existent version",
			userID:        userID,
			fromVersionID: "nonexistent_v999",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildDependencyChain(ctx, client, tt.userID, tt.fromVersionID)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRegenerateCascadeDryRun(t *testing.T) {
	ctx := context.Background()
	client, err := NewTestFirestoreClient(ctx)
	require.NoError(t, err)
	defer client.Close()

	// Clean up test data
	testPrefix := "test-cascade-dry-"
	DeleteTestData(t, client, testPrefix)
	t.Cleanup(func() {
		DeleteTestData(t, client, testPrefix)
	})

	timestamp := time.Now().UnixNano()
	uniqueEmail := fmt.Sprintf("test-cascade-dry-%d@example.com", timestamp)
	user := CreateTestUser(t, client, "Test Cascade Dry User", uniqueEmail, "+14251234567", false)
	userID := user.ID

	// Create a chain of versions with source data (only real systems)
	transcriptVersion := createTestStatementVersionWithSourceData(t, ctx, client, userID, 1, "transcript", "transcript.user-statement.v001.txt")
	_ = createTestStatementVersionWithSourceData(t, ctx, client, userID, 2, "feedback", "statement-update.v001.txt")

	// Create test registry
	registry := createTestStatementGeneratorRegistry(t, client)

	// Test dry run
	report, err := RegenerateCascade(ctx, client, registry, transcriptVersion.ID, "", true)
	require.NoError(t, err)
	assert.NotNil(t, report)
	assert.True(t, report.Operation.DryRun)
	assert.Equal(t, "completed", report.Operation.Status)
	assert.Equal(t, 2, report.VersionsProcessed)
	assert.Len(t, report.Results, 2)

	// Verify that no actual changes were made
	for _, result := range report.Results {
		assert.Contains(t, []string{"would_regenerate", "failed"}, result.Status)
		assert.Nil(t, result.NewVersionID)
	}

	// Verify versions were not actually modified
	updatedTranscript, err := getStatementVersion(t, ctx, client, userID, transcriptVersion.ID)
	require.NoError(t, err)
	assert.Equal(t, transcriptVersion.StatementText, updatedTranscript.StatementText)
	assert.Equal(t, transcriptVersion.PromptFile, updatedTranscript.PromptFile)
}

func TestRegenerateCascadeErrors(t *testing.T) {
	ctx := context.Background()
	client, err := NewTestFirestoreClient(ctx)
	require.NoError(t, err)
	defer client.Close()

	// Clean up test data
	testPrefix := "test-cascade-errors2-"
	DeleteTestData(t, client, testPrefix)
	t.Cleanup(func() {
		DeleteTestData(t, client, testPrefix)
	})

	// Create test registry
	registry := createTestStatementGeneratorRegistry(t, client)

	tests := []struct {
		name      string
		versionID string
		prompt    string
		expectErr bool
	}{
		{
			name:      "Empty version ID",
			versionID: "",
			prompt:    "test.v001.txt",
			expectErr: true,
		},
		{
			name:      "Empty prompt",
			versionID: "test_v1",
			prompt:    "",
			expectErr: true,
		},
		{
			name:      "Invalid version ID format",
			versionID: "invalid_format",
			prompt:    "test.v001.txt",
			expectErr: true,
		},
		{
			name:      "Non-existent version",
			versionID: "nonexistent_v999",
			prompt:    "test.v001.txt",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RegenerateCascade(ctx, client, registry, tt.versionID, tt.prompt, true)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper functions for testing

func createTestStatementVersion(t *testing.T, ctx context.Context, fs *FirestoreClient, userID string, versionNum int, generatedBy, promptFile string) *UserStatementVersion {
	version := &UserStatementVersion{
		ID:               fmt.Sprintf("%s_v%d", userID, versionNum),
		VersionNumber:    versionNum,
		IsLatest:         versionNum == 1, // Only first version is latest for simplicity
		StatementText:    fmt.Sprintf("Test statement %d generated by %s", versionNum, generatedBy),
		StatementHash:    generateStatementHash(fmt.Sprintf("Test statement %d", versionNum)),
		GeneratedBy:      generatedBy,
		GenerationReason: fmt.Sprintf("Test %s generation", generatedBy),
		PromptFile:       promptFile,
		PromptVersion:    ExtractPromptVersion(promptFile),
		LLMModel:         "test-model",
		SourceData:       map[string]any{}, // Empty for basic test
		CreatedAt:        time.Now(),
		ChangeType:       "test",
		SystemType:       generatedBy,
	}

	// Save the version
	versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(version.ID)
	_, err := versionRef.Set(ctx, version)
	require.NoError(t, err)

	return version
}

func createTestStatementVersionWithSourceData(t *testing.T, ctx context.Context, fs *FirestoreClient, userID string, versionNum int, generatedBy, promptFile string) *UserStatementVersion {
	version := createTestStatementVersion(t, ctx, fs, userID, versionNum, generatedBy, promptFile)

	// Add minimal source data based on generation type
	switch generatedBy {
	case "transcript":
		version.SourceData = map[string]any{
			"transcript_text": "Test transcript content",
			"messages": []minds.Message{
				{Role: minds.RoleUser, Content: "Test transcript content"},
			},
		}
	case "feedback":
		version.SourceData = map[string]any{
			"transcript_text": "Test transcript content",
			"feedback_data": map[string]any{
				"liked_themes": []string{"theme1", "theme2"},
			},
		}
	case "followup":
		version.SourceData = map[string]any{
			"transcript_text": "Test transcript content",
			"question_answers": []map[string]any{
				{"question": "test", "answer": "test answer"},
			},
		}
	}

	// Update the version with source data
	versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(version.ID)
	_, err := versionRef.Set(ctx, version)
	require.NoError(t, err)

	return version
}

func getStatementVersion(t *testing.T, ctx context.Context, fs *FirestoreClient, userID, versionID string) (*UserStatementVersion, error) {
	versionRef := fs.Collection(usersCollection).Doc(userID).Collection(userStatementsCollection).Doc(versionID)
	doc, err := versionRef.Get(ctx)
	if err != nil {
		return nil, err
	}

	var version UserStatementVersion
	if err := doc.DataTo(&version); err != nil {
		return nil, err
	}

	return &version, nil
}
