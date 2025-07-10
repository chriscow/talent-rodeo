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

// ToolRegistryImpl implements the ToolRegistry interface
type ToolRegistryImpl struct {
	tools   map[string]Tool
	service ChatService
	logger  *slog.Logger
}

// NewToolRegistry creates a new tool registry with default tools
func NewToolRegistry(service ChatService, logger *slog.Logger) ToolRegistry {
	registry := &ToolRegistryImpl{
		tools:   make(map[string]Tool),
		service: service,
		logger:  logger,
	}

	// Register default tools
	registry.registerDefaultTools()

	return registry
}

// RegisterTool adds a new tool to the registry
func (r *ToolRegistryImpl) RegisterTool(name string, tool Tool) error {
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if tool == nil {
		return fmt.Errorf("tool cannot be nil")
	}
	r.tools[name] = tool
	return nil
}

// GetAllTools returns all registered tools as OpenAI tool definitions
func (r *ToolRegistryImpl) GetAllTools() []openai.Tool {
	tools := make([]openai.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool.GetDefinition())
	}
	return tools
}

// ExecuteTool executes a tool by name with the given arguments
func (r *ToolRegistryImpl) ExecuteTool(ctx context.Context, toolName string, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	tool, exists := r.tools[toolName]
	if !exists {
		return &ToolResponse{
			Name:    toolName,
			Content: "Tool not found",
			Success: false,
		}, fmt.Errorf("tool '%s' not found", toolName)
	}

	return tool.Execute(ctx, args, session)
}

// registerDefaultTools registers the built-in chat tools
func (r *ToolRegistryImpl) registerDefaultTools() {
	// Register all the default tools
	r.RegisterTool("complete_chat", NewCompleteChatTool(r.logger))
	r.RegisterTool("update_user_preferences", NewUpdateUserPreferencesTool(r.logger))
	r.RegisterTool("schedule_followup", NewScheduleFollowupTool(r.logger))
	r.RegisterTool("get_checklist_status", NewGetChecklistStatusTool(r.logger))
	r.RegisterTool("mark_topic_progress", NewMarkTopicProgressTool(r.logger))
	r.RegisterTool("validate_information_quality", NewValidateInformationQualityTool(r.logger))
	r.RegisterTool("check_completion_readiness", NewCheckCompletionReadinessTool(r.logger))
	r.RegisterTool("override_completion", NewOverrideCompletionTool(r.logger))
}

// BaseTool provides common functionality for all tools
type BaseTool struct {
	logger *slog.Logger
}

// Helper function to safely extract string from args with validation
func (t *BaseTool) extractString(args map[string]any, key string, required bool) (string, error) {
	value, exists := args[key]
	if !exists {
		if required {
			return "", fmt.Errorf("required parameter '%s' not found", key)
		}
		return "", nil
	}

	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("parameter '%s' must be a string, got %T", key, value)
	}

	return str, nil
}

// Helper function to safely extract map from args with validation
func (t *BaseTool) extractMap(args map[string]any, key string, required bool) (map[string]any, error) {
	value, exists := args[key]
	if !exists {
		if required {
			return nil, fmt.Errorf("required parameter '%s' not found", key)
		}
		return nil, nil
	}

	mapVal, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parameter '%s' must be an object, got %T", key, value)
	}

	return mapVal, nil
}

