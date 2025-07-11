package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"talent-rodeo/assets"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/utils"
	"time"

	"sort"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds/tools"
	"github.com/chriscow/vapi-go"
	"github.com/samber/lo"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/api/iterator"
)

// AppointmentType represents the different types of appointments
type AppointmentType string

const (
	AppointmentTypeVoice AppointmentType = "voice"
	AppointmentTypeChat  AppointmentType = "chat"
)

// AppointmentStatus represents the possible states of an appointment
type AppointmentStatus string

const (
	appointmentsCollection = "appointments"
	// top-level collection to track appointments needing processing
	processingAppointmentsCollection = "processing_appointments"

	AppointmentStatusScheduled  AppointmentStatus = "scheduled"
	AppointmentStatusRunning    AppointmentStatus = "running"
	AppointmentStatusProcessing AppointmentStatus = "processing"
	AppointmentStatusCompleted  AppointmentStatus = "completed"
	AppointmentStatusCancelled  AppointmentStatus = "cancelled"
	AppointmentStatusFailed     AppointmentStatus = "failed"
	AppointmentStatusOther      AppointmentStatus = "other"
)

// ChatMessage represents a single message in a chat conversation
type ChatMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ConversationChecklist tracks the progress of gathering required information during chat
type ConversationChecklist struct {
	Items              []ChecklistItem `json:"items"`
	OverallStatus      string          `json:"overall_status"`      // "in_progress", "ready_for_completion", "completed"
	CompletionAttempts int             `json:"completion_attempts"` // Track attempts to prevent infinite loops
	LastUpdated        time.Time       `json:"last_updated"`
	MessageCount       int             `json:"message_count"` // Track conversation length
}

// ChecklistItem represents a specific topic or piece of information to be gathered
type ChecklistItem struct {
	ID              string                 `json:"id"`
	Topic           string                 `json:"topic"`
	Description     string                 `json:"description"`
	Status          string                 `json:"status"`           // "pending", "in_progress", "completed"
	Quality         string                 `json:"quality"`          // "low", "medium", "high"
	RequiredInfo    []string               `json:"required_info"`    // What specific information is needed
	ExtractedInfo   map[string]interface{} `json:"extracted_info"`   // What was actually gathered
	ValidationNotes string                 `json:"validation_notes"` // Notes about information quality
	Required        bool                   `json:"required"`         // Whether this item is required for completion
	LastUpdated     time.Time              `json:"last_updated"`
}

// Appointment represents a scheduled phone interview or chat session
type Appointment struct {
	ID            string
	Name          string
	UserID        string
	Type          AppointmentType // "voice" or "chat"
	ScheduledTime time.Time
	Timezone      string
	Status        AppointmentStatus

	// This is the end reason given by VAPI when we get the end of call report
	EndReason *string

	// Error is an internal error that occurred during the appointment
	Error       *string
	CallID      *string
	Summary     *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
	CancelledAt *time.Time
	FailedAt    *time.Time
	Notes       *string

	// Chat-specific fields
	ChatMessages []ChatMessage

	// Conversation checklist for chat appointments
	ConversationChecklist *ConversationChecklist

	RecapSent bool // Whether the recap has been sent to the user
}

func CreateAppointment(ctx context.Context, fs *FirestoreClient, userID string, appointment *Appointment) error {
	// Validate user exists first
	_, found, err := GetUser(ctx, fs, userID)
	if err != nil {
		return fmt.Errorf("failed to validate user: %w", err)
	}
	if !found {
		return fmt.Errorf("user %s not found", userID)
	}

	if appointment.ID == "" {
		appointment.ID = funcs.Slugify(fmt.Sprintf("%s-%d", appointment.Name, time.Now().UnixNano()))
	}

	if appointment.Type == "" {
		return fmt.Errorf("appointment type is required")
	}

	appointment.UserID = userID
	appointment.CreatedAt = time.Now()
	appointment.UpdatedAt = appointment.CreatedAt

	_, err = fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Doc(appointment.ID).Set(ctx, appointment)
	if err != nil {
		return fmt.Errorf("failed to create appointment: %w", err)
	}

	return nil
}

func GetAppointment(ctx context.Context, fs *FirestoreClient, userID string, appointmentID string) (*Appointment, error) {
	doc, err := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Doc(appointmentID).Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get appointment for user %s: %w", userID, err)
	}

	var appointment Appointment
	if err := doc.DataTo(&appointment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal appointment: %w", err)
	}

	return &appointment, nil
}

