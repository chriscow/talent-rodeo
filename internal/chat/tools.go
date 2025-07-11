package chat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/transcript"
	"time"

	"github.com/sashabaranov/go-openai"
)

// Chat-specific tool implementations using the generic LLM tool system

// CompleteChatTool handles chat completion
type CompleteChatTool struct {
	internal.BaseLLMTool
}

func NewCompleteChatTool(logger *slog.Logger) internal.LLMTool {
	return &CompleteChatTool{
		BaseLLMTool: internal.NewBaseLLMTool(logger),
	}
}

func (t *CompleteChatTool) GetDefinition() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "complete_chat",
			Description: "Call this when the chat conversation is complete and you have gathered sufficient information about the user's career preferences. This will transition the user to the next step in their onboarding process.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"summary": map[string]any{
						"type":        "string",
						"description": "A brief summary of what was learned about the user's career preferences during this chat",
					},
					"confidence_level": map[string]any{
						"type":        "string",
						"enum":        []string{"low", "medium", "high"},
						"description": "Your confidence level in the information gathered (low/medium/high)",
					},
					"next_step_recommendation": map[string]any{
						"type":        "string",
						"description": "What you recommend as the next step for this user (e.g., 'schedule_call', 'review_profile', 'job_search')",
					},
				},
				"required": []string{"summary", "confidence_level", "next_step_recommendation"},
			},
		},
	}
}

func (t *CompleteChatTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	// Cast to ChatSession to get chat-specific data
	chatSession, ok := session.(*ChatSession)
	if !ok {
		return &internal.LLMToolResponse{
			Name:    "complete_chat",
			Content: "Invalid session type for chat tool",
			Success: false,
		}, fmt.Errorf("expected ChatSession, got %T", session)
	}

	session.GetLogger().Debug("Chat completion requested", "user_id", chatSession.User.ID, "appointment_id", chatSession.Appointment.ID, "args", internal.RedactSensitiveInfo(args))

	// Extract and validate arguments using the new base tool methods
	summary, err := t.ExtractString(args, "summary", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "complete_chat",
			Content: "Invalid summary parameter: " + err.Error(),
			Success: false,
		}, err
	}

	confidenceLevel, err := t.ExtractString(args, "confidence_level", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "complete_chat",
			Content: "Invalid confidence_level parameter: " + err.Error(),
			Success: false,
		}, err
	}

	nextStepRecommendation, err := t.ExtractString(args, "next_step_recommendation", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "complete_chat",
			Content: "Invalid next_step_recommendation parameter: " + err.Error(),
			Success: false,
		}, err
	}

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, chatSession); err != nil {
		return &internal.LLMToolResponse{
			Name:    "complete_chat",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	// Increment completion attempt counter
	internal.IncrementCompletionAttempt(chatSession.Appointment.ConversationChecklist)

	// Validate checklist completion
	canComplete, issues, err := internal.ValidateChecklistCompletion(chatSession.Appointment.ConversationChecklist)
	if err != nil {
		session.GetLogger().Error("Failed to validate checklist completion", "error", err)
		return &internal.LLMToolResponse{
			Name:    "complete_chat",
			Content: "Failed to validate completion requirements",
			Success: false,
		}, err
	}

	if !canComplete {
		session.GetLogger().Info("Chat completion denied due to incomplete checklist",
			"user_id", chatSession.User.ID,
			"issues", issues,
			"completion_attempts", chatSession.Appointment.ConversationChecklist.CompletionAttempts,
		)

		// Save the updated appointment with incremented attempt counter
		if err := chatSession.SaveAppointment(ctx); err != nil {
			session.GetLogger().Error("Failed to save appointment with completion attempt", "error", err)
		}

		return &internal.LLMToolResponse{
			Name:    "complete_chat",
			Content: fmt.Sprintf("Cannot complete chat yet. Please address these issues first: %s", strings.Join(issues, ", ")),
			Success: false,
		}, nil
	}

	// Mark checklist as completed
	chatSession.Appointment.ConversationChecklist.OverallStatus = internal.ChecklistStatusCompleted
	chatSession.Appointment.ConversationChecklist.LastUpdated = time.Now()

	// Only process completion for successful calls
	// This actually just starts a goroutine to process the appointment and
	// doesn't return an error but we check anyway in case that changes
	// in the future.
	deps := chatSession.GetServiceDeps()
	if deps == nil {
		return &internal.LLMToolResponse{
			Name:    "complete_chat",
			Content: "Service dependencies not available",
			Success: false,
		}, fmt.Errorf("service dependencies not available")
	}

	go transcript.ProcessCompletedCall(deps, chatSession.User, chatSession.Appointment)

	// Create response with UI actions using helper functions
	response := &internal.LLMToolResponse{
		Name:    "complete_chat",
		Content: "Chat session completed successfully. Preparing next steps...",
		Success: true,
		UIActions: []internal.LLMToolUIAction{
			internal.CreateMessageAction(
				"Great! I've learned about your career preferences. Let me prepare your next steps...",
				"success",
				1000,
			),
			internal.CreateRedirectAction("/dashboard", 3000), // this will show the "dashboard-waiting-call.html" page
		},
	}

	session.GetLogger().Info("Chat completed successfully",
		"user_id", chatSession.User.ID,
		"summary", summary,
		"confidence", confidenceLevel,
		"next_step", nextStepRecommendation,
		"completion_attempts", chatSession.Appointment.ConversationChecklist.CompletionAttempts,
	)

	return response, nil
}

// ensureChecklistExists ensures the appointment has a checklist and saves if needed
func ensureChecklistExists(ctx context.Context, session *ChatSession) error {
	if session.Appointment.ConversationChecklist == nil {
		session.Appointment.ConversationChecklist = internal.InitializeConversationChecklist()
		// We'll need to add a method to save the appointment through the service
		// For now, we'll assume this is handled elsewhere
	}
	return nil
}

// GetChecklistStatusTool retrieves current checklist status
type GetChecklistStatusTool struct {
	internal.BaseLLMTool
}

func NewGetChecklistStatusTool(logger *slog.Logger) internal.LLMTool {
	return &GetChecklistStatusTool{
		BaseLLMTool: internal.NewBaseLLMTool(logger),
	}
}

func (t *GetChecklistStatusTool) GetDefinition() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "get_checklist_status",
			Description: "Get the current status of the conversation checklist to see what topics have been covered and what information is still needed.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	}
}

