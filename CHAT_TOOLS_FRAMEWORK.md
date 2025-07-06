# Chat Tools Framework Documentation

## Overview

The Chat Tools Framework enables the LLM (Large Language Model) in the chat interface to call specific functions during conversations. This allows the AI to perform actions like completing the chat session, updating user preferences, and scheduling follow-ups based on the conversation context.

## Architecture

### Core Components

1. **ChatTool** - Represents a single tool with its OpenAI function definition and handler
2. **ChatToolRegistry** - Manages all available tools and their execution
3. **WebSocket Integration** - Handles tool calls in real-time during streaming responses
4. **UI Actions** - Allows tools to trigger client-side actions like redirects or notifications

### Data Flow

```
User Message → OpenAI API (with tools) → Tool Call → Tool Handler → Tool Response → WebSocket → Client UI Actions
```

## Built-in Tools

### 1. complete_chat

**Purpose**: Indicates the chat session is complete and transitions the user to the next onboarding step.

**Parameters**:
- `summary` (required): Brief summary of career preferences learned
- `confidence_level` (required): "low", "medium", or "high" 
- `next_step_recommendation` (required): Recommended next step

**Example Usage**:
```json
{
  "summary": "User prefers remote software engineering roles in fintech",
  "confidence_level": "high", 
  "next_step_recommendation": "schedule_call"
}
```

**Actions Performed**:
- Marks appointment as complete in database
- Sends success message to user
- Redirects to profile page after 3 seconds

### 2. update_user_preferences

**Purpose**: Updates user's career preferences based on chat conversation.

**Parameters** (all optional):
- `job_titles`: Array of job titles user is interested in
- `industries`: Array of industries user wants to work in
- `work_location_preference`: "remote", "hybrid", "onsite", or "flexible"
- `salary_range`: Object with `min` and `max` numbers
- `experience_level`: "entry", "mid", "senior", or "executive"

**Example Usage**:
```json
{
  "job_titles": ["Software Engineer", "Full Stack Developer"],
  "industries": ["Technology", "Finance"],
  "work_location_preference": "remote",
  "salary_range": {"min": 80000, "max": 120000},
  "experience_level": "mid"
}
```

### 3. schedule_followup

**Purpose**: Schedules follow-up actions based on user needs.

**Parameters**:
- `followup_type` (required): "recruiter_call", "job_review", "resume_review", or "interview_prep"
- `priority` (required): "low", "medium", "high", or "urgent"
- `notes` (optional): Additional notes about the follow-up
- `suggested_timeframe` (optional): When follow-up should happen

**Example Usage**:
```json
{
  "followup_type": "recruiter_call",
  "priority": "high",
  "notes": "User needs help with salary negotiation",
  "suggested_timeframe": "within 24 hours"
}
```

## Adding New Tools

### Step 1: Define the Tool

Add a new tool to the `registerDefaultTools()` method:

```go
r.RegisterTool("tool_name", ChatTool{
    Definition: openai.Tool{
        Type: openai.ToolTypeFunction,
        Function: &openai.FunctionDefinition{
            Name:        "tool_name",
            Description: "What this tool does and when to use it",
            Parameters: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "param1": map[string]interface{}{
                        "type":        "string",
                        "description": "Description of parameter",
                    },
                    "param2": map[string]interface{}{
                        "type":        "number",
                        "description": "Another parameter",
                    },
                },
                "required": []string{"param1"},
            },
        },
    },
    Handler: r.handleToolName,
})
```

### Step 2: Implement the Handler

Create a handler function:

```go
func (r *ChatToolRegistry) handleToolName(ctx context.Context, app *application, user *internal.User, appointment *internal.Appointment, args map[string]interface{}) (*ToolResponse, error) {
    app.logger.Info("Tool executed", "tool_name", "tool_name", "user_id", user.ID, "args", args)
    
    // Extract and validate arguments
    param1, ok := args["param1"].(string)
    if !ok {
        return &ToolResponse{
            Name:    "tool_name",
            Content: "Missing required parameter: param1",
            Success: false,
        }, nil
    }
    
    param2, _ := args["param2"].(float64) // Optional parameter
    
    // Perform the tool's business logic
    // ... your implementation here ...
    
    // Return response with optional UI actions
    return &ToolResponse{
        Name:    "tool_name",
        Content: "Tool executed successfully",
        Success: true,
        UIActions: []UIAction{
            {
                Type: "show_message",
                Data: map[string]interface{}{
                    "message": "Tool completed!",
                    "type":    "success",
                },
                Delay: 1000,
            },
        },
    }, nil
}
```

## UI Actions

Tools can trigger client-side actions through the `UIActions` field:

### Available Action Types

1. **show_message**: Display a message to the user
   ```go
   UIAction{
       Type: "show_message",
       Data: map[string]interface{}{
           "message": "Your message here",
           "type":    "success", // success, error, info, warning
       },
       Delay: 1000, // milliseconds
   }
   ```

2. **redirect**: Navigate to a different page
   ```go
   UIAction{
       Type: "redirect", 
       Data: map[string]interface{}{
           "url": "/profile",
       },
       Delay: 3000,
   }
   ```