func GetLatestAppointment(ctx context.Context, fs *FirestoreClient, userID string) (*Appointment, bool, error) {
	iter := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).OrderBy("CompletedAt", firestore.Desc).Limit(1).Documents(ctx)

	doc, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get latest appointment: %w", err)
	}

	var appt Appointment
	if err := doc.DataTo(&appt); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal appointment data: %w", err)
	}

	return &appt, true, nil
}

func GetLatestCompletedAppointment(ctx context.Context, fs *FirestoreClient, userID string) (*Appointment, bool, error) {
	iter := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Where("Status", "==", AppointmentStatusCompleted).OrderBy("CompletedAt", firestore.Desc).Limit(1).Documents(ctx)

	doc, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get latest completed appointment: %w", err)
	}

	var appt Appointment
	if err := doc.DataTo(&appt); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal appointment data: %w", err)
	}

	return &appt, true, nil
}

// GetLatestRunningChatAppointment retrieves the latest running chat appointment for a user
func GetLatestRunningChatAppointment(ctx context.Context, fs *FirestoreClient, userID string) (*Appointment, bool, error) {
	iter := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).
		Where("Status", "==", AppointmentStatusRunning).
		Where("Type", "==", AppointmentTypeChat).
		OrderBy("CreatedAt", firestore.Desc).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get latest running chat appointment: %w", err)
	}

	var appt Appointment
	if err := doc.DataTo(&appt); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal appointment data: %w", err)
	}

	return &appt, true, nil
}

func ListAppointments(ctx context.Context, fs *FirestoreClient, userID string) ([]*Appointment, error) {
	iter := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Documents(ctx)

	var appointments []*Appointment
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate appointments: %w", err)
		}

		var appointment Appointment
		if err := doc.DataTo(&appointment); err != nil {
			return nil, fmt.Errorf("failed to unmarshal appointment: %w", err)
		}

		appointments = append(appointments, &appointment)
	}

	return appointments, nil
}

// ListCompletedAppointments retrieves all completed appointments for a user
// and returns them sorted by CompletedAt in descending order
func ListCompletedAppointments(ctx context.Context, fs *FirestoreClient, userID string) ([]*Appointment, error) {
	// Create a query for completed appointments where completed_at exists
	query := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).
		Where("Status", "==", AppointmentStatusCompleted).
		Where("CompletedAt", ">", time.Time{}) // Filter for non-nil completed_at values

	iter := query.Documents(ctx)

	var appointments []*Appointment
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate completed appointments: %w", err)
		}

		var appointment Appointment
		if err := doc.DataTo(&appointment); err != nil {
			return nil, fmt.Errorf("failed to unmarshal appointment: %w", err)
		}

		// Double-check that CompletedAt is not nil - this shouldn't happen based on our query
		// but it's good practice to validate
		if appointment.CompletedAt == nil {
			// Log a warning since this indicates data inconsistency
			fmt.Printf(BGYellow("WARNING: Found appointment %s with status 'completed' but nil CompletedAt field\n"), appointment.ID)
			continue
		}

		appointments = append(appointments, &appointment)
	}

	// Sort appointments by CompletedAt field in descending order (newest first)
	sort.SliceStable(appointments, func(i, j int) bool {
		return appointments[i].CompletedAt.After(*appointments[j].CompletedAt)
	})

	return appointments, nil
}

func UpdateAppointment(ctx context.Context, fs *FirestoreClient, appointment *Appointment) error {
	if appointment == nil {
		return fmt.Errorf("appointment cannot be nil")
	}
	if appointment.ID == "" || appointment.UserID == "" {
		return fmt.Errorf("appointment ID and UserID are required")
	}
	_, err := fs.Collection(usersCollection).Doc(appointment.UserID).Collection(appointmentsCollection).Doc(appointment.ID).Set(ctx, appointment)
	if err != nil {
		return fmt.Errorf("failed to update appointment: %w", err)
	}
	return nil
}

func UpdateAppointmentTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, appointment *Appointment) error {
	tx.Set(fs.Collection(usersCollection).Doc(appointment.UserID).Collection(appointmentsCollection).Doc(appointment.ID), appointment)
	return nil
}