func (t *GetChecklistStatusTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	chatSession, ok := session.(*ChatSession)
	if !ok {
		return &internal.LLMToolResponse{
			Name:    "get_checklist_status",
			Content: "Invalid session type for chat tool",
			Success: false,
		}, fmt.Errorf("expected ChatSession, got %T", session)
	}

	session.GetLogger().Info("Getting checklist status", "user_id", chatSession.User.ID, "appointment_id", chatSession.Appointment.ID)

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, chatSession); err != nil {
		return &internal.LLMToolResponse{
			Name:    "get_checklist_status",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	summary := internal.GetChecklistSummary(chatSession.Appointment.ConversationChecklist)

	// Create a readable summary for the LLM
	contentParts := []string{
		fmt.Sprintf("Checklist Status: %s", summary["overall_status"]),
		fmt.Sprintf("Completion Attempts: %d/%d", summary["completion_attempts"], internal.MaxCompletionAttempts),
		fmt.Sprintf("Message Count: %d/%d", summary["message_count"], internal.MaxConversationLength),
		fmt.Sprintf("Progress: %d/%d completed (%d required)", summary["completed_items"], summary["total_items"], summary["required_items"]),
		fmt.Sprintf("High Quality Items: %d", summary["high_quality_items"]),
		fmt.Sprintf("Can Complete: %v", summary["can_complete"]),
	}

	if issues, ok := summary["issues"].([]string); ok && len(issues) > 0 {
		contentParts = append(contentParts, fmt.Sprintf("Issues: %s", strings.Join(issues, ", ")))
	}

	return &internal.LLMToolResponse{
		Name:    "get_checklist_status",
		Content: strings.Join(contentParts, "\n"),
		Success: true,
	}, nil
}

// MarkTopicProgressTool updates progress on conversation topics
type MarkTopicProgressTool struct {
	internal.BaseLLMTool
}

func NewMarkTopicProgressTool(logger *slog.Logger) internal.LLMTool {
	return &MarkTopicProgressTool{
		BaseLLMTool: internal.NewBaseLLMTool(logger),
	}
}

func (t *MarkTopicProgressTool) GetDefinition() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "mark_topic_progress",
			Description: "Mark progress on a specific conversation topic and record the quality of information gathered.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"topic_id": map[string]any{
						"type":        "string",
						"enum":        []string{"introduction", "warm_up", "deal_breakers", "search_strategy", "work_environment", "location_preferences", "validation", "final_confirmation"},
						"description": "The ID of the topic to update",
					},
					"status": map[string]any{
						"type":        "string",
						"enum":        []string{"pending", "in_progress", "completed"},
						"description": "Current status of the topic",
					},
					"quality": map[string]any{
						"type":        "string",
						"enum":        []string{"low", "medium", "high"},
						"description": "Quality level of the information gathered",
					},
					"extracted_info": map[string]any{
						"type":        "object",
						"description": "Key information extracted from the conversation about this topic",
					},
					"notes": map[string]any{
						"type":        "string",
						"description": "Additional notes about the information quality or any concerns",
					},
				},
				"required": []string{"topic_id", "status", "quality"},
			},
		},
	}
}

