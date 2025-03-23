package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/utils"
	"talent-rodeo/internal/vapi"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// handleVapiCall handles incoming webhook calls from VAPI
func (app *application) handleVapiCall(w http.ResponseWriter, r *http.Request) {
	app.logger.Debug("VAPI call received", "method", r.Method, "url", r.URL.String())

	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// verify all env vars are set
	if os.Getenv("VAPI_ASSISTANT_ID") == "" || os.Getenv("VAPI_OUTBOUND_PHONE_ID") == "" {
		app.logger.Error("Missing required environment variables",
			"assistant_id", os.Getenv("VAPI_ASSISTANT_ID") == "",
			"phone_id", os.Getenv("VAPI_OUTBOUND_PHONE_ID") == "")
		app.reportServerError(r, fmt.Errorf("missing required environment variables"))
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "missing required environment variables",
		})
		return
	}

	// Get assistant and phone IDs from environment
	assistantID := os.Getenv("VAPI_ASSISTANT_ID")
	outboundPhoneID := os.Getenv("VAPI_OUTBOUND_PHONE_ID")

	if assistantID == "" || outboundPhoneID == "" {
		app.logger.Error("Missing required environment variables",
			"assistant_id", assistantID == "",
			"phone_id", outboundPhoneID == "")
		app.reportServerError(r, fmt.Errorf("missing required environment variables"))
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "missing required environment variables",
		})
		return
	}

	// Parse the request body
	var callRequest struct {
		CustomerID string `json:"customer_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&callRequest); err != nil {
		app.logger.Error("Failed to decode request body", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to decode request body: %w", err))
		response.JSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// validate that none of the fields are empty
	if callRequest.CustomerID == "" {
		app.logger.Error("Missing required fields", "customer_id_missing", callRequest.CustomerID == "")
		app.reportServerError(r, fmt.Errorf("missing required fields"))
		response.JSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing required fields",
		})
		return
	}

	user, found, err := internal.GetUser(r.Context(), app.fs, callRequest.CustomerID)
	if err != nil {
		app.logger.Error("Failed to get user", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to get user: %w", err))
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to get user",
		})
		return
	}

	if !found {
		app.logger.Error("User not found", "customer_id", callRequest.CustomerID)
		app.reportServerError(r, fmt.Errorf("user not found"))
		response.JSON(w, http.StatusNotFound, map[string]string{
			"error": "user not found",
		})
		return
	}

	if user.Phone == nil || *user.Phone == "" {
		app.logger.Error("User has no phone number", "customer_id", callRequest.CustomerID)
		app.reportServerError(r, fmt.Errorf("user has no phone number"))
		response.JSON(w, http.StatusNotFound, map[string]string{
			"error": "user has no phone number",
		})
		return
	}

	if user.State.Status == internal.UserStatusError {
		app.logger.Error("User is in error state", "customer_id", callRequest.CustomerID)
		app.reportServerError(r, fmt.Errorf("user is in error state"))
		response.JSON(w, http.StatusBadRequest, map[string]string{
			"error": "user is in error state",
		})
		return
	}

	if user.State.Status < internal.UserStatusIntroCallScheduled {
		app.logger.Error("User is not ready for a call", "customer_id", callRequest.CustomerID)
		app.reportServerError(r, fmt.Errorf("user is not ready for a call"))
		response.JSON(w, http.StatusBadRequest, map[string]string{
			"error": "user is not ready for a call",
		})
		return
	}

	resume, err := internal.GetLatestResume(r.Context(), app.fs, user.ID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			app.logger.Error("User has no resume", "customer_id", callRequest.CustomerID)
			app.serverError(w, r, fmt.Errorf("user has no resume"))
			return
		}
		app.logger.Error("Failed to get resume", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to get resume: %w", err))
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to get resume",
		})
		return
	}

	if !resume.IsValid {
		app.logger.Error("Resume is not valid", "customer_id", callRequest.CustomerID)
		app.reportServerError(r, fmt.Errorf("resume is not valid"))
		response.JSON(w, http.StatusBadRequest, map[string]string{
			"error": "resume is not valid",
		})
		return
	}

	resumeText := resume.RawText
	if resume.Annotated != nil {
		resumeText = *resume.Annotated
	}

	// Create model config with system prompt
	tmpl, err := utils.CreateTemplate("prompts/shirely.001.txt")
	if err != nil {
		app.logger.Error("Failed to create template", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to create template: %w", err))
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to create template",
		})
		return
	}

	systemPrompt, err := tmpl.Execute(map[string]interface{}{
		"FullName": user.Name,
		"Resume":   resumeText,
	})
	if err != nil {
		app.logger.Error("Failed to render template", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to render template: %w", err))
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to render template",
		})
		return
	}

	modelConfig := vapi.DefaultModelConfig
	modelConfig.Messages = []vapi.ModelMessage{{
		Role:    "system",
		Content: systemPrompt,
	}}

	// Create call config
	callConfig := vapi.CallConfig{
		AssistantID:   &assistantID,
		PhoneNumberID: &outboundPhoneID,
		AssistantOverrides: &vapi.Assistant{
			Model: &modelConfig,
			Server: &vapi.ServerConfig{
				URL:            os.Getenv("VAPI_WEBHOOK_URL"),
				TimeoutSeconds: 20,
			},
			ServerMessages: []string{
				"conversation-update",
				"end-of-call-report",
				"function-call",
				"hang",
				"speech-update",
				"status-update",
				"tool-calls",
				"transfer-destination-request",
				"user-interrupted",
			},
		},
		Customer: &vapi.Customer{
			Number: *user.Phone,
			Name:   user.Name,
		},
	}

	// Create the call
	result, err := vapi.CreateCall(r.Context(), callConfig)
	if err != nil {
		app.reportServerError(r, fmt.Errorf("handleVapiCall CreateCall: %w", err))
		response.JSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to create call: " + err.Error(),
		})
		return
	}

	app.logger.Info("Call created successfully",
		"call_id", result.ID,
		"status", result.Status,
	)

	// Return success response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"call_id": result.ID,
		"status":  result.Status,
	})
}

// handleVapiWebhook handles incoming webhook notifications from VAPI
func (app *application) handleVapiWebhook(w http.ResponseWriter, r *http.Request) {
	app.logger.Debug("VAPI webhook received", "method", r.Method, "url", r.URL.String())

	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read and log the raw request body for initial debugging
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		app.logger.Error("Failed to decode webhook payload", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to decode webhook payload: %w", err))
		response.JSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Log the webhook payload for debugging
	app.logger.Debug("Received VAPI webhook payload", "payload", payload)

	// Return a 200 OK to acknowledge receipt
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "received",
	})
}