func CancelAppointment(ctx context.Context, fs *FirestoreClient, userID string, appointmentID string) error {
	// First get the appointment
	appointment, err := GetAppointment(ctx, fs, userID, appointmentID)
	if err != nil {
		return fmt.Errorf("failed to get appointment for cancellation: %w", err)
	}

	// Update status and add a cancellation timestamp
	appointment.Status = AppointmentStatusCancelled
	now := time.Now()
	appointment.CancelledAt = &now
	appointment.UpdatedAt = now

	// Update the full appointment
	if err := UpdateAppointment(ctx, fs, appointment); err != nil {
		return fmt.Errorf("failed to cancel appointment: %w", err)
	}

	return nil
}

func GetAppointmentTranscript(ctx context.Context, fs *FirestoreClient, callID string) (*Transcript, error) {
	// Check if the appointment has a call ID
	if callID == "" {
		return nil, fmt.Errorf("appointment does not have a call ID")
	}

	// Get the call from VAPI
	call, err := vapi.GetCall(ctx, callID)
	if err != nil {
		return nil, fmt.Errorf("failed to get call from VAPI: %w", err)
	}

	return NewTranscriptFromCallResult(*call), nil
}

func DeleteAppointment(ctx context.Context, fs *FirestoreClient, userID, appointmentID string) error {
	_, err := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Doc(appointmentID).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete appointment: %w", err)
	}
	return nil
}

type processingAppointment struct {
	UserID        string
	AppointmentID string
	StartedAt     time.Time
}

// When an appointment enters processing state, add a reference to this collection
func ProcessAppointmentStarted(ctx context.Context, fs *FirestoreClient, user *User, appointment *Appointment) error {
	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Update the appointment as normal
		appointment.Status = AppointmentStatusProcessing
		appointment.UpdatedAt = time.Now()

		if err := UpdateAppointmentTx(ctx, fs, tx, appointment); err != nil {
			return fmt.Errorf("failed to update appointment in transaction: %w", err)
		}

		// Add reference to the processing tracking collection
		processingRef := processingAppointment{
			UserID:        appointment.UserID,
			AppointmentID: appointment.ID,
			StartedAt:     time.Now(),
		}

		if err := tx.Set(fs.Collection(processingAppointmentsCollection).Doc(appointment.ID), processingRef); err != nil {
			return fmt.Errorf("failed to add processing reference in transaction: %w", err)
		}

		if err := UpdateUserStatusTx(ctx, fs, tx, user, UserStatusCallProcessing); err != nil {
			return fmt.Errorf("failed to update user state: %w", err)
		}
		return nil
	})
}

// When processing is complete, remove from tracking collection
func ProcessAppointmentComplete(ctx context.Context, fs *FirestoreClient, user *User, appointment *Appointment) error {
	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Update appointment status
		appointment.Status = AppointmentStatusCompleted
		appointment.UpdatedAt = time.Now()
		appointment.CompletedAt = &appointment.UpdatedAt

		// Get references to both documents we need to update/delete
		appointmentRef := fs.Collection(usersCollection).Doc(appointment.UserID).Collection(appointmentsCollection).Doc(appointment.ID)
		processingRef := fs.Collection(processingAppointmentsCollection).Doc(appointment.ID)

		// Within the transaction, update the appointment and delete the processing reference
		if err := tx.Set(appointmentRef, appointment); err != nil {
			return fmt.Errorf("failed to update appointment in transaction: %w", err)
		}

		if err := tx.Delete(processingRef); err != nil {
			return fmt.Errorf("failed to delete processing reference in transaction: %w", err)
		}

		// Send completion state update
		if err := UpdateUserStatusTx(ctx, fs, tx, user, UserStatusCallCompleted); err != nil {
			return fmt.Errorf("failed to update user state: %w", err)
		}

		// Store user's timezone from appointment
		if err := UpdateUserTimezoneTx(ctx, fs, tx, user, appointment.Timezone); err != nil {
			return fmt.Errorf("failed to update user timezone: %w", err)
		}

		// Calculate trial end in user's timezone (11:59 PM on 7th day)
		subscriptionEnd, err := utils.CalculateTrialEndDate(time.Now(), appointment.Timezone)
		if err != nil {
			// Fallback to 7 days UTC if timezone parsing fails
			subscriptionEnd = time.Now().Add(7 * 24 * time.Hour)
		}
		if err := UpdateUserSubscriptionEndAtTx(ctx, fs, tx, user, subscriptionEnd); err != nil {
			return fmt.Errorf("failed to update user subscription end at: %w", err)
		}

		// User always starts as a free-trial user.  If they pay, we'll upgrade them to Pro.
		if err := UpdateUserTierTx(ctx, fs, tx, user, UserTierFree); err != nil {
			return fmt.Errorf("failed to update user tier to Pro: %w", err)
		}

		return nil
	})
}

