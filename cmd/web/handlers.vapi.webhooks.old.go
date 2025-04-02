package main

/*
// handleVapiWebhook handles incoming webhook notifications from VAPI
func (app *application) handleVapiWebhook(w http.ResponseWriter, r *http.Request) {
	app.logger.Debug("VAPI webhook received", "method", r.Method, "url", r.URL.String())

	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the raw request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		app.logger.Error("Failed to read request body", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to read request body: %w", err))
		response.JSON(w, http.StatusBadRequest, map[string]string{
			"error": "failed to read request body",
		})
		return
	}
	// Restore the body for later use
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	// Parse the webhook message
	var webhookMsg vapi.WebhookMessage
	if err := json.NewDecoder(r.Body).Decode(&webhookMsg); err != nil {
		app.logger.Error("Failed to decode webhook message", "error", err)
		app.reportServerError(r, fmt.Errorf("failed to decode webhook message: %w", err))
		response.JSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Save webhook payload to file for testing
	if os.Getenv("SAVE_WEBHOOK_PAYLOADS") == "true" {
		// Ensure the webhooks directory exists
		webhooksDir := "testdata/webhooks"
		if err := os.MkdirAll(webhooksDir, 0755); err != nil {
			app.logger.Error("Failed to create webhooks directory", "error", err)
			// Continue execution since this is just for testing purposes
		}

		// Create a filename based on message type and timestamp
		filename := fmt.Sprintf("%s/%s_%d.json",
			webhooksDir,
			webhookMsg.Message.Type,
			time.Now().UnixNano(),
		)

		// Save the raw body instead of re-marshaling
		if err := os.WriteFile(filename, body, 0644); err != nil {
			app.logger.Error("Failed to save webhook payload", "error", err)
		} else {
			app.logger.Info("Saved webhook payload", "filename", filename)
		}
	}

	// Handle different message types
	switch webhookMsg.Message.Type {
	case vapi.MsgTypeEndOfCallReport:
		if err := app.handleEndOfCallReport(r, webhookMsg); err != nil {
			app.logger.Error("Failed to handle end of call report", "error", err)
			app.reportServerError(r, fmt.Errorf("failed to handle end of call report: %w", err))
		}
	}

	// Return a 200 OK to acknowledge receipt
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "received",
	})
}
*/
