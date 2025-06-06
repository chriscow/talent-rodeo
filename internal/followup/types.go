package followup

import "time"

// Question represents a normalized, canonical question aggregated from multiple user jobs.
// It is stored in the `user_followup_questions` collection.
type Question struct {
	ID             string
	UserID         string
	QuestionText   string // The normalized/canonical question text.
	QuestionType   string // e.g., "interest_level", "application_status", "concerns"
	EstimatedCount int    // LLM's estimate of how many jobs asked this question.
	Priority       int    // Ranking from aggregation process.

	// Email Delivery Tracking (managed by nudge package)
	SentToUserAt    *time.Time
	EmailTrackingID *string // Short ID of the email that sent this question.

	// Response Tracking
	Answer             *string // User's answer extracted by LLM.
	ResponseReceivedAt *time.Time
	AnswerEmailID      *string // Reference to the inbox email containing the answer.

	// Metadata
	CreatedAt time.Time // When this normalized question was first created.
	UpdatedAt time.Time // Last updated by any process.
}

// EmailTracking represents a single follow-up email sent to a user.
// It is stored in the `followup_email_tracking` collection.
type EmailTracking struct {
	ID          string // Short ID (e.g., "A7K9M2")
	UserID      string
	SentAt      time.Time
	QuestionIDs []string // Firestore document IDs of questions sent in this email.

	// Response tracking
	ResponseReceived *time.Time
	ResponseEmailID  *string
}

// LLMProcessedQuestion represents the structured output for a single question
// from the LLM's aggregation and normalization process.
type LLMProcessedQuestion struct {
	ID                  string   `json:"id"`
	NormalizedText      string   `json:"normalized_text"`
	Priority            int      `json:"priority"`
	Type                string   `json:"type"`
	EstimatedCount      int      `json:"estimated_count"`
	MatchedRawQuestions []string `json:"matched_raw_questions"`
	// This will be populated by our code to link back to the source jobs
	SourceJobIDs []string `json:"-"`
}

// LLMAggregationResponse represents the top-level JSON structure
// expected from the LLM.
type LLMAggregationResponse struct {
	ProcessedQuestions []LLMProcessedQuestion `json:"processed_questions"`
}