func (t *MarkTopicProgressTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	chatSession, ok := session.(*ChatSession)
	if !ok {
		return &internal.LLMToolResponse{
			Name:    "mark_topic_progress",
			Content: "Invalid session type for chat tool",
			Success: false,
		}, fmt.Errorf("expected ChatSession, got %T", session)
	}

	topicID, err := t.ExtractString(args, "topic_id", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "mark_topic_progress",
			Content: "Invalid topic_id parameter: " + err.Error(),
			Success: false,
		}, err
	}

	status, err := t.ExtractString(args, "status", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "mark_topic_progress",
			Content: "Invalid status parameter: " + err.Error(),
			Success: false,
		}, err
	}

	quality, err := t.ExtractString(args, "quality", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "mark_topic_progress",
			Content: "Invalid quality parameter: " + err.Error(),
			Success: false,
		}, err
	}

	notes, _ := t.ExtractString(args, "notes", false) // Optional parameter

	session.GetLogger().Debug("Marking topic progress", "user_id", chatSession.User.ID, "topic_id", topicID, "status", status, "quality", quality)

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, chatSession); err != nil {
		return &internal.LLMToolResponse{
			Name:    "mark_topic_progress",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	// Extract info from args
	extractedInfo, _ := t.ExtractMap(args, "extracted_info", false)

	// Update the checklist item
	err = internal.UpdateChecklistItem(chatSession.Appointment.ConversationChecklist, topicID, status, quality, extractedInfo, notes)
	if err != nil {
		session.GetLogger().Error("Failed to update checklist item", "error", err, "topic_id", topicID)
		return &internal.LLMToolResponse{
			Name:    "mark_topic_progress",
			Content: fmt.Sprintf("Failed to update topic %s: %s", topicID, err.Error()),
			Success: false,
		}, err
	}

	// Save the updated appointment through service layer
	if err := chatSession.SaveAppointment(ctx); err != nil {
		session.GetLogger().Error("Failed to save appointment with updated checklist", "error", err)
		return &internal.LLMToolResponse{
			Name:    "mark_topic_progress",
			Content: "Failed to save progress",
			Success: false,
		}, err
	}

	return &internal.LLMToolResponse{
		Name:    "mark_topic_progress",
		Content: fmt.Sprintf("Successfully updated topic '%s' to status '%s' with quality '%s'", topicID, status, quality),
		Success: true,
	}, nil
}

// Placeholder implementations for remaining tools - these will be fully implemented later
// For now, they return "not implemented" to maintain compatibility

type UpdateUserPreferencesTool struct{ internal.BaseLLMTool }

func NewUpdateUserPreferencesTool(logger *slog.Logger) internal.LLMTool {
	return &UpdateUserPreferencesTool{BaseLLMTool: internal.NewBaseLLMTool(logger)}
}

func (t *UpdateUserPreferencesTool) GetDefinition() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "update_user_preferences",
			Description: "Update the user's career preferences based on information gathered during the chat conversation.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"job_titles": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Job titles the user is interested in",
					},
					"industries": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Industries the user wants to work in",
					},
					"work_location_preference": map[string]any{
						"type":        "string",
						"enum":        []string{"remote", "hybrid", "onsite", "flexible"},
						"description": "User's work location preference",
					},
					"salary_range": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"min": map[string]any{"type": "number"},
							"max": map[string]any{"type": "number"},
						},
						"description": "Desired salary range",
					},
					"experience_level": map[string]any{
						"type":        "string",
						"enum":        []string{"entry", "mid", "senior", "executive"},
						"description": "User's experience level",
					},
				},
				"required": []string{},
			},
		},
	}
}

