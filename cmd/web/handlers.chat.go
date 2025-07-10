package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"talent-rodeo/cmd/web/simulate"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"time"

	"github.com/chriscow/vapi-go"
	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
)

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
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

// WebSocket upgrader with CORS support
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for WebSocket connections (you may want to restrict this in production)
		return true
	},
}

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type      string `json:"type"`
	Content   string `json:"content"`
	Error     string `json:"error,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	// Tool-related fields
	ToolCall     *ToolCallData `json:"tool_call,omitempty"`
	ToolResponse *ToolResponse `json:"tool_response,omitempty"`
}

// ToolCallData represents a tool call made by the LLM
type ToolCallData struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolResponse represents the response from a tool execution
type ToolResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Success bool   `json:"success"`
	// Optional: UI actions to perform on the client
	UIActions []UIAction `json:"ui_actions,omitempty"`
}

// UIAction represents an action for the client to perform
type UIAction struct {
	Type  string         `json:"type"`  // "redirect", "show_message", "update_status", etc.
	Data  map[string]any `json:"data"`  // Action-specific data
	Delay int            `json:"delay"` // Delay in milliseconds before executing
}

// ChatTool represents a tool that can be called by the LLM
type ChatTool struct {
	Definition openai.Tool
	Handler    func(ctx context.Context, app *application, user *internal.User, appointment *internal.Appointment, args map[string]any) (*ToolResponse, error)
}

// ChatToolRegistry holds all available tools for the chat system
type ChatToolRegistry struct {
	tools map[string]ChatTool
}

// NewChatToolRegistry creates a new tool registry with default tools
func NewChatToolRegistry() *ChatToolRegistry {
	registry := &ChatToolRegistry{
		tools: make(map[string]ChatTool),
	}

	// Register default tools
	registry.registerDefaultTools()

	return registry
}

// registerDefaultTools registers the built-in chat tools
func (r *ChatToolRegistry) registerDefaultTools() {
	// Tool 1: Complete Chat - Indicates the chat session is complete and user is ready for next step
	r.RegisterTool("complete_chat", ChatTool{
		Definition: openai.Tool{
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
		},
		Handler: r.handleCompleteChat,
	})

	// Tool 2: Update User Preferences - Updates user's career preferences based on chat
	r.RegisterTool("update_user_preferences", ChatTool{
		Definition: openai.Tool{
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
		},
		Handler: r.handleUpdateUserPreferences,
	})

	// Tool 3: Schedule Follow-up - Schedules a follow-up call or action
	r.RegisterTool("schedule_followup", ChatTool{
		Definition: openai.Tool{
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
		},
		Handler: r.handleScheduleFollowup,
	})

	// Tool 4: Get Checklist Status - Shows current conversation progress
	r.RegisterTool("get_checklist_status", ChatTool{
		Definition: openai.Tool{
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
		},
		Handler: r.handleGetChecklistStatus,
	})

	// Tool 5: Mark Topic Progress - Updates progress on a conversation topic
	r.RegisterTool("mark_topic_progress", ChatTool{
		Definition: openai.Tool{
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
		},
		Handler: r.handleMarkTopicProgress,
	})

	// Tool 6: Validate Information Quality - Assesses the quality of gathered information
	r.RegisterTool("validate_information_quality", ChatTool{
		Definition: openai.Tool{
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
		},
		Handler: r.handleValidateInformationQuality,
	})

	// Tool 7: Check Completion Readiness - Validates if chat is ready for completion
	r.RegisterTool("check_completion_readiness", ChatTool{
		Definition: openai.Tool{
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
		},
		Handler: r.handleCheckCompletionReadiness,
	})

	// Tool 8: Override Completion - Emergency completion with reason logging
	r.RegisterTool("override_completion", ChatTool{
		Definition: openai.Tool{
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
		},
		Handler: r.handleOverrideCompletion,
	})
}

// RegisterTool adds a new tool to the registry
func (r *ChatToolRegistry) RegisterTool(name string, tool ChatTool) {
	r.tools[name] = tool
}

// GetTool retrieves a tool by name
func (r *ChatToolRegistry) GetTool(name string) (ChatTool, bool) {
	tool, exists := r.tools[name]
	return tool, exists
}

// GetAllTools returns all registered tools as OpenAI tool definitions
func (r *ChatToolRegistry) GetAllTools() []openai.Tool {
	tools := make([]openai.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool.Definition)
	}
	return tools
}

// ExecuteTool executes a tool by name with the given arguments
func (r *ChatToolRegistry) ExecuteTool(ctx context.Context, app *application, user *internal.User, appointment *internal.Appointment, toolName string, args map[string]any) (*ToolResponse, error) {
	tool, exists := r.GetTool(toolName)
	if !exists {
		return &ToolResponse{
			Name:    toolName,
			Content: "Tool not found",
			Success: false,
		}, nil
	}

	return tool.Handler(ctx, app, user, appointment, args)
}

// Tool Handler Functions

// handleCompleteChat handles the complete_chat tool call
func (r *ChatToolRegistry) handleCompleteChat(ctx context.Context, app *application, user *internal.User, appointment *internal.Appointment, args map[string]any) (*ToolResponse, error) {
	app.logger.Debug("Chat completion requested", "user_id", user.ID, "appointment_id", appointment.ID, "args", redactSensitiveInfo(args))

	// Extract arguments
	summary, _ := args["summary"].(string)
	confidenceLevel, _ := args["confidence_level"].(string)
	nextStepRecommendation, _ := args["next_step_recommendation"].(string)

	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = internal.InitializeConversationChecklist()
	}

	// Increment completion attempt counter
	internal.IncrementCompletionAttempt(appointment.ConversationChecklist)

	// Validate checklist completion
	canComplete, issues, err := internal.ValidateChecklistCompletion(appointment.ConversationChecklist)
	if err != nil {
		app.logger.Error("Failed to validate checklist completion", "error", err)
		return &ToolResponse{
			Name:    "complete_chat",
			Content: "Failed to validate completion requirements",
			Success: false,
		}, err
	}

	if !canComplete {
		app.logger.Info("Chat completion denied due to incomplete checklist",
			"user_id", user.ID,
			"issues", issues,
			"completion_attempts", appointment.ConversationChecklist.CompletionAttempts,
		)

		// Save the updated appointment with incremented attempt counter
		if err := internal.UpdateAppointment(ctx, app.fs, appointment); err != nil {
			app.logger.Error("Failed to save appointment with completion attempt", "error", err)
		}

		return &ToolResponse{
			Name:    "complete_chat",
			Content: fmt.Sprintf("Cannot complete chat yet. Please address these issues first: %s", strings.Join(issues, ", ")),
			Success: false,
		}, nil
	}

	// Mark checklist as completed
	appointment.ConversationChecklist.OverallStatus = internal.ChecklistStatusCompleted
	appointment.ConversationChecklist.LastUpdated = time.Now()

	// Update appointment status to indicate chat is complete
	if err := internal.ProcessAppointmentStarted(ctx, app.fs, user, appointment); err != nil {
		app.logger.Error("Failed to mark appointment as complete", "error", err)
		return &ToolResponse{
			Name:    "complete_chat",
			Content: "Failed to complete chat session",
			Success: false,
		}, err
	}

	// Create response with UI actions
	response := &ToolResponse{
		Name:    "complete_chat",
		Content: "Chat session completed successfully. Preparing next steps...",
		Success: true,
		UIActions: []UIAction{
			{
				Type: "show_message",
				Data: map[string]any{
					"message": "Great! I've learned about your career preferences. Let me prepare your next steps...",
					"type":    "success",
				},
				Delay: 1000,
			},
			{
				Type: "redirect",
				Data: map[string]any{
					"url": "/onboarding",
				},
				Delay: 3000,
			},
		},
	}

	app.logger.Info("Chat completed successfully",
		"user_id", user.ID,
		"summary", summary,
		"confidence", confidenceLevel,
		"next_step", nextStepRecommendation,
		"completion_attempts", appointment.ConversationChecklist.CompletionAttempts,
	)

	return response, nil
}

// handleUpdateUserPreferences handles the update_user_preferences tool call
func (r *ChatToolRegistry) handleUpdateUserPreferences(ctx context.Context, app *application, user *internal.User, appointment *internal.Appointment, args map[string]any) (*ToolResponse, error) {
	app.logger.Debug("Updating user preferences", "user_id", user.ID, "args", redactSensitiveInfo(args))

	// TODO: Implement actual user preference updates
	// This would involve updating the user's profile in the database with the new preferences

	// For now, we'll log the preferences and return success
	if jobTitles, ok := args["job_titles"].([]any); ok {
		app.logger.Info("Job titles updated", "user_id", user.ID, "job_titles", jobTitles)
	}

	if industries, ok := args["industries"].([]any); ok {
		app.logger.Info("Industries updated", "user_id", user.ID, "industries", industries)
	}

	if workLocation, ok := args["work_location_preference"].(string); ok {
		app.logger.Info("Work location preference updated", "user_id", user.ID, "work_location", workLocation)
	}

	if salaryRange, ok := args["salary_range"].(map[string]any); ok {
		app.logger.Info("Salary range updated", "user_id", user.ID, "salary_range", salaryRange)
	}

	if experienceLevel, ok := args["experience_level"].(string); ok {
		app.logger.Info("Experience level updated", "user_id", user.ID, "experience_level", experienceLevel)
	}

	return &ToolResponse{
		Name:    "update_user_preferences",
		Content: "User preferences updated successfully",
		Success: true,
	}, nil
}

// handleScheduleFollowup handles the schedule_followup tool call
func (r *ChatToolRegistry) handleScheduleFollowup(ctx context.Context, app *application, user *internal.User, appointment *internal.Appointment, args map[string]any) (*ToolResponse, error) {
	app.logger.Debug("Scheduling follow-up", "user_id", user.ID, "args", redactSensitiveInfo(args))

	followupType, _ := args["followup_type"].(string)
	priority, _ := args["priority"].(string)
	notes, _ := args["notes"].(string)
	suggestedTimeframe, _ := args["suggested_timeframe"].(string)

	// TODO: Implement actual follow-up scheduling
	// This could involve:
	// 1. Creating a new appointment record
	// 2. Sending notifications to relevant team members
	// 3. Adding tasks to a task management system

	app.logger.Info("Follow-up scheduled",
		"user_id", user.ID,
		"type", followupType,
		"priority", priority,
		"timeframe", suggestedTimeframe,
		"notes", notes,
	)

	return &ToolResponse{
		Name:    "schedule_followup",
		Content: "Follow-up scheduled successfully",
		Success: true,
		UIActions: []UIAction{
			{
				Type: "show_message",
				Data: map[string]any{
					"message": "I've scheduled a follow-up for you. You'll receive more details soon!",
					"type":    "info",
				},
				Delay: 500,
			},
		},
	}, nil
}

// handleGetChecklistStatus retrieves the current checklist status
func (r *ChatToolRegistry) handleGetChecklistStatus(ctx context.Context, app *application, user *internal.User, appointment *internal.Appointment, args map[string]any) (*ToolResponse, error) {
	app.logger.Info("Getting checklist status", "user_id", user.ID, "appointment_id", appointment.ID)

	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = internal.InitializeConversationChecklist()
		if err := internal.UpdateAppointment(ctx, app.fs, appointment); err != nil {
			app.logger.Error("Failed to initialize checklist", "error", err)
			return &ToolResponse{
				Name:    "get_checklist_status",
				Content: "Failed to initialize checklist",
				Success: false,
			}, err
		}
	}

	summary := internal.GetChecklistSummary(appointment.ConversationChecklist)

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

// handleMarkTopicProgress updates progress on a conversation topic
func (r *ChatToolRegistry) handleMarkTopicProgress(ctx context.Context, app *application, user *internal.User, appointment *internal.Appointment, args map[string]any) (*ToolResponse, error) {
	topicID, _ := args["topic_id"].(string)
	status, _ := args["status"].(string)
	quality, _ := args["quality"].(string)
	notes, _ := args["notes"].(string)

	app.logger.Debug(internal.Green("Marking topic progress"), "name", user.Name, "user_id", user.ID, "topic_id", topicID, "status", status, "quality", quality)

	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = internal.InitializeConversationChecklist()
	}

	// Extract info from args
	var extractedInfo map[string]any
	if info, ok := args["extracted_info"].(map[string]any); ok {
		extractedInfo = info
	}

	// Update the checklist item
	err := internal.UpdateChecklistItem(appointment.ConversationChecklist, topicID, status, quality, extractedInfo, notes)
	if err != nil {
		app.logger.Error("Failed to update checklist item", "error", err, "topic_id", topicID)
		return &ToolResponse{
			Name:    "mark_topic_progress",
			Content: fmt.Sprintf("Failed to update topic %s: %s", topicID, err.Error()),
			Success: false,
		}, err
	}

	// Save the updated appointment
	if err := internal.UpdateAppointment(ctx, app.fs, appointment); err != nil {
		app.logger.Error("Failed to save appointment with updated checklist", "error", err)
		return &ToolResponse{
			Name:    "mark_topic_progress",
			Content: "Failed to save progress",
			Success: false,
		}, err
	}

	return &ToolResponse{
		Name:    "mark_topic_progress",
		Content: fmt.Sprintf("Successfully updated topic '%s' to status '%s' with quality '%s'", topicID, status, quality),
		Success: true,
	}, nil
}

// handleValidateInformationQuality validates the quality of gathered information
func (r *ChatToolRegistry) handleValidateInformationQuality(ctx context.Context, app *application, user *internal.User, appointment *internal.Appointment, args map[string]any) (*ToolResponse, error) {
	topicID, _ := args["topic_id"].(string)
	validationNotes, _ := args["validation_notes"].(string)

	app.logger.Debug(internal.Green("Validating information quality"), "name", user.Name, "user_id", user.ID, "topic_id", topicID)

	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = internal.InitializeConversationChecklist()
	}

	// Get the checklist item
	item, err := internal.GetChecklistItem(appointment.ConversationChecklist, topicID)
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
	appointment.ConversationChecklist.LastUpdated = time.Now()

	// Save the updated appointment
	if err := internal.UpdateAppointment(ctx, app.fs, appointment); err != nil {
		app.logger.Error("Failed to save appointment with validation notes", "error", err)
		return &ToolResponse{
			Name:    "validate_information_quality",
			Content: "Failed to save validation notes",
			Success: false,
		}, err
	}

	return &ToolResponse{
		Name:    "validate_information_quality",
		Content: fmt.Sprintf("Validation notes updated for topic '%s'. Current status: %s, Quality: %s", topicID, item.Status, item.Quality),
		Success: true,
	}, nil
}

// handleCheckCompletionReadiness checks if the conversation is ready for completion
func (r *ChatToolRegistry) handleCheckCompletionReadiness(ctx context.Context, app *application, user *internal.User, appointment *internal.Appointment, args map[string]any) (*ToolResponse, error) {
	app.logger.Debug(internal.Green("Checking completion readiness"), "name", user.Name, "user_id", user.ID, "appointment_id", appointment.ID)

	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = internal.InitializeConversationChecklist()
	}

	// Validate completion readiness
	canComplete, issues, err := internal.ValidateChecklistCompletion(appointment.ConversationChecklist)
	if err != nil {
		app.logger.Error("Failed to validate checklist completion", "error", err)
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

// handleOverrideCompletion handles emergency completion override
func (r *ChatToolRegistry) handleOverrideCompletion(ctx context.Context, app *application, user *internal.User, appointment *internal.Appointment, args map[string]any) (*ToolResponse, error) {
	reason, _ := args["reason"].(string)
	summary, _ := args["summary"].(string)

	app.logger.Debug(internal.Yellow("Chat completion override requested"), "name", user.Name, "user_id", user.ID, "appointment_id", appointment.ID, "reason", reason)

	// Ensure checklist exists
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = internal.InitializeConversationChecklist()
	}

	// Mark as completed with override status
	appointment.ConversationChecklist.OverallStatus = internal.ChecklistStatusCompleted
	appointment.ConversationChecklist.LastUpdated = time.Now()

	// Process appointment completion
	if err := internal.ProcessAppointmentStarted(ctx, app.fs, user, appointment); err != nil {
		app.logger.Error("Failed to process appointment completion", "error", err)
		return &ToolResponse{
			Name:    "override_completion",
			Content: "Failed to complete chat session",
			Success: false,
		}, err
	}

	// Log the override for audit purposes
	app.logger.Debug(internal.Green("Chat completion override applied"), "name", user.Name, "user_id", user.ID, "appointment_id", appointment.ID, "reason", reason, "summary", summary)

	return &ToolResponse{
		Name:    "override_completion",
		Content: "Chat session completed with override. Thank you for your time!",
		Success: true,
		UIActions: []UIAction{
			{
				Type: "show_message",
				Data: map[string]any{
					"message": "Chat session completed. You'll be redirected to the next step shortly.",
					"type":    "success",
				},
				Delay: 1000,
			},
			{
				Type: "redirect",
				Data: map[string]any{
					"url": "/onboarding",
				},
				Delay: 3000,
			},
		},
	}, nil
}

// Global tool registry instance
var chatToolRegistry = NewChatToolRegistry()

// chatStartRequest represents the request to start a new chat session
type chatStartRequest struct {
	CSRFToken string `json:"csrf_token"`
	Timezone  string `json:"timezone"`
}

// chatMessageRequest represents a message sent in the chat
type chatMessageRequest struct {
	Content   string `json:"content"`
	CSRFToken string `json:"csrf_token"`
}

// chatStartResponse represents the response when starting a chat
type chatStartResponse struct {
	AppointmentID string `json:"appointment_id"`
	Status        string `json:"status"`
	FirstMessage  string `json:"first_message"`
}

// startChat handles starting a new chat session (creates a chat appointment)
func (app *application) startChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		response.JSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	var req chatStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.logger.Error("Failed to decode chat start request", "error", err)
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Build assistant overrides to get the first message
	assistantOverrides, err := internal.BuildAssistantOverrides(r.Context(), user, app.fs, app.logger)
	if err != nil {
		app.logger.Error("Failed to build assistant overrides for chat start", "error", err, "user_id", user.ID)
		// Continue with default message
	}

	firstMessage := "Hi! I'm Shirely. I'm excited to learn about your career preferences. What kind of work environment do you prefer?"
	if assistantOverrides != nil && assistantOverrides.FirstMessage != nil {
		// Adapt the phone-oriented message for chat
		firstMessage = *assistantOverrides.FirstMessage
		firstMessage = strings.Replace(firstMessage, "calling back", "coming back to chat", -1)
		firstMessage = strings.Replace(firstMessage, "Am I speaking with", "Am I chatting with", -1)
	}

	// Create a new chat appointment
	appointment := &internal.Appointment{
		Name:          "initial-chat",
		Type:          internal.AppointmentTypeChat,
		ScheduledTime: time.Now(),
		Timezone:      req.Timezone,
		Status:        internal.AppointmentStatusRunning,
		ChatMessages: []internal.ChatMessage{
			{
				Role:      "assistant",
				Content:   firstMessage,
				Timestamp: time.Now(),
			},
		},
		ConversationChecklist: internal.InitializeConversationChecklist(),
	}

	if err := internal.CreateAppointment(r.Context(), app.fs, user.ID, appointment); err != nil {
		app.logger.Error("Failed to create chat appointment", "error", err, "user_id", user.ID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create chat session"})
		return
	}

	// Update user status to indicate chat started
	if err := internal.UpdateUserStatus(r.Context(), app.fs, user, internal.UserStatusCallStarted); err != nil {
		app.logger.Error("Failed to update user status for chat", "error", err, "user_id", user.ID)
		// Continue despite error - chat session is already created
	}

	app.logger.Info("Chat session started", "user_id", user.ID, "appointment_id", appointment.ID)

	response.JSON(w, http.StatusOK, chatStartResponse{
		AppointmentID: appointment.ID,
		Status:        "started",
		FirstMessage:  firstMessage,
	})
}

// sendChatMessage is deprecated - now handled via WebSocket in streamChatResponse
// Keeping for backward compatibility during transition
func (app *application) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		response.JSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	if appointmentID == "" {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "missing appointment_id"})
		return
	}

	var req chatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.logger.Error("Failed to decode chat message request", "error", err)
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Get the appointment
	appointment, err := internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get appointment", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusNotFound, map[string]string{"error": "appointment not found"})
		return
	}

	if appointment.Type != internal.AppointmentTypeChat {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "not a chat appointment"})
		return
	}

	// Add the user's message
	app.logger.Debug("sendChatMessage -> AddChatMessage", "user_id", user.ID, "appointment_id", appointment.ID, "user_message", req.Content)
	if err := internal.AddChatMessage(r.Context(), app.fs, appointment, "user", req.Content); err != nil {
		app.logger.Error("Failed to add user message", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save message"})
		return
	}

	app.logger.Info("Chat message saved", "user_id", user.ID, "appointment_id", appointmentID)

	response.JSON(w, http.StatusOK, map[string]string{
		"status": "message_saved",
	})
}

// streamChatResponse handles WebSocket connections for real-time chat streaming
func (app *application) streamChatResponse(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	if appointmentID == "" {
		http.Error(w, "Missing appointment_id", http.StatusBadRequest)
		return
	}

	// Get the appointment
	appointment, err := internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get appointment for WebSocket", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		http.Error(w, "Appointment not found", http.StatusNotFound)
		return
	}

	app.logger.Debug("streamChatResponse -> appointment", "chatHistory", len(appointment.ChatMessages))

	if appointment.Type != internal.AppointmentTypeChat {
		http.Error(w, "Not a chat appointment", http.StatusBadRequest)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.logger.Error("Failed to upgrade to WebSocket", "error", err)
		return
	}
	defer conn.Close()

	app.logger.Info("webSocket connection established", "user_id", user.ID, "appointment_id", appointmentID)

	// Send connection confirmation
	confirmMsg := WebSocketMessage{
		Type:    "connected",
		Content: "WebSocket connection established",
	}
	if err := conn.WriteJSON(confirmMsg); err != nil {
		app.logger.Error("Failed to send connection confirmation", "error", err)
		return
	}

	// Handle WebSocket messages
	for {
		var msg WebSocketMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				app.logger.Warn("WebSocket unexpected close", "error", err)
			}
			break
		}

		switch msg.Type {
		case "user_message":
			// Save user message and generate AI response
			go app.handleWebSocketUserMessage(conn, user, appointmentID, msg.Content)
		case "ping":
			// Respond to ping with pong
			pong := WebSocketMessage{Type: "pong"}
			conn.WriteJSON(pong)
		default:
			app.logger.Warn("Unknown WebSocket message type", "type", msg.Type, "user_id", user.ID)
		}
	}
}

// handleWebSocketUserMessage processes a user message and streams the AI response via WebSocket
func (app *application) handleWebSocketUserMessage(conn *websocket.Conn, user *internal.User, appointmentID string, userMessage string) {
	ctx := context.Background()

	// ALWAYS fetch fresh appointment data to avoid stale data issues
	appointment, err := internal.GetAppointment(ctx, app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get fresh appointment data", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		errorMsg := WebSocketMessage{
			Type:  "error",
			Error: "Failed to get appointment data",
		}
		conn.WriteJSON(errorMsg)
		return
	}

	// Log the initial state
	app.logger.Debug("handleWebSocketUserMessage ENTRY",
		"appointment_id", appointment.ID,
		"message_count_before", len(appointment.ChatMessages),
		"user_message", userMessage)

	// Ensure checklist is initialized
	if appointment.ConversationChecklist == nil {
		app.logger.Warn("Chat appointment missing checklist, initializing", "user_id", user.ID, "appointment_id", appointment.ID)
		appointment.ConversationChecklist = internal.InitializeConversationChecklist()
		// Save the appointment with the new checklist
		if err := internal.UpdateAppointment(ctx, app.fs, appointment); err != nil {
			app.logger.Error("Failed to save appointment with initialized checklist", "error", err)
		}
	}

	// Save user message to appointment
	app.logger.Debug("handleWebSocketUserMessage -> AddChatMessage", "name", user.Name, "user_message", userMessage, "user_id", user.ID, "appointment_id", appointment.ID)
	if err := internal.AddChatMessage(ctx, app.fs, appointment, "user", userMessage); err != nil {
		app.logger.Error("Failed to save user message", "error", err, "user_id", user.ID)
		errorMsg := WebSocketMessage{
			Type:  "error",
			Error: "Failed to save your message",
		}
		conn.WriteJSON(errorMsg)
		return
	}

	// Verify the message was saved by reading it back immediately
	verifyAppointment, err := internal.GetAppointment(ctx, app.fs, user.ID, appointment.ID)
	if err != nil {
		app.logger.Error("Failed to verify message save", "error", err)
	} else {
		app.logger.Debug("Message save verification",
			"expected_count", len(appointment.ChatMessages),
			"actual_count", len(verifyAppointment.ChatMessages),
			"save_successful", len(verifyAppointment.ChatMessages) == len(appointment.ChatMessages))
	}

	// Increment message count in checklist
	internal.IncrementMessageCount(appointment.ConversationChecklist)

	// Check if conversation is getting too long and warn user
	if appointment.ConversationChecklist.MessageCount >= internal.MaxConversationLength-10 {
		warningMsg := WebSocketMessage{
			Type:    "warning",
			Content: fmt.Sprintf("This conversation is getting quite long (%d/%d messages). Consider wrapping up soon.", appointment.ConversationChecklist.MessageCount, internal.MaxConversationLength),
		}
		conn.WriteJSON(warningMsg)
	}

	// HARD STOP: Force completion if conversation exceeds maximum length
	if appointment.ConversationChecklist.MessageCount >= internal.MaxConversationLength {
		app.logger.Warn("Conversation exceeded maximum length, forcing completion",
			"user_id", user.ID,
			"appointment_id", appointmentID,
			"message_count", appointment.ConversationChecklist.MessageCount,
			"max_length", internal.MaxConversationLength)

		// First try to check completion readiness
		canComplete, issues, err := internal.ValidateChecklistCompletion(appointment.ConversationChecklist)
		if err != nil {
			app.logger.Error("Failed to validate checklist during hard stop", "error", err)
		}

		if canComplete {
			// Trigger complete_chat
			args := map[string]any{
				"summary":                  "Conversation completed due to length limit",
				"confidence_level":         "medium",
				"next_step_recommendation": "review_profile",
			}
			toolResponse, err := chatToolRegistry.ExecuteTool(ctx, app, user, appointment, "complete_chat", args)
			if err != nil {
				app.logger.Error("Failed to execute complete_chat during hard stop", "error", err)
			} else {
				// Send completion response
				responseMsg := WebSocketMessage{
					Type:         "tool_response",
					ToolResponse: toolResponse,
				}
				conn.WriteJSON(responseMsg)
				return // Don't continue streaming
			}
		} else {
			// Trigger override_completion
			args := map[string]any{
				"reason":  fmt.Sprintf("Conversation exceeded maximum length (%d messages)", internal.MaxConversationLength),
				"summary": fmt.Sprintf("Conversation ended due to length limit. Issues: %s", strings.Join(issues, ", ")),
			}
			toolResponse, err := chatToolRegistry.ExecuteTool(ctx, app, user, appointment, "override_completion", args)
			if err != nil {
				app.logger.Error("Failed to execute override_completion during hard stop", "error", err)
			} else {
				// Send completion response
				responseMsg := WebSocketMessage{
					Type:         "tool_response",
					ToolResponse: toolResponse,
				}
				conn.WriteJSON(responseMsg)
				return // Don't continue streaming
			}
		}

		// If both completion methods fail, send error and stop
		errorMsg := WebSocketMessage{
			Type:  "error",
			Error: "Conversation exceeded maximum length and completion failed",
		}
		conn.WriteJSON(errorMsg)
		return
	}

	// Stream AI response
	app.streamAIResponseViaWebSocket(ctx, conn, user, appointmentID)
}

// streamAIResponseViaWebSocket generates and streams the AI response via WebSocket
func (app *application) streamAIResponseViaWebSocket(ctx context.Context, conn *websocket.Conn, user *internal.User, appointmentID string) {
	// Log initial state
	app.logger.Debug("streamAIResponseViaWebSocket ENTRY",
		"appointment_id", appointmentID,
		"initial_message_count", 0,
		"user_id", user.ID)

	// IMPORTANT: Always fetch fresh appointment data to avoid stale data issues
	// This ensures we see any updates made by other handlers (like fastForwardChat)
	var freshAppointment *internal.Appointment
	var err error

	// Implement retry with exponential backoff to handle eventual consistency
	maxRetries := 5
	for retries := 0; retries < maxRetries; retries++ {
		freshAppointment, err = internal.GetAppointment(ctx, app.fs, user.ID, appointmentID)
		if err != nil {
			app.logger.Error("Failed to fetch fresh appointment data",
				"error", err,
				"user_id", user.ID,
				"appointment_id", appointmentID,
				"retry", retries)

			if retries < maxRetries-1 {
				backoffMs := 100 * (1 << retries) // 100ms, 200ms, 400ms, etc.
				time.Sleep(time.Duration(backoffMs) * time.Millisecond)
				continue
			}

			// Final retry failed
			errorMsg := WebSocketMessage{
				Type:  "error",
				Error: "Failed to get latest appointment data",
			}
			conn.WriteJSON(errorMsg)
			return
		}

		// Log what we got
		app.logger.Debug("Fetched appointment data",
			"retry", retries,
			"message_count", len(freshAppointment.ChatMessages),
			"expecting_more_than", 2)

		// For fastForward scenario, we expect more than 2 messages
		// If we're still seeing only 2, it might be a consistency issue
		if len(freshAppointment.ChatMessages) <= 2 && retries < maxRetries-1 {
			// Check if this might be a fastForward scenario by looking for super user
			if user.IsSuperUser {
				app.logger.Warn("Potential stale data detected for super user, retrying",
					"current_messages", len(freshAppointment.ChatMessages),
					"retry", retries)

				backoffMs := 100 * (1 << retries)
				time.Sleep(time.Duration(backoffMs) * time.Millisecond)
				continue
			}
		}

		// We got data, break out of retry loop
		break
	}

	// Use the fresh appointment data from here on
	appointment := freshAppointment
	app.logger.Debug(internal.BGYellow("streamAIResponseViaWebSocket USING FRESH DATA"),
		"chatHistory", len(freshAppointment.ChatMessages),
		"appointment_id", freshAppointment.ID)

	// Build assistant overrides for personalized chat
	assistantOverrides, err := internal.BuildAssistantOverrides(ctx, user, app.fs, app.logger)
	if err != nil {
		app.logger.Error("Failed to build assistant overrides", "error", err, "user_id", user.ID)
		errorMsg := WebSocketMessage{
			Type:  "error",
			Error: "Failed to prepare assistant",
		}
		conn.WriteJSON(errorMsg)
		return
	}

	// Convert chat history to OpenAI format
	messages := []openai.ChatCompletionMessage{}

	// Add system message from assistant overrides
	systemPrompt := "You are Shirely, an AI career assistant helping job seekers find the right opportunities."
	if assistantOverrides.Model != nil && assistantOverrides.Model.Messages != nil && len(assistantOverrides.Model.Messages) > 0 {
		// Use the system prompt from assistant overrides
		systemPrompt = assistantOverrides.Model.Messages[0].Content
	}

	// Add checklist status awareness to system prompt (compact JSON format)
	if appointment.ConversationChecklist != nil {
		checklistSummary := internal.GetChecklistSummary(appointment.ConversationChecklist)

		// Use compact JSON format to save tokens
		systemPrompt += fmt.Sprintf(`