func ProcessAppointmentFailed(ctx context.Context, fs *FirestoreClient, user *User, appointment *Appointment, err error) error {
	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Update appointment status
		appointment.Status = AppointmentStatusFailed
		appointment.UpdatedAt = time.Now()
		appointment.FailedAt = &appointment.UpdatedAt

		// Get references to both documents we need to update/delete
		appointmentRef := fs.Collection(usersCollection).Doc(appointment.UserID).Collection(appointmentsCollection).Doc(appointment.ID)
		processingRef := fs.Collection(processingAppointmentsCollection).Doc(appointment.ID)

		// Within the transaction, update the appointment and delete the processing reference
		if err := tx.Set(appointmentRef, appointment); err != nil {
			return fmt.Errorf("failed to update appointment in transaction: %w", err)
		}

		if err := tx.Delete(processingRef); err != nil {
			return fmt.Errorf("failed to delete processing reference in transaction: %w", err)
		}

		// Reset the user status so they can try again
		if err := UpdateUserStatusTx(ctx, fs, tx, user, UserStatusResumeUploaded); err != nil {
			return fmt.Errorf("failed to update user status: %w", err)
		}

		return nil
	})
}

// GenerateJobDescription generates an idealized, halucinated job description for an appointment transcript and resume
func GenerateJobDescription(ctx context.Context, fs *FirestoreClient, logger *slog.Logger, userID string, resume Resume, appointment Appointment) (string, error) {
	tmpl, err := assets.EmbeddedFiles.ReadFile("prompts/transcript-060-job-description.txt")
	if err != nil {
		return "", fmt.Errorf("failed to read job description prompt: %w", err)
	}

	if appointment.CallID == nil {
		return "", fmt.Errorf("appointment call ID is nil")
	}

	call, err := vapi.GetCall(ctx, *appointment.CallID)
	if err != nil {
		return "", fmt.Errorf("failed to get call: %w", err)
	}

	// remove any system messages
	msgs := call.Artifact.MessagesOpenAIFormatted
	msgs = lo.Filter(msgs, func(msg openai.ChatCompletionMessage, _ int) bool {
		return msg.Role != openai.ChatMessageRoleSystem
	})

	if len(msgs) == 0 {
		return "", fmt.Errorf("no messages found in call")
	}

	msgsJSON, err := json.Marshal(msgs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal call messages: %w", err)
	}

	if resume.RawText == "" {
		return "", fmt.Errorf("resume is empty")
	}

	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"Resume":     resume.RawText,
		"Transcript": string(msgsJSON),
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate prompt: %w", err)
	}

	const maxTokens = 8192
	resp := openai.ChatCompletionResponse{}
	desc, err := tools.Ask(ctx, prompt, tools.WithModel(tools.DeepSeekChat), tools.WithMaxTokens(maxTokens), tools.WantCompletionResponse(&resp))
	if err != nil {
		return "", fmt.Errorf("failed to generate job description: %w", err)
	}

	// Record LLM cost for job description generation
	if err := RecordLLMCost(ctx, fs, &userID, tools.DeepSeekChat, resp, "job_description_generation", nil); err != nil {
		// Log error but don't fail the operation
		logger.Error("failed to record LLM cost for job description generation", "error", err)
	}

	// get the text between <ideal_job_description> and </ideal_job_description> tags
	// Use (?s) flag to make dot match newlines (DOTALL mode)
	re := regexp.MustCompile(`(?s)<ideal_job_description>(.*?)</ideal_job_description>`)
	matches := re.FindStringSubmatch(desc)
	if len(matches) > 1 {
		return matches[1], nil
	}

	return desc, nil
}

