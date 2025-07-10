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

type GetChecklistStatusTool struct{ BaseTool }

func NewGetChecklistStatusTool(logger *slog.Logger) Tool {
	return &GetChecklistStatusTool{BaseTool{logger}}
}
func (t *GetChecklistStatusTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *GetChecklistStatusTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	return &ToolResponse{Name: "get_checklist_status", Content: "Not implemented", Success: false}, nil
}

type MarkTopicProgressTool struct{ BaseTool }

func NewMarkTopicProgressTool(logger *slog.Logger) Tool {
	return &MarkTopicProgressTool{BaseTool{logger}}
}
func (t *MarkTopicProgressTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *MarkTopicProgressTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	return &ToolResponse{Name: "mark_topic_progress", Content: "Not implemented", Success: false}, nil
}

type ValidateInformationQualityTool struct{ BaseTool }

func NewValidateInformationQualityTool(logger *slog.Logger) Tool {
	return &ValidateInformationQualityTool{BaseTool{logger}}
}
func (t *ValidateInformationQualityTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *ValidateInformationQualityTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	return &ToolResponse{Name: "validate_information_quality", Content: "Not implemented", Success: false}, nil
}

type CheckCompletionReadinessTool struct{ BaseTool }

func NewCheckCompletionReadinessTool(logger *slog.Logger) Tool {
	return &CheckCompletionReadinessTool{BaseTool{logger}}
}
func (t *CheckCompletionReadinessTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *CheckCompletionReadinessTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	return &ToolResponse{Name: "check_completion_readiness", Content: "Not implemented", Success: false}, nil
}

type OverrideCompletionTool struct{ BaseTool }

func NewOverrideCompletionTool(logger *slog.Logger) Tool {
	return &OverrideCompletionTool{BaseTool{logger}}
}
func (t *OverrideCompletionTool) GetDefinition() openai.Tool { return openai.Tool{} }
func (t *OverrideCompletionTool) Execute(ctx context.Context, args map[string]any, session *ChatSession) (*ToolResponse, error) {
	return &ToolResponse{Name: "override_completion", Content: "Not implemented", Success: false}, nil
}