CHECKLIST: {"status":"%s","attempts":%d/%d,"messages":%d/%d,"progress":%d/%d,"required":%d,"ready":%v}

YOUR TASK: Complete this checklist by calling 'mark_topic_progress' after each user response. Before asking questions, check if the slot is already high quality using 'get_checklist_status'. Ask clarifying questions only for low-quality slots.
`,
			checklistSummary["overall_status"],
			checklistSummary["completion_attempts"],
			internal.MaxCompletionAttempts,
			checklistSummary["message_count"],
			internal.MaxConversationLength,
			checklistSummary["completed_items"],
			checklistSummary["total_items"],
			checklistSummary["required_items"],
			checklistSummary["can_complete"],
		)
	}

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	})

	// Add conversation history
	chatHistory, err := internal.GetChatHistory(appointment)
	if err != nil {
		app.logger.Error("Failed to get chat history", "error", err)
		errorMsg := WebSocketMessage{
			Type:  "error",
			Error: "Failed to get chat history",
		}
		conn.WriteJSON(errorMsg)
		return
	}

	for _, msg := range chatHistory {
		// Skip messages with empty content
		if msg.Content == "" {
			continue
		}

		// Handle different message types according to OpenAI spec
		switch msg.Role {
		case openai.ChatMessageRoleUser:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: msg.Content,
			})
		case openai.ChatMessageRoleAssistant:
			// Check if this is a tool call message or regular assistant message
			if strings.HasPrefix(msg.Content, "TOOL_CALLS:") {
				// Parse the stored tool calls and add them as an assistant message with tool calls
				jsonStr := strings.TrimPrefix(msg.Content, "TOOL_CALLS:")
				var toolCalls []openai.ToolCall
				if err := json.Unmarshal([]byte(jsonStr), &toolCalls); err != nil {
					app.logger.Error("Failed to parse stored tool calls", "error", err, "content", msg.Content)
					continue
				}
				messages = append(messages, openai.ChatCompletionMessage{
					Role:      openai.ChatMessageRoleAssistant,
					ToolCalls: toolCalls,
					// Content is empty for tool call messages per OpenAI spec
				})
			} else {
				// Regular assistant message
				messages = append(messages, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: msg.Content,
				})
			}
		case openai.ChatMessageRoleTool:
			// Parse tool response and add as tool message
			if strings.HasPrefix(msg.Content, "TOOL_RESPONSE:") {
				jsonStr := strings.TrimPrefix(msg.Content, "TOOL_RESPONSE:")
				var toolData map[string]any
				if err := json.Unmarshal([]byte(jsonStr), &toolData); err != nil {
					app.logger.Error("Failed to parse stored tool response", "error", err, "content", msg.Content)
					continue
				}

				// Extract tool call ID and response content
				if toolCallID, ok := toolData["tool_call_id"].(string); ok {
					if content, ok := toolData["content"].(string); ok {
						messages = append(messages, openai.ChatCompletionMessage{
							Role:       openai.ChatMessageRoleTool,
							Content:    content,
							ToolCallID: toolCallID,
						})
					}
				}
			}
		case openai.ChatMessageRoleSystem:
			// System messages are typically handled separately, skip in main conversation
			continue
		default:
			// Skip unknown message types
			app.logger.Debug("Skipping unknown message type", "role", msg.Role)
			continue
		}
	}

	// Create streaming request with tools
	req := openai.ChatCompletionRequest{
		Model:    openai.GPT4Dot1,
		Messages: messages,
		Stream:   true,
		// Enable tool calling by providing available tools
		Tools: chatToolRegistry.GetAllTools(),
		// Allow the model to choose whether to call tools or respond normally
		ToolChoice: "auto",
	}

	// Create OpenAI client
	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		app.logger.Error("Failed to create chat completion stream", "error", err)
		errorMsg := WebSocketMessage{
			Type:  "error",
			Error: "Failed to generate response",
		}
		conn.WriteJSON(errorMsg)
		return
	}
	defer stream.Close()

	var fullResponse strings.Builder
	var toolCalls []openai.ToolCall

	// Stream the response
	for {
		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			app.logger.Error("Error receiving stream", "error", err)
			errorMsg := WebSocketMessage{
				Type:  "error",
				Error: "Stream error",
			}
			conn.WriteJSON(errorMsg)
			return
		}

		if len(response.Choices) > 0 {
			choice := response.Choices[0]

			// Handle regular content streaming
			if choice.Delta.Content != "" {
				content := choice.Delta.Content
				fullResponse.WriteString(content)

				// Send the content chunk via WebSocket
				msg := WebSocketMessage{
					Type:    "content",
					Content: content,
				}

				if err := conn.WriteJSON(msg); err != nil {
					app.logger.Error("Failed to send WebSocket message", "error", err)
					return
				}
			}

			// Handle tool calls
			if choice.Delta.ToolCalls != nil {
				for _, toolCall := range choice.Delta.ToolCalls {
					// Initialize tool call if it's the first delta for this tool call
					if toolCall.Index != nil && int(*toolCall.Index) >= len(toolCalls) {
						// Extend the toolCalls slice if necessary
						for len(toolCalls) <= int(*toolCall.Index) {
							toolCalls = append(toolCalls, openai.ToolCall{})
						}
					}

					if toolCall.Index != nil {
						idx := int(*toolCall.Index)

						// Accumulate tool call data
						if toolCall.ID != "" {
							toolCalls[idx].ID = toolCall.ID
						}
						if toolCall.Type != "" {
							toolCalls[idx].Type = toolCall.Type
						}
						if toolCall.Function.Name != "" {
							toolCalls[idx].Function.Name = toolCall.Function.Name
						}
						if toolCall.Function.Arguments != "" {
							toolCalls[idx].Function.Arguments += toolCall.Function.Arguments
						}
					}
				}
			}
		}
	}

	// Save the complete assistant response if there was text content
	if fullResponse.Len() > 0 {
		if err := internal.AddChatMessage(ctx, app.fs, appointment, "assistant", fullResponse.String()); err != nil {
			app.logger.Error("Failed to save assistant message", "error", err)
		} else {
			// Ensure checklist is initialized
			if appointment.ConversationChecklist == nil {
				app.logger.Warn("Chat appointment missing checklist during AI response, initializing", "user_id", user.ID, "appointment_id", appointment.ID)
				appointment.ConversationChecklist = internal.InitializeConversationChecklist()
			}

			// Increment message count in checklist
			internal.IncrementMessageCount(appointment.ConversationChecklist)

			// Save the updated appointment with incremented message count
			if err := internal.UpdateAppointment(ctx, app.fs, appointment); err != nil {
				app.logger.Error("Failed to save appointment with updated message count", "error", err)
			}
		}
	}

	// Handle tool calls if any were made
	if len(toolCalls) > 0 {
		app.logger.Info("Processing tool calls", "count", len(toolCalls), "user_id", user.ID)

		// Save the assistant's tool call message to chat history with proper role
		// According to OpenAI spec: assistant role with ToolCalls array and empty/minimal content
		toolCallsJSON, err := json.Marshal(toolCalls)
		if err != nil {
			app.logger.Error("Failed to marshal tool calls", "error", err)
		} else {
			// Store as assistant message with special marker to distinguish from regular assistant messages
			toolCallContent := "TOOL_CALLS:" + string(toolCallsJSON)
			if err := internal.AddChatMessage(ctx, app.fs, appointment, openai.ChatMessageRoleAssistant, toolCallContent); err != nil {
				app.logger.Error("Failed to save tool calls message", "error", err)
			}
		}

		// Collect tool call messages to add to conversation
		var toolMessages []openai.ChatCompletionMessage

		// Add the assistant's tool call message
		if len(toolCalls) > 0 {
			assistantMsg := openai.ChatCompletionMessage{
				Role:      openai.ChatMessageRoleAssistant,
				ToolCalls: toolCalls,
			}
			toolMessages = append(toolMessages, assistantMsg)
		}

		// Execute each tool call
		for _, toolCall := range toolCalls {
			if toolCall.Function.Name == "" {
				app.logger.Warn("Tool call missing function name", "tool_call_id", toolCall.ID)
				continue
			}

			// Parse the function arguments
			var args map[string]any
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
				app.logger.Error("Failed to parse tool call arguments", "error", err, "tool_call_id", toolCall.ID)

				// Send error response for this tool call
				errorResponse := WebSocketMessage{
					Type: "tool_response",
					ToolResponse: &ToolResponse{
						ID:      toolCall.ID,
						Name:    toolCall.Function.Name,
						Content: "Failed to parse tool arguments",
						Success: false,
					},
				}
				conn.WriteJSON(errorResponse)
				continue
			}

			app.logger.Info("Executing tool call",
				"tool_name", toolCall.Function.Name,
				"tool_id", toolCall.ID,
				"args", redactSensitiveInfo(args),
			)

			// Execute the tool
			toolResponse, err := chatToolRegistry.ExecuteTool(ctx, app, user, appointment, toolCall.Function.Name, args)
			if err != nil {
				app.logger.Error("Tool execution failed", "error", err, "tool_name", toolCall.Function.Name)
				toolResponse = &ToolResponse{
					ID:      toolCall.ID,
					Name:    toolCall.Function.Name,
					Content: "Tool execution failed: " + err.Error(),
					Success: false,
				}
			}

			// Set the tool call ID in the response
			toolResponse.ID = toolCall.ID

			// Send tool response via WebSocket
			responseMsg := WebSocketMessage{
				Type:         "tool_response",
				ToolResponse: toolResponse,
			}

			if err := conn.WriteJSON(responseMsg); err != nil {
				app.logger.Error("Failed to send tool response", "error", err)
				return
			}

			// Add tool result to conversation messages
			toolResultMsg := openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    toolResponse.Content,
				ToolCallID: toolCall.ID,
			}
			toolMessages = append(toolMessages, toolResultMsg)

			// Log the tool call and response for debugging/auditing
			toolCallLog := map[string]any{
				"tool_name":  toolCall.Function.Name,
				"tool_id":    toolCall.ID,
				"arguments":  args,
				"success":    toolResponse.Success,
				"response":   toolResponse.Content,
				"ui_actions": toolResponse.UIActions,
			}

			// Save tool call to chat history as a special message type
			// According to OpenAI spec: use "tool" role with ToolCallID
			toolCallData := map[string]any{
				"tool_call_id": toolCall.ID,
				"content":      toolResponse.Content,
			}
			toolCallJSON, _ := json.Marshal(toolCallData)
			toolResponseContent := "TOOL_RESPONSE:" + string(toolCallJSON)
			if err := internal.AddChatMessage(ctx, app.fs, appointment, openai.ChatMessageRoleTool, toolResponseContent); err != nil {
				app.logger.Error("Failed to save tool response message", "error", err)
			}

			app.logger.Info("Tool call completed", "tool_call", toolCallLog)
		}

		// Generate follow-up response from AI with tool results
		if len(toolMessages) > 0 {
			app.logger.Debug("generating follow-up response after tool calls", "name", user.Name, "user_id", user.ID, "appointment_id", appointment.ID)

			// Get current conversation history
			chatHistory, err := internal.GetChatHistory(appointment)
			if err != nil {
				app.logger.Error("Failed to get chat history for follow-up", "error", err)
			} else {
				// Build messages for follow-up request
				followUpMessages := []openai.ChatCompletionMessage{}

				// Add system message
				systemPrompt := "You are Shirely, an AI career assistant helping job seekers find the right opportunities."
				if assistantOverrides, err := internal.BuildAssistantOverrides(ctx, user, app.fs, app.logger); err == nil {
					if assistantOverrides.Model != nil && assistantOverrides.Model.Messages != nil && len(assistantOverrides.Model.Messages) > 0 {
						systemPrompt = assistantOverrides.Model.Messages[0].Content
					}
				}

				// Add checklist status to system prompt (compact JSON format)
				if appointment.ConversationChecklist != nil {
					checklistSummary := internal.GetChecklistSummary(appointment.ConversationChecklist)
					systemPrompt += fmt.Sprintf(`