func (t *UpdateUserPreferencesTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	chatSession, ok := session.(*ChatSession)
	if !ok {
		return &internal.LLMToolResponse{
			Name:    "update_user_preferences",
			Content: "Invalid session type for chat tool",
			Success: false,
		}, fmt.Errorf("expected ChatSession, got %T", session)
	}

	session.GetLogger().Debug("Updating user preferences", "user_id", chatSession.User.ID, "args", internal.RedactSensitiveInfo(args))

	// For MVP, we'll just log the preferences and return success
	// In the future, this would update the user's profile in the database with the new preferences

	if jobTitlesArray, err := t.ExtractArray(args, "job_titles", false); err == nil && len(jobTitlesArray) > 0 {
		// Convert to string array
		jobTitles := make([]string, 0, len(jobTitlesArray))
		for _, title := range jobTitlesArray {
			if str, ok := title.(string); ok {
				jobTitles = append(jobTitles, str)
			}
		}
		if len(jobTitles) > 0 {
			session.GetLogger().Info("Job titles updated", "user_id", chatSession.User.ID, "job_titles", jobTitles)
		}
	}

	if industriesArray, err := t.ExtractArray(args, "industries", false); err == nil && len(industriesArray) > 0 {
		// Convert to string array
		industries := make([]string, 0, len(industriesArray))
		for _, industry := range industriesArray {
			if str, ok := industry.(string); ok {
				industries = append(industries, str)
			}
		}
		if len(industries) > 0 {
			session.GetLogger().Info("Industries updated", "user_id", chatSession.User.ID, "industries", industries)
		}
	}

	if workLocation, err := t.ExtractString(args, "work_location_preference", false); err == nil && workLocation != "" {
		session.GetLogger().Info("Work location preference updated", "user_id", chatSession.User.ID, "work_location", workLocation)
	}

	if salaryRange, err := t.ExtractMap(args, "salary_range", false); err == nil && len(salaryRange) > 0 {
		session.GetLogger().Info("Salary range updated", "user_id", chatSession.User.ID, "salary_range", salaryRange)
	}

	if experienceLevel, err := t.ExtractString(args, "experience_level", false); err == nil && experienceLevel != "" {
		session.GetLogger().Info("Experience level updated", "user_id", chatSession.User.ID, "experience_level", experienceLevel)
	}

	return &internal.LLMToolResponse{
		Name:    "update_user_preferences",
		Content: "User preferences updated successfully",
		Success: true,
	}, nil
}

type ScheduleFollowupTool struct{ internal.BaseLLMTool }

func NewScheduleFollowupTool(logger *slog.Logger) internal.LLMTool {
	return &ScheduleFollowupTool{BaseLLMTool: internal.NewBaseLLMTool(logger)}
}

func (t *ScheduleFollowupTool) GetDefinition() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "schedule_followup",
			Description: "Schedule a follow-up call or action based on the user's needs identified during the chat.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"followup_type": map[string]any{
						"type":        "string",
						"enum":        []string{"recruiter_call", "job_review", "resume_review", "interview_prep"},
						"description": "Type of follow-up needed",
					},
					"priority": map[string]any{
						"type":        "string",
						"enum":        []string{"low", "medium", "high", "urgent"},
						"description": "Priority level for the follow-up",
					},
					"notes": map[string]any{
						"type":        "string",
						"description": "Additional notes about what should be covered in the follow-up",
					},
					"suggested_timeframe": map[string]any{
						"type":        "string",
						"description": "When this follow-up should ideally happen (e.g., 'within 24 hours', 'this week', 'next week')",
					},
				},
				"required": []string{"followup_type", "priority"},
			},
		},
	}
}

func (t *ScheduleFollowupTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	chatSession, ok := session.(*ChatSession)
	if !ok {
		return &internal.LLMToolResponse{
			Name:    "schedule_followup",
			Content: "Invalid session type for chat tool",
			Success: false,
		}, fmt.Errorf("expected ChatSession, got %T", session)
	}

	followupType, err := t.ExtractString(args, "followup_type", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "schedule_followup",
			Content: "Invalid followup_type parameter: " + err.Error(),
			Success: false,
		}, err
	}

	priority, err := t.ExtractString(args, "priority", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "schedule_followup",
			Content: "Invalid priority parameter: " + err.Error(),
			Success: false,
		}, err
	}

	notes, _ := t.ExtractString(args, "notes", false)
	suggestedTimeframe, _ := t.ExtractString(args, "suggested_timeframe", false)

	session.GetLogger().Debug("Scheduling follow-up", "user_id", chatSession.User.ID, "args", internal.RedactSensitiveInfo(args))

	// For MVP, we'll just log the follow-up request
	// In the future, this could:
	// 1. Create a new appointment record
	// 2. Send notifications to relevant team members
	// 3. Add tasks to a task management system

	session.GetLogger().Info("Follow-up scheduled",
		"user_id", chatSession.User.ID,
		"type", followupType,
		"priority", priority,
		"timeframe", suggestedTimeframe,
		"notes", notes,
	)

	return &internal.LLMToolResponse{
		Name:    "schedule_followup",
		Content: "Follow-up scheduled successfully",
		Success: true,
		UIActions: []internal.LLMToolUIAction{
			internal.CreateMessageAction(
				"I've scheduled a follow-up for you. You'll receive more details soon!",
				"info",
				500,
			),
		},
	}, nil
}