3. **update_status**: Update the chat status area
   ```go
   UIAction{
       Type: "update_status",
       Data: map[string]interface{}{
           "message": "Processing...",
           "type":    "info",
       },
   }
   ```

4. **disable_input**: Disable chat input
   ```go
   UIAction{
       Type: "disable_input",
       Data: map[string]interface{}{
           "message": "Chat completed",
       },
   }
   ```

5. **show_completion**: Show chat completion UI
   ```go
   UIAction{
       Type: "show_completion",
       Data: map[string]interface{}{
           "message":    "Thank you for chatting!",
           "next_steps": "Check your email for next steps",
       },
   }
   ```

### Adding New Action Types

To add a new UI action type, update the `executeUIAction` method in the JavaScript:

```javascript
case 'your_new_action':
    this.handleYourNewAction(action.data);
    break;
```

## WebSocket Message Types

The framework adds new WebSocket message types:

### tool_response
```json
{
  "type": "tool_response",
  "tool_response": {
    "id": "call_abc123",
    "name": "complete_chat", 
    "content": "Chat completed successfully",
    "success": true,
    "ui_actions": [...]
  }
}
```

## Error Handling

### Tool Execution Errors

- Invalid arguments: Tool receives error response with details
- Missing tool: Returns "Tool not found" error
- Handler exceptions: Logged and returns generic error response

### WebSocket Errors

- Connection loss during tool execution: Tool calls are logged for recovery
- Invalid tool responses: Logged as warnings, chat continues

## Logging and Debugging

### Server-side Logging

All tool executions are logged with:
- Tool name and ID
- User ID and appointment ID  
- Input arguments
- Success/failure status
- Response content
- UI actions triggered

### Client-side Debugging

Tool responses are logged to browser console:
```javascript
console.log('Tool response received:', toolResponse);
console.log(`Tool "${toolResponse.name}" executed:`, {
    success: toolResponse.success,
    content: toolResponse.content,
    ui_actions: toolResponse.ui_actions
});
```

## Best Practices

### Tool Design

1. **Single Responsibility**: Each tool should have one clear purpose
2. **Descriptive Names**: Use clear, descriptive function names
3. **Comprehensive Descriptions**: Help the LLM understand when to use the tool
4. **Validate Arguments**: Always validate and sanitize input parameters
5. **Graceful Errors**: Return helpful error messages, don't throw exceptions

### Parameter Design

1. **Required vs Optional**: Mark parameters as required only when necessary
2. **Enums for Choices**: Use enum constraints for limited choice parameters
3. **Clear Descriptions**: Explain what each parameter does and expected format
4. **Sensible Defaults**: Provide good default behavior for optional parameters

### UI Actions

1. **Progressive Enhancement**: Ensure chat works even if UI actions fail
2. **Appropriate Delays**: Give users time to read messages before redirects
3. **Feedback**: Always provide feedback when tools execute
4. **Graceful Degradation**: Handle cases where client doesn't support new action types

### Error Handling

1. **Log Everything**: Comprehensive logging for debugging and auditing
2. **User-Friendly Errors**: Don't expose technical details to users
3. **Fail Gracefully**: Continue chat even if tools fail
4. **Retry Logic**: Consider retry mechanisms for transient failures

## Testing

### Unit Testing Tools

```go
func TestCompleteChat(t *testing.T) {
    registry := NewChatToolRegistry()
    
    args := map[string]interface{}{
        "summary": "Test summary",
        "confidence_level": "high",
        "next_step_recommendation": "schedule_call",
    }
    
    response, err := registry.ExecuteTool(ctx, app, user, appointment, "complete_chat", args)
    
    if err != nil {
        t.Fatalf("Tool execution failed: %v", err)
    }
    
    if !response.Success {
        t.Errorf("Expected success, got failure: %s", response.Content)
    }
}
```

### Integration Testing

Test the full flow from WebSocket message to tool execution to UI actions.

## Security Considerations

1. **Input Validation**: Validate all tool parameters
2. **Authorization**: Ensure users can only execute tools on their own data
3. **Rate Limiting**: Consider rate limiting tool executions
4. **Audit Logging**: Log all tool executions for security auditing
5. **Sanitization**: Sanitize any user-provided data before processing

## Performance Considerations

1. **Async Execution**: Tool handlers should be efficient and non-blocking
2. **Database Connections**: Reuse database connections, don't create new ones
3. **Caching**: Cache frequently accessed data
4. **Timeouts**: Implement timeouts for long-running operations
5. **Resource Cleanup**: Properly clean up resources in tool handlers

## Future Enhancements

Potential improvements to the framework:

1. **Tool Chaining**: Allow tools to call other tools
2. **Conditional Execution**: Execute tools based on conditions
3. **Batch Operations**: Execute multiple tools in a single call
4. **Tool Permissions**: Role-based access to different tools
5. **External APIs**: Tools that call external services
6. **Scheduled Tools**: Tools that execute at specific times
7. **Tool Analytics**: Track tool usage and effectiveness 