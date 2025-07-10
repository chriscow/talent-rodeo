package chat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"talent-rodeo/internal"
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

		// TODO: Save the updated appointment with incremented attempt counter
		// This needs to be handled through the service layer

		return &internal.LLMToolResponse{
			Name:    "complete_chat",
			Content: fmt.Sprintf("Cannot complete chat yet. Please address these issues first: %s", strings.Join(issues, ", ")),
			Success: false,
		}, nil
	}

	// Mark checklist as completed
	chatSession.Appointment.ConversationChecklist.OverallStatus = internal.ChecklistStatusCompleted
	chatSession.Appointment.ConversationChecklist.LastUpdated = time.Now()

	// TODO: Process appointment completion through service layer
	// For now, just log the completion

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
			internal.CreateRedirectAction("/onboarding", 3000),
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

	// TODO: Save the updated appointment through service layer
	// For now, we'll assume this gets saved elsewhere

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
func (t *UpdateUserPreferencesTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *UpdateUserPreferencesTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	return &internal.LLMToolResponse{Name: "update_user_preferences", Content: "Not implemented", Success: false}, nil
}

type ScheduleFollowupTool struct{ internal.BaseLLMTool }

func NewScheduleFollowupTool(logger *slog.Logger) internal.LLMTool {
	return &ScheduleFollowupTool{BaseLLMTool: internal.NewBaseLLMTool(logger)}
}
func (t *ScheduleFollowupTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *ScheduleFollowupTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	return &internal.LLMToolResponse{Name: "schedule_followup", Content: "Not implemented", Success: false}, nil
}

type CheckCompletionReadinessTool struct{ internal.BaseLLMTool }

func NewCheckCompletionReadinessTool(logger *slog.Logger) internal.LLMTool {
	return &CheckCompletionReadinessTool{BaseLLMTool: internal.NewBaseLLMTool(logger)}
}
func (t *CheckCompletionReadinessTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *CheckCompletionReadinessTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	return &internal.LLMToolResponse{Name: "check_completion_readiness", Content: "Not implemented", Success: false}, nil
}

type OverrideCompletionTool struct{ internal.BaseLLMTool }

func NewOverrideCompletionTool(logger *slog.Logger) internal.LLMTool {
	return &OverrideCompletionTool{BaseLLMTool: internal.NewBaseLLMTool(logger)}
}
func (t *OverrideCompletionTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *OverrideCompletionTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	return &internal.LLMToolResponse{Name: "override_completion", Content: "Not implemented", Success: false}, nil
}

type ValidateInformationQualityTool struct{ internal.BaseLLMTool }

func NewValidateInformationQualityTool(logger *slog.Logger) internal.LLMTool {
	return &ValidateInformationQualityTool{BaseLLMTool: internal.NewBaseLLMTool(logger)}
}
func (t *ValidateInformationQualityTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *ValidateInformationQualityTool) Execute(ctx context.Context, args map[string]any, session internal.LLMToolSession) (*internal.LLMToolResponse, error) {
	return &internal.LLMToolResponse{Name: "validate_information_quality", Content: "Not implemented", Success: false}, nil
}