type CheckCompletionReadinessTool struct{ internal.BaseLLMTool }

func NewCheckCompletionReadinessTool(logger *slog.Logger) internal.LLMTool {
	return &CheckCompletionReadinessTool{BaseLLMTool: internal.NewBaseLLMTool(logger)}
}

func (t *CheckCompletionReadinessTool) GetDefinition() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "check_completion_readiness",
			Description: "Check if the conversation has gathered sufficient information to complete the chat session.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	}
}

func (t *CheckCompletionReadinessTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	chatSession, ok := session.(*ChatSession)
	if !ok {
		return &internal.LLMToolResponse{
			Name:    "check_completion_readiness",
			Content: "Invalid session type for chat tool",
			Success: false,
		}, fmt.Errorf("expected ChatSession, got %T", session)
	}

	session.GetLogger().Debug("Checking completion readiness", "user_id", chatSession.User.ID, "appointment_id", chatSession.Appointment.ID)

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, chatSession); err != nil {
		return &internal.LLMToolResponse{
			Name:    "check_completion_readiness",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	// Validate completion readiness
	canComplete, issues, err := internal.ValidateChecklistCompletion(chatSession.Appointment.ConversationChecklist)
	if err != nil {
		session.GetLogger().Error("Failed to validate checklist completion", "error", err)
		return &internal.LLMToolResponse{
			Name:    "check_completion_readiness",
			Content: "Failed to validate completion readiness",
			Success: false,
		}, err
	}

	var responseContent string
	if canComplete {
		responseContent = "✅ Conversation is ready for completion. All required information has been gathered with sufficient quality."
	} else {
		responseContent = fmt.Sprintf("❌ Conversation is not ready for completion. Issues: %s", strings.Join(issues, ", "))
	}

	return &internal.LLMToolResponse{
		Name:    "check_completion_readiness",
		Content: responseContent,
		Success: canComplete,
	}, nil
}

type OverrideCompletionTool struct{ internal.BaseLLMTool }

func NewOverrideCompletionTool(logger *slog.Logger) internal.LLMTool {
	return &OverrideCompletionTool{BaseLLMTool: internal.NewBaseLLMTool(logger)}
}

func (t *OverrideCompletionTool) GetDefinition() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "override_completion",
			Description: "Override completion requirements in emergency situations or when user needs to end the conversation.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"reason": map[string]any{
						"type":        "string",
						"description": "Reason for overriding completion requirements",
					},
					"summary": map[string]any{
						"type":        "string",
						"description": "Summary of information gathered despite incomplete checklist",
					},
				},
				"required": []string{"reason", "summary"},
			},
		},
	}
}

