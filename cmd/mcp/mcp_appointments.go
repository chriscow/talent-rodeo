package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"talent-rodeo/internal"

	"github.com/chriscow/minds"
	"github.com/mark3labs/mcp-go/mcp"
)

type GetAppointmentChatParams struct {
	UserID           string `json:"user_id" description:"ID of the user who owns the appointment"`
	AppointmentID    string `json:"appointment_id" description:"ID of the appointment to get chat messages for"`
	IncludeChecklist bool   `json:"include_checklist" description:"Whether to include conversation checklist details"`
}

// NewGetAppointmentChatTool creates a new tool for retrieving appointment chat messages
func NewGetAppointmentChatTool() mcp.Tool {
	schema, err := minds.GenerateSchema(GetAppointmentChatParams{})
	if err != nil {
		panic("Could not generate schema from GetAppointmentChatParams")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("get_appointment_chat",
		"Get chat messages for a specific appointment with optional checklist details",
		schemaJSON,
	)
}

// getAppointmentChatHandler handles the get_appointment_chat tool call
func getAppointmentChatHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fmt.Fprintf(os.Stderr, "DEBUG: get_appointment_chat called with args: %+v\n", request.Params.Arguments)

	// Extract arguments
	var args GetAppointmentChatParams

	argsData, err := getJSONData(request.Params.Arguments)
	if err != nil {
		return nil, fmt.Errorf("error getting JSON data: %w", err)
	}

	if err := json.Unmarshal(argsData, &args); err != nil {
		return nil, fmt.Errorf("error unmarshaling arguments: %w", err)
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Parsed args - UserID: %s, AppointmentID: %s, IncludeChecklist: %t\n",
		args.UserID, args.AppointmentID, args.IncludeChecklist)

	// Get Firestore client
	fs, err := createFirestoreClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting Firestore client: %w", err)
	}
	defer fs.Close()

	// Get the appointment - use internal package functions
	appointment, err := internal.GetAppointment(ctx, fs, args.UserID, args.AppointmentID)
	if err != nil {
		return nil, fmt.Errorf("error getting appointment: %w", err)
	}

	// Check if this is a chat appointment
	if appointment.Type != internal.AppointmentTypeChat {
		return nil, fmt.Errorf("appointment %s is not a chat appointment (type: %s)",
			args.AppointmentID, appointment.Type)
	}

	// Get chat messages
	messages, err := internal.GetChatMessages(ctx, fs, args.UserID, args.AppointmentID)
	if err != nil {
		return nil, fmt.Errorf("error getting chat messages: %w", err)
	}

	// Build result
	result := map[string]any{
		"appointment_id": args.AppointmentID,
		"user_id":        args.UserID,
		"type":           string(appointment.Type),
		"status":         string(appointment.Status),
		"message_count":  len(messages),
		"messages":       messages,
	}

	if args.IncludeChecklist && appointment.ConversationChecklist != nil {
		result["checklist"] = appointment.ConversationChecklist
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Returning result with %d messages\n", len(messages))
	return createResult(result)
}

type ListAppointmentsParams struct {
	UserID          string `json:"user_id" description:"ID of the user to list appointments for"`
	AppointmentType string `json:"appointment_type" description:"Filter by appointment type (chat, voice)"`
	Limit           int    `json:"limit" description:"Maximum number of appointments to return"`
}

// NewListAppointmentsTool creates a new tool for listing appointments for a user
func NewListAppointmentsTool() mcp.Tool {
	schema, err := minds.GenerateSchema(ListAppointmentsParams{})
	if err != nil {
		panic("Could not generate schema from ListAppointmentsParams")
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic("Could not marshal schema to JSON")
	}

	return mcp.NewToolWithRawSchema("list_appointments",
		"List all appointments for a specific user with optional type filtering",
		schemaJSON,
	)
}

// listAppointmentsHandler handles the list_appointments tool call
func listAppointmentsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fmt.Fprintf(os.Stderr, "DEBUG: list_appointments called with args: %+v\n", request.Params.Arguments)

	// Extract arguments
	var args ListAppointmentsParams

	argsData, err := getJSONData(request.Params.Arguments)
	if err != nil {
		return nil, fmt.Errorf("error getting JSON data: %w", err)
	}

	if err := json.Unmarshal(argsData, &args); err != nil {
		return nil, fmt.Errorf("error unmarshaling arguments: %w", err)
	}

	if args.Limit == 0 {
		args.Limit = 10
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Parsed args - UserID: %s, Type: %s, Limit: %d\n",
		args.UserID, args.AppointmentType, args.Limit)

	// Get Firestore client
	fs, err := createFirestoreClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting Firestore client: %w", err)
	}
	defer fs.Close()

	// Get appointments using internal function
	appointments, err := internal.ListAppointments(ctx, fs, args.UserID)
	if err != nil {
		return nil, fmt.Errorf("error listing appointments: %w", err)
	}

	// Filter by type if specified
	if args.AppointmentType != "" {
		var filteredAppointments []*internal.Appointment
		for _, appt := range appointments {
			if string(appt.Type) == args.AppointmentType {
				filteredAppointments = append(filteredAppointments, appt)
			}
		}
		appointments = filteredAppointments
	}

	// Apply limit
	if args.Limit > 0 && len(appointments) > args.Limit {
		appointments = appointments[:args.Limit]
	}

	// Build result
	result := map[string]any{
		"user_id":          args.UserID,
		"appointment_type": args.AppointmentType,
		"count":            len(appointments),
		"appointments":     appointments,
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Returning result with %d appointments\n", len(appointments))
	return createResult(result)
}