// ListAppointmentsNeedingProcessing retrieves all appointments that are currently marked as processing
func ListAppointmentsNeedingProcessing(ctx context.Context, fs *FirestoreClient, logger *slog.Logger) ([]*Appointment, error) {
	var appointments []*Appointment
	var orphanedRefs []string

	// Query the processing tracking collection directly
	iter := fs.Collection(processingAppointmentsCollection).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate processing appointments: %w", err)
		}

		// Parse the processing reference using our struct
		var procRef processingAppointment
		if err := doc.DataTo(&procRef); err != nil {
			logger.Warn(BGYellow("failed to parse processing reference %s: %v\n"), doc.Ref.ID, err)
			orphanedRefs = append(orphanedRefs, doc.Ref.ID)
			continue
		}

		// Get the actual appointment
		appointment, err := GetAppointment(ctx, fs, procRef.UserID, procRef.AppointmentID)
		if err != nil {
			// Log the orphaned reference with more visibility
			logger.Warn(BGYellow("orphaned processing reference found"), "appointmentID", procRef.AppointmentID, "userID", procRef.UserID, "error", err)

			// Add to list of orphaned references to clean up
			orphanedRefs = append(orphanedRefs, doc.Ref.ID)
			continue
		}

		// Include both voice and chat appointments now that processing is unified
		if appointment.Type == AppointmentTypeVoice || appointment.Type == AppointmentTypeChat {
			appointments = append(appointments, appointment)
		}
	}

	// Clean up orphaned references synchronously
	if len(orphanedRefs) > 0 {
		logger.Warn("found orphaned processing references that need cleanup", "count", len(orphanedRefs))
		cleanupOrphanedProcessingRefs(ctx, fs, logger, orphanedRefs)
	}

	return appointments, nil
}

// cleanupOrphanedProcessingRefs removes references to appointments that no longer exist
func cleanupOrphanedProcessingRefs(ctx context.Context, fs *FirestoreClient, logger *slog.Logger, refIDs []string) {
	for _, refID := range refIDs {
		_, err := fs.Collection(processingAppointmentsCollection).Doc(refID).Delete(ctx)
		if err != nil {
			logger.Error("failed to clean up orphaned processing reference", "refID", refID, "error", err)
		}
	}
}

// AddChatMessage adds a new message to a chat appointment
func AddChatMessage(ctx context.Context, fs *FirestoreClient, appointment *Appointment, role, content string) error {
	if appointment.Type != AppointmentTypeChat {
		return fmt.Errorf("appointment is not a chat type")
	}

	message := ChatMessage{
		ID:        funcs.Slugify(fmt.Sprintf("msg-%d", time.Now().UnixNano())),
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}

	appointment.ChatMessages = append(appointment.ChatMessages, message)
	appointment.UpdatedAt = time.Now()

	// Auto-invoke checklist update based on message content
	if appointment.ConversationChecklist != nil {
		if err := UpdateChecklistFromMessage(appointment.ConversationChecklist, message); err != nil {
			// Log error but don't fail the message save - checklist update is supplementary
			// The error will be visible in logs for debugging
			fmt.Printf("Warning: Failed to auto-update checklist from message: %v\n", err)
		}
	}

	return UpdateAppointment(ctx, fs, appointment)
}

// AddChatMessageSafe adds a new message to a chat appointment using a transaction for thread safety
// This ensures that concurrent updates don't overwrite each other
func AddChatMessageSafe(ctx context.Context, fs *FirestoreClient, userID, appointmentID string, role, content string) error {
	return fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Get the latest appointment data within the transaction
		docRef := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Doc(appointmentID)
		doc, err := tx.Get(docRef)
		if err != nil {
			return fmt.Errorf("failed to get appointment in transaction: %w", err)
		}

		var appointment Appointment
		if err := doc.DataTo(&appointment); err != nil {
			return fmt.Errorf("failed to unmarshal appointment in transaction: %w", err)
		}

		if appointment.Type != AppointmentTypeChat {
			return fmt.Errorf("appointment is not a chat type")
		}

		// Create the new message
		message := ChatMessage{
			ID:        funcs.Slugify(fmt.Sprintf("msg-%d", time.Now().UnixNano())),
			Role:      role,
			Content:   content,
			Timestamp: time.Now(),
		}

		// Append to the messages
		appointment.ChatMessages = append(appointment.ChatMessages, message)
		appointment.UpdatedAt = time.Now()

		// Auto-invoke checklist update based on message content
		if appointment.ConversationChecklist != nil {
			if err := UpdateChecklistFromMessage(appointment.ConversationChecklist, message); err != nil {
				// Log error but don't fail the message save - checklist update is supplementary
				fmt.Printf("Warning: Failed to auto-update checklist from message: %v\n", err)
			}
		}

		// Save within the transaction
		return tx.Set(docRef, appointment)
	})
}