// redactSensitiveInfo removes or masks sensitive information from tool arguments for logging
func redactSensitiveInfo(args map[string]any) map[string]any {
	redacted := make(map[string]any)

	for key, value := range args {
		switch strings.ToLower(key) {
		case "salary_range", "salary", "compensation", "pay":
			redacted[key] = "[REDACTED]"
		case "location", "address", "city", "state", "zip", "zipcode":
			redacted[key] = "[REDACTED]"
		case "phone", "phone_number", "email", "email_address":
			redacted[key] = "[REDACTED]"
		case "name", "first_name", "last_name", "full_name":
			redacted[key] = "[REDACTED]"
		case "ssn", "social_security_number", "tax_id":
			redacted[key] = "[REDACTED]"
		case "notes", "summary", "extracted_info":
			// For notes and summaries, truncate if too long
			if str, ok := value.(string); ok && len(str) > 100 {
				redacted[key] = str[:100] + "...[TRUNCATED]"
			} else {
				redacted[key] = value
			}
		default:
			// Keep other values as-is
			redacted[key] = value
		}
	}

	return redacted
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

// Tool implementations - we'll create separate files for these if they get large

// CompleteChatTool handles chat completion
type CompleteChatTool struct {
	BaseTool
}

func NewCompleteChatTool(logger *slog.Logger) Tool {
	return &CompleteChatTool{
		BaseTool: BaseTool{logger: logger},
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

func (t *CompleteChatTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	t.logger.Debug("Chat completion requested", "user_id", session.User.ID, "appointment_id", session.Appointment.ID, "args", redactSensitiveInfo(args))

	// Extract and validate arguments
	summary, err := t.extractString(args, "summary", true)
	if err != nil {
		return &ToolResponse{
			Name:    "complete_chat",
			Content: "Invalid summary parameter: " + err.Error(),
			Success: false,
		}, err
	}

	confidenceLevel, err := t.extractString(args, "confidence_level", true)
	if err != nil {
		return &ToolResponse{
			Name:    "complete_chat",
			Content: "Invalid confidence_level parameter: " + err.Error(),
			Success: false,
		}, err
	}

	nextStepRecommendation, err := t.extractString(args, "next_step_recommendation", true)
	if err != nil {
		return &ToolResponse{
			Name:    "complete_chat",
			Content: "Invalid next_step_recommendation parameter: " + err.Error(),
			Success: false,
		}, err
	}

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, session); err != nil {
		return &ToolResponse{
			Name:    "complete_chat",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	// Increment completion attempt counter
	internal.IncrementCompletionAttempt(session.Appointment.ConversationChecklist)

	// Validate checklist completion
	canComplete, issues, err := internal.ValidateChecklistCompletion(session.Appointment.ConversationChecklist)
	if err != nil {
		t.logger.Error("Failed to validate checklist completion", "error", err)
		return &ToolResponse{
			Name:    "complete_chat",
			Content: "Failed to validate completion requirements",
			Success: false,
		}, err
	}

	if !canComplete {
		t.logger.Info("Chat completion denied due to incomplete checklist",
			"user_id", session.User.ID,
			"issues", issues,
			"completion_attempts", session.Appointment.ConversationChecklist.CompletionAttempts,
		)

		// TODO: Save the updated appointment with incremented attempt counter
		// This needs to be handled through the service layer

		return &ToolResponse{
			Name:    "complete_chat",
			Content: fmt.Sprintf("Cannot complete chat yet. Please address these issues first: %s", strings.Join(issues, ", ")),
			Success: false,
		}, nil
	}

	// Mark checklist as completed
	session.Appointment.ConversationChecklist.OverallStatus = internal.ChecklistStatusCompleted
	session.Appointment.ConversationChecklist.LastUpdated = time.Now()

	// TODO: Process appointment completion through service layer
	// For now, just log the completion

	// Create response with UI actions
	response := &ToolResponse{
		Name:    "complete_chat",
		Content: "Chat session completed successfully. Preparing next steps...",
		Success: true,
		UIActions: []UIAction{
			{
				Type: "show_message",
				Data: map[string]interface{}{
					"message": "Great! I've learned about your career preferences. Let me prepare your next steps...",
					"type":    "success",
				},
				Delay: 1000,
			},
			{
				Type: "redirect",
				Data: map[string]interface{}{
					"url": "/onboarding",
				},
				Delay: 3000,
			},
		},
	}

	t.logger.Info("Chat completed successfully",
		"user_id", session.User.ID,
		"summary", summary,
		"confidence", confidenceLevel,
		"next_step", nextStepRecommendation,
		"completion_attempts", session.Appointment.ConversationChecklist.CompletionAttempts,
	)

	return response, nil
}

// Placeholder implementations for other tools - we'll implement these as we continue
// For now, they return "not implemented" to avoid compilation errors

type UpdateUserPreferencesTool struct{ BaseTool }

func NewUpdateUserPreferencesTool(logger *slog.Logger) Tool {
	return &UpdateUserPreferencesTool{BaseTool{logger}}
}
func (t *UpdateUserPreferencesTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *UpdateUserPreferencesTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	return &ToolResponse{Name: "update_user_preferences", Content: "Not implemented", Success: false}, nil
}

type ScheduleFollowupTool struct{ BaseTool }

func NewScheduleFollowupTool(logger *slog.Logger) Tool {
	return &ScheduleFollowupTool{BaseTool{logger}}
}
func (t *ScheduleFollowupTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *ScheduleFollowupTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	return &ToolResponse{Name: "schedule_followup", Content: "Not implemented", Success: false}, nil
}

// GetChecklistStatusTool retrieves current checklist status
type GetChecklistStatusTool struct{ BaseTool }

func NewGetChecklistStatusTool(logger *slog.Logger) Tool {
	return &GetChecklistStatusTool{BaseTool{logger}}
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

func (t *GetChecklistStatusTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	t.logger.Info("Getting checklist status", "user_id", session.User.ID, "appointment_id", session.Appointment.ID)

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, session); err != nil {
		return &ToolResponse{
			Name:    "get_checklist_status",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	summary := internal.GetChecklistSummary(session.Appointment.ConversationChecklist)

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

	return &ToolResponse{
		Name:    "get_checklist_status",
		Content: strings.Join(contentParts, "\n"),
		Success: true,
	}, nil
}

// MarkTopicProgressTool updates progress on conversation topics
type MarkTopicProgressTool struct{ BaseTool }

func NewMarkTopicProgressTool(logger *slog.Logger) Tool {
	return &MarkTopicProgressTool{BaseTool{logger}}
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

func (t *MarkTopicProgressTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	topicID, err := t.extractString(args, "topic_id", true)
	if err != nil {
		return &ToolResponse{
			Name:    "mark_topic_progress",
			Content: "Invalid topic_id parameter: " + err.Error(),
			Success: false,
		}, err
	}

	status, err := t.extractString(args, "status", true)
	if err != nil {
		return &ToolResponse{
			Name:    "mark_topic_progress",
			Content: "Invalid status parameter: " + err.Error(),
			Success: false,
		}, err
	}

	quality, err := t.extractString(args, "quality", true)
	if err != nil {
		return &ToolResponse{
			Name:    "mark_topic_progress",
			Content: "Invalid quality parameter: " + err.Error(),
			Success: false,
		}, err
	}

	notes, _ := t.extractString(args, "notes", false) // Optional parameter

	t.logger.Debug("Marking topic progress", "user_id", session.User.ID, "topic_id", topicID, "status", status, "quality", quality)

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, session); err != nil {
		return &ToolResponse{
			Name:    "mark_topic_progress",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	// Extract info from args
	extractedInfo, _ := t.extractMap(args, "extracted_info", false)

	// Update the checklist item
	err = internal.UpdateChecklistItem(session.Appointment.ConversationChecklist, topicID, status, quality, extractedInfo, notes)
	if err != nil {
		t.logger.Error("Failed to update checklist item", "error", err, "topic_id", topicID)
		return &ToolResponse{
			Name:    "mark_topic_progress",
			Content: fmt.Sprintf("Failed to update topic %s: %s", topicID, err.Error()),
			Success: false,
		}, err
	}

	// TODO: Save the updated appointment through service layer
	// For now, we'll assume this gets saved elsewhere

	return &ToolResponse{
		Name:    "mark_topic_progress",
		Content: fmt.Sprintf("Successfully updated topic '%s' to status '%s' with quality '%s'", topicID, status, quality),
		Success: true,
	}, nil
}

// CheckCompletionReadinessTool validates if chat is ready for completion
type CheckCompletionReadinessTool struct{ BaseTool }

func NewCheckCompletionReadinessTool(logger *slog.Logger) Tool {
	return &CheckCompletionReadinessTool{BaseTool{logger}}
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

func (t *CheckCompletionReadinessTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	t.logger.Debug("Checking completion readiness", "user_id", session.User.ID, "appointment_id", session.Appointment.ID)

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, session); err != nil {
		return &ToolResponse{
			Name:    "check_completion_readiness",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	// Validate completion readiness
	canComplete, issues, err := internal.ValidateChecklistCompletion(session.Appointment.ConversationChecklist)
	if err != nil {
		t.logger.Error("Failed to validate checklist completion", "error", err)
		return &ToolResponse{
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

	return &ToolResponse{
		Name:    "check_completion_readiness",
		Content: responseContent,
		Success: canComplete,
	}, nil
}

// OverrideCompletionTool handles emergency completion override
type OverrideCompletionTool struct{ BaseTool }

func NewOverrideCompletionTool(logger *slog.Logger) Tool {
	return &OverrideCompletionTool{BaseTool{logger}}
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

func (t *OverrideCompletionTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	reason, err := t.extractString(args, "reason", true)
	if err != nil {
		return &ToolResponse{
			Name:    "override_completion",
			Content: "Invalid reason parameter: " + err.Error(),
			Success: false,
		}, err
	}

	summary, err := t.extractString(args, "summary", true)
	if err != nil {
		return &ToolResponse{
			Name:    "override_completion",
			Content: "Invalid summary parameter: " + err.Error(),
			Success: false,
		}, err
	}

	t.logger.Debug("Chat completion override requested", "user_id", session.User.ID, "appointment_id", session.Appointment.ID, "reason", reason)

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, session); err != nil {
		return &ToolResponse{
			Name:    "override_completion",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	// Mark as completed with override status
	session.Appointment.ConversationChecklist.OverallStatus = internal.ChecklistStatusCompleted
	session.Appointment.ConversationChecklist.LastUpdated = time.Now()

	// TODO: Process appointment completion through service layer

	// Log the override for audit purposes
	t.logger.Info("Chat completion override applied", "user_id", session.User.ID, "appointment_id", session.Appointment.ID, "reason", reason, "summary", summary)

	return &ToolResponse{
		Name:    "override_completion",
		Content: "Chat session completed with override. Thank you for your time!",
		Success: true,
		UIActions: []UIAction{
			{
				Type: "show_message",
				Data: map[string]interface{}{
					"message": "Chat session completed. You'll be redirected to the next step shortly.",
					"type":    "success",
				},
				Delay: 1000,
			},
			{
				Type: "redirect",
				Data: map[string]interface{}{
					"url": "/onboarding",
				},
				Delay: 3000,
			},
		},
	}, nil
}

// ValidateInformationQualityTool validates information quality
type ValidateInformationQualityTool struct{ BaseTool }

func NewValidateInformationQualityTool(logger *slog.Logger) Tool {
	return &ValidateInformationQualityTool{BaseTool{logger}}
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

func (t *ValidateInformationQualityTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	topicID, err := t.extractString(args, "topic_id", true)
	if err != nil {
		return &ToolResponse{
			Name:    "validate_information_quality",
			Content: "Invalid topic_id parameter: " + err.Error(),
			Success: false,
		}, err
	}

	validationNotes, err := t.extractString(args, "validation_notes", true)
	if err != nil {
		return &ToolResponse{
			Name:    "validate_information_quality",
			Content: "Invalid validation_notes parameter: " + err.Error(),
			Success: false,
		}, err
	}

	t.logger.Debug("Validating information quality", "user_id", session.User.ID, "topic_id", topicID)

	// Ensure checklist exists
	if err := ensureChecklistExists(ctx, session); err != nil {
		return &ToolResponse{
			Name:    "validate_information_quality",
			Content: "Failed to initialize checklist",
			Success: false,
		}, err
	}

	// Get the checklist item
	item, err := internal.GetChecklistItem(session.Appointment.ConversationChecklist, topicID)
	if err != nil {
		return &ToolResponse{
			Name:    "validate_information_quality",
			Content: fmt.Sprintf("Topic %s not found", topicID),
			Success: false,
		}, err
	}

	// Update validation notes
	item.ValidationNotes = validationNotes
	item.LastUpdated = time.Now()
	session.Appointment.ConversationChecklist.LastUpdated = time.Now()

	// TODO: Save the updated appointment through service layer

	return &ToolResponse{
		Name:    "validate_information_quality",
		Content: fmt.Sprintf("Validation notes updated for topic '%s'. Current status: %s, Quality: %s", topicID, item.Status, item.Quality),
		Success: true,
	}, nil
}