CHECKLIST: {"status":"%s","attempts":%d/%d,"messages":%d/%d,"progress":%d/%d,"required":%d,"ready":%v}

Continue the conversation naturally after processing the tool results.
`,
						checklistSummary["overall_status"],
						checklistSummary["completion_attempts"],
						internal.MaxCompletionAttempts,
						checklistSummary["message_count"],
						internal.MaxConversationLength,
						checklistSummary["completed_items"],
						checklistSummary["total_items"],
						checklistSummary["required_items"],
						checklistSummary["can_complete"],
					)
				}

				followUpMessages = append(followUpMessages, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleSystem,
					Content: systemPrompt,
				})

				// Add conversation history
				for _, msg := range chatHistory {
					// Skip messages with empty content
					if msg.Content == "" {
						continue
					}

					// Handle different message types according to OpenAI spec
					switch msg.Role {
					case openai.ChatMessageRoleUser:
						followUpMessages = append(followUpMessages, openai.ChatCompletionMessage{
							Role:    openai.ChatMessageRoleUser,
							Content: msg.Content,
						})
					case openai.ChatMessageRoleAssistant:
						// Check if this is a tool call message or regular assistant message
						if strings.HasPrefix(msg.Content, "TOOL_CALLS:") {
							// Parse the stored tool calls and add them as an assistant message with tool calls
							jsonStr := strings.TrimPrefix(msg.Content, "TOOL_CALLS:")
							var toolCalls []openai.ToolCall
							if err := json.Unmarshal([]byte(jsonStr), &toolCalls); err != nil {
								app.logger.Error("Failed to parse stored tool calls in follow-up", "error", err)
								continue
							}
							followUpMessages = append(followUpMessages, openai.ChatCompletionMessage{
								Role:      openai.ChatMessageRoleAssistant,
								ToolCalls: toolCalls,
								// Content is empty for tool call messages per OpenAI spec
							})
						} else {
							// Regular assistant message
							followUpMessages = append(followUpMessages, openai.ChatCompletionMessage{
								Role:    openai.ChatMessageRoleAssistant,
								Content: msg.Content,
							})
						}
					case openai.ChatMessageRoleTool:
						// Parse tool response and add as tool message
						if strings.HasPrefix(msg.Content, "TOOL_RESPONSE:") {
							jsonStr := strings.TrimPrefix(msg.Content, "TOOL_RESPONSE:")
							var toolData map[string]any
							if err := json.Unmarshal([]byte(jsonStr), &toolData); err != nil {
								app.logger.Error("Failed to parse stored tool response in follow-up", "error", err)
								continue
							}

							if toolCallID, ok := toolData["tool_call_id"].(string); ok {
								if content, ok := toolData["content"].(string); ok {
									followUpMessages = append(followUpMessages, openai.ChatCompletionMessage{
										Role:       openai.ChatMessageRoleTool,
										Content:    content,
										ToolCallID: toolCallID,
									})
								}
							}
						}
					case openai.ChatMessageRoleSystem:
						// System messages are handled separately
						continue
					default:
						app.logger.Debug("Skipping unknown message type in follow-up", "role", msg.Role)
						continue
					}
				}

				// Add the tool call messages
				followUpMessages = append(followUpMessages, toolMessages...)

				// Create follow-up request
				followUpReq := openai.ChatCompletionRequest{
					Model:      openai.GPT4Dot1,
					Messages:   followUpMessages,
					Stream:     true,
					Tools:      chatToolRegistry.GetAllTools(),
					ToolChoice: "none", // Prevent cascading tool calls after processing results
				}

				// Stream the follow-up response
				client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
				followUpStream, err := client.CreateChatCompletionStream(ctx, followUpReq)
				if err != nil {
					app.logger.Error("Failed to create follow-up stream", "error", err)
					return
				}
				defer followUpStream.Close()

				var followUpResponse strings.Builder

				// Stream the follow-up response
				for {
					response, err := followUpStream.Recv()
					if err != nil {
						if err == io.EOF {
							break
						}
						app.logger.Error("Error receiving follow-up stream", "error", err)
						return
					}

					if len(response.Choices) > 0 {
						choice := response.Choices[0]

						// Handle content streaming
						if choice.Delta.Content != "" {
							content := choice.Delta.Content
							followUpResponse.WriteString(content)

							// Send the content chunk via WebSocket
							msg := WebSocketMessage{
								Type:    "content",
								Content: content,
							}

							if err := conn.WriteJSON(msg); err != nil {
								app.logger.Error("Failed to send follow-up WebSocket message", "error", err)
								return
							}
						}
					}
				}

				// Save the follow-up response
				if followUpResponse.Len() > 0 {
					if err := internal.AddChatMessage(ctx, app.fs, appointment, "assistant", followUpResponse.String()); err != nil {
						app.logger.Error("Failed to save follow-up assistant message", "error", err)
					} else {
						// Increment message count for follow-up response
						internal.IncrementMessageCount(appointment.ConversationChecklist)
						if err := internal.UpdateAppointment(ctx, app.fs, appointment); err != nil {
							app.logger.Error("Failed to save appointment with follow-up message count", "error", err)
						}
					}
				}
			}
		}
	}

	// Send completion event
	doneMsg := WebSocketMessage{
		Type: "done",
	}
	conn.WriteJSON(doneMsg)
}

// endChat handles ending a chat session
func (app *application) endChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		response.JSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	if appointmentID == "" {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "missing appointment_id"})
		return
	}

	// Get the appointment
	appointment, err := internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get appointment for ending", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusNotFound, map[string]string{"error": "appointment not found"})
		return
	}

	if appointment.Type != internal.AppointmentTypeChat {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "not a chat appointment"})
		return
	}

	// Mark appointment as processing for transcript processing
	if err := internal.ProcessAppointmentStarted(r.Context(), app.fs, user, appointment); err != nil {
		app.logger.Error("Failed to start appointment processing", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to process chat session"})
		return
	}

	app.logger.Info("Chat session ended", "user_id", user.ID, "appointment_id", appointmentID)

	response.JSON(w, http.StatusOK, map[string]string{
		"status":         "ended",
		"appointment_id": appointmentID,
	})
}

// showChatPage displays the dedicated chat page
func (app *application) showChatPage(w http.ResponseWriter, r *http.Request) {
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	var appointment *internal.Appointment
	var err error

	if appointmentID == "" {
		// No appointment ID provided, try to find the latest running chat appointment
		app.logger.Debug("No appointment ID provided, looking for latest running chat appointment", "user_id", user.ID)

		var found bool
		appointment, found, err = internal.GetLatestRunningChatAppointment(r.Context(), app.fs, user.ID)
		if err != nil {
			app.logger.Error("Failed to get latest running chat appointment", "error", err, "user_id", user.ID)
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}

		if !found {
			app.logger.Info("No running chat appointment found for user", "user_id", user.ID)
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}

		appointmentID = appointment.ID
		app.logger.Info("Found running chat appointment", "user_id", user.ID, "appointment_id", appointmentID)
	} else {
		// Verify the appointment exists and belongs to this user
		appointment, err = internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
		if err != nil {
			app.logger.Error("Failed to get appointment for chat page", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}
	}

	if appointment.Type != internal.AppointmentTypeChat {
		app.logger.Error("Appointment is not a chat type", "user_id", user.ID, "appointment_id", appointmentID, "type", appointment.Type)
		http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
		return
	}

	data := app.newTemplateData(r)
	data["AppointmentID"] = appointmentID
	data["Appointment"] = appointment
	data["User"] = user

	err = response.Page(w, http.StatusOK, data, "pages/chat.html")
	if err != nil {
		app.logger.Error("Failed to render chat page", "error", err)
		app.serverError(w, r, err)
	}
}

// isDebugMessage checks if a message role is considered a debug message
func isDebugMessage(role string) bool {
	return role == openai.ChatMessageRoleTool || role == openai.ChatMessageRoleSystem
}

// isDebugMessageContent checks if message content indicates a debug message (tool calls)
func isDebugMessageContent(role, content string) bool {
	return role == openai.ChatMessageRoleAssistant && strings.HasPrefix(content, "TOOL_CALLS:")
}

// getChatHistory returns the chat history for an appointment
func (app *application) getChatHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		response.JSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	if appointmentID == "" {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "missing appointment_id"})
		return
	}

	// Get the appointment
	appointment, err := internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get appointment for history", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusNotFound, map[string]string{"error": "appointment not found"})
		return
	}

	if appointment.Type != internal.AppointmentTypeChat {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "not a chat appointment"})
		return
	}

	// Get chat history
	messages, err := internal.GetChatHistory(appointment)
	if err != nil {
		app.logger.Error("Failed to get chat history", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get chat history"})
		return
	}

	// Filter debug messages for non-super users
	var filteredMessages []internal.ChatMessage
	for _, message := range messages {
		// Skip debug messages for non-super users
		if !user.IsSuperUser && (isDebugMessage(message.Role) || isDebugMessageContent(message.Role, message.Content)) {
			continue
		}
		filteredMessages = append(filteredMessages, message)
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"messages":       filteredMessages,
		"appointment_id": appointmentID,
		"debug_mode":     user.IsSuperUser, // Let client know if debug mode is enabled
	})
}

// fastForwardChat injects a test transcript into the current chat session for testing
func (app *application) fastForwardChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	user := contextGetAuthenticatedUser(r)
	if user == nil {
		response.JSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	// Only allow super users to use this feature
	if !user.IsSuperUser {
		response.JSON(w, http.StatusForbidden, map[string]string{"error": "forbidden - super user required"})
		return
	}

	appointmentID := r.URL.Query().Get("appointment_id")
	if appointmentID == "" {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "missing appointment_id"})
		return
	}

	// Get the appointment
	appointment, err := internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to get appointment for fast-forward", "error", err, "user_id", user.ID, "appointment_id", appointmentID)
		response.JSON(w, http.StatusNotFound, map[string]string{"error": "appointment not found"})
		return
	}

	if appointment.Type != internal.AppointmentTypeChat {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "not a chat appointment"})
		return
	}

	// Load test call data using the TestingCallID
	app.logger.Info("Loading test call data for fast-forward", "appointment_id", appointmentID, "test_call_id", simulate.TestingCallID)

	call, err := vapi.GetCall(r.Context(), simulate.TestingCallID)
	if err != nil {
		app.logger.Error("Failed to get test call data", "error", err, "test_call_id", simulate.TestingCallID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load test call data"})
		return
	}

	// Extract transcript messages from the call
	var messages []internal.ChatMessage
	if call.Artifact != nil && call.Artifact.Messages != nil {
		app.logger.Info("Fast-forward: Processing call artifact", "artifact_messages_count", len(call.Artifact.Messages))

		tx := internal.NewTranscriptFromCallResult(*call)
		app.logger.Info("Fast-forward: Transcript processed", "transcript_messages_count", len(tx.Messages))

		for i, msg := range tx.Messages {
			app.logger.Debug("Fast-forward: Processing message", "index", i, "role", msg.Role, "content_length", len(msg.Content))

			// Skip messages with empty content
			if msg.Content == "" {
				app.logger.Debug("Fast-forward: Skipping empty message", "index", i)
				continue
			}

			chatMessage := internal.ChatMessage{
				Role:      string(msg.Role),
				Content:   msg.Content,
				Timestamp: time.Now(),
			}
			messages = append(messages, chatMessage)
			contentPreview := chatMessage.Content
			if len(contentPreview) > 50 {
				contentPreview = contentPreview[:50] + "..."
			}
			app.logger.Debug("Fast-forward: Added message", "index", i, "role", chatMessage.Role, "content_preview", contentPreview)
		}
	} else {
		app.logger.Error("Fast-forward: No artifact or messages found", "has_artifact", call.Artifact != nil, "has_messages", call.Artifact != nil && call.Artifact.Messages != nil)
	}

	app.logger.Info("Fast-forward: Extracted messages", "count", len(messages))
	if len(messages) == 0 {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "no messages found in test call"})
		return
	}

	// Inject the messages into the current chat appointment
	app.logger.Info("Injecting transcript messages into chat", "appointment_id", appointmentID, "message_count", len(messages))

	// Clear existing messages first (except the initial assistant message)
	app.logger.Info("Fast-forward: Current chat messages before clearing", "count", len(appointment.ChatMessages))
	if len(appointment.ChatMessages) > 0 {
		appointment.ChatMessages = appointment.ChatMessages[:0] // Remove all messages
		app.logger.Info("Fast-forward: Cleared messages", "remaining_count", len(appointment.ChatMessages))
	}

	// Add the transcript messages
	app.logger.Info("Fast-forward: Adding transcript messages", "current_count", len(appointment.ChatMessages), "adding_count", len(messages))
	appointment.ChatMessages = append(appointment.ChatMessages, messages...)
	app.logger.Info("Fast-forward: Messages after adding transcript", "total_count", len(appointment.ChatMessages))

	// Update the conversation checklist to reflect that topics have been covered
	if appointment.ConversationChecklist == nil {
		appointment.ConversationChecklist = internal.InitializeConversationChecklist()
	}

	// Mark all major topics as completed since we're injecting a full transcript
	topicsToComplete := []string{
		"introduction", "warm_up", "deal_breakers", "search_strategy",
		"work_environment", "location_preferences", "final_confirmation",
	}

	for _, topicID := range topicsToComplete {
		extractedInfo := map[string]any{
			"source":         "transcript_injection",
			"auto_populated": true,
			"timestamp":      time.Now(),
		}

		err := internal.UpdateChecklistItem(
			appointment.ConversationChecklist,
			topicID,
			internal.ChecklistItemStatusCompleted,
			internal.ChecklistItemQualityMedium,
			extractedInfo,
			"Auto-populated from injected transcript for testing",
		)
		if err != nil {
			app.logger.Error("Failed to update checklist item", "error", err, "topic_id", topicID)
			// Continue with other topics even if one fails
		}
	}

	// Update the checklist message count to reflect the injected messages
	appointment.ConversationChecklist.MessageCount = len(appointment.ChatMessages)
	appointment.ConversationChecklist.LastUpdated = time.Now()

	// Mark the overall status as ready for completion since we've covered all topics
	appointment.ConversationChecklist.OverallStatus = internal.ChecklistStatusReadyForCompletion

	// Update the appointment with the new messages and checklist
	app.logger.Info(internal.BGYellow("Fast-forward: Saving appointment"), "appointment_id", appointmentID, "final_message_count", len(appointment.ChatMessages))
	if err := internal.UpdateAppointment(r.Context(), app.fs, appointment); err != nil {
		app.logger.Error("Failed to update appointment with transcript", "error", err, "appointment_id", appointmentID)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update appointment"})
		return
	}

	// Verify the write was successful by reading it back
	app.logger.Info("Verifying appointment write...", "appointment_id", appointmentID)
	verifyAppointment, err := internal.GetAppointment(r.Context(), app.fs, user.ID, appointmentID)
	if err != nil {
		app.logger.Error("Failed to verify appointment write", "error", err, "appointment_id", appointmentID)
	} else {
		app.logger.Info("Appointment write verification",
			"expected_messages", len(appointment.ChatMessages),
			"actual_messages", len(verifyAppointment.ChatMessages),
			"write_successful", len(verifyAppointment.ChatMessages) == len(appointment.ChatMessages))

		if len(verifyAppointment.ChatMessages) != len(appointment.ChatMessages) {
			app.logger.Error("CRITICAL: Appointment write verification failed - data inconsistency detected",
				"expected", len(appointment.ChatMessages),
				"actual", len(verifyAppointment.ChatMessages))
		}
	}

	app.logger.Info("Chat fast-forwarded successfully", "appointment_id", appointmentID, "injected_messages", len(messages), "final_total_messages", len(appointment.ChatMessages))

	response.JSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"appointment_id":    appointmentID,
		"injected_messages": len(messages),
	})
}