func (t *OverrideCompletionTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	chatSession, ok := session.(*ChatSession)
	if !ok {
		return &internal.LLMToolResponse{
			Name:    "override_completion",
			Content: "Invalid session type for chat tool",
			Success: false,
		}, fmt.Errorf("expected ChatSession, got %T", session)
	}

	reason, err := t.ExtractString(args, "reason", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "override_completion",
			Content: "Invalid reason parameter: " + err.Error(),
			Success: false,
		}, err
	}

	summary, err := t.ExtractString(args, "summary", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "override_completion",
			Content: "Invalid summary parameter: " + err.Error(),
			Success: false,
		}, err
	}

	session.GetLogger().Debug("Chat completion override requested", "user_id", chatSession.User.ID, "appointment_id", chatSession.Appointment.ID, "reason", reason)

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, chatSession); err != nil {
		return &internal.LLMToolResponse{
			Name:    "override_completion",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	// Mark as completed with override status
	chatSession.Appointment.ConversationChecklist.OverallStatus = internal.ChecklistStatusCompleted
	chatSession.Appointment.ConversationChecklist.LastUpdated = time.Now()

	// Process appointment completion by marking it as processing
	deps := chatSession.GetServiceDeps()
	if deps == nil {
		return &internal.LLMToolResponse{
			Name:    "override_completion",
			Content: "Service dependencies not available",
			Success: false,
		}, fmt.Errorf("service dependencies not available")
	}

	if err := internal.ProcessAppointmentStarted(ctx, deps.FS, chatSession.User, chatSession.Appointment); err != nil {
		session.GetLogger().Error("Failed to process appointment completion", "error", err)
		return &internal.LLMToolResponse{
			Name:    "override_completion",
			Content: "Failed to complete chat session",
			Success: false,
		}, err
	}

	// Log the override for audit purposes
	session.GetLogger().Info("Chat completion override applied", "user_id", chatSession.User.ID, "appointment_id", chatSession.Appointment.ID, "reason", reason, "summary", summary)

	return &internal.LLMToolResponse{
		Name:    "override_completion",
		Content: "Chat session completed with override. Thank you for your time!",
		Success: true,
		UIActions: []internal.LLMToolUIAction{
			internal.CreateMessageAction(
				"Chat session completed. You'll be redirected to the next step shortly.",
				"success",
				1000,
			),
			internal.CreateRedirectAction("/onboarding", 3000),
		},
	}, nil
}

type ValidateInformationQualityTool struct{ internal.BaseLLMTool }

func NewValidateInformationQualityTool(logger *slog.Logger) internal.LLMTool {
	return &ValidateInformationQualityTool{BaseLLMTool: internal.NewBaseLLMTool(logger)}
}

func (t *ValidateInformationQualityTool) GetDefinition() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "validate_information_quality",
			Description: "Validate the quality and completeness of information gathered for job matching purposes.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"topic_id": map[string]any{
						"type":        "string",
						"enum":        []string{"introduction", "warm_up", "deal_breakers", "search_strategy", "work_environment", "location_preferences", "validation", "final_confirmation"},
						"description": "The topic to validate",
					},
					"validation_notes": map[string]any{
						"type":        "string",
						"description": "Notes about the validation results and any concerns",
					},
				},
				"required": []string{"topic_id", "validation_notes"},
			},
		},
	}
}

func (t *ValidateInformationQualityTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	chatSession, ok := session.(*ChatSession)
	if !ok {
		return &internal.LLMToolResponse{
			Name:    "validate_information_quality",
			Content: "Invalid session type for chat tool",
			Success: false,
		}, fmt.Errorf("expected ChatSession, got %T", session)
	}

	topicID, err := t.ExtractString(args, "topic_id", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "validate_information_quality",
			Content: "Invalid topic_id parameter: " + err.Error(),
			Success: false,
		}, err
	}

	validationNotes, err := t.ExtractString(args, "validation_notes", true)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "validate_information_quality",
			Content: "Invalid validation_notes parameter: " + err.Error(),
			Success: false,
		}, err
	}

	session.GetLogger().Debug("Validating information quality", "user_id", chatSession.User.ID, "topic_id", topicID)

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, chatSession); err != nil {
		return &internal.LLMToolResponse{
			Name:    "validate_information_quality",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	// Get the checklist item
	item, err := internal.GetChecklistItem(chatSession.Appointment.ConversationChecklist, topicID)
	if err != nil {
		return &internal.LLMToolResponse{
			Name:    "validate_information_quality",
			Content: fmt.Sprintf("Topic %s not found", topicID),
			Success: false,
		}, err
	}

	// Update validation notes
	item.ValidationNotes = validationNotes
	item.LastUpdated = time.Now()
	chatSession.Appointment.ConversationChecklist.LastUpdated = time.Now()

	// Save the updated appointment through service layer
	if err := chatSession.SaveAppointment(ctx); err != nil {
		session.GetLogger().Error("Failed to save appointment with validation notes", "error", err)
		return &internal.LLMToolResponse{
			Name:    "validate_information_quality",
			Content: "Failed to save validation notes",
			Success: false,
		}, err
	}

	return &internal.LLMToolResponse{
		Name:    "validate_information_quality",
		Content: fmt.Sprintf("Validation notes updated for topic '%s'. Current status: %s, Quality: %s", topicID, item.Status, item.Quality),
		Success: true,
	}, nil
}