// GetChatHistory returns the chat messages for a chat appointment
func GetChatHistory(appointment *Appointment) ([]ChatMessage, error) {
	if appointment.Type != AppointmentTypeChat {
		return nil, fmt.Errorf("appointment is not a chat type")
	}
	return appointment.ChatMessages, nil
}

// GetChatMessages retrieves chat messages for an appointment by fetching the appointment first
func GetChatMessages(ctx context.Context, fs *FirestoreClient, userID, appointmentID string) ([]ChatMessage, error) {
	// Search through the appointments collection to find the appointment
	query := fs.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Where("ID", "==", appointmentID).Limit(1)

	iter := query.Documents(ctx)
	doc, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return nil, fmt.Errorf("appointment %s not found", appointmentID)
		}
		return nil, fmt.Errorf("failed to query appointment: %w", err)
	}

	var appointment Appointment
	if err := doc.DataTo(&appointment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal appointment: %w", err)
	}

	return GetChatHistory(&appointment)
}

// Constants for checklist management
const (
	ChecklistStatusInProgress         = "in_progress"
	ChecklistStatusReadyForCompletion = "ready_for_completion"
	ChecklistStatusCompleted          = "completed"

	ChecklistItemStatusPending    = "pending"
	ChecklistItemStatusInProgress = "in_progress"
	ChecklistItemStatusCompleted  = "completed"

	ChecklistItemQualityLow    = "low"
	ChecklistItemQualityMedium = "medium"
	ChecklistItemQualityHigh   = "high"

	MaxConversationLength = 50
	MaxCompletionAttempts = 3
)

