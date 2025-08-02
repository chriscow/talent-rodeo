package feedback

import (
	"context"
	"fmt"
	"talent-rodeo/internal"
	"time"
)

// mockLLMClient is a test mock implementation of LLMClient
type mockLLMClient struct {
	analyzeFeedbackFunc     func(ctx context.Context, prompt string) (*SemanticFeedbackAnalysis, error)
	updateSteeringQueryFunc func(ctx context.Context, prompt string) (*internal.SteeringQuery, error)
	updateStatementFunc     func(ctx context.Context, prompt string) (string, error)
}

func (m *mockLLMClient) AnalyzeFeedback(ctx context.Context, prompt string) (*SemanticFeedbackAnalysis, error) {
	if m.analyzeFeedbackFunc != nil {
		return m.analyzeFeedbackFunc(ctx, prompt)
	}
	return nil, nil
}

func (m *mockLLMClient) UpdateSteeringQuery(ctx context.Context, prompt string) (*internal.SteeringQuery, error) {
	if m.updateSteeringQueryFunc != nil {
		return m.updateSteeringQueryFunc(ctx, prompt)
	}
	return nil, nil
}

func (m *mockLLMClient) UpdateStatement(ctx context.Context, prompt string) (string, error) {
	if m.updateStatementFunc != nil {
		return m.updateStatementFunc(ctx, prompt)
	}
	return "", nil
}

// mockFeedbackStore is a test mock implementation of FeedbackStore
type mockFeedbackStore struct {
	feedbacks         []*internal.UserFeedback
	statements        map[string]*string
	steeringQueries   map[string]*internal.SteeringQuery
	processedFeedback map[string]time.Time
}

func (m *mockFeedbackStore) GetUnprocessedUserFeedback(ctx context.Context, userID string) ([]*internal.UserFeedback, error) {
	var unprocessed []*internal.UserFeedback
	for _, feedback := range m.feedbacks {
		if feedback.UserID == userID && (feedback.ProcessedAt == nil || feedback.ProcessedAt.IsZero()) {
			unprocessed = append(unprocessed, feedback)
		}
	}
	return unprocessed, nil
}

func (m *mockFeedbackStore) GetUserSteeringQuery(ctx context.Context, userID string) (*internal.SteeringQuery, error) {
	if sq, exists := m.steeringQueries[userID]; exists {
		return sq, nil
	}
	return &internal.SteeringQuery{}, nil
}

func (m *mockFeedbackStore) SetUserSteeringQuery(ctx context.Context, userID string, sq internal.SteeringQuery) error {
	if m.steeringQueries == nil {
		m.steeringQueries = make(map[string]*internal.SteeringQuery)
	}
	m.steeringQueries[userID] = &sq
	return nil
}

func (m *mockFeedbackStore) GetUserStatement(ctx context.Context, userID string) (*string, error) {
	return m.statements[userID], nil
}

func (m *mockFeedbackStore) SetUserStatement(ctx context.Context, userID string, statement string) error {
	if m.statements == nil {
		m.statements = make(map[string]*string)
	}
	m.statements[userID] = &statement
	return nil
}

func (m *mockFeedbackStore) CreateStatementVersionFromFeedback(ctx context.Context, userID string, feedbackData map[string]any, promptFile string) (*internal.UserStatementVersion, error) {
	// Mock implementation - create a fake version
	version := &internal.UserStatementVersion{
		ID:                fmt.Sprintf("%s_v%d", userID, 2), // Assume this is version 2
		VersionNumber:     2,
		IsLatest:          true,
		StatementText:     "Enhanced statement based on feedback",
		GeneratedBy:       "feedback",
		GenerationReason:  "Enhanced statement based on user feedback",
		PromptFile:        promptFile,
		SourceData:        feedbackData,
		CreatedAt:         time.Now(),
		ChangeType:        "feedback_driven",
		SignificantChange: true,
	}

	// Update the mock store's statement
	if m.statements == nil {
		m.statements = make(map[string]*string)
	}
	m.statements[userID] = &version.StatementText

	return version, nil
}

func (m *mockFeedbackStore) MarkFeedbackAsProcessed(ctx context.Context, feedbackIDs []string, processedAt time.Time) error {
	if m.processedFeedback == nil {
		m.processedFeedback = make(map[string]time.Time)
	}
	for _, id := range feedbackIDs {
		m.processedFeedback[id] = processedAt
		// Also mark in the feedbacks slice
		for _, feedback := range m.feedbacks {
			if feedback.ID == id {
				feedback.ProcessedAt = &processedAt
			}
		}
	}
	return nil
}