// InitializeConversationChecklist creates a new checklist for a chat appointment
func InitializeConversationChecklist() *ConversationChecklist {
	now := time.Now()

	items := []ChecklistItem{
		{
			ID:            "introduction",
			Topic:         "Introduction and Verification",
			Description:   "Basic introduction and verification of caller identity",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"name_confirmed", "good_time_to_chat"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "warm_up",
			Topic:         "Warm-Up and Resume Acknowledgment",
			Description:   "Personal background exploration and resume discussion",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"career_background", "personal_insights", "achievements"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "deal_breakers",
			Topic:         "Identifying What They Want to Avoid",
			Description:   "Understanding work experiences and environments to avoid",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"work_dislikes", "management_style_dislikes", "company_culture_dislikes"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "search_strategy",
			Topic:         "Determine Search Strategy",
			Description:   "Primary search terms, skills, and job focus areas",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"primary_search_terms", "additional_skills", "things_to_avoid"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "work_environment",
			Topic:         "Work Environment Preferences",
			Description:   "Company type, work motivation, and collaboration style",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"company_type_preference", "work_motivation", "collaboration_style"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "location_preferences",
			Topic:         "Preferred Work Location",
			Description:   "Geographic location and remote/hybrid preferences",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"current_location", "work_location_preference", "flexibility"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
		{
			ID:            "validation",
			Topic:         "Clarifying and Validating Statements",
			Description:   "Clarification of vague or inconsistent responses",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"clarifications_complete", "consistency_verified"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      false, // Optional - depends on conversation quality
			LastUpdated:   now,
		},
		{
			ID:            "final_confirmation",
			Topic:         "Closing and Confirming Understanding",
			Description:   "Final validation of all gathered information",
			Status:        ChecklistItemStatusPending,
			Quality:       ChecklistItemQualityLow,
			RequiredInfo:  []string{"summary_confirmed", "additional_info_requested"},
			ExtractedInfo: make(map[string]interface{}),
			Required:      true,
			LastUpdated:   now,
		},
	}

	return &ConversationChecklist{
		Items:              items,
		OverallStatus:      ChecklistStatusInProgress,
		CompletionAttempts: 0,
		LastUpdated:        now,
		MessageCount:       0,
	}
}

// UpdateChecklistItem updates a specific checklist item
func UpdateChecklistItem(checklist *ConversationChecklist, itemID string, status string, quality string, extractedInfo map[string]interface{}, notes string) error {
	for i := range checklist.Items {
		if checklist.Items[i].ID == itemID {
			checklist.Items[i].Status = status
			checklist.Items[i].Quality = quality
			checklist.Items[i].ValidationNotes = notes
			checklist.Items[i].LastUpdated = time.Now()

			// Merge extracted info
			if extractedInfo != nil {
				for key, value := range extractedInfo {
					checklist.Items[i].ExtractedInfo[key] = value
				}
			}

			checklist.LastUpdated = time.Now()
			return nil
		}
	}
	return fmt.Errorf("checklist item with ID %s not found", itemID)
}

// GetChecklistItem retrieves a specific checklist item
func GetChecklistItem(checklist *ConversationChecklist, itemID string) (*ChecklistItem, error) {
	for i := range checklist.Items {
		if checklist.Items[i].ID == itemID {
			return &checklist.Items[i], nil
		}
	}
	return nil, fmt.Errorf("checklist item with ID %s not found", itemID)
}

// ValidateChecklistCompletion checks if the checklist is ready for completion
func ValidateChecklistCompletion(checklist *ConversationChecklist) (bool, []string, error) {
	if checklist == nil {
		return false, []string{"checklist not initialized"}, nil
	}

	var missingItems []string
	var qualityIssues []string

	// Check if we've exceeded maximum attempts
	if checklist.CompletionAttempts >= MaxCompletionAttempts {
		// Allow completion with lower quality after max attempts
		return ValidateChecklistCompletionWithLowerThreshold(checklist)
	}

	// Check if conversation is too long
	if checklist.MessageCount >= MaxConversationLength {
		return ValidateChecklistCompletionWithLowerThreshold(checklist)
	}

	// Check required items
	for _, item := range checklist.Items {
		if item.Required {
			if item.Status != ChecklistItemStatusCompleted {
				missingItems = append(missingItems, fmt.Sprintf("%s: %s", item.Topic, "not completed"))
			} else if item.Quality == ChecklistItemQualityLow {
				qualityIssues = append(qualityIssues, fmt.Sprintf("%s: %s", item.Topic, "low quality information"))
			}
		}
	}

	if len(missingItems) > 0 || len(qualityIssues) > 0 {
		var allIssues []string
		allIssues = append(allIssues, missingItems...)
		allIssues = append(allIssues, qualityIssues...)
		return false, allIssues, nil
	}

	return true, nil, nil
}

// ValidateChecklistCompletionWithLowerThreshold allows completion with lower quality thresholds
func ValidateChecklistCompletionWithLowerThreshold(checklist *ConversationChecklist) (bool, []string, error) {
	var missingItems []string

	// Only check that required items are at least started
	for _, item := range checklist.Items {
		if item.Required && item.Status == ChecklistItemStatusPending {
			missingItems = append(missingItems, fmt.Sprintf("%s: %s", item.Topic, "not started"))
		}
	}

	if len(missingItems) > 0 {
		return false, missingItems, nil
	}

	return true, nil, nil
}

// IncrementCompletionAttempt increments the completion attempt counter
func IncrementCompletionAttempt(checklist *ConversationChecklist) {
	checklist.CompletionAttempts++
	checklist.LastUpdated = time.Now()
}

// IncrementMessageCount increments the message counter
func IncrementMessageCount(checklist *ConversationChecklist) {
	checklist.MessageCount++
	checklist.LastUpdated = time.Now()
}

// GetChecklistSummary returns a summary of the checklist status
func GetChecklistSummary(checklist *ConversationChecklist) map[string]interface{} {
	if checklist == nil {
		return map[string]interface{}{
			"error": "checklist not initialized",
		}
	}

	completedCount := 0
	highQualityCount := 0
	requiredCount := 0

	for _, item := range checklist.Items {
		if item.Required {
			requiredCount++
		}
		if item.Status == ChecklistItemStatusCompleted {
			completedCount++
		}
		if item.Quality == ChecklistItemQualityHigh {
			highQualityCount++
		}
	}

	canComplete, issues, _ := ValidateChecklistCompletion(checklist)

	return map[string]interface{}{
		"overall_status":      checklist.OverallStatus,
		"completion_attempts": checklist.CompletionAttempts,
		"message_count":       checklist.MessageCount,
		"total_items":         len(checklist.Items),
		"required_items":      requiredCount,
		"completed_items":     completedCount,
		"high_quality_items":  highQualityCount,
		"can_complete":        canComplete,
		"issues":              issues,
		"items":               checklist.Items,
		"last_updated":        checklist.LastUpdated,
	}
}

// UpdateChecklistFromMessage automatically updates checklist based on message content
// This provides a lightweight guard to ensure every message pushes checklist state forward
func UpdateChecklistFromMessage(checklist *ConversationChecklist, message ChatMessage) error {
	if checklist == nil {
		return fmt.Errorf("checklist is nil")
	}

	// Skip system messages and tool calls
	if message.Role == "system" || message.Role == "tool" {
		return nil
	}

	// Skip messages with empty content
	if message.Content == "" {
		return nil
	}

	// Skip tool call messages (these are handled by explicit tool calls)
	if message.Role == "assistant" && (strings.Contains(message.Content, "TOOL_CALLS:") || strings.Contains(message.Content, "TOOL_RESPONSE:")) {
		return nil
	}

	return nil
}

// autoUpdateChecklistItem updates a checklist item only if it improves the current state
func autoUpdateChecklistItem(checklist *ConversationChecklist, itemID string, status string, quality string) error {
	item, err := GetChecklistItem(checklist, itemID)
	if err != nil {
		return err
	}

	// Only update if the new status is an improvement
	shouldUpdate := false
	if item.Status == ChecklistItemStatusPending && status != ChecklistItemStatusPending {
		shouldUpdate = true
	} else if item.Status == ChecklistItemStatusInProgress && status == ChecklistItemStatusCompleted {
		shouldUpdate = true
	}

	// Also update if quality is improving
	if item.Quality == ChecklistItemQualityLow && quality != ChecklistItemQualityLow {
		shouldUpdate = true
	} else if item.Quality == ChecklistItemQualityMedium && quality == ChecklistItemQualityHigh {
		shouldUpdate = true
	}

	if shouldUpdate {
		extractedInfo := map[string]interface{}{
			"auto_detected":  true,
			"detection_time": time.Now(),
			"source":         "message_analysis",
		}
		return UpdateChecklistItem(checklist, itemID, status, quality, extractedInfo, "Auto-detected from message content")
	}

	return nil
}

// Pattern matching functions for different checklist topics
func checkIntroductionPatterns(content string) bool {
	patterns := []string{
		"my name is", "i'm", "yes this is", "speaking", "good time to talk", "good time to chat",
		"thanks for calling", "hi", "hello", "yes that's me", "correct", "that's right",
	}
	return containsAnyPattern(content, patterns)
}

func checkBackgroundPatterns(content string) bool {
	patterns := []string{
		"background", "experience", "worked at", "current role", "previous", "career", "resume",
		"graduated", "degree", "university", "college", "skills", "expertise", "accomplishment",
		"achievement", "project", "responsibility", "manager", "team", "years", "developed",
	}
	return containsAnyPattern(content, patterns)
}

func checkDealBreakerPatterns(content string) bool {
	patterns := []string{
		"don't want", "avoid", "not interested", "dislike", "hate", "never again", "terrible",
		"worst", "bad experience", "nightmare", "toxic", "micromanag", "bureaucracy",
		"politics", "overtime", "weekend", "on-call", "travel", "relocate", "commute",
	}
	return containsAnyPattern(content, patterns)
}

func checkWorkEnvironmentPatterns(content string) bool {
	patterns := []string{
		"company", "startup", "corporation", "team", "culture", "environment", "atmosphere",
		"collaboration", "independent", "autonomous", "structured", "flexible", "formal",
		"casual", "fast-paced", "slow", "innovative", "traditional", "creative", "analytical",
	}
	return containsAnyPattern(content, patterns)
}

func checkLocationPatterns(content string) bool {
	patterns := []string{
		"location", "remote", "hybrid", "office", "work from home", "wfh", "onsite", "in-person",
		"commute", "travel", "relocate", "move", "city", "state", "country", "timezone",
		"flexible", "anywhere", "local", "nearby", "distance",
	}
	return containsAnyPattern(content, patterns)
}

func checkSearchStrategyPatterns(content string) bool {
	patterns := []string{
		"looking for", "searching", "interested in", "want to", "hoping", "ideal", "perfect",
		"title", "role", "position", "job", "opportunity", "skills", "technology", "language",
		"framework", "industry", "domain", "field", "focus", "specialize", "expertise",
	}
	return containsAnyPattern(content, patterns)
}

// containsAnyPattern checks if the content contains any of the given patterns
func containsAnyPattern(content string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	return false
}
